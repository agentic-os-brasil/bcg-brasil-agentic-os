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
	ids := []string{"item-a", "attempt-a"}
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
			{ID: "unit-tests", Type: CriterionCommandCheck},
			{ID: "artifact", Type: CriterionArtifactSnapshot},
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
		body, err := os.ReadFile(filepath.Join(root, entry.Name()))
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
