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

func TestWindowsPromptHistoryLockRetainsGuardAcrossReaderAndReacquire(t *testing.T) {
	path := filepath.Join(t.TempDir(), promptHistoryLockName)
	unlock, err := acquirePromptHistoryLock(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if unlock != nil {
			unlock()
		}
	})
	before, err := os.Stat(path)
	if err != nil || before.Size() != 0 {
		t.Fatal("native guard is not empty", err)
	}
	competing, err := os.OpenFile(path, os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	var overlapped windows.Overlapped
	err = windows.LockFileEx(windows.Handle(competing.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &overlapped)
	if err == nil {
		windows.UnlockFileEx(windows.Handle(competing.Fd()), 0, 1, 0, &overlapped)
	}
	closeErr := competing.Close()
	if !errors.Is(err, windows.ERROR_LOCK_VIOLATION) || closeErr != nil {
		t.Fatal("independent handle did not observe native exclusion", err, closeErr)
	}
	if err := os.Remove(path); err == nil {
		t.Fatal("held native guard could be deleted")
	}
	replacement := filepath.Join(filepath.Dir(path), "unrelated")
	if err := os.WriteFile(replacement, []byte("unrelated bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, path); err == nil {
		t.Fatal("held native guard could be replaced")
	}
	if body, err := os.ReadFile(replacement); err != nil || string(body) != "unrelated bytes" {
		t.Fatal("replacement refusal changed unrelated bytes", err)
	}
	control := filepath.Join(filepath.Dir(path), "unheld-control")
	if err := os.WriteFile(control, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, control); err != nil {
		t.Fatal("ordinary same-directory replacement control failed", err)
	}
	if body, err := os.ReadFile(control); err != nil || string(body) != "unrelated bytes" {
		t.Fatal("ordinary replacement control changed bytes", err)
	}
	legacy, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if legacy != nil {
		legacy.Close()
	}
	if !errors.Is(err, os.ErrExist) {
		t.Fatal("legacy token writer did not observe the retained guard", err)
	}
	reader, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	if err := unlock(); err != nil {
		t.Fatal("release tried to remove the guard while a reader retained it", err)
	}
	unlock = nil
	after, err := os.Stat(path)
	if err != nil || !os.SameFile(before, after) || after.Size() != 0 {
		t.Fatal("release lost the persistent guard", err)
	}
	unlock, err = acquirePromptHistoryLock(path)
	if err != nil {
		t.Fatal("retained reader blocked reacquiring the same empty guard", err)
	}
	if err := unlock(); err != nil {
		t.Fatal(err)
	}
	unlock = nil
	after, err = os.Stat(path)
	if err != nil || !os.SameFile(before, after) || after.Size() != 0 {
		t.Fatal("reacquisition replaced or changed the guard", err)
	}
}

func TestWindowsPromptHistoryLockPreservesLegacyTokenUntilItsOwnerReleases(t *testing.T) {
	path := filepath.Join(t.TempDir(), promptHistoryLockName)
	const token = "0123456789abcdef0123456789abcdef"
	legacy, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, writeErr := legacy.WriteString(token)
	if err := errors.Join(writeErr, legacy.Close()); err != nil {
		t.Fatal(err)
	}
	unlock, err := acquirePromptHistoryLock(path)
	if unlock != nil {
		unlock()
	}
	if err == nil || errors.Is(err, ErrPromptHistoryLocked) || !strings.Contains(err.Error(), "legacy or unexpected data") {
		t.Fatal("nonempty legacy guard was not refused distinctly", err)
	}
	if body, err := os.ReadFile(path); err != nil || string(body) != token {
		t.Fatal("refusal changed legacy token bytes", err)
	}
	if err := releasePromptHistoryLock(path, token); err != nil {
		t.Fatal("native refusal prevented the legacy owner's own release", err)
	}
	unlock, err = acquirePromptHistoryLock(path)
	if err != nil {
		t.Fatal("native lock remained held after legacy-token refusal", err)
	}
	if err := unlock(); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(path); err != nil || info.Size() != 0 {
		t.Fatal("native release did not retain its empty guard", err)
	}
}
