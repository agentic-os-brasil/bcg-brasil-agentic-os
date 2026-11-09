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
	closeHeld := func() error {
		var result error
		for i := len(held) - 1; i >= 0; i-- {
			result = errors.Join(result, held[i].Close())
		}
		return result
	}
	paths := []string{engine.root, engine.journalDirPath("")}
	if engine.workspace != nil {
		paths = []string{engine.workspace.DataRoot, engine.workspace.WorkspacePath, engine.root, engine.journalDirPath("")}
	}
	for _, path := range paths {
		d, err := winprivate.OpenPinned(path)
		if err != nil {
			return nil, errors.Join(err, closeHeld())
		}
		held = append(held, d)
	}
	if err := engine.validateBinding(); err != nil {
		return nil, errors.Join(err, closeHeld())
	}
	journal := held[len(held)-1]
	file, err := journal.OpenLock("grants.lock")
	if err != nil {
		return nil, errors.Join(err, closeHeld())
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		var overlapped windows.Overlapped
		err = windows.LockFileEx(windows.Handle(file.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &overlapped)
		if err == nil {
			break
		}
		if !errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
			return nil, errors.Join(err, file.Close(), closeHeld())
		}
		if time.Now().After(deadline) {
			return nil, errors.Join(privatelock.ErrBusy, file.Close(), closeHeld())
		}
		time.Sleep(5 * time.Millisecond)
	}
	release := func() error {
		var o windows.Overlapped
		return errors.Join(windows.UnlockFileEx(windows.Handle(file.Fd()), 0, 1, 0, &o), file.Close(), closeHeld())
	}
	info, err := file.Stat()
	if err != nil {
		return nil, errors.Join(err, release())
	}
	// Legacy token writers do not honor the native byte-range lock. Inspect
	// the same qualified handle only after acquiring it; never truncate/repair.
	if info.Size() != 0 {
		return nil, errors.Join(errors.New("private transition lock contains legacy or unexpected data; preserve it and close older clients before retrying"), release())
	}
	if err = engine.validateBinding(); err != nil {
		return nil, errors.Join(err, release())
	}
	return release, nil
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
