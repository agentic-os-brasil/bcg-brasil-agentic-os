//go:build !windows

package execution

import (
	"errors"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
	"path/filepath"
)

func publishSetupItem(from, to string) error {
	if filepath.Dir(from) != filepath.Dir(to) {
		return errors.New("setup ledger publication parent changed")
	}
	dir, err := openLedgerDirectory(filepath.Dir(to), false)
	if err != nil {
		return err
	}
	defer dir.Close()
	if err = scheduler.RenamePrivateFileExclusive(dir, filepath.Base(from), filepath.Base(to)); err != nil {
		return err
	}
	return dir.Sync()
}
