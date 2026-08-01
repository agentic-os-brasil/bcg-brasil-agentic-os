//go:build windows

package maestro

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

var errDispatchLockBusy = errors.New("maestro dispatch lock is busy")

func tryLockDispatchFile(file *os.File) error {
	var overlapped windows.Overlapped
	err := windows.LockFileEx(windows.Handle(file.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &overlapped)
	if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
		return errDispatchLockBusy
	}
	return err
}

func unlockDispatchFile(file *os.File) error {
	var overlapped windows.Overlapped
	return windows.UnlockFileEx(windows.Handle(file.Fd()), 0, 1, 0, &overlapped)
}
