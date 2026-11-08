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
	return RenamePrivateFileExclusiveBetween(directory, source, directory, target)
}

// RenamePrivateFileExclusiveBetween moves only between the caller's held parents.
func RenamePrivateFileExclusiveBetween(from *os.File, source string, to *os.File, target string) error {
	if from == nil || to == nil {
		return errors.New("exclusive rename requires a held directory")
	}
	if err := validateSecureLeaf(source); err != nil {
		return err
	}
	if err := validateSecureLeaf(target); err != nil {
		return err
	}
	return renamePrivateExclusiveBetween(int(from.Fd()), source, int(to.Fd()), target)
}

func renamePrivateExclusive(directory int, source, target string) error {
	return renamePrivateExclusiveBetween(directory, source, directory, target)
}
