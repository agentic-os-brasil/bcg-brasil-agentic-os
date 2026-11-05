//go:build !windows

package setup

import (
	"errors"
	"os"
	"syscall"
)

// Check the same held descriptor used for bounded reading or backup deletion.
func validateClientPrivateFile(info os.FileInfo, mode os.FileMode) error {
	s, ok := info.Sys().(*syscall.Stat_t)
	if !ok || s.Uid != uint32(os.Geteuid()) || s.Nlink != 1 || !info.Mode().IsRegular() || info.Mode().Perm() != mode {
		return errors.New("client file requires exact-mode single-link owned regular file")
	}
	return nil
}
