//go:build windows

package releaseprovider

import (
	"errors"
	"syscall"
	"testing"
	"unsafe"
)

func TestWindowsCredentialLayoutMatchesCredentialWOnAMD64(t *testing.T) {
	var credential windowsCredential
	if size := unsafe.Sizeof(credential); size != 80 {
		t.Fatalf("windowsCredential size = %d, want 80", size)
	}
	if offset := unsafe.Offsetof(credential.CredentialBlob); offset != 40 {
		t.Fatalf("CredentialBlob offset = %d, want 40", offset)
	}
	if offset := unsafe.Offsetof(credential.UserName); offset != 72 {
		t.Fatalf("UserName offset = %d, want 72", offset)
	}
}

func TestWindowsCredentialManagerErrorMappingFailsClosed(t *testing.T) {
	if err := mapCredentialManagerError("read", syscall.Errno(windowsErrorSuccess)); err != nil {
		t.Fatalf("success error = %v", err)
	}
	if err := mapCredentialManagerError("read", syscall.Errno(windowsErrorNotFound)); !errors.Is(err, ErrCredentialNotFound) {
		t.Fatalf("not-found error = %v", err)
	}
	for _, code := range []uintptr{
		windowsErrorAccessDenied,
		windowsErrorNoSuchLogonSession,
		windowsErrorCancelled,
		windowsErrorLogonFailure,
	} {
		if err := mapCredentialManagerError("read", syscall.Errno(code)); !errors.Is(err, ErrSecureStoreUnavailable) {
			t.Fatalf("code %d error = %v, want unavailable", code, err)
		}
	}
	if err := mapCredentialManagerError("read", syscall.Errno(9999)); err == nil ||
		errors.Is(err, ErrCredentialNotFound) ||
		errors.Is(err, ErrSecureStoreUnavailable) {
		t.Fatalf("unexpected error mapping = %v", err)
	}
}

func TestWindowsCredentialManagerBackendIsDiscoverableWithoutWritingASecret(t *testing.T) {
	store := NewNativeSecureStore()
	if err := store.Available(); err != nil {
		t.Fatalf("Credential Manager availability error = %v", err)
	}
	if _, err := store.Get("maestro/test/known-absent-credential-manager-item"); !errors.Is(err, ErrCredentialNotFound) &&
		!errors.Is(err, ErrSecureStoreUnavailable) {
		t.Fatalf("Credential Manager probe failed outside the fail-closed contract: %v", err)
	}
}
