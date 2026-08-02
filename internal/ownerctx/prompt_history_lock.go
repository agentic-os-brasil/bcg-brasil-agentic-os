package ownerctx

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const promptHistoryLockName = ".lock"

var ErrPromptHistoryLocked = errors.New("prompt history store is locked")

func withPromptHistoryLock(root string, operation func(entriesPath, configPath string) error) error {
	entriesPath, configPath, err := ensurePromptHistoryStore(root)
	if err != nil {
		return err
	}
	unlock, err := acquirePromptHistoryLock(filepath.Join(filepath.Dir(entriesPath), promptHistoryLockName))
	if err != nil {
		return err
	}
	operationErr := operation(entriesPath, configPath)
	unlockErr := unlock()
	if operationErr != nil {
		return operationErr
	}
	return unlockErr
}

func acquirePromptHistoryLock(path string) (func() error, error) {
	tokenBytes := make([]byte, 16)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, err
	}
	token := hex.EncodeToString(tokenBytes)
	deadline := time.Now().Add(2 * time.Second)
	for {
		if info, err := os.Lstat(path); err == nil {
			if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
				return nil, fmt.Errorf("prompt history lock is not a private regular file")
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			if _, writeErr := file.WriteString(token); writeErr != nil {
				_ = file.Close()
				_ = os.Remove(path)
				return nil, writeErr
			}
			if closeErr := file.Close(); closeErr != nil {
				_ = os.Remove(path)
				return nil, closeErr
			}
			return func() error { return releasePromptHistoryLock(path, token) }, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, err
		}
		if time.Now().After(deadline) {
			return nil, ErrPromptHistoryLocked
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func releasePromptHistoryLock(path, token string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return errors.New("prompt history lock is not a private regular file")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if string(body) != token {
		return errors.New("prompt history lock ownership changed")
	}
	return os.Remove(path)
}
