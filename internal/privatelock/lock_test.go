package privatelock

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestPersistentUnlockedFileIsRecoverable(t *testing.T) {
	for _, test := range []struct{ name, body string }{{"empty", ""}, {"legacy-data", "stale legacy sentinel"}} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), ".transition.lock")
			if err := os.WriteFile(path, []byte(test.body), 0o600); err != nil {
				t.Fatal(err)
			}
			unlock, err := Acquire(path)
			if runtime.GOOS == "windows" && test.body != "" {
				if err == nil || unlock != nil {
					t.Fatal("legacy token data acquired a native lock")
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				if err := unlock(); err != nil {
					t.Fatal(err)
				}
			}
			body, err := os.ReadFile(path)
			if err != nil || string(body) != test.body {
				t.Fatal("recovery changed retained guard bytes", err)
			}
		})
	}
}

func TestAdvisoryLockIsReleasedWhenOwnerProcessCrashes(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".transition.lock")
	command := exec.Command(os.Args[0], "-test.run=TestPrivateLockCrashHelper", "--", path)
	command.Env = append(os.Environ(), "BCGOS_PRIVATE_LOCK_CRASH_HELPER=1")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("crash helper: %v (%s)", err, output)
	}
	unlock, err := Acquire(path)
	if err != nil {
		t.Fatalf("lock was not released by process exit: %v", err)
	}
	if err := unlock(); err != nil {
		t.Fatal(err)
	}
}

func TestPrivateLockCrashHelper(t *testing.T) {
	if os.Getenv("BCGOS_PRIVATE_LOCK_CRASH_HELPER") != "1" {
		return
	}
	path := os.Args[len(os.Args)-1]
	if _, err := Acquire(path); err != nil {
		os.Exit(2)
	}
	os.Exit(0)
}
