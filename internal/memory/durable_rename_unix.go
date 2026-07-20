//go:build !windows

package memory

import (
	"os"
	"path/filepath"
)

func durableRename(source, target string) error {
	if err := os.Rename(source, target); err != nil {
		return err
	}
	directory, err := os.Open(filepath.Dir(target))
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
