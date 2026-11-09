//go:build windows

package scheduler_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

// Refusal preserves source bytes, but cannot qualify successful serialization.
// When native guarded updates land, this unavailable witness must be replaced
// by actual positive coverage and its inspection-policy classification removed.
func TestPrivateAtomicUpdateWindowsUnavailablePreservesExistingState(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "state.json")
	before := []byte("existing-private-state")
	if err := os.WriteFile(path, before, 0600); err != nil {
		t.Fatal(err)
	}
	called := false
	err = scheduler.UpdatePrivateFile(path, 128, func([]byte) ([]byte, error) {
		called = true
		return []byte("must-not-replace"), nil
	})
	if err == nil || err.Error() != "private atomic update unavailable on this platform" || called {
		t.Fatalf("unsupported private update entered mutation: callback=%t err=%v", called, err)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("unsupported private update changed source: %q %v", after, err)
	}
}
