//go:build !windows

package execution

import (
	"golang.org/x/sys/unix"
	"path/filepath"
)

func durableRename(source, target string) error {
	from, err := openLedgerDirectory(filepath.Dir(source), false)
	if err != nil {
		return err
	}
	defer from.Close()
	to, err := openLedgerDirectory(filepath.Dir(target), false)
	if err != nil {
		return err
	}
	defer to.Close()
	if err := unix.Renameat(int(from.Fd()), filepath.Base(source), int(to.Fd()), filepath.Base(target)); err != nil {
		return err
	}
	if err := to.Sync(); err != nil {
		return err
	}
	return from.Sync()
}
