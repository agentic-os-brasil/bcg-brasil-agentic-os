package scheduler_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

func TestGuardedPrivateUpdateRefusesAtFinalPublicationAndRestoresExactAbsence(t *testing.T) {
	root := filepath.Join(t.TempDir(), "private")
	if e := scheduler.EnsurePrivateDirectory(root); e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(root, "current.json")
	denied := errors.New("live grant revoked")
	if e := scheduler.UpdatePrivateFileGuarded(path, 1024, func(func() error) error { return denied }, func([]byte) ([]byte, error) { return []byte("new"), nil }); !errors.Is(e, denied) {
		t.Fatal(e)
	}
	if _, e := os.Stat(path); !os.IsNotExist(e) {
		t.Fatal("refused final guard still published")
	}
	allow := func(f func() error) error { return f() }
	if e := scheduler.UpdatePrivateFileGuarded(path, 1024, allow, func([]byte) ([]byte, error) { return []byte("new"), nil }); e != nil {
		t.Fatal(e)
	}
	if e := scheduler.RestorePrivateFileGuarded(path, 1024, []byte("other"), nil, allow); e == nil {
		t.Fatal("compensation deleted another parent")
	}
	body, e := scheduler.ReadPrivateStateFile(path, 1024)
	if e != nil || !bytes.Equal(body, []byte("new")) {
		t.Fatal("stale compensation changed bytes")
	}
	if e := scheduler.RestorePrivateFileGuarded(path, 1024, []byte("new"), nil, allow); e != nil {
		t.Fatal(e)
	}
	if _, e := os.Stat(path); !os.IsNotExist(e) {
		t.Fatal("exact absence not restored")
	}
}
