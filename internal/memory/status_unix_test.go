//go:build !windows

package memory

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStatusUsesTemporaryAlias(t *testing.T) {
	base := t.TempDir()
	physical, temporary := filepath.Join(base, "physical"), filepath.Join(base, "temporary")
	if err := os.Mkdir(physical, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(physical, temporary); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMPDIR", temporary)
	root := filepath.Join(temporary, "memory")
	if err := Bootstrap(root, "case-a"); err != nil {
		t.Fatal(err)
	}
	engine := &Engine{Root: root}
	status, err := engine.Status("case-a")
	if err != nil || status.State != "empty" || status.ActivationLocked {
		t.Fatalf("temporary alias status=%#v err=%v", status, err)
	}
	lock := filepath.Join(physical, "memory", "workspaces", "case-a", ".locks", "activation.lock")
	if err := os.Mkdir(lock, 0700); err != nil {
		t.Fatal(err)
	}
	status, err = engine.Status("case-a")
	if err != nil || !status.ActivationLocked {
		t.Fatalf("temporary alias lost active lock: status=%#v err=%v", status, err)
	}
}

func statusDirectoryAlias(t *testing.T, alias, target string) {
	t.Helper()
	if err := os.Symlink(target, alias); err != nil {
		t.Fatal(err)
	}
}

func statusSecuritySnapshot(t *testing.T, path string) string { t.Helper(); return "" }
