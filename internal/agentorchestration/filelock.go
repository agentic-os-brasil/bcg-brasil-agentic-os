package agentorchestration

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

var errStateLockBusy = errors.New("orchestration state lock is busy")

func acquireStateFileLock(statePath string) (func() error, error) {
	if err := validateStateParents(statePath); err != nil {
		return nil, err
	}
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return nil, err
	}
	token := hex.EncodeToString(value)
	lockPath := statePath + ".lock"
	deadline := time.Now().Add(2 * time.Second)
	for {
		if info, err := os.Lstat(lockPath); err == nil {
			if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
				return nil, errors.New("orchestration state lock is not a regular file")
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			if _, err := file.WriteString(token); err != nil {
				_ = file.Close()
				_ = os.Remove(lockPath)
				return nil, err
			}
			if err := file.Close(); err != nil {
				_ = os.Remove(lockPath)
				return nil, err
			}
			return func() error {
				body, err := os.ReadFile(lockPath)
				if err != nil {
					return err
				}
				if string(body) != token {
					return errors.New("orchestration state lock ownership changed")
				}
				return os.Remove(lockPath)
			}, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, err
		}
		if time.Now().After(deadline) {
			return nil, errStateLockBusy
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func validateStateParents(path string) error {
	directory := filepath.Dir(path)
	abs, err := filepath.Abs(directory)
	if err != nil {
		return err
	}
	// Collect the components by walking up to the filesystem root, stopping at
	// the fixed point of filepath.Dir rather than at a hardcoded separator. On
	// Windows the root is a volume whose parent is itself (filepath.Dir(`C:\`)
	// is `C:\`), so a separator-only guard never terminates and every caller
	// hangs before it can take the lock. Starting the descent from that same
	// fixed point keeps the walk correct on both platforms.
	parts := []string{}
	current := abs
	for {
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		parts = append([]string{filepath.Base(current)}, parts...)
		current = parent
	}
	for _, part := range parts {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			if err := os.Mkdir(current, 0o700); err != nil {
				return err
			}
			info, err = os.Lstat(current)
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			// macOS exposes the writable temporary tree through these system
			// compatibility links; product-owned links below them are rejected.
			if runtime.GOOS == "darwin" && (current == "/var" || current == "/tmp") {
				continue
			}
			return errors.New("orchestration state parent is not a private directory")
		}
		if !info.IsDir() {
			return errors.New("orchestration state parent is not a private directory")
		}
	}
	return nil
}
