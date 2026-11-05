//go:build windows

package ownerctx

import (
	"errors"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/privatelock"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/winprivate"
	"golang.org/x/sys/windows"
	"os"
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
	d, e := winprivate.Open(filepath.Dir(path), true)
	if e != nil {
		return nil, e
	}
	defer d.Close()
	f, e := d.OpenFile(filepath.Base(path), os.O_CREATE|os.O_RDWR)
	if e != nil {
		return nil, e
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		var overlapped windows.Overlapped
		e = windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &overlapped)
		if e == nil {
			return func() error {
				var o windows.Overlapped
				return errors.Join(windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, &o), f.Close())
			}, nil
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
