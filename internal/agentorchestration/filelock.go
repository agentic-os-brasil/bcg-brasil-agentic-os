package agentorchestration

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
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
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	info, err := os.Lstat(directory)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("orchestration state parent is not a private directory")
	}
	return nil
}
