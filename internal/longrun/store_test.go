package longrun

import (
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
)

type memoryAnchor struct {
	mu    sync.Mutex
	heads map[string]AnchorRecord
}

func newMemoryAnchor() *memoryAnchor { return &memoryAnchor{heads: map[string]AnchorRecord{}} }
func (anchor *memoryAnchor) Load(id string) (AnchorRecord, error) {
	anchor.mu.Lock()
	defer anchor.mu.Unlock()
	head, ok := anchor.heads[id]
	if !ok {
		return AnchorRecord{}, os.ErrNotExist
	}
	return head, nil
}
func (anchor *memoryAnchor) Store(head AnchorRecord) error {
	anchor.mu.Lock()
	defer anchor.mu.Unlock()
	previous, exists := anchor.heads[head.GoalID]
	if exists && head.Sequence < previous.Sequence {
		return errors.New("anchor sequence cannot decrease")
	}
	anchor.heads[head.GoalID] = head
	return nil
}
func testStore(t *testing.T) Store {
	t.Helper()
	return Store{Root: t.TempDir(), Anchor: newMemoryAnchor()}
}

func TestStorePersistsAndRestoresResumableGoal(t *testing.T) {
	store := testStore(t)
	goal, err := NewGoal("maestro-pilot", validDoneContract())
	if err != nil {
		t.Fatal(err)
	}
	if err := goal.activate("pilot-runtime"); err != nil {
		t.Fatal(err)
	}
	if err := goal.recordEvidence(Evidence{ID: "test-suite", Class: EvidenceTest, Reference: "test://federation", Verified: true}); err != nil {
		t.Fatal(err)
	}
	if err := store.Create(goal); err != nil {
		t.Fatal(err)
	}
	restored, err := store.Load(goal.ID())
	if err != nil {
		t.Fatal(err)
	}
	if restored.Status() != Active || restored.Phase() != "pilot-runtime" || len(restored.Evidence()) != 1 || restored.Contract().ObjectiveRef != goal.Contract().ObjectiveRef {
		t.Fatalf("restored goal = %#v", restored)
	}
}

func TestStoreRejectsTamperedSnapshotEvenWithSignedReceipts(t *testing.T) {
	store := testStore(t)
	goal, err := NewGoal("maestro-pilot", validDoneContract())
	if err != nil {
		t.Fatal(err)
	}
	if err := goal.activate("pilot-runtime"); err != nil {
		t.Fatal(err)
	}
	if err := store.Create(goal); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(store.statePath("maestro-pilot"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.statePath("maestro-pilot"), []byte(strings.Replace(string(contents), `"status":"active"`, `"status":"completed"`, 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load("maestro-pilot"); err == nil {
		t.Fatal("receipt-backed state accepted a tampered completion")
	}
}

func TestStoreRejectsSignedPrefixRollbackAfterCompletion(t *testing.T) {
	store := testStore(t)
	goal := readyForWalter(t)
	if err := goal.applyWalterReview(approvedReview(goal)); err != nil {
		t.Fatal(err)
	}
	if err := goal.recordCompletionAudit(completionAudit(goal)); err != nil {
		t.Fatal(err)
	}
	if err := goal.complete(); err != nil {
		t.Fatal(err)
	}
	if err := store.Create(goal); err != nil {
		t.Fatal(err)
	}
	key, err := store.loadKey()
	if err != nil {
		t.Fatal(err)
	}
	prefixEvents := goal.Events()[:len(goal.Events())-1]
	prefix, err := NewGoal(goal.ID(), goal.Contract())
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range prefixEvents {
		event.MAC = ""
		if err := replay(prefix, event); err != nil {
			t.Fatal(err)
		}
	}
	if err := writeAtomicJSON(store.statePath(goal.ID()), snapshotWithReceipts(prefix, key)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(goal.ID()); err == nil {
		t.Fatal("signed event prefix rolled back terminal completion")
	}
}

func TestStoreConcurrentCommitsNeverMoveAnchorBackward(t *testing.T) {
	anchor := newMemoryAnchor()
	store := Store{Root: t.TempDir(), Anchor: anchor}
	goal, err := NewActiveGoal("maestro-pilot", validDoneContract(), "pilot-runtime")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Create(goal); err != nil {
		t.Fatal(err)
	}
	newer, err := store.Load(goal.ID())
	if err != nil {
		t.Fatal(err)
	}
	stale, err := store.Load(goal.ID())
	if err != nil {
		t.Fatal(err)
	}
	if err := newer.recordEvidence(Evidence{ID: "test-suite", Class: EvidenceTest, Reference: "test://federation", Verified: true}); err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 2)
	go func() { results <- store.Save(newer) }()
	go func() { results <- store.Save(stale) }()
	for range 2 {
		<-results
	}
	head, err := anchor.Load(goal.ID())
	if err != nil {
		t.Fatal(err)
	}
	if head.Sequence < 1 {
		t.Fatalf("anchor moved backward: %#v", head)
	}
	if _, err := store.Load(goal.ID()); err != nil {
		t.Fatalf("concurrent commit left unrecoverable state: %v", err)
	}
}

func TestStoreRejectsUnknownOrWorkspaceBearingState(t *testing.T) {
	store := testStore(t)
	goal, err := NewActiveGoal("maestro-pilot", validDoneContract(), "pilot-runtime")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Create(goal); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(store.statePath("maestro-pilot"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.statePath("maestro-pilot"), []byte(strings.Replace(string(contents), "{", `{"workspace_path":"/client-secret-CANARY",`, 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load("maestro-pilot"); err == nil {
		t.Fatal("unknown workspace-bearing state was accepted")
	}
}

func TestPublishedGoalStateSchemaIsRecognized(t *testing.T) {
	if err := ValidateSchemaFile("../../schemas/long-running-goal.schema.json"); err != nil {
		t.Fatal(err)
	}
}
