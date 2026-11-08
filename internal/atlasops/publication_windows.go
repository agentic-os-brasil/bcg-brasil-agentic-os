//go:build windows

package atlasops

import (
	"os"
	"path/filepath"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/winprivate"
)

// The held publisher, eager binding snapshots and directory/lock custody were
// qualified under an actual ordinary account before composition with workspace
// effects (native run 42246115162, source 8558e693). The actual owner provider
// must still be available; every effect independently pins and validates its
// exact registered roots, ACLs, leaf and corpus lock.
func workspaceMutationAvailable() error {
	_, err := winprivate.CurrentUser()
	return err
}

// Retain the admitted parent through complete native publication and cleanup.
func publishAtlasFile(path string, body []byte, exclusive, _ bool) error {
	d, err := winprivate.OpenPinned(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Publish(filepath.Base(path), body, exclusive)
}
func readAtlasFile(path string, maximum int64, _ bool) ([]byte, error) {
	d, err := winprivate.OpenPinned(filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	defer d.Close()
	file, err := d.OpenFile(filepath.Base(path), os.O_RDONLY)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return winprivate.Read(file, maximum)
}

func moveAtlasFile(source, target, expected string) error {
	from, err := winprivate.OpenPinnedTree(filepath.Dir(source))
	if err != nil {
		return err
	}
	defer from.Close()
	to, err := winprivate.OpenPinnedTree(filepath.Dir(target))
	if err != nil {
		return err
	}
	defer to.Close()
	return from.RenamePinnedExclusive(filepath.Base(source), to, filepath.Base(target), expected)
}

func sameAtlasFile(a, b string) (bool, error) {
	inspect := func(path string) (os.FileInfo, error) {
		d, err := winprivate.OpenPinnedTree(filepath.Dir(path))
		if err != nil {
			return nil, err
		}
		defer d.Close()
		f, err := d.OpenFile(filepath.Base(path), os.O_RDONLY)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		return f.Stat()
	}
	x, err := inspect(a)
	if err != nil {
		return false, err
	}
	y, err := inspect(b)
	if err != nil {
		return false, err
	}
	return os.SameFile(x, y), nil
}
