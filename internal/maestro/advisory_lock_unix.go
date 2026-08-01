//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package maestro

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

var errDispatchLockBusy = errors.New("maestro dispatch lock is busy")

func tryLockDispatchFile(file *os.File) error {
	err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
		return errDispatchLockBusy
	}
	return err
}

func unlockDispatchFile(file *os.File) error {
	return unix.Flock(int(file.Fd()), unix.LOCK_UN)
}
