//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package priorworksync

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

var errClaimGuardBusy = errors.New("prior-work scheduler claim guard is busy")

func tryLockClaimGuard(file *os.File) error {
	err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
		return errClaimGuardBusy
	}
	return err
}

func unlockClaimGuard(file *os.File) error {
	return unix.Flock(int(file.Fd()), unix.LOCK_UN)
}
