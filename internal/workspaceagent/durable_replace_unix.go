//go:build !windows

package workspaceagent

import (
	"os"
	"path/filepath"
)

func durableReplace(source, target string) error {
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
