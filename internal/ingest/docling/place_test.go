//go:build darwin || linux

package docling

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func placementParent(t *testing.T) string {
	t.Helper()
	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = filepath.WalkDir(parent, func(path string, d os.DirEntry, err error) error {
			if err == nil && d.IsDir() {
				return os.Chmod(path, 0700)
			}
			return err
		})
	})
	return parent
}

func TestPlacementPublishesThenSealsRoot(t *testing.T) {
	parent := placementParent(t)
	entries := map[string]entry{"nested/payload": {body: []byte("filesystem placement only"), mode: 0444}}
	if err := placeEntries(context.Background(), parent, "installed", entries); err != nil {
		t.Fatal("placement failed before the immutable tree was published:", err)
	}
	root := filepath.Join(parent, "installed")
	if err := verifyInstalled(context.Background(), root, entries); err != nil {
		t.Fatal("published tree failed the independent final verifier:", err)
	}
	children, err := os.ReadDir(parent)
	if err != nil || len(children) != 1 || children[0].Name() != "installed" {
		t.Fatalf("placement left staging entries: %v %v", children, err)
	}
}

func TestPlacementPreservesOccupiedDestination(t *testing.T) {
	for _, occupied := range []string{"empty directory", "nonempty directory", "file", "symlink"} {
		t.Run(occupied, func(t *testing.T) {
			parent := placementParent(t)
			target := filepath.Join(parent, "installed")
			switch occupied {
			case "empty directory", "nonempty directory":
				if err := os.Mkdir(target, 0700); err != nil {
					t.Fatal(err)
				}
				if occupied == "nonempty directory" {
					if err := os.WriteFile(filepath.Join(target, "preserved"), []byte("existing"), 0600); err != nil {
						t.Fatal(err)
					}
				}
			case "file":
				if err := os.WriteFile(target, []byte("existing"), 0600); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				if err := os.Symlink("missing", target); err != nil {
					t.Fatal(err)
				}
			}
			before, err := os.Lstat(target)
			if err != nil {
				t.Fatal(err)
			}
			err = placeEntries(context.Background(), parent, "installed", map[string]entry{"payload": {body: []byte("new"), mode: 0444}})
			if !errors.Is(err, os.ErrExist) {
				t.Fatalf("occupied destination was not refused exclusively: %v", err)
			}
			after, err := os.Lstat(target)
			if err != nil || !os.SameFile(before, after) || before.Mode() != after.Mode() {
				t.Fatalf("occupied destination changed: %v", err)
			}
			children, err := os.ReadDir(parent)
			if err != nil || len(children) != 1 {
				t.Fatalf("failed placement left staging entries: %v %v", children, err)
			}
		})
	}
}

func TestPlacementInterruptedAfterPublicationCanRetry(t *testing.T) {
	parent := placementParent(t)
	entries := map[string]entry{"nested/payload": {body: []byte("filesystem placement only"), mode: 0444}}
	interrupted := errors.New("interrupted before final sealing")
	placementAfterPublish = func() error {
		for name, mode := range map[string]os.FileMode{"installed": 0700, "installed/nested": 0555} {
			info, err := os.Stat(filepath.Join(parent, name))
			if err != nil || info.Mode().Perm() != mode {
				t.Fatalf("publication mode for %s is not %04o: %v %v", name, mode, info, err)
			}
		}
		return interrupted
	}
	t.Cleanup(func() { placementAfterPublish = nil })
	if err := placeEntries(context.Background(), parent, "installed", entries); !errors.Is(err, interrupted) {
		t.Fatalf("interruption lost: %v", err)
	}
	children, err := os.ReadDir(parent)
	if err != nil || len(children) != 0 {
		t.Fatalf("interruption left a target or staging retry blocker: %v %v", children, err)
	}
	placementAfterPublish = nil
	if err := placeEntries(context.Background(), parent, "installed", entries); err != nil {
		t.Fatal("retry failed:", err)
	}
	if err := verifyInstalled(context.Background(), filepath.Join(parent, "installed"), entries); err != nil {
		t.Fatal("retry did not produce a verified immutable tree:", err)
	}
}

func TestPlacementKeepsHeldParentWhenPathIsReplaced(t *testing.T) {
	base := placementParent(t)
	parent := filepath.Join(base, "parent")
	if err := os.Mkdir(parent, 0700); err != nil {
		t.Fatal(err)
	}
	entries := map[string]entry{"nested/payload": {body: []byte("held tree"), mode: 0444}}
	placementAfterPublish = func() error {
		if err := os.Rename(parent, parent+"-held"); err != nil {
			return err
		}
		return os.Mkdir(parent, 0700)
	}
	t.Cleanup(func() { placementAfterPublish = nil })
	if err := placeEntries(context.Background(), parent, "installed", entries); err != nil {
		t.Fatal("publication lost its held parent:", err)
	}
	if err := verifyInstalled(context.Background(), filepath.Join(parent+"-held", "installed"), entries); err != nil {
		t.Fatal("held tree was not sealed:", err)
	}
	children, err := os.ReadDir(parent)
	if err != nil || len(children) != 0 {
		t.Fatalf("publication changed the replacement parent: %v %v", children, err)
	}
}
