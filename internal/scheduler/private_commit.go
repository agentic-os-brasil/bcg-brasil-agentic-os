package scheduler

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"runtime"
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
	dir, err := OpenDirectory(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer dir.Close()
	before, err := dir.Stat()
	if err != nil {
		return err
	}
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer root.Close()
	after, err := root.Stat(".")
	if err != nil || !os.SameFile(before, after) {
		return errors.New("private publication directory changed")
	}
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return err
	}
	name := ".pending-" + hex.EncodeToString(nonce[:])
	f, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer root.Remove(name)
	if _, err := f.Write(body); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := root.Rename(name, filepath.Base(path)); err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		return nil
	}
	return dir.Sync()
}
