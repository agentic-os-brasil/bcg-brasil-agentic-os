//go:build windows

package scheduler

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestWindowsSecureSchedulerStateWorksAsStandardUser(t *testing.T) {
	root := t.TempDir()
	store := Store{Root: root}
	now := time.Now().UTC().Truncate(time.Second)
	if _, err := store.EnsureEnrollment("workspace-a", now); err != nil {
		t.Fatalf("enrollment failed under the current user: %v", err)
	}
	if err := store.AppendReceipt("workspace-a", Receipt{
		JobID:        "darwin-housekeeping-daily",
		ScheduledFor: now,
		AttemptedAt:  now,
		State:        Succeeded,
	}); err != nil {
		t.Fatalf("receipt failed under the current user: %v", err)
	}
	lease, err := store.TryAcquireLease("workspace-a", "darwin-housekeeping-daily", "occurrence", "windows-test", now, time.Minute)
	if err != nil {
		t.Fatalf("lease acquisition failed under the current user: %v", err)
	}
	if err := store.ArmLease(lease); err != nil {
		t.Fatalf("lease arm failed under the current user: %v", err)
	}
	if err := store.ReleaseLease(lease); err != nil {
		t.Fatalf("lease release failed under the current user: %v", err)
	}
}

func TestWindowsSecureSchedulerRejectsReparseAncestor(t *testing.T) {
	for _, kind := range []string{"root", "workspace"} {
		t.Run(kind, func(t *testing.T) {
			root, outside := t.TempDir(), t.TempDir()
			storeRoot := root
			alias := filepath.Join(root, "alias")
			if kind == "root" {
				storeRoot = filepath.Join(alias, "scheduler")
			} else {
				if err := EnsurePrivateDirectory(filepath.Join(root, "workspaces")); err != nil {
					t.Fatal(err)
				}
				alias = filepath.Join(root, "workspaces", "workspace-a")
			}
			schedulerDirectoryAlias(t, alias, outside)
			if _, err := (Store{Root: storeRoot}).EnsureEnrollment("workspace-a", time.Now().UTC()); err == nil {
				t.Fatal("scheduler admitted a reparse ancestor")
			}
			if entries, err := os.ReadDir(outside); err != nil || len(entries) != 0 {
				t.Fatalf("scheduler touched the alias target: %v %v", entries, err)
			}
		})
	}
}

func TestWindowsSecureSchedulerCreatesAndReadsNestedDirectory(t *testing.T) {
	root := filepath.Join(t.TempDir(), "BCGOS", "scheduler", "nested")
	if err := EnsurePrivateDirectory(root); err != nil {
		t.Fatalf("create nested scheduler directory: %v", err)
	}
	marker := filepath.Join(root, "marker.json")
	if err := secureWriteFile(marker, []byte(`{"ok":true}`)); err != nil {
		t.Fatalf("write scheduler marker: %v", err)
	}
	entries, err := secureReadDir(root)
	if err != nil {
		t.Fatalf("read nested scheduler directory: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "marker.json" {
		t.Fatalf("nested scheduler entries = %#v", entries)
	}
}

func schedulerDirectoryAlias(t *testing.T, alias, target string) {
	t.Helper()
	shell := filepath.Join(os.Getenv("SystemRoot"), "System32", "cmd.exe")
	if out, err := exec.Command(shell, "/c", "mklink", "/J", alias, target).CombinedOutput(); err != nil {
		t.Fatalf("required ordinary-user junction fixture unavailable: %s %v", out, err)
	}
}

func TestWindowsSecurePathWalkSurvivesAncestorRenameToSymlink(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	workspaces := filepath.Join(root, "workspaces")
	if err := os.Mkdir(workspaces, 0o700); err != nil {
		t.Fatal(err)
	}
	moved := filepath.Join(root, "workspaces-original")
	previous := securePathStepHook
	t.Cleanup(func() { securePathStepHook = previous })
	securePathStepHook = func(component string) {
		if component != "workspaces" {
			return
		}
		if err := os.Rename(workspaces, moved); err != nil {
			t.Fatalf("rename ancestor: %v", err)
		}
		schedulerDirectoryAlias(t, workspaces, outside)
	}
	if _, err := ensurePrivateTree(root, "workspaces", "workspace-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(moved, "workspace-1")); err != nil {
		t.Fatalf("descriptor-relative creation missed original ancestor: %v", err)
	}
	entries, err := os.ReadDir(outside)
	if err != nil || len(entries) != 0 {
		t.Fatalf("path walk followed renamed symlink: entries=%#v err=%v", entries, err)
	}
}

func TestWindowsSecureLeafIOSurvivesAncestorSwapForEnrollmentReceiptAndLease(t *testing.T) {
	cases := []struct {
		name string
		rel  []string
		file string
	}{
		{name: "enrollment", rel: []string{"workspaces", "case-a"}, file: "enrollment.json"},
		{name: "receipt", rel: []string{"workspaces", "case-a", "receipts", "memory-daily"}, file: "receipt.json"},
		{name: "lease", rel: []string{"workspaces", "case-a", "leases", "memory-daily"}, file: "lease.json"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			root, outside := t.TempDir(), t.TempDir()
			parent, err := ensurePrivateTree(root, testCase.rel...)
			if err != nil {
				t.Fatal(err)
			}
			workspaces := filepath.Join(root, "workspaces")
			moved := filepath.Join(root, "workspaces-original")
			fired := false
			previous := secureLeafStepHook
			t.Cleanup(func() { secureLeafStepHook = previous })
			secureLeafStepHook = func(string) {
				if fired {
					return
				}
				fired = true
				if err := os.Rename(workspaces, moved); err != nil {
					t.Fatalf("rename ancestor: %v", err)
				}
				schedulerDirectoryAlias(t, workspaces, outside)
			}
			path := filepath.Join(parent, testCase.file)
			if err := secureWriteNewFile(path, []byte("metadata-only\n")); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(filepath.Join(moved, filepath.Join(append(testCase.rel[1:], testCase.file)...))); err != nil {
				t.Fatalf("secure leaf write did not stay on opened ancestor: %v", err)
			}
			entries, err := os.ReadDir(outside)
			if err != nil || len(entries) != 0 {
				t.Fatalf("secure leaf write followed swapped ancestor: entries=%#v err=%v", entries, err)
			}
		})
	}
}

func TestWindowsSecureLeafReadAndRemoveSurviveAncestorSwap(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	parent, err := ensurePrivateTree(root, "workspaces", "case-a", "receipts", "memory-daily")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(parent, "receipt.json")
	if err := secureWriteNewFile(path, []byte("original\n")); err != nil {
		t.Fatal(err)
	}

	swap := func(fired *bool) func(string) {
		return func(string) {
			if *fired {
				return
			}
			*fired = true
			workspaces := filepath.Join(root, "workspaces")
			moved := filepath.Join(root, "workspaces-original")
			if err := os.Rename(workspaces, moved); err != nil {
				t.Fatalf("rename ancestor: %v", err)
			}
			schedulerDirectoryAlias(t, workspaces, outside)
		}
	}

	previous := secureLeafStepHook
	t.Cleanup(func() { secureLeafStepHook = previous })
	fired := false
	secureLeafStepHook = swap(&fired)
	body, err := secureReadFile(path)
	if err != nil || string(body) != "original\n" {
		t.Fatalf("secure leaf read followed swapped ancestor: body=%q err=%v", body, err)
	}

	// Recreate the lexical ancestor only after the read assertion; the opened
	// descriptor still identifies the moved tree, while the path now points out.
	if err := os.Remove(filepath.Join(root, "workspaces")); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(root, "workspaces-original"), filepath.Join(root, "workspaces")); err != nil {
		t.Fatal(err)
	}
	fired = false
	secureLeafStepHook = swap(&fired)
	if err := secureRemoveFile(path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "workspaces-original", "case-a", "receipts", "memory-daily", "receipt.json")); !os.IsNotExist(err) {
		t.Fatalf("secure leaf remove did not remove original file: %v", err)
	}
	if entries, err := os.ReadDir(outside); err != nil || len(entries) != 0 {
		t.Fatalf("secure leaf remove followed swapped ancestor: entries=%#v err=%v", entries, err)
	}
}

func TestWindowsLeaseTransactionPinsGuardAndStateToOneDirectory(t *testing.T) {
	root := t.TempDir()
	store := Store{Root: root}
	now := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	lease, err := store.TryAcquireLease("case-a", "memory-daily", "occurrence", "worker-a", now, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	first, _, err := store.openLeaseDirectory("case-a", "memory-daily")
	if err != nil {
		t.Fatal(err)
	}
	defer first.close()
	second, _, err := store.openLeaseDirectory("case-a", "memory-daily")
	if err != nil {
		t.Fatal(err)
	}
	defer second.close()
	name := safeLeaseName(lease.OccurrenceKey)
	guard, err := acquireLeaseGuard(first, name+".guard")
	if err != nil {
		t.Fatal(err)
	}
	defer guard.release()
	if _, err := acquireLeaseGuard(second, name+".guard"); !errors.Is(err, errLeaseGuardBusy) {
		t.Fatalf("second pinned worker acquired the shared guard: %v", err)
	}

	workspaces := filepath.Join(root, "workspaces")
	moved := filepath.Join(root, "workspaces-original")
	if err := os.Rename(workspaces, moved); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(workspaces, 0o700); err != nil {
		t.Fatal(err)
	}
	current, err := readLeaseInDirectory(first, name+".json", "pinned")
	if err != nil || !sameLeaseIdentity(current, lease) {
		t.Fatalf("pinned guard/state transaction followed replacement: lease=%#v err=%v", current, err)
	}
	if _, err := os.Stat(filepath.Join(moved, "case-a", "leases", "memory-daily", name+".json")); err != nil {
		t.Fatalf("original pinned state was not retained: %v", err)
	}
	if entries, err := os.ReadDir(workspaces); err != nil || len(entries) != 0 {
		t.Fatalf("replacement directory received state: entries=%#v err=%v", entries, err)
	}
}

func TestWindowsReplacePrivateFileRejectsSymlinkedLeaf(t *testing.T) {
	root := t.TempDir()
	target, alias := filepath.Join(root, "target"), filepath.Join(root, "alias")
	if err := WriteNewPrivateFile(target, []byte("preserve target bytes")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, alias); err != nil {
		t.Fatal("required native file-symlink fixture unavailable", err)
	}
	if err := ReplacePrivateFile(alias, []byte("must not truncate")); err == nil {
		t.Fatal("private replacement followed a symlinked leaf")
	}
	body, err := ReadPrivateFile(target, 128)
	if err != nil || string(body) != "preserve target bytes" {
		t.Fatalf("symlink target changed: %q %v", body, err)
	}
	info, err := os.Lstat(alias)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("refusal replaced the alias", err)
	}
}
