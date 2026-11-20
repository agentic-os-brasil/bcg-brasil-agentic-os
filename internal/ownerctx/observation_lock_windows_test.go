//go:build windows

package ownerctx

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func TestWindowsObservationLockSupportsMutationSnapshotAndExclusion(t *testing.T) {
	root := t.TempDir()
	if _, err := Initialize(root); err != nil {
		t.Fatal(err)
	}
	if err := withObservationReadSnapshot(root, func(records []observationRecord) error {
		if len(records) != 0 {
			return errors.New("fresh snapshot is not empty")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(observationDirectory(root)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("fresh snapshot created observation state", err)
	}

	receipt, _, err := AppendObservation(root, observationInput(SignalExplicitCorrection, "native_lock", "episode-native-lock", true, true))
	if err != nil || !receipt.Persisted {
		t.Fatal("native observation append failed", receipt, err)
	}
	lockPath := filepath.Join(observationDirectory(root), promptHistoryLockName)
	before, err := os.Stat(lockPath)
	if err != nil || before.Size() != 0 {
		t.Fatal("persistent native guard is not empty", err)
	}
	if err := withObservationReadSnapshot(root, func(records []observationRecord) error {
		if len(records) != 1 || records[0].ID != receipt.ID {
			return errors.New("snapshot lost the appended observation")
		}
		return nil
	}); err != nil {
		t.Fatal("native observation snapshot failed", err)
	}
	if err := withObservationLock(root, func(string) error {
		competing, err := os.OpenFile(lockPath, os.O_RDWR, 0600)
		if err != nil {
			return err
		}
		var overlapped windows.Overlapped
		lockErr := windows.LockFileEx(windows.Handle(competing.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &overlapped)
		if lockErr == nil {
			windows.UnlockFileEx(windows.Handle(competing.Fd()), 0, 1, 0, &overlapped)
		}
		closeErr := competing.Close()
		if !errors.Is(lockErr, windows.ERROR_LOCK_VIOLATION) || closeErr != nil {
			return errors.Join(errors.New("native exclusion was not held during mutation"), lockErr, closeErr)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(lockPath)
	if err != nil || !os.SameFile(before, after) || after.Size() != 0 {
		t.Fatal("mutation or snapshot replaced the native guard", err)
	}
}

func TestWindowsObservationLocksPreserveLegacyToken(t *testing.T) {
	root := t.TempDir()
	if _, err := Initialize(root); err != nil {
		t.Fatal(err)
	}
	if err := ensureOwnerDirectory(observationDirectory(root)); err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(observationDirectory(root), promptHistoryLockName)
	const token = "0123456789abcdef0123456789abcdef"
	if err := publishOwnerFile(lockPath, []byte(token), true); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	called := false
	err = withObservationLock(root, func(string) error { called = true; return nil })
	if err == nil || !strings.Contains(err.Error(), "legacy") || called {
		t.Fatal("legacy token admitted a mutation", err, called)
	}
	err = withObservationReadSnapshot(root, func([]observationRecord) error { called = true; return nil })
	if err == nil || !strings.Contains(err.Error(), "legacy") || called {
		t.Fatal("legacy token admitted a snapshot", err, called)
	}
	body, err := readOwnerFile(lockPath, 4096)
	if err != nil || string(body) != token {
		t.Fatal("legacy token changed", err)
	}
	after, err := os.Stat(lockPath)
	if err != nil || !os.SameFile(before, after) || after.Size() != int64(len(token)) {
		t.Fatal("legacy token file replaced", err)
	}
	if _, err := os.Lstat(filepath.Join(observationDirectory(root), "observations.jsonl")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("legacy refusal created observation records", err)
	}
}
