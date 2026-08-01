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

func TestDispatchBoundaryStoreAcceptsDistinctPlansAndBindsOrderedChain(t *testing.T) {
	root := t.TempDir()
	inputA := caseInput(false)
	planA, err := PlanFor(inputA)
	if err != nil {
		t.Fatal(err)
	}
	chainA, err := NewChain(planA, DefaultLoopPolicy)
	if err != nil {
		t.Fatal(err)
	}
	first, err := PersistDispatchBoundary(root, "owner-a", "session-a", "dispatch-a", planA, chainA)
	if err != nil || first.Epoch != 1 {
		t.Fatalf("first dispatch boundary = %#v, err=%v", first, err)
	}
	inputB := caseInput(false)
	inputB.ScopeID = "transformation-b"
	inputB.AccountScopeID = "client-beta"
	inputB.AvailableAgents[0].ID = "account-agent-client-beta"
	inputB.AvailableAgents[0].ScopeID = "client-beta"
	inputB.AvailableAgents[1].ID = "case-agent-transformation-b"
	inputB.AvailableAgents[1].ScopeID = "transformation-b"
	inputB.AvailableAgents[1].ParentScopeID = "client-beta"
	planB, err := PlanFor(inputB)
	if err != nil {
		t.Fatal(err)
	}
	chainB, err := NewChain(planB, DefaultLoopPolicy)
	if err != nil {
		t.Fatal(err)
	}
	second, err := PersistDispatchBoundary(root, "owner-a", "session-a", "dispatch-b", planB, chainB)
	if err != nil || second.Epoch != 2 {
		t.Fatalf("distinct second dispatch boundary = %#v, err=%v", second, err)
	}
	if first.BindingChainDigest == second.BindingChainDigest {
		t.Fatal("distinct ordered plans shared a binding-chain digest")
	}
	reversed := append([]AgentBinding(nil), planA.Bindings...)
	reversed[0], reversed[1] = reversed[1], reversed[0]
	if orderedBindingDigest(planA) == orderedBindingDigest(Plan{Bindings: reversed}) {
		t.Fatal("ordered Account-to-Case binding digest ignored chain order")
	}
	third, err := PersistDispatchBoundary(root, "owner-a", "session-a", "dispatch-b", planB, chainB)
	if err != nil || third.Epoch != second.Epoch {
		t.Fatalf("same boundary was not idempotent: %#v, err=%v", third, err)
	}
	fourth, err := PersistDispatchBoundary(root, "owner-a", "session-a", "dispatch-c", planB, chainB)
	if err != nil || fourth.Epoch != second.Epoch+1 {
		t.Fatalf("distinct occurrence was not assigned a new epoch: %#v, err=%v", fourth, err)
	}
	if _, err := PersistDispatchBoundary(root, "owner-a", "session-a", "dispatch-b", planA, chainA); err == nil {
		t.Fatal("occurrence reuse with different content was accepted")
	}
	receipts, err := filepath.Glob(filepath.Join(root, "owner", "maestro", "dispatch", "receipts", "*.json"))
	if err != nil || len(receipts) != 3 {
		t.Fatalf("append-only dispatch receipts = %v, err=%v", receipts, err)
	}
}

func TestDispatchBoundaryPointerFailurePreservesPreviousCurrentReceipt(t *testing.T) {
	root := t.TempDir()
	plan, err := PlanFor(caseInput(false))
	if err != nil {
		t.Fatal(err)
	}
	chain, err := NewChain(plan, DefaultLoopPolicy)
	if err != nil {
		t.Fatal(err)
	}
	first, err := PersistDispatchBoundary(root, "owner-a", "session-a", "dispatch-a", plan, chain)
	if err != nil {
		t.Fatal(err)
	}
	originalSync := syncDispatchDirectoryFunc
	defer func() { syncDispatchDirectoryFunc = originalSync }()
	calls := 0
	syncDispatchDirectoryFunc = func(directory string) error {
		calls++
		if calls == 2 {
			return errors.New("injected current pointer fsync failure")
		}
		return originalSync(directory)
	}
	if _, err := PersistDispatchBoundary(root, "owner-a", "session-a", "dispatch-b", plan, chain); err == nil {
		t.Fatal("pointer durability failure was accepted")
	}
	current, body, err := readDispatchCurrentPointer(filepath.Join(root, "owner", "maestro", "dispatch"), filepath.Join(root, "owner", "maestro", "dispatch", "receipts"))
	if err != nil || current == nil || len(body) == 0 || current.Epoch != first.Epoch || current.ReceiptName != first.ReceiptName {
		t.Fatalf("previous current receipt was not preserved: %#v, err=%v", current, err)
	}
	receipts, err := filepath.Glob(filepath.Join(root, "owner", "maestro", "dispatch", "receipts", "*.json"))
	if err != nil || len(receipts) != 2 {
		t.Fatalf("append-only receipts after failed pointer update = %v, err=%v", receipts, err)
	}
	recovered, err := PersistDispatchBoundary(root, "owner-a", "session-a", "dispatch-b", plan, chain)
	if err != nil || recovered.Epoch != first.Epoch+1 {
		t.Fatalf("same occurrence did not recover its durable pointer: %#v, err=%v", recovered, err)
	}
	current, _, err = readDispatchCurrentPointer(filepath.Join(root, "owner", "maestro", "dispatch"), filepath.Join(root, "owner", "maestro", "dispatch", "receipts"))
	if err != nil || current == nil || current.Epoch != recovered.Epoch {
		t.Fatalf("recovered current pointer = %#v, err=%v", current, err)
	}
}

func TestDispatchBoundaryRejectsSymlinkedCurrentPointer(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink privilege is not available on all Windows runners")
	}
	root := t.TempDir()
	plan, err := PlanFor(caseInput(false))
	if err != nil {
		t.Fatal(err)
	}
	chain, err := NewChain(plan, DefaultLoopPolicy)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := PersistDispatchBoundary(root, "owner-a", "session-a", "dispatch-a", plan, chain); err != nil {
		t.Fatal(err)
	}
	currentPath := filepath.Join(root, "owner", "maestro", "dispatch", "current.json")
	victim := filepath.Join(root, "victim-current.json")
	if err := os.WriteFile(victim, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(currentPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(victim, currentPath); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := PersistDispatchBoundary(root, "owner-a", "session-a", "dispatch-b", plan, chain); err == nil {
		t.Fatal("symlinked current pointer was followed")
	}
}
