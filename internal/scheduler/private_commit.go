package scheduler

import (
	"context"
	"errors"
	"path/filepath"
)

// WithPrivateLock serializes short metadata transitions across processes. The
// caller must not acquire a domain lock while holding this guard.
func WithPrivateLock(path string, action func() error) error {
	return withPrivateLock(context.Background(), path, action, acquireLeaseGuard)
}

// WithPrivateLockContext waits only for acquisition; an entered action runs once.
func WithPrivateLockContext(ctx context.Context, path string, action func() error) error {
	return withPrivateLock(ctx, path, action, func(directory *secureDirectory, name string) (*leaseGuard, error) {
		return acquireLeaseGuardContext(ctx, directory, name)
	})
}

func withPrivateLock(ctx context.Context, path string, action func() error, acquire func(*secureDirectory, string) (*leaseGuard, error)) error {
	if ctx == nil {
		return errors.New("private transition context is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if action == nil {
		return errors.New("private transition is required")
	}
	dir, err := openSecureDirectory(filepath.Dir(path), true)
	if err != nil {
		return err
	}
	defer dir.close()
	guard, err := acquire(dir, filepath.Base(path))
	if errors.Is(err, errLeaseGuardBusy) {
		return ErrLeaseBusy
	}
	if err != nil {
		return err
	}
	defer guard.release()
	if err := ctx.Err(); err != nil {
		return err
	}
	return action()
}

// WritePrivateFileAtomic publishes through a held, confined directory. A
// failure leaves the previous complete file or the next complete file.
func WritePrivateFileAtomic(path string, body []byte) error {
	return writePrivateFileAtomic(path, body)
}
