//go:build windows

package ownerctx

import (
	"errors"
	"fmt"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/privatelock"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
	"os"
	"path/filepath"
)

// Product mutation admission remains unavailable on Windows. Preserve source
// callers until the product obtains a qualified ownership/confinement boundary.
func readOwnerFile(path string, maximum int64) ([]byte, error) {
	return scheduler.ReadPrivateFile(path, maximum)
}

func ensureOwnerDirectory(path string) error                   { return scheduler.EnsurePrivateDirectory(path) }
func acquireOwnerTransition(path string) (func() error, error) { return privatelock.Acquire(path) }
func publishOwnerFile(path string, body []byte, exclusive bool) error {
	if !exclusive {
		return legacyAtomicPrivateWrite(path, body)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	if err := validatePrivateParents(filepath.Dir(path)); err != nil {
		return err
	}
	return scheduler.WriteNewPrivateFile(path, body)
}

func legacyAtomicPrivateWrite(path string, body []byte) error {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	if err := validatePrivateParents(directory); err != nil {
		return err
	}
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return fmt.Errorf("private target is not a regular file: %s", path)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	temporary, err := os.CreateTemp(directory, ".private-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(body); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return err
	}
	return syncPrivateDirectory(directory)
}
