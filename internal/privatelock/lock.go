// Package privatelock provides an OS-released cross-process boundary for
// user-local scan/create and confirm/compaction transitions.
package privatelock

import (
	"errors"
	"os"
	"path/filepath"
	"time"
)

var ErrBusy = errors.New("private state transition is busy")

func Acquire(path string) (func() error, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		before, err := os.Lstat(path)
		missing := errors.Is(err, os.ErrNotExist)
		if err != nil && !missing {
			return nil, err
		}
		if !missing && (before.Mode()&os.ModeSymlink != 0 || !before.Mode().IsRegular()) {
			return nil, errors.New("private transition lock is not a regular file")
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
		opened, statErr := file.Stat()
		after, lstatErr := os.Lstat(path)
		if statErr != nil || lstatErr != nil || after.Mode()&os.ModeSymlink != 0 || !after.Mode().IsRegular() || !os.SameFile(opened, after) {
			file.Close()
			return nil, errors.New("private transition lock changed during secure open")
		}
		if err := tryLockFile(file); err == nil {
			return func() error {
				unlockErr := unlockFile(file)
				closeErr := file.Close()
				if unlockErr != nil {
					return unlockErr
				}
				return closeErr
			}, nil
		} else if !errors.Is(err, ErrBusy) {
			file.Close()
			return nil, err
		}
		file.Close()
		if time.Now().After(deadline) {
			return nil, ErrBusy
		}
		time.Sleep(5 * time.Millisecond)
	}
}
