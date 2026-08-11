package nativeagentflow

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStrategicRouteRequiresCaseAndAccountValidation(t *testing.T) {
	store, err := New(t.TempDir(), "workspace-a")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.BeginTurn("session-a"); err != nil {
		t.Fatal(err)
	}
	if err := store.Start("session-a", "account-1", "client-account-agent"); err != nil {
		t.Fatal(err)
	}
	if err := store.Stop("session-a", "account-1", "client-account-agent"); err != nil {
		t.Fatal(err)
	}
	if ready, _, err := store.Finalize("session-a"); err != nil || ready {
		t.Fatalf("ready=%v err=%v", ready, err)
	}
	if err := store.Start("session-a", "case-1", "case-agent"); err != nil {
		t.Fatal(err)
	}
	if err := store.Stop("session-a", "case-1", "case-agent"); err != nil {
		t.Fatal(err)
	}
	if ready, _, err := store.Finalize("session-a"); err != nil || ready {
		t.Fatalf("ready=%v err=%v", ready, err)
	}
	if err := store.Start("session-a", "account-2", "client-account-agent"); err != nil {
		t.Fatal(err)
	}
	if err := store.Stop("session-a", "account-2", "client-account-agent"); err != nil {
		t.Fatal(err)
	}
	if ready, reason, err := store.Finalize("session-a"); err != nil || !ready || reason != "" {
		t.Fatalf("ready=%v reason=%q err=%v", ready, reason, err)
	}
}

func TestDirectCaseAndOptionalWalterCanFinish(t *testing.T) {
	store, _ := New(t.TempDir(), "workspace-a")
	if err := store.BeginTurn("session-a"); err != nil {
		t.Fatal(err)
	}
	if err := store.Start("session-a", "case-1", "case-agent"); err != nil {
		t.Fatal(err)
	}
	if err := store.Stop("session-a", "case-1", "case-agent"); err != nil {
		t.Fatal(err)
	}
	if ready, _, _ := store.Finalize("session-a"); !ready {
		t.Fatal("direct Case route did not finish")
	}
	if err := store.Start("session-a", "walter-1", "walter"); err != nil {
		t.Fatal(err)
	}
	if err := store.Stop("session-a", "walter-1", "walter"); err != nil {
		t.Fatal(err)
	}
	if ready, _, _ := store.Finalize("session-a"); !ready {
		t.Fatal("Walter-refined route did not finish")
	}
}

func TestWrongOrderAndParallelAgentsFailClosed(t *testing.T) {
	store, _ := New(t.TempDir(), "workspace-a")
	if err := store.BeginTurn("session-a"); err != nil {
		t.Fatal(err)
	}
	if err := store.Start("session-a", "walter-1", "walter"); err == nil {
		t.Fatal("Walter started before Case")
	}
	if err := store.Start("session-a", "account-1", "client-account-agent"); err != nil {
		t.Fatal(err)
	}
	if err := store.Start("session-a", "case-1", "case-agent"); err == nil {
		t.Fatal("parallel Case was allowed")
	}
	if err := store.Stop("session-a", "wrong", "client-account-agent"); err == nil {
		t.Fatal("mismatched stop was allowed")
	}
}

func TestBeginTurnClearsCompletedPriorRoute(t *testing.T) {
	store, _ := New(t.TempDir(), "workspace-a")
	if err := store.Start("session-a", "account-1", "client-account-agent"); err != nil {
		t.Fatal(err)
	}
	if err := store.Stop("session-a", "account-1", "client-account-agent"); err != nil {
		t.Fatal(err)
	}
	if err := store.BeginTurn("session-a"); err != nil {
		t.Fatal(err)
	}
	if ready, _, err := store.Finalize("session-a"); err != nil || !ready {
		t.Fatalf("ready=%v err=%v", ready, err)
	}
}

func TestBeginTurnRetryIsNoOpAndCannotEraseActiveAgent(t *testing.T) {
	store, _ := New(t.TempDir(), "workspace-a")
	if err := store.BeginTurn("session-a"); err != nil {
		t.Fatal(err)
	}
	if err := store.BeginTurn("session-a"); err != nil {
		t.Fatalf("pristine hook retry failed: %v", err)
	}
	if err := store.Start("session-a", "case-1", "case-agent"); err != nil {
		t.Fatal(err)
	}
	if err := store.BeginTurn("session-a"); err == nil {
		t.Fatal("new turn erased an active specialist")
	}
	if err := store.Stop("session-a", "case-1", "case-agent"); err != nil {
		t.Fatalf("active specialist was lost after rejected turn: %v", err)
	}
}

func TestHookRetriesAreIdempotent(t *testing.T) {
	store, _ := New(t.TempDir(), "workspace-a")
	if err := store.Start("session-a", "case-1", "case-agent"); err != nil {
		t.Fatal(err)
	}
	if err := store.Start("session-a", "case-1", "case-agent"); err != nil {
		t.Fatal(err)
	}
	if err := store.Stop("session-a", "case-1", "case-agent"); err != nil {
		t.Fatal(err)
	}
	if err := store.Stop("session-a", "case-1", "case-agent"); err != nil {
		t.Fatal(err)
	}
}

func TestInterruptedLockIsRecoveredWithoutWeakeningLiveLock(t *testing.T) {
	store, _ := New(t.TempDir(), "workspace-a")
	if err := os.MkdirAll(store.root, 0o700); err != nil {
		t.Fatal(err)
	}
	lockPath := store.statePath("session-a") + ".lock"
	if err := os.WriteFile(lockPath, []byte("999999\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := store.Start("session-a", "case-1", "case-agent"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Clean(lockPath)); !os.IsNotExist(err) {
		t.Fatalf("stale lock survived: %v", err)
	}
}
