//go:build windows

package priorworksync

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

var errClaimGuardBusy = errors.New("prior-work scheduler claim guard is busy")

func tryLockClaimGuard(file *os.File) error {
	var overlapped windows.Overlapped
	err := windows.LockFileEx(
		windows.Handle(file.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
		0,
		1,
		0,
		&overlapped,
	)
	if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
		return errClaimGuardBusy
	}
	return err
}

func unlockClaimGuard(file *os.File) error {
	var overlapped windows.Overlapped
	return windows.UnlockFileEx(windows.Handle(file.Fd()), 0, 1, 0, &overlapped)
}
