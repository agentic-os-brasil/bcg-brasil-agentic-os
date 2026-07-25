package execution

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	testWorkspaceID    = "0123456789abcdef0123456789abcdef"
	anotherWorkspaceID = "abcdef0123456789abcdef0123456789"
)

func testStore(t *testing.T) Store {
	t.Helper()
	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	ids := []string{"item-a", "attempt-a", "checkpoint-a", "attempt-b", "checkpoint-b"}
	return Store{
		Root: t.TempDir(),
		Now:  func() time.Time { return now },
		NewID: func(kind string) (string, error) {
			if len(ids) == 0 {
				t.Fatal("unexpected ID request")
			}
			id := ids[0]
			ids = ids[1:]
			return id, nil
		},
	}
}

func testCreateInput() CreateInput {
	return CreateInput{
		WorkspaceID:     testWorkspaceID,
		Objective:       "Implement a resumable execution contract.",
		InitialNextStep: "Write the smallest failing contract test.",
		Criteria: []Criterion{
			{ID: "unit-tests", Type: CriterionCommandCheck, Command: []string{"go", "version"}},
			{ID: "artifact", Type: CriterionArtifactSnapshot, TargetRef: "bcgos://workspace/specs/018"},
		},
		AllowedRefs: []string{"bcgos://workspace/specs/018"},
	}
}

func TestCreateStartInspectAndRestart(t *testing.T) {
	store := testStore(t)
	created, err := store.Create(testCreateInput())
	if err != nil {
		t.Fatal(err)
	}
	if created.Contract.ItemID != "item-a" || created.State.State != StateReady || created.State.StateRevision != 1 {
		t.Fatalf("created item = %#v", created)
	}

	started, err := store.Start(testWorkspaceID, "item-a", 1)
	if err != nil {
		t.Fatal(err)
	}
	if started.State.State != StateRunning || started.State.StateRevision != 2 || started.State.ActiveAttemptID != "attempt-a" {
		t.Fatalf("started item = %#v", started)
	}
	if started.Attempt == nil || started.Attempt.State != AttemptActive {
		t.Fatalf("attempt = %#v", started.Attempt)
	}

	restartedStore := Store{Root: store.Root}
	inspected, err := restartedStore.Inspect(testWorkspaceID, "item-a")
	if err != nil {
		t.Fatal(err)
	}
	if inspected.State != started.State || inspected.Contract.Objective != created.Contract.Objective {
		t.Fatalf("restart inspection = %#v", inspected)
	}
}

func TestStartRejectsStaleRevisionAndContractMutation(t *testing.T) {
	store := testStore(t)
	created, err := store.Create(testCreateInput())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Start(testWorkspaceID, created.Contract.ItemID, 99); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale revision error = %v", err)
	}

	contractPath := filepath.Join(store.Root, "workspaces", testWorkspaceID, "execution", "items", created.Contract.ItemID, "contract.v1.json")
	var contract Contract
	body, err := os.ReadFile(contractPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(body, &contract); err != nil {
		t.Fatal(err)
	}
	contract.Objective = "Tampered objective."
	body, err = json.Marshal(contract)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(contractPath, body, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Start(testWorkspaceID, created.Contract.ItemID, 1); !errors.Is(err, ErrContractChanged) {
		t.Fatalf("contract mutation error = %v", err)
	}
}

func TestStoreRejectsCrossWorkspaceAndUnsafeIdentifiers(t *testing.T) {
	store := testStore(t)
	created, err := store.Create(testCreateInput())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Inspect(anotherWorkspaceID, created.Contract.ItemID); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cross-workspace inspection error = %v", err)
	}
	input := testCreateInput()
	input.WorkspaceID = "../escape"
	if _, err := store.Create(input); err == nil {
		t.Fatal("unsafe workspace ID was accepted")
	}
	input.WorkspaceID = "cliente-hdi"
	if _, err := store.Create(input); err == nil {
		t.Fatal("semantic workspace ID was accepted")
	}
}

func TestTransitionHistoryIsAllowlistedAndPrivate(t *testing.T) {
	store := testStore(t)
	created, err := store.Create(testCreateInput())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Start(testWorkspaceID, created.Contract.ItemID, 1); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(store.Root, "workspaces", testWorkspaceID, "execution", "items", created.Contract.ItemID, "revisions")
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("transition count = %d", len(entries))
	}
	for _, entry := range entries {
		var revision Revision
		if err := readStrictJSON(filepath.Join(root, entry.Name()), &revision); err != nil {
			t.Fatal(err)
		}
		body, err := json.Marshal(revision.Transition)
		if err != nil {
			t.Fatal(err)
		}
		text := string(body)
		for _, prohibited := range []string{"objective", "next_step", "Implement a resumable", "/private/", "prompt", "response"} {
			if strings.Contains(text, prohibited) {
				t.Fatalf("transition leaked %q: %s", prohibited, text)
			}
		}
	}
}

func TestCheckpointPauseNextAndResumeAcrossAttempts(t *testing.T) {
	store := testStore(t)
	created, err := store.Create(testCreateInput())
	if err != nil {
		t.Fatal(err)
	}
	started, err := store.Start(testWorkspaceID, created.Contract.ItemID, 1)
	if err != nil {
		t.Fatal(err)
	}

	checkpointed, err := store.Checkpoint(testWorkspaceID, created.Contract.ItemID, CheckpointInput{
		ExpectedRevision: 2,
		AttemptID:        started.State.ActiveAttemptID,
		Summary:          "The contract test now captures the observable handoff.",
		NextStep:         "Implement pause and resume against immutable revisions.",
		Blocker:          "None.",
		ArtifactRefs:     []string{"bcgos://workspace/specs/018"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if checkpointed.State.State != StateRunning || checkpointed.State.StateRevision != 3 {
		t.Fatalf("checkpointed state = %#v", checkpointed.State)
	}
	if checkpointed.Checkpoint == nil || checkpointed.Checkpoint.AttemptID != started.State.ActiveAttemptID {
		t.Fatalf("checkpoint = %#v", checkpointed.Checkpoint)
	}

	paused, err := store.Pause(testWorkspaceID, created.Contract.ItemID, 3, started.State.ActiveAttemptID)
	if err != nil {
		t.Fatal(err)
	}
	if paused.State.State != StatePaused || paused.State.StateRevision != 4 || paused.State.ActiveAttemptID != "" {
		t.Fatalf("paused state = %#v", paused.State)
	}
	if paused.Attempt == nil || paused.Attempt.State != AttemptInterrupted {
		t.Fatalf("paused attempt = %#v", paused.Attempt)
	}

	next, err := store.Next(testWorkspaceID, created.Contract.ItemID)
	if err != nil {
		t.Fatal(err)
	}
	if next.State != StatePaused || next.StateRevision != 4 || next.NextStep != checkpointed.Checkpoint.NextStep {
		t.Fatalf("next projection = %#v", next)
	}
	body, err := json.Marshal(next)
	if err != nil {
		t.Fatal(err)
	}
	if len(body) > MaximumNextProjectionBytes {
		t.Fatalf("next projection contains %d bytes", len(body))
	}
	if strings.Contains(string(body), created.Contract.Objective) {
		t.Fatalf("next projection leaked immutable contract: %s", body)
	}

	resumed, err := store.Resume(testWorkspaceID, created.Contract.ItemID, 4)
	if err != nil {
		t.Fatal(err)
	}
	if resumed.State.State != StateRunning || resumed.State.StateRevision != 5 || resumed.State.ActiveAttemptID != "attempt-b" {
		t.Fatalf("resumed state = %#v", resumed.State)
	}
	if resumed.Attempt == nil || resumed.Attempt.State != AttemptActive {
		t.Fatalf("resumed attempt = %#v", resumed.Attempt)
	}

	if _, err := store.Checkpoint(testWorkspaceID, created.Contract.ItemID, CheckpointInput{
		ExpectedRevision: 5,
		AttemptID:        started.State.ActiveAttemptID,
		Summary:          "A stale writer must not commit.",
		NextStep:         "This should fail.",
	}); !errors.Is(err, ErrAttemptConflict) {
		t.Fatalf("stale attempt error = %v", err)
	}
}

func TestActivePointerExposesOnlyTheLogicalResolverAndFailsClosed(t *testing.T) {
	store := testStore(t)

	pointer, err := store.ActivePointer(testWorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if pointer.Available || pointer.State != ActivePointerUnavailable || pointer.Path != "" {
		t.Fatalf("empty active pointer = %#v", pointer)
	}

	created, err := store.Create(testCreateInput())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Start(testWorkspaceID, created.Contract.ItemID, created.State.StateRevision); err != nil {
		t.Fatal(err)
	}
	pointer, err = Store{Root: store.Root}.ActivePointer(testWorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if !pointer.Available || pointer.State != ActivePointerAvailable || pointer.Path != ActivePointerPath {
		t.Fatalf("single active pointer = %#v", pointer)
	}
	body, err := json.Marshal(pointer)
	if err != nil {
		t.Fatal(err)
	}
	for _, prohibited := range []string{created.Contract.ItemID, created.Contract.Objective, "attempt-a"} {
		if strings.Contains(string(body), prohibited) {
			t.Fatalf("active pointer leaked %q: %s", prohibited, body)
		}
	}

	secondStore := Store{
		Root: store.Root,
		Now:  store.Now,
		NewID: func(kind string) (string, error) {
			switch kind {
			case "item":
				return "item-b", nil
			case "attempt":
				return "attempt-c", nil
			default:
				return "", errors.New("unexpected ID request")
			}
		},
	}
	second, err := secondStore.Create(testCreateInput())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := secondStore.Start(testWorkspaceID, second.Contract.ItemID, second.State.StateRevision); err != nil {
		t.Fatal(err)
	}
	pointer, err = Store{Root: store.Root}.ActivePointer(testWorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if pointer.Available || pointer.State != ActivePointerAmbiguous || pointer.Path != "" {
		t.Fatalf("ambiguous active pointer = %#v", pointer)
	}
}

func TestCheckpointRequiresCurrentAttemptAndBoundedAllowedProjection(t *testing.T) {
	store := testStore(t)
	created, err := store.Create(testCreateInput())
	if err != nil {
		t.Fatal(err)
	}
	started, err := store.Start(testWorkspaceID, created.Contract.ItemID, 1)
	if err != nil {
		t.Fatal(err)
	}
	base := CheckpointInput{
		ExpectedRevision: 2,
		AttemptID:        started.State.ActiveAttemptID,
		Summary:          "Bounded summary.",
		NextStep:         "Bounded next step.",
	}

	disallowed := base
	disallowed.ArtifactRefs = []string{"bcgos://workspace/private/unapproved"}
	if _, err := store.Checkpoint(testWorkspaceID, created.Contract.ItemID, disallowed); err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("disallowed reference error = %v", err)
	}

	oversized := base
	oversized.Summary = strings.Repeat("x", MaximumCheckpointSummaryBytes+1)
	if _, err := store.Checkpoint(testWorkspaceID, created.Contract.ItemID, oversized); err == nil || !strings.Contains(err.Error(), "summary") {
		t.Fatalf("oversized checkpoint error = %v", err)
	}

	if _, err := store.Pause(testWorkspaceID, created.Contract.ItemID, 2, started.State.ActiveAttemptID); err == nil || !strings.Contains(err.Error(), "checkpoint") {
		t.Fatalf("pause without checkpoint error = %v", err)
	}
}

func TestCreateRejectsInitialNextActionThatCannotFitProjection(t *testing.T) {
	store := testStore(t)
	input := testCreateInput()
	input.InitialNextStep = strings.Repeat("x", 2048)
	if _, err := store.Create(input); err == nil || !strings.Contains(err.Error(), "projection") {
		t.Fatalf("oversized initial projection error = %v", err)
	}
}

func TestNextProjectionEnforcesExactSerializedByteLimit(t *testing.T) {
	projection := NextProjection{
		SchemaVersion: 1,
		ItemID:        "item-a",
		State:         StateReady,
		StateRevision: 1,
		NextStep:      "",
	}
	empty, err := json.Marshal(projection)
	if err != nil {
		t.Fatal(err)
	}
	projection.NextStep = strings.Repeat("x", MaximumNextProjectionBytes-len(empty)-1)
	body, err := json.Marshal(projection)
	if err != nil {
		t.Fatal(err)
	}
	if len(body)+1 != MaximumNextProjectionBytes {
		t.Fatalf("boundary projection contains %d bytes", len(body)+1)
	}
	if err := validateNextProjection(projection); err != nil {
		t.Fatalf("boundary projection error = %v", err)
	}
	projection.NextStep += "x"
	if err := validateNextProjection(projection); err == nil {
		t.Fatal("projection above the serialized byte limit was accepted")
	}
}

func TestCheckpointRevisionSurvivesProjectionCrash(t *testing.T) {
	store := testStore(t)
	created, err := store.Create(testCreateInput())
	if err != nil {
		t.Fatal(err)
	}
	started, err := store.Start(testWorkspaceID, created.Contract.ItemID, 1)
	if err != nil {
		t.Fatal(err)
	}
	store.FaultPoint = func(point string) error {
		if point == "after_revision_commit" {
			return errors.New("simulated projection crash")
		}
		return nil
	}
	_, err = store.Checkpoint(testWorkspaceID, created.Contract.ItemID, CheckpointInput{
		ExpectedRevision: 2,
		AttemptID:        started.State.ActiveAttemptID,
		Summary:          "Durable checkpoint.",
		NextStep:         "Recover from the immutable revision.",
	})
	if err == nil || !strings.Contains(err.Error(), "simulated") {
		t.Fatalf("checkpoint crash error = %v", err)
	}

	recovered, err := (Store{Root: store.Root}).Inspect(testWorkspaceID, created.Contract.ItemID)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.State.StateRevision != 3 || recovered.Checkpoint == nil || recovered.Checkpoint.Summary != "Durable checkpoint." {
		t.Fatalf("recovered checkpoint = %#v", recovered)
	}
}

func TestNextActiveFailsClosedWhenSelectionIsAmbiguous(t *testing.T) {
	store := testStore(t)
	created, err := store.Create(testCreateInput())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Start(testWorkspaceID, created.Contract.ItemID, 1); err != nil {
		t.Fatal(err)
	}
	active, err := store.NextActive(testWorkspaceID)
	if err != nil || active.ItemID != created.Contract.ItemID {
		t.Fatalf("active projection = %#v, err = %v", active, err)
	}

	ids := []string{"item-b", "attempt-c"}
	store.NewID = func(kind string) (string, error) {
		id := ids[0]
		ids = ids[1:]
		return id, nil
	}
	second, err := store.Create(testCreateInput())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Start(testWorkspaceID, second.Contract.ItemID, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := store.NextActive(testWorkspaceID); !errors.Is(err, ErrActiveItemAmbiguous) {
		t.Fatalf("ambiguous active item error = %v", err)
	}
}

func TestExportAndConfirmedDelete(t *testing.T) {
	store := testStore(t)
	created, err := store.Create(testCreateInput())
	if err != nil {
		t.Fatal(err)
	}
	exported, err := store.Export(testWorkspaceID, created.Contract.ItemID)
	if err != nil || exported.Contract.ItemID != created.Contract.ItemID || len(exported.Transitions) != 1 {
		t.Fatalf("export = %#v, err = %v", exported, err)
	}
	if err := store.Delete(testWorkspaceID, created.Contract.ItemID, 1, false); !errors.Is(err, ErrConfirmationRequired) {
		t.Fatalf("unconfirmed delete error = %v", err)
	}
	if err := store.Delete(testWorkspaceID, created.Contract.ItemID, 1, true); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Inspect(testWorkspaceID, created.Contract.ItemID); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("deleted item inspection error = %v", err)
	}
}

func TestRevisionCommitSurvivesProjectionCrash(t *testing.T) {
	store := testStore(t)
	created, err := store.Create(testCreateInput())
	if err != nil {
		t.Fatal(err)
	}
	store.FaultPoint = func(point string) error {
		if point == "after_revision_commit" {
			return errors.New("simulated crash")
		}
		return nil
	}
	if _, err := store.Start(testWorkspaceID, created.Contract.ItemID, 1); err == nil {
		t.Fatal("fault injection did not interrupt start")
	}
	if err := os.Remove(filepath.Join(store.Root, "workspaces", testWorkspaceID, "execution", "items", created.Contract.ItemID, "state.json")); err != nil {
		t.Fatal(err)
	}
	inspected, err := Store{Root: store.Root}.Inspect(testWorkspaceID, created.Contract.ItemID)
	if err != nil {
		t.Fatal(err)
	}
	if inspected.State.State != StateRunning || inspected.State.StateRevision != 2 || inspected.Attempt == nil {
		t.Fatalf("recovered item = %#v", inspected)
	}
}

func TestStaleMutationLockIsRecoverable(t *testing.T) {
	store := testStore(t)
	created, err := store.Create(testCreateInput())
	if err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(store.Root, "workspaces", testWorkspaceID, "execution", "items", created.Contract.ItemID, ".mutation.lock")
	if err := os.Mkdir(lockPath, 0o700); err != nil {
		t.Fatal(err)
	}
	stale := store.now().Add(-3 * time.Minute)
	if err := os.Chtimes(lockPath, stale, stale); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Start(testWorkspaceID, created.Contract.ItemID, 1); err != nil {
		t.Fatalf("recover stale lock: %v", err)
	}
}

func TestFreshIncompleteLockRemainsBusy(t *testing.T) {
	store := testStore(t)
	created, err := store.Create(testCreateInput())
	if err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(store.Root, "workspaces", testWorkspaceID, "execution", "items", created.Contract.ItemID, ".mutation.lock")
	if err := os.Mkdir(lockPath, 0o700); err != nil {
		t.Fatal(err)
	}
	fresh := store.now()
	if err := os.Chtimes(lockPath, fresh, fresh); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Start(testWorkspaceID, created.Contract.ItemID, 1); !errors.Is(err, ErrItemBusy) {
		t.Fatalf("fresh incomplete lock error = %v", err)
	}
}

func TestStaleTakeoverOldUnlockCannotRemoveNewOwner(t *testing.T) {
	store := testStore(t)
	created, err := store.Create(testCreateInput())
	if err != nil {
		t.Fatal(err)
	}
	oldUnlock, err := store.lock(testWorkspaceID, created.Contract.ItemID)
	if err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(store.Root, "workspaces", testWorkspaceID, "execution", "items", created.Contract.ItemID, ".mutation.lock")
	stale := store.now().Add(-3 * time.Minute)
	if err := os.Chtimes(lockPath, stale, stale); err != nil {
		t.Fatal(err)
	}
	newStore := store
	newStore.Now = func() time.Time { return store.now().Add(3 * time.Minute) }
	newUnlock, err := newStore.lock(testWorkspaceID, created.Contract.ItemID)
	if err != nil {
		t.Fatal(err)
	}
	oldUnlock()
	if _, err := os.Stat(lockPath); err != nil {
		t.Fatalf("old unlock removed new owner: %v", err)
	}
	newUnlock()
	if _, err := os.Stat(lockPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("new owner lock remains: %v", err)
	}
}

func TestImmutableRevisionPublishNeverClobbers(t *testing.T) {
	store := testStore(t)
	created, err := store.Create(testCreateInput())
	if err != nil {
		t.Fatal(err)
	}
	itemRoot := filepath.Join(store.Root, "workspaces", testWorkspaceID, "execution", "items", created.Contract.ItemID)
	revision, err := latestValidRevision(itemRoot)
	if err != nil {
		t.Fatal(err)
	}
	revision.State.StateRevision = 2
	revision.Transition.StateRevision = 2
	path := filepath.Join(itemRoot, "revisions", revisionName(2))
	if err := writeImmutableRevision(path, revision); err != nil {
		t.Fatal(err)
	}
	if err := writeImmutableRevision(path, revision); err == nil {
		t.Fatal("immutable revision was overwritten")
	}
}

func TestStartAndDeleteCannotBothCommit(t *testing.T) {
	store := testStore(t)
	created, err := store.Create(testCreateInput())
	if err != nil {
		t.Fatal(err)
	}
	var wait sync.WaitGroup
	wait.Add(2)
	results := make(chan error, 2)
	go func() {
		defer wait.Done()
		_, err := store.Start(testWorkspaceID, created.Contract.ItemID, 1)
		results <- err
	}()
	go func() {
		defer wait.Done()
		results <- store.Delete(testWorkspaceID, created.Contract.ItemID, 1, true)
	}()
	wait.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("start/delete successful mutations = %d, want exactly one", successes)
	}
}

func TestStaleStartCannotCommitAfterDeleteTakeover(t *testing.T) {
	store := testStore(t)
	base := time.Now().UTC()
	store.Now = func() time.Time { return base }
	created, err := store.Create(testCreateInput())
	if err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	release := make(chan struct{})
	store.FaultPoint = func(point string) error {
		if point == "before_revision_commit" {
			close(entered)
			<-release
		}
		return nil
	}
	startResult := make(chan error, 1)
	go func() {
		_, err := store.Start(testWorkspaceID, created.Contract.ItemID, 1)
		startResult <- err
	}()
	<-entered

	deleteStore := Store{Root: store.Root, Now: func() time.Time { return base.Add(3 * time.Minute) }}
	deleteErr := deleteStore.Delete(testWorkspaceID, created.Contract.ItemID, 1, true)
	close(release)
	startErr := <-startResult
	if deleteErr != nil {
		t.Fatalf("delete takeover failed: %v", deleteErr)
	}
	if startErr == nil {
		t.Fatal("stale start committed after delete takeover")
	}
	if _, err := deleteStore.Inspect(testWorkspaceID, created.Contract.ItemID); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("deleted item was recreated: %v", err)
	}
}

func TestValidateSchemaFile(t *testing.T) {
	if err := ValidateSchemaFile(filepath.Join("..", "..", "schemas", "execution-state.schema.json")); err != nil {
		t.Fatal(err)
	}
}
