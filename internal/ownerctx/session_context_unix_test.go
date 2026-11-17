//go:build !windows

package ownerctx_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ownerctx"
	"golang.org/x/sys/unix"
)

func TestAcceptedSessionContextExcludesNativeWriter(t *testing.T) {
	root := acceptedSessionRoot(t)
	file, err := os.OpenFile(filepath.Join(root, "owner", "interview", ".transition.lock"), os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err = unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		t.Fatal(err)
	}
	defer unix.Flock(int(file.Fd()), unix.LOCK_UN)
	if view, err := ownerctx.ProjectAcceptedSessionContext(root); err == nil || len(view.Facets) != 0 {
		t.Fatal("active native writer not excluded")
	}
	if err = unix.Flock(int(file.Fd()), unix.LOCK_UN); err != nil {
		t.Fatal(err)
	}
	if _, err = ownerctx.ProjectAcceptedSessionContext(root); err != nil {
		t.Fatal(err)
	}
}

func TestOwnerSnapshotAndAcceptedSessionRefuseAliases(t *testing.T) {
	for _, kind := range []string{"symlink", "hardlink"} {
		t.Run(kind, func(t *testing.T) {
			root := acceptedSessionRoot(t)
			path := filepath.Join(root, "owner", "self", "communication-style.md")
			external := filepath.Join(root, "external.md")
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(external, body, 0600); err != nil {
				t.Fatal(err)
			}
			if err = os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if kind == "symlink" {
				err = os.Symlink(external, path)
			} else {
				err = os.Link(external, path)
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err = ownerctx.ProjectSnapshot(root, []string{"communication-style"}); err == nil {
				t.Fatal("snapshot followed facet alias")
			}
			if view, err := ownerctx.ProjectAcceptedSessionContext(root); err == nil || len(view.Facets) != 0 {
				t.Fatal("accepted session followed facet alias")
			}
		})
	}
}
