package releaseprovider

import (
	"bytes"
	"errors"
	"testing"
)

func TestNativeSecureStoreConformsWithoutPlaintextFallback(t *testing.T) {
	backend := &memorySecretBackend{values: map[string][]byte{}}
	store := newNativeSecureStore(backend)
	secret := []byte(`{"access_token":"secret"}`)

	if err := store.Available(); err != nil {
		t.Fatalf("Available() error = %v", err)
	}
	if err := store.Put("maestro/private-release/test", secret); err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	secret[0] = 'X'
	got, err := store.Get("maestro/private-release/test")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if string(got) != `{"access_token":"secret"}` {
		t.Fatalf("Get() = %q", got)
	}
	got[0] = 'X'
	again, err := store.Get("maestro/private-release/test")
	if err != nil {
		t.Fatalf("second Get() error = %v", err)
	}
	if string(again) != `{"access_token":"secret"}` {
		t.Fatal("Get() exposed backend storage for mutation")
	}
	if err := store.Delete("maestro/private-release/test"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, err := store.Get("maestro/private-release/test"); !errors.Is(err, ErrCredentialNotFound) {
		t.Fatalf("Get(deleted) error = %v, want ErrCredentialNotFound", err)
	}
}

func TestNativeSecureStoreRejectsUnsafeKeysAndPayloadsBeforeBackend(t *testing.T) {
	backend := &memorySecretBackend{values: map[string][]byte{}}
	store := newNativeSecureStore(backend)
	for name, key := range map[string]string{
		"empty":    "",
		"nul":      "maestro/\x00/private",
		"oversize": "maestro/" + string(bytes.Repeat([]byte{'a'}, maximumCredentialKeyBytes)),
	} {
		t.Run(name, func(t *testing.T) {
			if err := store.Put(key, []byte("secret")); err == nil {
				t.Fatal("Put() accepted an unsafe credential key")
			}
		})
	}
	if err := store.Put("maestro/private-release/test", nil); err == nil {
		t.Fatal("Put() accepted an empty credential")
	}
	if err := store.Put("maestro/private-release/test", bytes.Repeat([]byte{'x'}, maximumCredentialBytes+1)); err == nil {
		t.Fatal("Put() accepted an oversized credential")
	}
	if len(backend.values) != 0 {
		t.Fatal("invalid input reached the native backend")
	}
}

func TestNativeSecureStoreFailsClosedWithoutBackend(t *testing.T) {
	store := newNativeSecureStore(nil)
	if err := store.Available(); !errors.Is(err, ErrSecureStoreUnavailable) {
		t.Fatalf("Available() error = %v, want unavailable", err)
	}
	if _, err := store.Get("maestro/private-release/test"); !errors.Is(err, ErrSecureStoreUnavailable) {
		t.Fatalf("Get() error = %v, want unavailable", err)
	}
}

type memorySecretBackend struct {
	values map[string][]byte
}

func (backend *memorySecretBackend) Available() error {
	return nil
}

func (backend *memorySecretBackend) Get(key string) ([]byte, error) {
	value, ok := backend.values[key]
	if !ok {
		return nil, ErrCredentialNotFound
	}
	return append([]byte(nil), value...), nil
}

func (backend *memorySecretBackend) Put(key string, value []byte) error {
	backend.values[key] = append([]byte(nil), value...)
	return nil
}

func (backend *memorySecretBackend) Delete(key string) error {
	if _, ok := backend.values[key]; !ok {
		return ErrCredentialNotFound
	}
	delete(backend.values, key)
	return nil
}
