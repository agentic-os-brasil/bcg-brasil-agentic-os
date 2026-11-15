package scheduler

import (
	"context"
	"errors"
	"os"
	"time"
)

type leaseGuard struct {
	file *os.File
}

func acquireLeaseGuard(directory *secureDirectory, name string) (*leaseGuard, error) {
	file, err := directory.openLock(name)
	return lockLeaseGuard(file, err)
}

func acquireLeaseGuardContext(ctx context.Context, directory *secureDirectory, name string) (*leaseGuard, error) {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		guard, err := acquireLeaseGuard(directory, name)
		if !errors.Is(err, errLeaseGuardBusy) {
			return guard, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}

func acquireExistingLeaseGuard(directory *secureDirectory, name string) (*leaseGuard, error) {
	file, err := directory.openExistingLock(name)
	return lockLeaseGuard(file, err)
}

func lockLeaseGuard(file *os.File, err error) (*leaseGuard, error) {
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
