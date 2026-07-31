//go:build !windows

package darwin

import (
	"os"
	"path/filepath"
)

func publishEvolutionFile(temporaryPath, path string) error {
	// A hard link publishes without replacing a concurrent winner. Syncing the
	// parent directory makes the newly published name durable across power loss.
	if err := os.Link(temporaryPath, path); err != nil {
		return err
	}
	return syncEvolutionDirectory(filepath.Dir(path))
}

func syncEvolutionDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
