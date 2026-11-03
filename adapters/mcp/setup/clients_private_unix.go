//go:build !windows

package setup

import (
	"errors"
	"os"
	"syscall"
)

// The backup is a large private packet, not small scheduler state. Inspect the
// same held descriptor used for its bounded digest before admitting deletion.
func validateDesktopBackupFile(info os.FileInfo) error {
	s, ok := info.Sys().(*syscall.Stat_t)
	if !ok || s.Uid != uint32(os.Geteuid()) || s.Nlink != 1 || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 {
		return errors.New("Desktop backup requires private single-link owned file")
	}
	return nil
}
