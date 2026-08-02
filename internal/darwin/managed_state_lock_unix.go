//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package darwin

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

var errManagedStateLockBusy = errors.New("Darwin managed state advisory lock is busy")

func tryLockManagedStateFile(file *os.File) error {
	err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
		return errManagedStateLockBusy
	}
	return err
}

func unlockManagedStateFile(file *os.File) error {
	return unix.Flock(int(file.Fd()), unix.LOCK_UN)
}
