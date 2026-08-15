//go:build windows

package yodaselfreview

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

var errWeeklyReceiptLockBusy = errors.New("Yoda weekly receipt advisory lock is busy")

func tryLockWeeklyReceiptFile(file *os.File) error {
	var overlapped windows.Overlapped
	err := windows.LockFileEx(windows.Handle(file.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &overlapped)
	if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
		return errWeeklyReceiptLockBusy
	}
	return err
}

func unlockWeeklyReceiptFile(file *os.File) error {
	var overlapped windows.Overlapped
	return windows.UnlockFileEx(windows.Handle(file.Fd()), 0, 1, 0, &overlapped)
}
