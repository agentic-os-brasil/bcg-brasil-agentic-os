//go:build !windows

package setup

import (
	"errors"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
	"path/filepath"
)

// Exclusive first publication never replaces an occupied client slot. This is
// not authority to replace or delete a previously shared client configuration.
func publishClientCandidate(source, target string) error {
	if filepath.Dir(source) != filepath.Dir(target) {
		return errors.New("client publication requires the selected same parent")
	}
	parent, err := scheduler.OpenDirectory(filepath.Dir(target))
	if err != nil {
		return err
	}
	defer parent.Close()
	if err := scheduler.RenamePrivateFileExclusive(parent, filepath.Base(source), filepath.Base(target)); err != nil {
		return err
	}
	return parent.Sync()
}
