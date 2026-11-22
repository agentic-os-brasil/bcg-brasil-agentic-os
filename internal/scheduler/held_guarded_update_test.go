package scheduler_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

func TestHeldPrivateUpdateGuardPreservesCurrentBytes(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err = scheduler.EnsurePrivateDirectory(root); err != nil {
		t.Fatal(err)
	}
	held, err := scheduler.HoldDirectory(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()
	if err = held.EnsureDirectory("metadata"); err != nil {
		t.Fatal(err)
	}
	child, err := held.HoldDirectory("metadata")
	if err != nil {
		t.Fatal(err)
	}
	defer child.Close()
	if err = child.UpdatePrivateFile("state.json", 128, func([]byte) ([]byte, error) { return []byte("before"), nil }); err != nil {
		t.Fatal(err)
	}
	refused := errors.New("live authority withdrawn")
	called := false
	err = child.UpdatePrivateFileGuarded("state.json", 128, func(func() error) error { called = true; return refused }, func(current []byte) ([]byte, error) {
		if string(current) != "before" {
			t.Fatalf("wrong current bytes: %q", current)
		}
		return []byte("after"), nil
	})
	if !called || !errors.Is(err, refused) {
		t.Fatalf("final live guard was not observed: called=%t err=%v", called, err)
	}
	body, err := child.ReadPrivateFile("state.json", 128)
	if err != nil || string(body) != "before" {
		t.Fatalf("denied publication changed prior bytes: %q %v", body, err)
	}
	err = child.UpdatePrivateFileGuarded("state.json", 128, func(publish func() error) error {
		if err := os.WriteFile(filepath.Join(root, "metadata", "state.json"), []byte("external"), 0600); err != nil {
			return err
		}
		return publish()
	}, func([]byte) ([]byte, error) { return []byte("stale"), nil })
	if err == nil {
		t.Fatal("final revision check accepted changed bytes")
	}
	body, err = child.ReadPrivateFile("state.json", 128)
	if err != nil || string(body) != "external" {
		t.Fatalf("external bytes lost: %q %v", body, err)
	}
	if err = held.UpdatePrivateFileGuarded("missing/state.json", 128, nil, func([]byte) ([]byte, error) { t.Fatal("nil guard reached update"); return nil, nil }); err == nil {
		t.Fatal("nil live guard admitted")
	}
	if _, err = os.Stat(filepath.Join(root, "missing")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("nil guard created state: %v", err)
	}
	if _, err = held.HoldDirectory("../outside"); err == nil {
		t.Fatal("relative child escaped retained root")
	}
}
