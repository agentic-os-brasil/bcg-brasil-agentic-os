//go:build darwin && cgo

package releaseprovider

import (
	"errors"
	"testing"
)

func TestMacOSKeychainStatusMappingFailsClosed(t *testing.T) {
	if err := mapKeychainStatus("read", keychainSuccess); err != nil {
		t.Fatalf("success status error = %v", err)
	}
	if err := mapKeychainStatus("read", keychainItemNotFound); !errors.Is(err, ErrCredentialNotFound) {
		t.Fatalf("item-not-found error = %v", err)
	}
	for _, status := range []int32{
		keychainInteractionNotAllowed,
		keychainAuthFailed,
		keychainNotAvailable,
		keychainUserCanceled,
	} {
		if err := mapKeychainStatus("read", status); !errors.Is(err, ErrSecureStoreUnavailable) {
			t.Fatalf("status %d error = %v, want unavailable", status, err)
		}
	}
	if err := mapKeychainStatus("read", -1); err == nil ||
		errors.Is(err, ErrCredentialNotFound) ||
		errors.Is(err, ErrSecureStoreUnavailable) {
		t.Fatalf("unexpected status error = %v", err)
	}
}

func TestMacOSKeychainBackendIsDiscoverableWithoutWritingASecret(t *testing.T) {
	store := NewNativeSecureStore()
	if err := store.Available(); err != nil {
		t.Fatalf("native macOS Keychain availability error = %v", err)
	}
	if _, err := store.Get("maestro/test/known-absent-keychain-item"); !errors.Is(err, ErrCredentialNotFound) &&
		!errors.Is(err, ErrSecureStoreUnavailable) {
		t.Fatalf("native macOS Keychain probe failed outside the fail-closed contract: %v", err)
	}
}
