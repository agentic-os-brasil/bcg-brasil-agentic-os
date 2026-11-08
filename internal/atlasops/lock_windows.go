//go:build windows

package atlasops

import (
	"errors"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/privatelock"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/winprivate"
	"golang.org/x/sys/windows"
	"os"
	"time"
)

// The actual directories and persistent lock filename remain pinned until the
// journal/page publication and cleanup finish. No pathname is reopened with
// DELETE sharing to broaden custody after the lock is acquired.
func (engine *Engine) acquireCorpusLock() (func() error, error) {
	var held []*winprivate.Directory
	closeHeld := func() {
		for i := len(held) - 1; i >= 0; i-- {
			_ = held[i].Close()
		}
	}
	paths := []string{engine.root, engine.journalDirPath("")}
	if engine.workspace != nil {
		paths = []string{engine.workspace.DataRoot, engine.workspace.WorkspacePath, engine.root, engine.journalDirPath("")}
	}
	for _, path := range paths {
		d, err := winprivate.OpenPinned(path)
		if err != nil {
			closeHeld()
			return nil, err
		}
		held = append(held, d)
	}
	if err := engine.validateBinding(); err != nil {
		closeHeld()
		return nil, err
	}
	journal := held[len(held)-1]
	file, err := journal.OpenLock("grants.lock")
	if err != nil {
		closeHeld()
		return nil, err
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		var overlapped windows.Overlapped
		err = windows.LockFileEx(windows.Handle(file.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &overlapped)
		if err == nil {
			break
		}
		if !errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
			_ = file.Close()
			closeHeld()
			return nil, err
		}
		if time.Now().After(deadline) {
			_ = file.Close()
			closeHeld()
			return nil, privatelock.ErrBusy
		}
		time.Sleep(5 * time.Millisecond)
	}
	if err = engine.validateBinding(); err != nil {
		var o windows.Overlapped
		_ = windows.UnlockFileEx(windows.Handle(file.Fd()), 0, 1, 0, &o)
		_ = file.Close()
		closeHeld()
		return nil, err
	}
	return func() error {
		var o windows.Overlapped
		unlockErr := windows.UnlockFileEx(windows.Handle(file.Fd()), 0, 1, 0, &o)
		closeErr := file.Close()
		closeHeld()
		if unlockErr != nil {
			return unlockErr
		}
		return closeErr
	}, nil
}

// File.Stat eagerly snapshots native file identity from the held handle. A
// path-based Lstat may defer that identity until after a replacement happened.
func boundAtlasDirectory(path string) (os.FileInfo, error) {
	d, err := winprivate.Open(path, false)
	if err != nil {
		return nil, err
	}
	defer d.Close()
	return d.File.Stat()
}
