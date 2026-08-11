package agentorchestration

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
)

func TestDurableStateRejectsUnknownFieldsNullPermissiveModesAndOversizedJSON(t *testing.T) {
	tests := []struct {
		name string
		body []byte
		mode os.FileMode
	}{
		{name: "unknown field", body: []byte(`{"unknown":true}` + "\n"), mode: 0o600},
		{name: "null snapshot", body: []byte("null\n"), mode: 0o600},
		{name: "permissive mode", body: []byte("{}\n"), mode: 0o644},
		{name: "oversized snapshot", body: []byte(`{"policy_sha256":"` + strings.Repeat("x", MaximumDurableStateBytes) + `"}` + "\n"), mode: 0o600},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.name == "permissive mode" && runtime.GOOS == "windows" {
				t.Skip("Unix mode bits are not an authority on Windows")
			}
			path := filepath.Join(t.TempDir(), "maestro-orchestration-state.json")
			if err := os.WriteFile(path, test.body, test.mode); err != nil {
				t.Fatal(err)
			}
			if _, err := NewDurableStateStore(path, "recovery-cap"); err == nil {
				t.Fatal("invalid durable state was accepted")
			}
			if err := EnsureDurableState(path, "recovery-cap"); err == nil {
				t.Fatal("invalid durable state was accepted by ensure")
			}
		})
	}
}

func TestEnsureDurableStateRepairsEmptyFileAndRejectsSymlink(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "maestro-orchestration-state.json")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := EnsureDurableState(path, "recovery-cap"); err != nil {
		t.Fatal(err)
	}
	store, err := NewDurableStateStore(path, "recovery-cap")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(store.Snapshot(), StateSnapshot{}) {
		t.Fatalf("repaired state = %#v, want empty snapshot", store.Snapshot())
	}
	outside := filepath.Join(directory, "outside.json")
	if err := os.WriteFile(outside, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, path); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if err := EnsureDurableState(path, "recovery-cap"); err == nil {
		t.Fatal("symlink state was accepted")
	}
}

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

func TestDurableStoreConcurrentInstancesFenceStaleStart(t *testing.T) {
	catalog := loadCatalog(t)
	path := filepath.Join(t.TempDir(), ".bcgos", "maestro-orchestration-state.json")
	first, err := NewDurableStateStore(path, "recovery-cap")
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewDurableStateStore(path, "recovery-cap")
	if err != nil {
		t.Fatal(err)
	}
	left, err := NewAdapter("claude", catalog, testAuthorizations(), first)
	if err != nil {
		t.Fatal(err)
	}
	right, err := NewAdapter("codex", catalog, testAuthorizations(), second)
	if err != nil {
		t.Fatal(err)
	}
	decisions := make(chan Decision, 2)
	var wait sync.WaitGroup
	wait.Add(2)
	go func() {
		defer wait.Done()
		decisions <- left.StartBranch("maestro", "maestro-cap", "case-agent-alpha", "run-left", "alpha", "case")
	}()
	go func() {
		defer wait.Done()
		decisions <- right.StartBranch("maestro", "maestro-cap", "client-account-agent-alpha", "run-right", "account-alpha", "account")
	}()
	wait.Wait()
	close(decisions)
	allowedCount := 0
	for decision := range decisions {
		if decision.Allowed {
			allowedCount++
		}
	}
	if allowedCount != 1 {
		t.Fatalf("concurrent stale instances allowed %d branches", allowedCount)
	}
}

func TestAuthorizeActiveRootRefreshesBeforeAuthorizingStaleInstance(t *testing.T) {
	catalog := loadCatalog(t)
	path := filepath.Join(t.TempDir(), ".bcgos", "maestro-orchestration-state.json")
	first, err := NewDurableStateStore(path, "recovery-cap")
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewDurableStateStore(path, "recovery-cap")
	if err != nil {
		t.Fatal(err)
	}
	left, err := NewAdapter("claude", catalog, testAuthorizations(), first)
	if err != nil {
		t.Fatal(err)
	}
	right, err := NewAdapter("codex", catalog, testAuthorizations(), second)
	if err != nil {
		t.Fatal(err)
	}
	if decision := left.StartBranch("maestro", "maestro-cap", "case-agent-alpha", "run-one", "alpha", "case"); !decision.Allowed {
		t.Fatal(decision)
	}
	if decision := right.AuthorizeActiveRoot("case-agent-alpha", "case-cap", "run-one", "alpha", "case"); !decision.Allowed {
		t.Fatalf("stale instance did not refresh active branch: %#v", decision)
	}
	if decision := left.FinishBranch("case-agent-alpha", "case-cap", "run-one"); !decision.Allowed {
		t.Fatal(decision)
	}
	if decision := left.StartBranch("maestro", "maestro-cap", "client-account-agent-alpha", "run-two", "account-alpha", "account"); !decision.Allowed {
		t.Fatal(decision)
	}
	if decision := right.AuthorizeActiveRoot("case-agent-alpha", "case-cap", "run-one", "alpha", "case"); decision.Allowed {
		t.Fatalf("stale branch was authorized after epoch change: %#v", decision)
	}
}
