//go:build windows

package scheduler

import (
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/winprivate"
	"os"
	"path/filepath"
)

func heldDirectoryChild(d *secureDirectory, name string, create bool) (*secureDirectory, error) {
	child, e := d.native.PinnedChild(name, create, false)
	if e != nil {
		return nil, e
	}
	return &secureDirectory{native: child, path: filepath.Join(d.path, name)}, nil
}
func heldDirectoryStat(d *secureDirectory) (os.FileInfo, error) { return d.native.File.Stat() }
func heldDirectoryRead(d *secureDirectory, max int) ([]os.DirEntry, error) {
	limit := 4096
	if max > limit {
		return nil, os.ErrInvalid
	}
	entries, e := d.native.ReadDir(limit)
	if e != nil {
		return nil, e
	}
	if max > 0 && len(entries) > max {
		entries = entries[:max]
	}
	return entries, nil
}
func heldDirectoryMkdir(d *secureDirectory, name string) error {
	c, e := d.native.PinnedChild(name, true, true)
	if e != nil {
		return e
	}
	return c.Close()
}
func heldDirectorySync(d *secureDirectory) error { return nil }

// Parent handles above retain the native no-delete-sharing ancestry while the
// existing native rename owner acquires its movable leaf and destination.
func heldDirectoryRename(a *secureDirectory, an string, b *secureDirectory, bn string, replace bool) error {
	return a.native.Rename(an, b.native, bn, replace)
}

func heldDirectoryRemove(d *secureDirectory, name string) error { return d.native.Remove(name, false) }
func heldDirectoryOwned(d *secureDirectory) error               { return nil }
func heldDirectoryPrivate(d *secureDirectory) error             { return nil }

func openRetainedDirectory(path string) (*secureDirectory, error) {
	d, e := winprivate.OpenPinnedTree(path)
	if e != nil {
		return nil, e
	}
	return &secureDirectory{native: d, path: path}, nil
}
