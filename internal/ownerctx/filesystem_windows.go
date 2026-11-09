//go:build windows

package ownerctx

import (
	"errors"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/privatelock"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/winprivate"
	"golang.org/x/sys/windows"
	"path/filepath"
	"time"
)

func readOwnerFile(path string, maximum int64) ([]byte, error) {
	return winprivate.ReadFile(path, maximum)
}
func ensureOwnerDirectory(path string) error { return winprivate.MkdirAll(path) }
func publishOwnerFile(path string, body []byte, exclusive bool) error {
	d, e := winprivate.Open(filepath.Dir(path), true)
	if e != nil {
		return e
	}
	defer d.Close()
	return d.Publish(filepath.Base(path), body, exclusive)
}
func acquireOwnerTransition(path string) (func() error, error) {
	return acquireOwnerTransitionFile(path, true)
}
func acquireExistingOwnerTransition(path string) (func() error, error) {
	return acquireOwnerTransitionFile(path, false)
}
func acquireOwnerTransitionFile(path string, create bool) (func() error, error) {
	d, e := winprivate.Open(filepath.Dir(path), create)
	if e != nil {
		return nil, e
	}
	defer d.Close()
	open := d.OpenExistingLock
	if create {
		open = d.OpenLock
	}
	f, e := open(filepath.Base(path))
	if e != nil {
		return nil, e
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		var overlapped windows.Overlapped
		e = windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &overlapped)
		if e == nil {
			unlock := func() error {
				var o windows.Overlapped
				return errors.Join(windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, &o), f.Close())
			}
			info, statErr := f.Stat()
			if statErr != nil {
				return nil, errors.Join(statErr, unlock())
			}
			// Legacy token writers do not honor the native byte-range lock.
			if info.Size() != 0 {
				return nil, errors.Join(errors.New("private transition lock contains legacy or unexpected data; preserve it and close older clients before retrying"), unlock())
			}
			return unlock, nil
		}
		if !errors.Is(e, windows.ERROR_LOCK_VIOLATION) {
			f.Close()
			return nil, e
		}
		if time.Now().After(deadline) {
			f.Close()
			return nil, privatelock.ErrBusy
		}
		time.Sleep(5 * time.Millisecond)
	}
}
func syncPrivateDirectory(path string) error { return winprivate.Sync(path) }
