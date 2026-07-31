package scheduler

import (
	"errors"
	"os"
)

type leaseGuard struct {
	file *os.File
}

func acquireLeaseGuard(path string) (*leaseGuard, error) {
	for range 3 {
		before, err := os.Lstat(path)
		missing := errors.Is(err, os.ErrNotExist)
		if err != nil && !missing {
			return nil, err
		}
		if !missing && (before.Mode()&os.ModeSymlink != 0 || !before.Mode().IsRegular()) {
			return nil, errors.New("invalid scheduler lease guard")
		}
		flags := os.O_RDWR
		if missing {
			flags |= os.O_CREATE | os.O_EXCL
		}
		file, err := os.OpenFile(path, flags, 0o600)
		if errors.Is(err, os.ErrExist) || errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		opened, err := file.Stat()
		if err != nil {
			_ = file.Close()
			return nil, err
		}
		after, err := os.Lstat(path)
		if err != nil || after.Mode()&os.ModeSymlink != 0 || !after.Mode().IsRegular() || !os.SameFile(opened, after) {
			_ = file.Close()
			return nil, errors.New("scheduler lease guard changed during secure open")
		}
		if err := tryLockLeaseGuard(file); err != nil {
			_ = file.Close()
			return nil, err
		}
		return &leaseGuard{file: file}, nil
	}
	return nil, errLeaseGuardBusy
}

func (guard *leaseGuard) release() {
	if guard == nil || guard.file == nil {
		return
	}
	_ = unlockLeaseGuard(guard.file)
	_ = guard.file.Close()
	guard.file = nil
}
