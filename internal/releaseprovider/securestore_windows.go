//go:build windows

package releaseprovider

import (
	"errors"
	"fmt"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	windowsCredentialTypeGeneric           = 1
	windowsCredentialPersistLocal          = 2
	windowsErrorSuccess            uintptr = 0
	windowsErrorAccessDenied       uintptr = 5
	windowsErrorCancelled          uintptr = 1223
	windowsErrorNoSuchLogonSession         = 1312
	windowsErrorLogonFailure               = 1326
	windowsErrorNotFound                   = 1168
)

const maestroCredentialTargetPrefix = "Maestro/"

var (
	advapi32CredentialDLL = windows.NewLazySystemDLL("Advapi32.dll")
	procCredReadW         = advapi32CredentialDLL.NewProc("CredReadW")
	procCredWriteW        = advapi32CredentialDLL.NewProc("CredWriteW")
	procCredDeleteW       = advapi32CredentialDLL.NewProc("CredDeleteW")
	procCredFree          = advapi32CredentialDLL.NewProc("CredFree")
)

type windowsCredential struct {
	Flags              uint32
	Type               uint32
	TargetName         *uint16
	Comment            *uint16
	LastWritten        syscall.Filetime
	CredentialBlobSize uint32
	CredentialBlob     *byte
	Persist            uint32
	AttributeCount     uint32
	Attributes         uintptr
	TargetAlias        *uint16
	UserName           *uint16
}

type windowsCredentialManagerBackend struct{}

func NewNativeSecureStore() SecureStore {
	return newNativeSecureStore(windowsCredentialManagerBackend{})
}

func (windowsCredentialManagerBackend) Available() error {
	target, err := syscall.UTF16PtrFromString(maestroCredentialTargetPrefix + "availability-probe")
	if err != nil {
		return ErrSecureStoreUnavailable
	}
	credential, err := windowsReadCredential(target)
	if credential != nil {
		procCredFree.Call(uintptr(unsafe.Pointer(credential)))
	}
	if errors.Is(err, ErrCredentialNotFound) {
		return nil
	}
	return err
}

func (windowsCredentialManagerBackend) Get(key string) ([]byte, error) {
	target, err := windowsCredentialTarget(key)
	if err != nil {
		return nil, err
	}
	credential, err := windowsReadCredential(target)
	if err != nil {
		return nil, err
	}
	defer procCredFree.Call(uintptr(unsafe.Pointer(credential)))
	if credential.Type != windowsCredentialTypeGeneric ||
		credential.CredentialBlobSize == 0 ||
		credential.CredentialBlobSize > maximumCredentialBytes ||
		credential.CredentialBlob == nil {
		return nil, errors.New("Windows Credential Manager returned an invalid credential payload")
	}
	value := unsafe.Slice(credential.CredentialBlob, int(credential.CredentialBlobSize))
	return append([]byte(nil), value...), nil
}

func (windowsCredentialManagerBackend) Put(key string, value []byte) error {
	target, err := windowsCredentialTarget(key)
	if err != nil {
		return err
	}
	username, err := syscall.UTF16PtrFromString("Maestro")
	if err != nil {
		return err
	}
	credential := windowsCredential{
		Type:               windowsCredentialTypeGeneric,
		TargetName:         target,
		CredentialBlobSize: uint32(len(value)),
		CredentialBlob:     &value[0],
		Persist:            windowsCredentialPersistLocal,
		UserName:           username,
	}
	result, _, callErr := procCredWriteW.Call(uintptr(unsafe.Pointer(&credential)), 0)
	runtime.KeepAlive(value)
	runtime.KeepAlive(target)
	runtime.KeepAlive(username)
	return credentialManagerCallError("write", result, callErr)
}

func (windowsCredentialManagerBackend) Delete(key string) error {
	target, err := windowsCredentialTarget(key)
	if err != nil {
		return err
	}
	result, _, callErr := procCredDeleteW.Call(
		uintptr(unsafe.Pointer(target)),
		windowsCredentialTypeGeneric,
		0,
	)
	runtime.KeepAlive(target)
	return credentialManagerCallError("delete", result, callErr)
}

func windowsReadCredential(target *uint16) (*windowsCredential, error) {
	var credential *windowsCredential
	result, _, callErr := procCredReadW.Call(
		uintptr(unsafe.Pointer(target)),
		windowsCredentialTypeGeneric,
		0,
		uintptr(unsafe.Pointer(&credential)),
	)
	runtime.KeepAlive(target)
	if err := credentialManagerCallError("read", result, callErr); err != nil {
		return nil, err
	}
	if credential == nil {
		return nil, errors.New("Windows Credential Manager returned an empty credential reference")
	}
	return credential, nil
}

func windowsCredentialTarget(key string) (*uint16, error) {
	target, err := syscall.UTF16PtrFromString(maestroCredentialTargetPrefix + key)
	if err != nil {
		return nil, errors.New("credential key cannot be represented by Windows Credential Manager")
	}
	return target, nil
}

func credentialManagerCallError(operation string, result uintptr, callErr error) error {
	if result != 0 {
		return nil
	}
	code, ok := callErr.(syscall.Errno)
	if !ok || code == 0 {
		return fmt.Errorf("Windows Credential Manager %s failed without an error code", operation)
	}
	return mapCredentialManagerError(operation, code)
}

func mapCredentialManagerError(operation string, code syscall.Errno) error {
	switch uintptr(code) {
	case windowsErrorSuccess:
		return nil
	case windowsErrorNotFound:
		return ErrCredentialNotFound
	case windowsErrorAccessDenied, windowsErrorCancelled, windowsErrorNoSuchLogonSession, windowsErrorLogonFailure:
		return fmt.Errorf("%w: Windows Credential Manager %s is unavailable", ErrSecureStoreUnavailable, operation)
	default:
		return fmt.Errorf("Windows Credential Manager %s failed with error %d", operation, code)
	}
}
