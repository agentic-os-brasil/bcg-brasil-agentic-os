package installtx

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/processwait"
)

func TestActivationLockExitedOwnerFixture(t *testing.T) {
	if os.Getenv("MAESTRO_ACTIVATION_LOCK_FIXTURE") == "exit" {
		os.Exit(0)
	}
}

func exitedActivationOwner(t *testing.T) int {
	t.Helper()
	child := exec.Command(os.Args[0], "-test.run=^TestActivationLockExitedOwnerFixture$")
	child.Env = append(os.Environ(), "MAESTRO_ACTIVATION_LOCK_FIXTURE=exit")
	if err := child.Run(); err != nil {
		t.Fatal(err)
	}
	return child.Process.Pid
}

func TestReconciliationLockSerializesConcurrentReclaimers(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".activation.lock")
	if err := os.WriteFile(path, []byte(fmt.Sprintf("%d\n", exitedActivationOwner(t))), 0600); err != nil {
		t.Fatal(err)
	}
	type acquired struct {
		unlock func()
		err    error
	}
	ready, resume := make(chan struct{}), make(chan struct{})
	first := make(chan acquired, 1)
	go func() {
		u, err := acquireReconciliationLockWith(root, processwait.UntilExit, func(path string) error {
			close(ready)
			<-resume
			return os.Remove(path)
		})
		first <- acquired{u, err}
	}()
	select {
	case <-ready:
	case <-time.After(5 * time.Second):
		close(resume)
		t.Fatal("first reclaimer never reached verified removal")
	}
	// The second canonical reclaimer must finish/refuse while the first is
	// paused; without serialization it replaces the orphan with its live lock.
	secondUnlock, secondErr := acquireReconciliationLock(root)
	close(resume)
	var a acquired
	select {
	case a = <-first:
	case <-time.After(5 * time.Second):
		t.Fatal("first reclaimer did not finish")
	}
	if a.unlock != nil {
		defer a.unlock()
	}
	if secondUnlock != nil {
		defer secondUnlock()
	}
	if a.err != nil || secondErr == nil {
		t.Fatalf("reclaimers both acquired or first failed: first=%v second=%v", a.err, secondErr)
	}
	body, err := os.ReadFile(path)
	if err != nil || string(body) != fmt.Sprintf("%d\n", os.Getpid()) {
		t.Fatal("live successor lock missing or changed", err)
	}
}

func TestReconciliationLockPreservesOrdinaryWriterWinningGap(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".activation.lock")
	if err := os.WriteFile(path, []byte(fmt.Sprintf("%d\n", exitedActivationOwner(t))), 0600); err != nil {
		t.Fatal(err)
	}
	var ordinaryUnlock func()
	var winner os.FileInfo
	unlock, err := acquireReconciliationLockWith(root, processwait.UntilExit, func(path string) error {
		if err := os.Remove(path); err != nil {
			return err
		}
		var e error
		ordinaryUnlock, e = acquireLock(root)
		if e == nil {
			winner, e = os.Lstat(path)
		}
		return e
	})
	if ordinaryUnlock != nil {
		defer ordinaryUnlock()
	}
	if unlock != nil {
		defer unlock()
	}
	current, statErr := os.Lstat(path)
	if err == nil || statErr != nil || winner == nil || !os.SameFile(winner, current) {
		t.Fatal("recovery replaced the ordinary writer", err, statErr)
	}
}

func TestReconciliationLockRetainsOneEmptyPrivateGuard(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".activation.lock")
	var original os.FileInfo
	for i := 0; i < 2; i++ {
		if err := os.WriteFile(path, []byte(fmt.Sprintf("%d\n", exitedActivationOwner(t))), 0600); err != nil {
			t.Fatal(err)
		}
		unlock, err := acquireReconciliationLock(root)
		if err != nil {
			t.Fatal(err)
		}
		unlock()
		if err := ValidateActivationRecoveryGuard(root); err != nil {
			t.Fatal("recovery did not retain its empty private guard", err)
		}
		current, err := os.Lstat(filepath.Join(root, ActivationRecoveryGuardName))
		if err != nil || original != nil && !os.SameFile(original, current) {
			t.Fatal("recovery recreated its retained guard", err)
		}
		original = current
	}
}

func TestReconciliationLockPreservesUnsafeActivationFiles(t *testing.T) {
	dead := fmt.Sprintf("%d\n", exitedActivationOwner(t))
	cases := map[string]string{
		"empty": "", "missing newline": strings.TrimSuffix(dead, "\n"),
		"noncanonical PID": "+" + dead, "invalid PID": "0\n",
		"current live owner": fmt.Sprintf("%d\n", os.Getpid()),
		"oversized":          strings.Repeat("1", 33) + "\n", "hardlink": dead,
	}
	if runtime.GOOS != "windows" {
		cases["public mode"] = dead
		cases["symlink"] = dead
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, ".activation.lock")
			target := filepath.Join(root, "original")
			if err := os.WriteFile(target, []byte(body), 0600); err != nil {
				t.Fatal(err)
			}
			var err error
			switch name {
			case "hardlink":
				err = os.Link(target, path)
			case "symlink":
				err = os.Symlink(target, path)
			default:
				err = os.Rename(target, path)
			}
			if err != nil {
				t.Fatal(err)
			}
			if name == "public mode" {
				if err := os.Chmod(path, 0644); err != nil {
					t.Fatal(err)
				}
			}
			before, err := os.Lstat(path)
			if err != nil {
				t.Fatal(err)
			}
			unlock, err := acquireReconciliationLock(root)
			if unlock != nil {
				unlock()
			}
			after, statErr := os.Lstat(path)
			actual, readErr := os.ReadFile(path)
			if err == nil || statErr != nil || readErr != nil || !os.SameFile(before, after) || string(actual) != body || before.Mode() != after.Mode() {
				t.Fatal("unsafe activation file was reclaimed or changed", err, statErr, readErr)
			}
		})
	}
}

func TestReconciliationLockPreservesUnexpectedRecoveryGuard(t *testing.T) {
	dead := []byte(fmt.Sprintf("%d\n", exitedActivationOwner(t)))
	for _, name := range []string{"nonempty", "directory", "hardlink"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, ".activation.lock")
			if err := os.WriteFile(path, dead, 0600); err != nil {
				t.Fatal(err)
			}
			before, err := os.Lstat(path)
			if err != nil {
				t.Fatal(err)
			}
			guard := filepath.Join(root, ActivationRecoveryGuardName)
			switch name {
			case "nonempty":
				err = os.WriteFile(guard, []byte("legacy"), 0600)
			case "directory":
				err = os.Mkdir(guard, 0700)
			case "hardlink":
				target := filepath.Join(root, "guard-original")
				if err = os.WriteFile(target, nil, 0600); err == nil {
					err = os.Link(target, guard)
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			unlock, err := acquireReconciliationLock(root)
			if unlock != nil {
				unlock()
			}
			after, statErr := os.Lstat(path)
			if err == nil || statErr != nil || !os.SameFile(before, after) {
				t.Fatal("unexpected recovery guard allowed stale removal", err, statErr)
			}
		})
	}
}

func TestReconciliationLockPreservesChangedOrUnknownOwner(t *testing.T) {
	for _, change := range []string{"same bytes new inode", "changed body", "unknown owner"} {
		t.Run(change, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, ".activation.lock")
			body := []byte(fmt.Sprintf("%d\n", exitedActivationOwner(t)))
			if err := os.WriteFile(path, body, 0600); err != nil {
				t.Fatal(err)
			}
			before, err := os.Lstat(path)
			if err != nil {
				t.Fatal(err)
			}
			var expected os.FileInfo
			wait := func(pid int, timeout time.Duration) error {
				if change == "unknown owner" {
					expected = before
					return os.ErrPermission
				}
				if err := processwait.UntilExit(pid, timeout); err != nil {
					return err
				}
				if change == "same bytes new inode" {
					if err := os.Rename(path, path+".retained"); err != nil {
						return err
					}
				} else {
					body = []byte(fmt.Sprintf("%d\n", os.Getpid()))
				}
				if err := os.WriteFile(path, body, 0600); err != nil {
					return err
				}
				var err error
				expected, err = os.Lstat(path)
				return err
			}
			unlock, err := acquireReconciliationLockWith(root, wait, os.Remove)
			if unlock != nil {
				unlock()
			}
			after, statErr := os.Lstat(path)
			actual, readErr := os.ReadFile(path)
			if err == nil || statErr != nil || readErr != nil || expected == nil || !os.SameFile(expected, after) || string(actual) != string(body) {
				t.Fatal("changed or unknown owner was reclaimed", err, statErr, readErr)
			}
			if change == "unknown owner" && !errors.Is(err, os.ErrPermission) {
				t.Fatal("lookup error lost", err)
			}
		})
	}
}
