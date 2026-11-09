//go:build !windows

package packaging

import (
	"errors"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
	"path/filepath"
)

func publishPacket(source, target string) error {
	if filepath.Dir(source) != filepath.Dir(target) {
		return errors.New("packet publication must stay in its held parent")
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
