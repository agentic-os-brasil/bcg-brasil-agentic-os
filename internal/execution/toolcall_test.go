package execution

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func toolCallTestStore(t *testing.T) Store {
	t.Helper()
	now := time.Date(2026, 7, 25, 16, 0, 0, 0, time.UTC)
	ids := []string{"item-tool", "attempt-tool", "call-tool"}
	return Store{
		Root: t.TempDir(),
		Now: func() time.Time {
			now = now.Add(time.Second)
			return now
		},
		NewID: func(kind string) (string, error) {
			if len(ids) == 0 {
				t.Fatalf("unexpected %s ID request", kind)
			}
			id := ids[0]
			ids = ids[1:]
			return id, nil
		},
	}
}

func TestToolCallLifecycleIsMetadataOnlyAndExported(t *testing.T) {
	store := toolCallTestStore(t)
	input := testCreateInput()
	created, err := store.Create(input)
	if err != nil {
		t.Fatal(err)
	}
	started, err := store.Start(testWorkspaceID, created.Contract.ItemID, 1)
	if err != nil {
		t.Fatal(err)
	}
	call, err := store.StartToolCall(testWorkspaceID, created.Contract.ItemID, ToolCallStartInput{
		ExpectedRevision: 2,
		AttemptID:        started.Attempt.AttemptID,
		AgentID:          "codex",
		ToolID:           "github",
	})
	if err != nil {
		t.Fatal(err)
	}
	if call.Item.State.StateRevision != 3 || call.Receipt.State != ToolCallStarted ||
		call.Receipt.FinishedAt != nil {
		t.Fatalf("started call = %#v", call)
	}
	finished, err := store.FinishToolCall(testWorkspaceID, created.Contract.ItemID, ToolCallFinishInput{
		ExpectedRevision: 3,
		AttemptID:        started.Attempt.AttemptID,
		ToolCallID:       call.Receipt.ToolCallID,
		Outcome:          ToolCallSucceeded,
	})
	if err != nil {
		t.Fatal(err)
	}
	if finished.Item.State.StateRevision != 4 || finished.Receipt.State != ToolCallSucceeded ||
		finished.Receipt.FinishedAt == nil {
		t.Fatalf("finished call = %#v", finished)
	}
	exported, err := store.Export(testWorkspaceID, created.Contract.ItemID)
	if err != nil {
		t.Fatal(err)
	}
	if len(exported.ToolCalls) != 2 || exported.ToolCalls[0].State != ToolCallStarted ||
		exported.ToolCalls[1].State != ToolCallSucceeded {
		t.Fatalf("tool call history = %#v", exported.ToolCalls)
	}
	body, err := json.Marshal(exported.ToolCalls)
	if err != nil {
		t.Fatal(err)
	}
	for _, prohibited := range []string{"prompt", "arguments", "stdout", "stderr", "payload", "error"} {
		if strings.Contains(strings.ToLower(string(body)), prohibited) {
			t.Fatalf("tool call history leaked %q: %s", prohibited, body)
		}
	}
}

func TestToolCallLifecycleRejectsStaleAttemptsAndDuplicateFinish(t *testing.T) {
	store := toolCallTestStore(t)
	created, err := store.Create(testCreateInput())
	if err != nil {
		t.Fatal(err)
	}
	started, err := store.Start(testWorkspaceID, created.Contract.ItemID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.StartToolCall(testWorkspaceID, created.Contract.ItemID, ToolCallStartInput{
		ExpectedRevision: 2, AttemptID: "stale-attempt", AgentID: "codex", ToolID: "shell",
	}); !errors.Is(err, ErrAttemptConflict) {
		t.Fatalf("stale attempt error = %v", err)
	}
	if _, err := store.StartToolCall(testWorkspaceID, created.Contract.ItemID, ToolCallStartInput{
		ExpectedRevision: 2, AttemptID: started.Attempt.AttemptID,
		AgentID: "client-query-slug", ToolID: "shell",
	}); err == nil || !strings.Contains(err.Error(), "canonical agent") {
		t.Fatalf("free-form agent error = %v", err)
	}
	if _, err := store.StartToolCall(testWorkspaceID, created.Contract.ItemID, ToolCallStartInput{
		ExpectedRevision: 2, AttemptID: started.Attempt.AttemptID,
		AgentID: "codex", ToolID: "encoded-payload",
	}); err == nil || !strings.Contains(err.Error(), "canonical tool") {
		t.Fatalf("free-form tool error = %v", err)
	}
	call, err := store.StartToolCall(testWorkspaceID, created.Contract.ItemID, ToolCallStartInput{
		ExpectedRevision: 2, AttemptID: started.Attempt.AttemptID,
		AgentID: "codex", ToolID: "shell",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.FinishToolCall(testWorkspaceID, created.Contract.ItemID, ToolCallFinishInput{
		ExpectedRevision: 3, AttemptID: started.Attempt.AttemptID,
		ToolCallID: call.Receipt.ToolCallID, Outcome: ToolCallStarted,
	}); err == nil || !strings.Contains(err.Error(), "terminal") {
		t.Fatalf("invalid outcome error = %v", err)
	}
	if _, err := store.FinishToolCall(testWorkspaceID, created.Contract.ItemID, ToolCallFinishInput{
		ExpectedRevision: 3, AttemptID: started.Attempt.AttemptID,
		ToolCallID: call.Receipt.ToolCallID, Outcome: ToolCallFailed,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.FinishToolCall(testWorkspaceID, created.Contract.ItemID, ToolCallFinishInput{
		ExpectedRevision: 4, AttemptID: started.Attempt.AttemptID,
		ToolCallID: call.Receipt.ToolCallID, Outcome: ToolCallSucceeded,
	}); err == nil || !strings.Contains(err.Error(), "already terminal") {
		t.Fatalf("duplicate finish error = %v", err)
	}
}

func TestRevisionRejectsHybridEvidenceAndToolCall(t *testing.T) {
	now := time.Date(2026, 7, 25, 16, 0, 0, 0, time.UTC)
	exitCode := 0
	revision := Revision{
		SchemaVersion: 1,
		State: State{
			SchemaVersion: 1, ItemID: "item-a", WorkspaceID: testWorkspaceID,
			State: StateRunning, StateRevision: 3, ContractVersion: 1,
			ContractSHA256: strings.Repeat("a", 64), ActiveAttemptID: "attempt-a",
			CreatedAt: now, UpdatedAt: now,
		},
		Attempt: &Attempt{
			SchemaVersion: 1, AttemptID: "attempt-a", ItemID: "item-a",
			WorkspaceID: testWorkspaceID, State: AttemptActive, StartedAt: now,
		},
		Evidence: &EvidenceReceipt{
			SchemaVersion: 1, EvidenceID: "evidence-a", ItemID: "item-a",
			WorkspaceID: testWorkspaceID, AttemptID: "attempt-a",
			CriterionID: "tests", Type: CriterionCommandCheck, Outcome: EvidencePassed,
			CommandSHA256: strings.Repeat("b", 64), ToolSHA256: strings.Repeat("c", 64),
			ExitCode: &exitCode, ObservedAt: now,
		},
		ToolCall: &ToolCallReceipt{
			SchemaVersion: 1, ToolCallID: "call-a", ItemID: "item-a",
			WorkspaceID: testWorkspaceID, AttemptID: "attempt-a",
			AgentID: "codex", ToolID: "shell", State: ToolCallStarted, StartedAt: now,
		},
		Transition: Transition{
			SchemaVersion: 1, ItemID: "item-a", WorkspaceID: testWorkspaceID,
			AttemptID: "attempt-a", State: StateRunning, StateRevision: 3, OccurredAt: now,
		},
	}
	if err := validateRevision(revision); err == nil || !strings.Contains(err.Error(), "both evidence") {
		t.Fatalf("hybrid revision error = %v", err)
	}
}
