//go:build !windows

package packaging

import (
	"errors"
	"os"
	"syscall"
)

func stablePrivateFile(info os.FileInfo) error {
	s, ok := info.Sys().(*syscall.Stat_t)
	if !ok || s.Uid != uint32(os.Geteuid()) || s.Nlink != 1 || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 && info.Mode().Perm() != 0700 {
		return errors.New("stable packet requires private single-link owned files")
	}
	return nil
}
