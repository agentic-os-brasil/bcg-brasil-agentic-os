//go:build windows

package docling

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

func windowsPlacementParent(t *testing.T) string {
	t.Helper()
	parent := filepath.Join(t.TempDir(), "private")
	if err := scheduler.EnsurePrivateDirectory(parent); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = filepath.WalkDir(parent, func(path string, d os.DirEntry, err error) error {
			if err == nil {
				return os.Chmod(path, 0700)
			}
			return err
		})
	})
	return parent
}

func TestPlacementWindowsSealsAndFlushesHeldRoot(t *testing.T) {
	parent := windowsPlacementParent(t)
	installed := filepath.Join(parent, "installed")
	var published os.FileInfo
	placementAfterPublish = func() error {
		var err error
		published, err = os.Stat(installed)
		return err
	}
	t.Cleanup(func() { placementAfterPublish = nil })
	entries := map[string]entry{"nested/payload": {body: []byte("held Windows placement"), mode: 0444}}
	if err := placeEntries(context.Background(), parent, "installed", entries); err != nil {
		t.Fatal("actual directory publication, sealing or flush failed:", err)
	}
	sealed, err := os.Stat(installed)
	if err != nil || published == nil || !os.SameFile(published, sealed) {
		t.Fatalf("publication and sealing changed root identity: %v", err)
	}
	if published.Mode().Perm()&0222 == 0 || sealed.Mode().Perm()&0222 != 0 {
		t.Fatalf("root was not sealed after publication: before=%v after=%v", published.Mode(), sealed.Mode())
	}
	body, err := os.ReadFile(filepath.Join(installed, "nested", "payload"))
	if err != nil || string(body) != "held Windows placement" {
		t.Fatalf("published contents changed: %q %v", body, err)
	}
	children, err := os.ReadDir(parent)
	if err != nil || len(children) != 1 || children[0].Name() != "installed" {
		t.Fatalf("publication left staging entries: %v %v", children, err)
	}
}

func TestPlacementWindowsPreservesOccupiedDestination(t *testing.T) {
	for _, directory := range []bool{false, true} {
		name := "file"
		if directory {
			name = "directory"
		}
		t.Run(name, func(t *testing.T) {
			parent := windowsPlacementParent(t)
			target := filepath.Join(parent, "installed")
			var err error
			if directory {
				err = scheduler.EnsurePrivateDirectory(target)
			} else {
				err = scheduler.WriteNewPrivateFile(target, []byte("preserved"))
			}
			if err != nil {
				t.Fatal(err)
			}
			before, err := os.Stat(target)
			if err != nil {
				t.Fatal(err)
			}
			err = placeEntries(context.Background(), parent, "installed", map[string]entry{"payload": {body: []byte("new"), mode: 0444}})
			if !errors.Is(err, os.ErrExist) {
				t.Fatalf("occupied destination was not refused exclusively: %v", err)
			}
			after, err := os.Stat(target)
			if err != nil || !os.SameFile(before, after) || before.Mode() != after.Mode() {
				t.Fatalf("occupied destination changed: %v", err)
			}
			children, err := os.ReadDir(parent)
			if err != nil || len(children) != 1 {
				t.Fatalf("failed publication left staging entries: %v %v", children, err)
			}
		})
	}
}
