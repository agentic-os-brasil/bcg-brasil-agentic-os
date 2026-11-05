package scheduler

import (
	"errors"
	"path/filepath"
)

// WithPrivateLock serializes short metadata transitions across processes. The
// caller must not acquire a domain lock while holding this guard.
func WithPrivateLock(path string, action func() error) error {
	if action == nil {
		return errors.New("private transition is required")
	}
	dir, err := openSecureDirectory(filepath.Dir(path), true)
	if err != nil {
		return err
	}
	defer dir.close()
	guard, err := acquireLeaseGuard(dir, filepath.Base(path))
	if errors.Is(err, errLeaseGuardBusy) {
		return ErrLeaseBusy
	}
	if err != nil {
		return err
	}
	defer guard.release()
	return action()
}

// WritePrivateFileAtomic publishes through a held, confined directory. A
// failure leaves the previous complete file or the next complete file.
func WritePrivateFileAtomic(path string, body []byte) error {
	return writePrivateFileAtomic(path, body)
}
