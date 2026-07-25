//go:build !windows

package execution

import (
	"os"
	"path/filepath"
)

func durableRename(source, target string) error {
	if err := os.Rename(source, target); err != nil {
		return err
	}
	return syncParent(target)
}

func durableReplace(source, target string) error {
	if err := os.Rename(source, target); err != nil {
		return err
	}
	return syncParent(target)
}

func durablePublishNoClobber(source, target string) error {
	if err := os.Link(source, target); err != nil {
		return err
	}
	if err := syncParent(target); err != nil {
		return err
	}
	return os.Remove(source)
}

func syncParent(path string) error {
	directory, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
