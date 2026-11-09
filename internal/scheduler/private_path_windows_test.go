//go:build windows

package scheduler

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/windows"
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

// Windows permits moving an opened directory itself, but refuses moving its
// ancestor while a descendant directory remains held. Both forms preserve
// namespace custody; exercise the observed native refusal with release controls.
func schedulerHeldAncestorRefusesRename(t *testing.T, held *secureDirectory, ancestor, moved string, before os.FileInfo) {
	t.Helper()
	info, err := held.native.File.Stat()
	if err != nil || !info.IsDir() {
		t.Fatal("native descendant custody is unavailable", err)
	}
	selected, err := os.Stat(held.path)
	if err != nil || !os.SameFile(info, selected) {
		t.Fatal("held directory does not identify the selected fixture", err)
	}
	err = os.Rename(ancestor, moved)
	if !errors.Is(err, windows.ERROR_ACCESS_DENIED) && !errors.Is(err, windows.ERROR_SHARING_VIOLATION) {
		t.Fatalf("held native ancestor rename must refuse without effects: %v", err)
	}
	after, err := os.Stat(ancestor)
	if err != nil || !os.SameFile(before, after) {
		t.Fatal("refused rename changed ancestor identity", err)
	}
	if _, err := os.Lstat(moved); !os.IsNotExist(err) {
		t.Fatal("refused rename published a destination", err)
	}
}

func schedulerRenamePositiveControl(t *testing.T, from, to string) {
	t.Helper()
	if err := os.Rename(from, to); err != nil {
		t.Fatal("unheld ancestor rename positive control failed", err)
	}
	if err := os.Rename(to, from); err != nil {
		t.Fatal("unheld ancestor restoration failed", err)
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
			ancestor, moved := filepath.Join(root, "workspaces"), filepath.Join(root, "workspaces-original")
			schedulerRenamePositiveControl(t, ancestor, moved)
			before, err := os.Stat(ancestor)
			if err != nil {
				t.Fatal(err)
			}
			held, err := openSecureDirectory(parent, false)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if held != nil {
					held.close()
				}
			}()
			previous, fired := secureLeafStepHook, false
			t.Cleanup(func() { secureLeafStepHook = previous })
			secureLeafStepHook = func(string) {
				if fired {
					return
				}
				fired = true
				schedulerHeldAncestorRefusesRename(t, held, ancestor, moved, before)
			}
			if err := held.writeNewFile(testCase.file, []byte("metadata-only\n")); err != nil {
				t.Fatal(err)
			}
			if !fired {
				t.Fatal("native write did not exercise held-parent refusal")
			}
			body, err := held.readFile(testCase.file)
			if err != nil || string(body) != "metadata-only\n" {
				t.Fatalf("held native write missed its original directory: %q %v", body, err)
			}
			if entries, err := os.ReadDir(outside); err != nil || len(entries) != 0 {
				t.Fatalf("native refusal redirected bytes outside: %v %v", entries, err)
			}
			secureLeafStepHook = previous
			if err := held.close(); err != nil {
				t.Fatal(err)
			}
			held = nil
			schedulerRenamePositiveControl(t, ancestor, moved)
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
	ancestor, moved := filepath.Join(root, "workspaces"), filepath.Join(root, "workspaces-original")
	schedulerRenamePositiveControl(t, ancestor, moved)
	before, err := os.Stat(ancestor)
	if err != nil {
		t.Fatal(err)
	}
	held, err := openSecureDirectory(parent, false)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if held != nil {
			held.close()
		}
	}()
	previous, attempts := secureLeafStepHook, 0
	t.Cleanup(func() { secureLeafStepHook = previous })
	secureLeafStepHook = func(string) { attempts++; schedulerHeldAncestorRefusesRename(t, held, ancestor, moved, before) }
	body, err := held.readFile("receipt.json")
	if err != nil || string(body) != "original\n" {
		t.Fatalf("native held read lost original bytes: %q %v", body, err)
	}
	if err := held.removeFile("receipt.json"); err != nil {
		t.Fatal(err)
	}
	if attempts != 2 {
		t.Fatalf("expected held read and remove refusal observations, got %d", attempts)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("held remove left the original file", err)
	}
	if entries, err := os.ReadDir(outside); err != nil || len(entries) != 0 {
		t.Fatalf("native refusal redirected read/remove: %v %v", entries, err)
	}
	secureLeafStepHook = previous
	if err := held.close(); err != nil {
		t.Fatal(err)
	}
	held = nil
	schedulerRenamePositiveControl(t, ancestor, moved)
}

func TestWindowsLeaseTransactionPinsGuardAndStateToOneDirectory(t *testing.T) {
	root := t.TempDir()
	store := Store{Root: root}
	now := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	lease, err := store.TryAcquireLease("case-a", "memory-daily", "occurrence", "worker-a", now, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	ancestor, moved := filepath.Join(root, "workspaces"), filepath.Join(root, "workspaces-original")
	schedulerRenamePositiveControl(t, ancestor, moved)
	before, err := os.Stat(ancestor)
	if err != nil {
		t.Fatal(err)
	}
	first, _, err := store.openLeaseDirectory("case-a", "memory-daily")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if first != nil {
			first.close()
		}
	}()
	second, _, err := store.openLeaseDirectory("case-a", "memory-daily")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if second != nil {
			second.close()
		}
	}()
	left, err := first.native.File.Stat()
	if err != nil {
		t.Fatal(err)
	}
	right, err := second.native.File.Stat()
	if err != nil || !os.SameFile(left, right) {
		t.Fatal("workers did not retain the same native lease directory", err)
	}
	name := safeLeaseName(lease.OccurrenceKey)
	guard, err := acquireLeaseGuard(first, name+".guard")
	if err != nil {
		t.Fatal(err)
	}
	defer guard.release()
	if _, err := acquireLeaseGuard(second, name+".guard"); !errors.Is(err, errLeaseGuardBusy) {
		t.Fatalf("second worker acquired held guard: %v", err)
	}
	schedulerHeldAncestorRefusesRename(t, first, ancestor, moved, before)
	current, err := readLeaseInDirectory(first, name+".json", "pinned")
	if err != nil || !sameLeaseIdentity(current, lease) {
		t.Fatalf("held guard/state changed after refused replacement: %+v %v", current, err)
	}
	if _, err := acquireLeaseGuard(second, name+".guard"); !errors.Is(err, errLeaseGuardBusy) {
		t.Fatalf("refused rename released shared exclusion: %v", err)
	}
	guard.release()
	if err := first.close(); err != nil {
		t.Fatal(err)
	}
	first = nil
	if err := second.close(); err != nil {
		t.Fatal(err)
	}
	second = nil
	if err := os.Rename(ancestor, moved); err != nil {
		t.Fatal("released native custody still blocked rename", err)
	}
	if err := os.Mkdir(ancestor, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(moved, "case-a", "leases", "memory-daily", name+".json")); err != nil {
		t.Fatal("original native state was not retained", err)
	}
	if entries, err := os.ReadDir(ancestor); err != nil || len(entries) != 0 {
		t.Fatalf("replacement directory received state: %v %v", entries, err)
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
