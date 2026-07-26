//go:build windows

package longrun

import (
	"encoding/json"
	"errors"
	"syscall"
	"unsafe"
)

const (
	credentialTypeGeneric                       = 1
	credentialPersistLocalMachine               = 2
	errorNotFound                 syscall.Errno = 1168
)

var (
	advapi32  = syscall.NewLazyDLL("advapi32.dll")
	credRead  = advapi32.NewProc("CredReadW")
	credWrite = advapi32.NewProc("CredWriteW")
	credFree  = advapi32.NewProc("CredFree")
)

// DefaultAnchor stores the monotonic record in the user's Windows Credential
// Manager, outside the BCGOS local-data tree and ordinary workspace backups.
func DefaultAnchor() (MonotonicAnchor, error) { return &credentialManagerAnchor{}, nil }

type credentialManagerAnchor struct{}

type credential struct {
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

func (anchor *credentialManagerAnchor) Load(goalID string) (AnchorRecord, error) {
	target, err := syscall.UTF16PtrFromString(anchorTarget(goalID))
	if err != nil {
		return AnchorRecord{}, err
	}
	var raw *credential
	r1, _, callErr := credRead.Call(uintptr(unsafe.Pointer(target)), uintptr(credentialTypeGeneric), 0, uintptr(unsafe.Pointer(&raw)))
	if r1 == 0 {
		if callErr == errorNotFound {
			return AnchorRecord{}, syscall.ENOENT
		}
		return AnchorRecord{}, ErrMonotonicAnchorUnavailable
	}
	defer credFree.Call(uintptr(unsafe.Pointer(raw)))
	if raw.CredentialBlob == nil || raw.CredentialBlobSize == 0 {
		return AnchorRecord{}, errors.New("invalid Windows Credential Manager long-running anchor")
	}
	body := unsafe.Slice(raw.CredentialBlob, raw.CredentialBlobSize)
	var record AnchorRecord
	if err := json.Unmarshal(body, &record); err != nil {
		return AnchorRecord{}, errors.New("invalid Windows Credential Manager long-running anchor")
	}
	return record, nil
}

func (anchor *credentialManagerAnchor) Store(next AnchorRecord) error {
	if current, err := anchor.Load(next.GoalID); err == nil {
		if next.Sequence < current.Sequence || (next.Sequence == current.Sequence && next.MAC != current.MAC) {
			return errors.New("Windows Credential Manager long-running anchor cannot move backward")
		}
	} else if !errors.Is(err, syscall.ENOENT) {
		return err
	}
	target, err := syscall.UTF16PtrFromString(anchorTarget(next.GoalID))
	if err != nil {
		return err
	}
	user, err := syscall.UTF16PtrFromString("Maestro")
	if err != nil {
		return err
	}
	body, err := json.Marshal(next)
	if err != nil {
		return err
	}
	entry := credential{Type: credentialTypeGeneric, TargetName: target, CredentialBlobSize: uint32(len(body)), CredentialBlob: &body[0], Persist: credentialPersistLocalMachine, UserName: user}
	r1, _, _ := credWrite.Call(uintptr(unsafe.Pointer(&entry)), 0)
	if r1 == 0 {
		return ErrMonotonicAnchorUnavailable
	}
	return nil
}

func anchorTarget(goalID string) string { return "BCGBrasil.Maestro.LongRun.v1/" + goalID }
