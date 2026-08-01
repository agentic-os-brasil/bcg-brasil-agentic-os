package maestro

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPersistChainStateRecoversExpiredLease(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, "owner", "maestro", "chains")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, ".lock"), []byte("dead-owner\n1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	state := ChainState{PlanDigest: strings.Repeat("a", 64), Stage: StageCaseExecution}
	path, err := PersistChainState(root, state)
	if err != nil {
		t.Fatalf("expired chain lease was not recovered: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}

func TestPersistChainStateRejectsSymlinkedOwnerAncestor(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink privilege is not available on all Windows runners")
	}
	root := t.TempDir()
	victim := filepath.Join(root, "victim")
	if err := os.Mkdir(victim, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(victim, filepath.Join(root, "owner")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := PersistChainState(root, ChainState{PlanDigest: strings.Repeat("b", 64), Stage: StageCaseExecution}); err == nil {
		t.Fatal("chain persistence followed a symlinked owner ancestor")
	}
}

func TestPersistChainStatePostRenameFailureLeavesRecoveryMarker(t *testing.T) {
	root := t.TempDir()
	originalSync := syncChainDirectoryFunc
	defer func() { syncChainDirectoryFunc = originalSync }()
	calls := 0
	syncChainDirectoryFunc = func(directory string) error {
		calls++
		if calls <= 2 {
			return errors.New("injected directory fsync failure")
		}
		return originalSync(directory)
	}
	state := ChainState{PlanDigest: strings.Repeat("c", 64), Stage: StageCaseExecution}
	if _, err := PersistChainState(root, state); err == nil {
		t.Fatal("post-rename fsync failure was accepted")
	}
	markerPath := filepath.Join(root, "owner", "maestro", "recovery", state.PlanDigest+".json")
	if _, err := os.Stat(markerPath); err != nil {
		t.Fatalf("recovery marker missing after cleanup fsync failure: %v", err)
	}
	markerBody, err := os.ReadFile(markerPath)
	if err != nil || strings.Contains(string(markerBody), state.PlanDigest[:8]) == false {
		t.Fatalf("recovery marker is invalid: %q, err=%v", markerBody, err)
	}
	if err := PersistDispatchRecoveryMarker(root, DispatchRecoveryMarker{SchemaVersion: 1, PlanDigest: state.PlanDigest, ChainPath: filepath.Join(root, "owner", "maestro", "chains", state.PlanDigest+".json"), Reason: "different unresolved failure"}); err == nil {
		t.Fatal("unresolved recovery marker was overwritten")
	}
}
