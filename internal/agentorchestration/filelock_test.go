package agentorchestration

import (
	"os"
	"path/filepath"
	"testing"
)

// validateStateParents walks from the filesystem root down to the state
// directory. The walk that collects the components must terminate on every
// platform: on Windows filepath.Dir("C:\\") returns "C:\\" itself, so a guard
// that only stops at "." or the separator never reaches a stop condition and
// the caller hangs before it can take the orchestration state lock.
func TestValidateStateParentsTerminatesAndCreatesTree(t *testing.T) {
	root := t.TempDir()
	statePath := filepath.Join(root, "BCGOS", "agents", "state.json")

	if err := validateStateParents(statePath); err != nil {
		t.Fatalf("validate state parents: %v", err)
	}

	directory := filepath.Dir(statePath)
	info, err := os.Lstat(directory)
	if err != nil {
		t.Fatalf("state parent was not created: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("state parent is not a directory: %v", info.Mode())
	}
}

func TestAcquireStateFileLockReleasesAndIsExclusive(t *testing.T) {
	root := t.TempDir()
	statePath := filepath.Join(root, "BCGOS", "agents", "state.json")

	release, err := acquireStateFileLock(statePath)
	if err != nil {
		t.Fatalf("acquire state lock: %v", err)
	}
	if _, err := acquireStateFileLock(statePath); err == nil {
		t.Fatal("a second holder acquired the orchestration state lock")
	}
	if err := release(); err != nil {
		t.Fatalf("release state lock: %v", err)
	}

	again, err := acquireStateFileLock(statePath)
	if err != nil {
		t.Fatalf("reacquire after release: %v", err)
	}
	if err := again(); err != nil {
		t.Fatalf("release after reacquire: %v", err)
	}
}
