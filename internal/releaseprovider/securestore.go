package releaseprovider

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	maximumCredentialKeyBytes = 256
	maximumCredentialBytes    = 5 * 512
)

type nativeSecretBackend interface {
	Available() error
	Get(string) ([]byte, error)
	Put(string, []byte) error
	Delete(string) error
}

type nativeSecureStore struct {
	backend nativeSecretBackend
}

func newNativeSecureStore(backend nativeSecretBackend) SecureStore {
	return nativeSecureStore{backend: backend}
}

func (store nativeSecureStore) Available() error {
	if store.backend == nil {
		return ErrSecureStoreUnavailable
	}
	if err := store.backend.Available(); err != nil {
		return err
	}
	return nil
}

func (store nativeSecureStore) Get(key string) ([]byte, error) {
	if err := store.Available(); err != nil {
		return nil, err
	}
	if err := validateCredentialKey(key); err != nil {
		return nil, err
	}
	value, err := store.backend.Get(key)
	if err != nil {
		return nil, err
	}
	if len(value) == 0 || len(value) > maximumCredentialBytes {
		return nil, errors.New("native credential store returned an invalid credential payload")
	}
	return append([]byte(nil), value...), nil
}

func (store nativeSecureStore) Put(key string, value []byte) error {
	if err := store.Available(); err != nil {
		return err
	}
	if err := validateCredentialKey(key); err != nil {
		return err
	}
	if len(value) == 0 || len(value) > maximumCredentialBytes {
		return fmt.Errorf("credential payload must contain 1 to %d bytes", maximumCredentialBytes)
	}
	return store.backend.Put(key, append([]byte(nil), value...))
}

func (store nativeSecureStore) Delete(key string) error {
	if err := store.Available(); err != nil {
		return err
	}
	if err := validateCredentialKey(key); err != nil {
		return err
	}
	return store.backend.Delete(key)
}

func validateCredentialKey(key string) error {
	if key == "" || len(key) > maximumCredentialKeyBytes || !utf8.ValidString(key) ||
		strings.ContainsRune(key, '\x00') {
		return fmt.Errorf("credential key must contain 1 to %d valid UTF-8 bytes without NUL", maximumCredentialKeyBytes)
	}
	return nil
}
