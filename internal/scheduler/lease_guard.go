package scheduler

import (
	"errors"
	"os"
)

type leaseGuard struct {
	file *os.File
}

func acquireLeaseGuard(path string) (*leaseGuard, error) {
	file, err := secureOpenLock(path)
	if err != nil {
		if errors.Is(err, errLeaseGuardBusy) {
			return nil, errLeaseGuardBusy
		}
		return nil, err
	}
	if err := tryLockLeaseGuard(file); err != nil {
		_ = file.Close()
		return nil, err
	}
	return &leaseGuard{file: file}, nil
}

func (guard *leaseGuard) release() {
	if guard == nil || guard.file == nil {
		return
	}
	_ = unlockLeaseGuard(guard.file)
	_ = guard.file.Close()
	guard.file = nil
}
