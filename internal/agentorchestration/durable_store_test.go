package agentorchestration

import (
	"path/filepath"
	"testing"
)

func TestDurableStoreRestartsFencesReplacementAndReplays(t *testing.T) {
	catalog := loadCatalog(t)
	path := filepath.Join(t.TempDir(), ".bcgos", "maestro-orchestration-state.json")
	store, err := NewDurableStateStore(path, "recovery-cap")
	if err != nil {
		t.Fatal(err)
	}
	claude, err := NewAdapter("claude", catalog, testAuthorizations(), store)
	if err != nil {
		t.Fatal(err)
	}
	if decision := claude.StartBranch("maestro", "maestro-cap", "case-agent-alpha", "run-one", "alpha", "case"); !decision.Allowed {
		t.Fatal(decision)
	}
	oldEpoch := claude.Snapshot().FenceEpoch
	restartedStore, err := NewDurableStateStore(path, "recovery-cap")
	if err != nil {
		t.Fatal(err)
	}
	codex, err := NewAdapter("codex", catalog, testAuthorizations(), restartedStore)
	if err != nil {
		t.Fatal(err)
	}
	if decision := codex.StartBranch("maestro", "maestro-cap", "client-account-agent-alpha", "run-two", "account-alpha", "account"); decision.Allowed || decision.Code != "branch_active" {
		t.Fatalf("parallel replacement was accepted: %#v", decision)
	}
	if decision := codex.Handle(NativeEvent{Name: "collaboration_branch_stop", BranchID: "run-one", ActorID: "case-agent-alpha", ActorCapability: "case-cap", FenceEpoch: oldEpoch + 1}); decision.Allowed || decision.Code != "fence_epoch_mismatch" {
		t.Fatalf("stale replay was accepted: %#v", decision)
	}
	if decision := claude.FinishBranch("case-agent-alpha", "case-cap", "run-one"); !decision.Allowed {
		t.Fatal(decision)
	}
	newStore, err := NewDurableStateStore(path, "recovery-cap")
	if err != nil {
		t.Fatal(err)
	}
	replacement, err := NewAdapter("codex", catalog, testAuthorizations(), newStore)
	if err != nil {
		t.Fatal(err)
	}
	if decision := replacement.StartBranch("maestro", "maestro-cap", "client-account-agent-alpha", "run-two", "account-alpha", "account"); !decision.Allowed {
		t.Fatal(decision)
	}
	if replacement.Snapshot().FenceEpoch <= oldEpoch {
		t.Fatalf("fence epoch did not advance: old=%d new=%d", oldEpoch, replacement.Snapshot().FenceEpoch)
	}
}
