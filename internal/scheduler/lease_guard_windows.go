//go:build windows

package scheduler

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

var errLeaseGuardBusy = errors.New("scheduler lease guard is busy")

func tryLockLeaseGuard(file *os.File) error {
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
		return errLeaseGuardBusy
	}
	return err
}

func unlockLeaseGuard(file *os.File) error {
	var overlapped windows.Overlapped
	return windows.UnlockFileEx(windows.Handle(file.Fd()), 0, 1, 0, &overlapped)
}
