//go:build darwin

package longrun

import (
	"encoding/base64"
	"errors"
	"os"
	"testing"
)

func TestKeychainAnchorPreservesMonotonicHead(t *testing.T) {
	secrets := map[string]string{}
	anchor := &keychainAnchor{service: "test-service", run: func(args ...string) ([]byte, error) {
		if args[0] == "find-generic-password" {
			value, ok := secrets[args[4]]
			if !ok {
				return nil, fakeSecurityExit{code: 44}
			}
			return []byte(value + "\n"), nil
		}
		if args[0] == "add-generic-password" {
			secrets[args[5]] = args[7]
			return nil, nil
		}
		return nil, errors.New("unexpected security command")
	}}
	first := AnchorRecord{GoalID: "maestro-pilot", Sequence: 2, MAC: "first"}
	if err := anchor.Store(first); err != nil {
		t.Fatal(err)
	}
	loaded, err := anchor.Load("maestro-pilot")
	if err != nil || loaded != first {
		t.Fatalf("loaded = %#v, err = %v", loaded, err)
	}
	if err := anchor.Store(AnchorRecord{GoalID: "maestro-pilot", Sequence: 1, MAC: "older"}); err == nil {
		t.Fatal("keychain anchor accepted a backward head")
	}
	if err := anchor.Store(AnchorRecord{GoalID: "maestro-pilot", Sequence: 2, MAC: "different"}); err == nil {
		t.Fatal("keychain anchor accepted a conflicting head")
	}
}

type fakeSecurityExit struct{ code int }

func (exit fakeSecurityExit) Error() string { return "security failure" }
func (exit fakeSecurityExit) ExitCode() int { return exit.code }
func (exit fakeSecurityExit) Unwrap() error { return os.ErrNotExist }

func TestKeychainAnchorRejectsInvalidStoredRecord(t *testing.T) {
	anchor := &keychainAnchor{service: "test-service", run: func(args ...string) ([]byte, error) {
		if args[0] == "find-generic-password" {
			return []byte(base64.RawStdEncoding.EncodeToString([]byte("not-json")) + "\n"), nil
		}
		return nil, nil
	}}
	if _, err := anchor.Load("maestro-pilot"); err == nil {
		t.Fatal("invalid keychain record was accepted")
	}
}
