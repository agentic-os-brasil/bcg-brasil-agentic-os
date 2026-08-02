package maestro

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func persistTestDispatchBoundary(root, ownerID, sessionID, dispatchID string, plan Plan, chain ChainState) (DispatchBoundaryState, error) {
	input := DispatchBoundaryInput{Root: root, OwnerID: ownerID, SessionID: sessionID, DispatchID: dispatchID, PromptDigest: SHA256Hex(plan.PlanDigest), PacketDigest: SHA256Hex(chainDigest(chain)), DraftDigest: SHA256Hex("test-draft"), Plan: plan, Chain: chain}
	prepared, err := input.PersistDispatchBoundary()
	if err != nil {
		return DispatchBoundaryState{}, err
	}
	return input.FinalizeDispatchBoundary(prepared)
}

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

func TestPersistChainStateRejectsSymlinkedAncestorAboveSuppliedRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink privilege is not available on all Windows runners")
	}
	base := t.TempDir()
	victim := filepath.Join(base, "victim")
	if err := os.MkdirAll(filepath.Join(victim, "data"), 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "linked-root")
	if err := os.Symlink(victim, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	root := filepath.Join(link, "data")
	if _, err := PersistChainState(root, ChainState{PlanDigest: strings.Repeat("d", 64), Stage: StageCaseExecution}); err == nil {
		t.Fatal("chain persistence followed a symlinked ancestor above the supplied root")
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
	if err := PersistDispatchRecoveryMarker(root, DispatchRecoveryMarker{SchemaVersion: 1, PlanDigest: state.PlanDigest, ArtifactKind: RecoveryArtifactChainState, TargetRef: filepath.Join(root, "owner", "maestro", "chains", state.PlanDigest+".json"), Reason: "different unresolved failure"}); err == nil {
		t.Fatal("unresolved recovery marker was overwritten")
	}
}

func TestDispatchRecoveryMarkersSeparateOccurrencesUnderOnePlan(t *testing.T) {
	root := t.TempDir()
	planDigest := strings.Repeat("e", 64)
	marker := func(dispatchID string) DispatchRecoveryMarker {
		return DispatchRecoveryMarker{SchemaVersion: 1, PlanDigest: planDigest, DispatchID: dispatchID, ArtifactKind: RecoveryArtifactDispatchReceipt, TargetRef: filepath.Join(root, "owner", "maestro", "dispatch", "receipts", dispatchID+".json"), Reason: "pointer durability failure"}
	}
	first := marker("dispatch-one")
	second := marker("dispatch-two")
	if err := PersistDispatchRecoveryMarker(root, first); err != nil {
		t.Fatal(err)
	}
	if err := PersistDispatchRecoveryMarker(root, second); err != nil {
		t.Fatal(err)
	}
	if err := PersistDispatchRecoveryMarker(root, first); err != nil {
		t.Fatalf("same marker was not idempotent: %v", err)
	}
	entries, err := os.ReadDir(filepath.Join(root, "owner", "maestro", "recovery"))
	if err != nil {
		t.Fatal(err)
	}
	markerCount := 0
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) == ".json" {
			markerCount++
		}
	}
	if markerCount != 2 {
		t.Fatalf("same-plan dispatch markers collided: %d marker files", markerCount)
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
	first, err := persistTestDispatchBoundary(root, "owner-a", "session-a", "dispatch-a", planA, chainA)
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
	second, err := persistTestDispatchBoundary(root, "owner-a", "session-a", "dispatch-b", planB, chainB)
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
	third, err := persistTestDispatchBoundary(root, "owner-a", "session-a", "dispatch-b", planB, chainB)
	if err != nil || third.Epoch != second.Epoch {
		t.Fatalf("same boundary was not idempotent: %#v, err=%v", third, err)
	}
	fourth, err := persistTestDispatchBoundary(root, "owner-a", "session-a", "dispatch-c", planB, chainB)
	if err != nil || fourth.Epoch != second.Epoch+1 {
		t.Fatalf("distinct occurrence was not assigned a new epoch: %#v, err=%v", fourth, err)
	}
	if _, err := persistTestDispatchBoundary(root, "owner-a", "session-a", "dispatch-b", planA, chainA); err == nil {
		t.Fatal("occurrence reuse with different content was accepted")
	}
	receipts, err := filepath.Glob(filepath.Join(root, "owner", "maestro", "dispatch", "receipts", "*.json"))
	if err != nil || len(receipts) != 3 {
		t.Fatalf("append-only dispatch receipts = %v, err=%v", receipts, err)
	}
}

func TestDispatchBoundaryFinalizationRestoresPreparedReceiptAfterFsyncFailure(t *testing.T) {
	root := t.TempDir()
	plan, err := PlanFor(caseInput(true))
	if err != nil {
		t.Fatal(err)
	}
	chain, err := NewChain(plan, DefaultLoopPolicy)
	if err != nil {
		t.Fatal(err)
	}
	input := DispatchBoundaryInput{Root: root, OwnerID: "owner-a", SessionID: "session-a", DispatchID: "dispatch-finalize", PromptDigest: SHA256Hex("prompt"), PacketDigest: SHA256Hex("packet"), DraftDigest: SHA256Hex("draft"), Plan: plan, Chain: chain}
	prepared, err := input.PersistDispatchBoundary()
	if err != nil || prepared.Status != "prepared" || !prepared.FinishedAt.IsZero() {
		t.Fatalf("prepared boundary = %#v, err=%v", prepared, err)
	}
	originalSync := syncDispatchDirectoryFunc
	defer func() { syncDispatchDirectoryFunc = originalSync }()
	calls := 0
	syncDispatchDirectoryFunc = func(directory string) error {
		calls++
		if calls == 1 {
			return errors.New("injected finalization fsync failure")
		}
		return originalSync(directory)
	}
	if _, err := input.FinalizeDispatchBoundary(prepared); err == nil {
		t.Fatal("finalization fsync failure was accepted")
	}
	receipts, _, err := scanDispatchReceipts(filepath.Join(root, "owner", "maestro", "dispatch", "receipts"))
	if err != nil || len(receipts) != 1 || receipts[0].Status != "prepared" {
		t.Fatalf("failed finalization did not restore prepared receipt: %#v, err=%v", receipts, err)
	}
	syncDispatchDirectoryFunc = originalSync
	finished, err := input.FinalizeDispatchBoundary(prepared)
	if err != nil || finished.Status != "finished" || finished.FinishedAt.IsZero() {
		t.Fatalf("prepared boundary did not recover: %#v, err=%v", finished, err)
	}
}

func TestDispatchBoundaryUnknownFinalizationStateRecordsOccurrenceRecoveryMarker(t *testing.T) {
	root := t.TempDir()
	plan, err := PlanFor(caseInput(true))
	if err != nil {
		t.Fatal(err)
	}
	chain, err := NewChain(plan, DefaultLoopPolicy)
	if err != nil {
		t.Fatal(err)
	}
	input := DispatchBoundaryInput{Root: root, OwnerID: "owner-a", SessionID: "session-a", DispatchID: "dispatch-unknown-finalization", PromptDigest: SHA256Hex("prompt"), PacketDigest: SHA256Hex("packet"), DraftDigest: SHA256Hex("draft"), Plan: plan, Chain: chain}
	prepared, err := input.PersistDispatchBoundary()
	if err != nil {
		t.Fatal(err)
	}
	originalSync := syncDispatchDirectoryFunc
	defer func() { syncDispatchDirectoryFunc = originalSync }()
	syncDispatchDirectoryFunc = func(directory string) error {
		return errors.New("injected finalization and prepared-restore fsync failure")
	}
	if _, err := input.FinalizeDispatchBoundary(prepared); err == nil || !DispatchBoundaryStateUnknown(err) {
		t.Fatalf("unknown finalization state was not surfaced: %v", err)
	}
	markers, err := filepath.Glob(filepath.Join(root, "owner", "maestro", "recovery", plan.PlanDigest+"-*.json"))
	if err != nil || len(markers) != 1 {
		t.Fatalf("occurrence-specific dispatch recovery marker missing: %v, err=%v", markers, err)
	}
	markerBody, err := os.ReadFile(markers[0])
	if err != nil || !strings.Contains(string(markerBody), "dispatch-unknown-finalization") || !strings.Contains(string(markerBody), RecoveryArtifactDispatchReceipt) {
		t.Fatalf("dispatch recovery marker is not occurrence-bound: %q, err=%v", markerBody, err)
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
	first, err := persistTestDispatchBoundary(root, "owner-a", "session-a", "dispatch-a", plan, chain)
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
	if _, err := persistTestDispatchBoundary(root, "owner-a", "session-a", "dispatch-b", plan, chain); err == nil {
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
	recovered, err := persistTestDispatchBoundary(root, "owner-a", "session-a", "dispatch-b", plan, chain)
	if err != nil || recovered.Epoch != first.Epoch+1 {
		t.Fatalf("same occurrence did not recover its durable pointer: %#v, err=%v", recovered, err)
	}
	current, _, err = readDispatchCurrentPointer(filepath.Join(root, "owner", "maestro", "dispatch"), filepath.Join(root, "owner", "maestro", "dispatch", "receipts"))
	if err != nil || current == nil || current.Epoch != recovered.Epoch {
		t.Fatalf("recovered current pointer = %#v, err=%v", current, err)
	}
}

func TestDispatchBoundaryAllocatesAfterOrphanAndDoesNotRollbackNewerCurrent(t *testing.T) {
	root := t.TempDir()
	plan, err := PlanFor(caseInput(false))
	if err != nil {
		t.Fatal(err)
	}
	chain, err := NewChain(plan, DefaultLoopPolicy)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := persistTestDispatchBoundary(root, "owner-a", "session-a", "dispatch-a", plan, chain); err != nil {
		t.Fatal(err)
	}
	originalSync := syncDispatchDirectoryFunc
	calls := 0
	syncDispatchDirectoryFunc = func(directory string) error {
		calls++
		if calls == 2 {
			return errors.New("injected orphan pointer fsync failure")
		}
		return originalSync(directory)
	}
	if _, err := persistTestDispatchBoundary(root, "owner-a", "session-a", "dispatch-b", plan, chain); err == nil {
		t.Fatal("orphan-producing pointer failure was accepted")
	}
	syncDispatchDirectoryFunc = originalSync
	c, err := persistTestDispatchBoundary(root, "owner-a", "session-a", "dispatch-c", plan, chain)
	if err != nil || c.Epoch != 3 {
		t.Fatalf("orphan epoch was not skipped: %#v, err=%v", c, err)
	}
	b, err := persistTestDispatchBoundary(root, "owner-a", "session-a", "dispatch-b", plan, chain)
	if err != nil || b.Epoch != 2 {
		t.Fatalf("older occurrence retry = %#v, err=%v", b, err)
	}
	current, _, err := readDispatchCurrentPointer(filepath.Join(root, "owner", "maestro", "dispatch"), filepath.Join(root, "owner", "maestro", "dispatch", "receipts"))
	if err != nil || current == nil || current.Epoch != c.Epoch || current.ReceiptName != c.ReceiptName {
		t.Fatalf("older retry rolled back current pointer: %#v, err=%v", current, err)
	}
}

func TestDispatchBoundaryRejectsTamperedPointerBindingAndUnexpectedReceiptEntry(t *testing.T) {
	root := t.TempDir()
	plan, err := PlanFor(caseInput(false))
	if err != nil {
		t.Fatal(err)
	}
	chain, err := NewChain(plan, DefaultLoopPolicy)
	if err != nil {
		t.Fatal(err)
	}
	first, err := persistTestDispatchBoundary(root, "owner-a", "session-a", "dispatch-a", plan, chain)
	if err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(root, "owner", "maestro", "dispatch")
	receiptsDirectory := filepath.Join(directory, "receipts")
	pointer, _, err := readDispatchCurrentPointer(directory, receiptsDirectory)
	if err != nil || pointer == nil {
		t.Fatalf("current pointer = %#v, err=%v", pointer, err)
	}
	pointer.Epoch++
	pointer.PointerDigest = dispatchCurrentPointerDigest(*pointer)
	body, _ := json.Marshal(pointer)
	if err := os.WriteFile(filepath.Join(directory, "current.json"), body, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := persistTestDispatchBoundary(root, "owner-a", "session-a", "dispatch-b", plan, chain); err == nil {
		t.Fatal("self-digested pointer with mismatched receipt epoch was accepted")
	}
	if err := os.WriteFile(filepath.Join(directory, "current.json"), append(body[:0], []byte("invalid")...), 0o600); err != nil {
		t.Fatal(err)
	}
	validPointer := newDispatchCurrentPointer(first)
	validBody, _ := json.Marshal(validPointer)
	if err := os.WriteFile(filepath.Join(directory, "current.json"), validBody, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(receiptsDirectory, "unexpected.txt"), []byte("tamper"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := persistTestDispatchBoundary(root, "owner-a", "session-a", "dispatch-c", plan, chain); err == nil {
		t.Fatal("unexpected regular receipt entry was ignored")
	}
}

func TestDispatchBoundaryRejectsDuplicateSelfDigestedEpochs(t *testing.T) {
	root := t.TempDir()
	plan, err := PlanFor(caseInput(false))
	if err != nil {
		t.Fatal(err)
	}
	chain, err := NewChain(plan, DefaultLoopPolicy)
	if err != nil {
		t.Fatal(err)
	}
	first, err := persistTestDispatchBoundary(root, "owner-a", "session-a", "dispatch-a", plan, chain)
	if err != nil {
		t.Fatal(err)
	}
	duplicate := first
	duplicate.DispatchID = "dispatch-b"
	duplicate.ReceiptName = canonicalDispatchReceiptName(duplicate.Epoch, duplicate.DispatchID)
	duplicate.StateDigest = dispatchBoundaryStateDigest(duplicate)
	body, _ := json.Marshal(duplicate)
	receiptsDirectory := filepath.Join(root, "owner", "maestro", "dispatch", "receipts")
	if err := os.WriteFile(filepath.Join(receiptsDirectory, duplicate.ReceiptName), append(body, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := persistTestDispatchBoundary(root, "owner-a", "session-a", "dispatch-c", plan, chain); err == nil {
		t.Fatal("duplicate self-digested epoch was accepted")
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
	if _, err := persistTestDispatchBoundary(root, "owner-a", "session-a", "dispatch-a", plan, chain); err != nil {
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
	if _, err := persistTestDispatchBoundary(root, "owner-a", "session-a", "dispatch-b", plan, chain); err == nil {
		t.Fatal("symlinked current pointer was followed")
	}
}
