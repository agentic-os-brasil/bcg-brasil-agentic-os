package maestro

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type DispatchBoundaryReceipt struct {
	SchemaVersion       int    `json:"schema_version"`
	PlanDigest          string `json:"plan_digest"`
	ChainDigest         string `json:"chain_digest"`
	PacketDigest        string `json:"packet_digest"`
	PromptDigest        string `json:"prompt_digest"`
	DraftDigest         string `json:"draft_digest"`
	AccountConsultation bool   `json:"account_consultation_required"`
	WalterRequired      bool   `json:"walter_required"`
	HistoryCount        int    `json:"history_count"`
	DurableFenceEpoch   uint64 `json:"durable_fence_epoch"`
	State               Stage  `json:"state"`
	Outcome             string `json:"outcome"`
}

type DispatchRecoveryMarker struct {
	SchemaVersion int       `json:"schema_version"`
	PlanDigest    string    `json:"plan_digest"`
	PromptID      string    `json:"prompt_id,omitempty"`
	ChainPath     string    `json:"chain_path,omitempty"`
	Reason        string    `json:"reason"`
	RecordedAt    time.Time `json:"recorded_at"`
}

var syncChainDirectoryFunc = syncChainDirectory

func PersistChainState(root string, state ChainState) (string, error) {
	if state.PlanDigest == "" {
		return "", errors.New("cannot persist a chain without a plan digest")
	}
	body, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return "", err
	}
	storedBody := append(append([]byte(nil), body...), '\n')
	directory := filepath.Join(root, "owner", "maestro", "chains")
	if err := ensurePrivateChainTree(root, directory); err != nil {
		return "", err
	}
	path := filepath.Join(directory, state.PlanDigest+".json")
	unlock, err := acquireChainLock(directory)
	if err != nil {
		return "", err
	}
	defer func() { _ = unlock() }()
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return "", errors.New("chain state target is not a regular private file")
		}
		current, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		if string(current) == string(storedBody) {
			return path, nil
		}
		return "", errors.New("chain state conflict for plan digest")
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	temporary, err := os.CreateTemp(directory, ".chain-*")
	if err != nil {
		return "", err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return "", err
	}
	if _, err := temporary.Write(storedBody); err != nil {
		_ = temporary.Close()
		return "", err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return "", err
	}
	if err := temporary.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return "", err
	}
	if err := syncChainDirectoryFunc(directory); err != nil {
		cleanupErr := os.Remove(path)
		if cleanupErr == nil {
			cleanupErr = syncChainDirectoryFunc(directory)
		}
		if cleanupErr != nil {
			markerErr := PersistDispatchRecoveryMarker(root, DispatchRecoveryMarker{SchemaVersion: 1, PlanDigest: state.PlanDigest, ChainPath: path, Reason: "chain post-rename durability failure"})
			if markerErr != nil {
				return "", fmt.Errorf("chain durability failed: %v; cleanup failed: %v; recovery marker failed: %w", err, cleanupErr, markerErr)
			}
			return "", fmt.Errorf("chain durability failed; recovery marker recorded: %w", err)
		}
		return "", fmt.Errorf("chain durability failed after rename; chain cleanup completed: %w", err)
	}
	return path, nil
}

func RemoveChainState(root string, state ChainState) error {
	if state.PlanDigest == "" {
		return errors.New("cannot remove a chain without a plan digest")
	}
	directory := filepath.Join(root, "owner", "maestro", "chains")
	if err := ensurePrivateChainTree(root, directory); err != nil {
		return err
	}
	unlock, err := acquireChainLock(directory)
	if err != nil {
		return err
	}
	defer func() { _ = unlock() }()
	path := filepath.Join(directory, state.PlanDigest+".json")
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return errors.New("chain state target is not a regular private file")
	}
	if err := os.Remove(path); err != nil {
		return err
	}
	return syncChainDirectoryFunc(directory)
}

func PersistDispatchRecoveryMarker(root string, marker DispatchRecoveryMarker) error {
	if marker.PlanDigest == "" || marker.Reason == "" {
		return errors.New("dispatch recovery marker is incomplete")
	}
	directory := filepath.Join(root, "owner", "maestro", "recovery")
	if err := ensurePrivateChainTree(root, directory); err != nil {
		return err
	}
	if marker.RecordedAt.IsZero() {
		marker.RecordedAt = time.Now().UTC()
	}
	body, err := json.MarshalIndent(marker, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(directory, marker.PlanDigest+".json")
	unlock, err := acquireChainLock(directory)
	if err != nil {
		return err
	}
	defer func() { _ = unlock() }()
	if existing, err := os.ReadFile(path); err == nil {
		var prior DispatchRecoveryMarker
		if json.Unmarshal(existing, &prior) == nil && prior.PlanDigest == marker.PlanDigest && prior.PromptID == marker.PromptID && prior.ChainPath == marker.ChainPath && prior.Reason == marker.Reason {
			return nil
		}
		return errors.New("unresolved dispatch recovery marker already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	temporary, err := os.CreateTemp(directory, ".recovery-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(append(body, '\n')); err != nil {
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
	return syncChainDirectoryFunc(directory)
}

func ensurePrivateChainTree(root, directory string) error {
	root, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return err
	}
	rootInfo, err := os.Lstat(root)
	if err != nil {
		return err
	}
	if rootInfo.Mode()&os.ModeSymlink != 0 || !rootInfo.IsDir() {
		return errors.New("chain state root is not a private directory")
	}
	relative, err := filepath.Rel(root, directory)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return errors.New("chain state directory escapes owner root")
	}
	current := root
	for _, part := range splitRelativePath(relative) {
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
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return errors.New("chain state parent is not a private directory")
		}
	}
	return nil
}

func splitRelativePath(value string) []string {
	if value == "." || value == "" {
		return nil
	}
	parts := []string{}
	for value != "." && value != string(filepath.Separator) {
		parts = append([]string{filepath.Base(value)}, parts...)
		value = filepath.Dir(value)
	}
	return parts
}

func acquireChainLock(directory string) (func() error, error) {
	lockPath := filepath.Join(directory, ".lock")
	deadline := time.Now().Add(2 * time.Second)
	for {
		value := make([]byte, 16)
		if _, err := rand.Read(value); err != nil {
			return nil, err
		}
		token := fmt.Sprintf("%x", value)
		lease := fmt.Sprintf("%s\n%d\n", token, time.Now().Add(3*time.Second).UnixNano())
		file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			if _, err := file.WriteString(lease); err != nil {
				_ = file.Close()
				_ = os.Remove(lockPath)
				return nil, err
			}
			if err := file.Sync(); err != nil {
				_ = file.Close()
				_ = os.Remove(lockPath)
				return nil, err
			}
			if err := file.Close(); err != nil {
				_ = os.Remove(lockPath)
				return nil, err
			}
			return func() error {
				current, err := os.ReadFile(lockPath)
				if err != nil {
					return err
				}
				if string(current) != lease {
					return errors.New("chain state lock ownership changed")
				}
				return os.Remove(lockPath)
			}, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, err
		}
		info, statErr := os.Lstat(lockPath)
		if statErr != nil {
			if errors.Is(statErr, os.ErrNotExist) {
				continue
			}
			return nil, statErr
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return nil, errors.New("chain state lock is not a private regular file")
		}
		leaseBody, readErr := os.ReadFile(lockPath)
		if readErr != nil {
			return nil, readErr
		}
		lines := strings.Split(strings.TrimSpace(string(leaseBody)), "\n")
		if len(lines) != 2 {
			return nil, errors.New("chain state lock lease is invalid")
		}
		var expiry int64
		if _, scanErr := fmt.Sscanf(lines[1], "%d", &expiry); scanErr != nil {
			return nil, errors.New("chain state lock lease is invalid")
		}
		if time.Now().UnixNano() < expiry {
			if time.Now().After(deadline) {
				return nil, errors.New("chain state lock is busy")
			}
			time.Sleep(5 * time.Millisecond)
			continue
		}
		if err := os.Remove(lockPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}
}

func syncChainDirectory(directory string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	file, err := os.Open(directory)
	if err != nil {
		return err
	}
	defer file.Close()
	return file.Sync()
}
