//go:build !windows

package datahome

import (
	"errors"
	"os"
	"syscall"
)

func validateOwnership(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != uint32(os.Getuid()) {
		return errors.New("state path is not owned by current user")
	}
	if info.Mode().Perm()&0022 != 0 {
		return errors.New("state path grants write access to other users")
	}
	return nil
}

func ownershipVerified() bool { return true }
