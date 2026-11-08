//go:build !windows

package scheduler

import (
	"errors"
	"os"
)

// RenamePrivateFileExclusive atomically moves a staged leaf to an absent leaf
// in the caller's held no-follow directory. The caller validates custody and
// syncs the directory after publication; unsupported platforms fail closed.
func RenamePrivateFileExclusive(directory *os.File, source, target string) error {
	if directory == nil {
		return errors.New("exclusive rename requires a held directory")
	}
	if err := validateSecureLeaf(source); err != nil {
		return err
	}
	if err := validateSecureLeaf(target); err != nil {
		return err
	}
	return renamePrivateExclusive(int(directory.Fd()), source, target)
}
