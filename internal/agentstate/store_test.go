package agentstate

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func newTestStore(t *testing.T, budget int) *Store {
	t.Helper()
	store := NewStore(t.TempDir())
	if budget > 0 {
		store.RuneBudget = budget
	}
	store.Now = func() time.Time { return time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC) }
	return store
}

func baseUpdate() SnapshotUpdate {
	return SnapshotUpdate{
		AgentID:      "maestro",
		WorkspaceID:  "workspace-a",
		Timestamp:    time.Date(2026, 8, 12, 11, 0, 0, 0, time.UTC),
		SectionLabel: "last_action",
		Body:         "Confirmed onboarding checkpoint and queued setup checkup.",
		SourceDigest: "sha256:aaa",
	}
}

func TestApplyRoundtrip(t *testing.T) {
	store := newTestStore(t, 0)
	update := baseUpdate()
	result, err := store.Apply(update)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if result.Idempotent {
		t.Fatalf("first apply must not be idempotent")
	}
	loaded, err := store.Load(update.WorkspaceID, update.AgentID)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(loaded.Sections) != 1 {
		t.Fatalf("expected 1 section, got %d", len(loaded.Sections))
	}
	if loaded.Sections[0].Body != update.Body {
		t.Fatalf("body mismatch: %q vs %q", loaded.Sections[0].Body, update.Body)
	}
}

func TestApplyIdempotent(t *testing.T) {
	store := newTestStore(t, 0)
	update := baseUpdate()
	if _, err := store.Apply(update); err != nil {
		t.Fatalf("first apply: %v", err)
	}
	// Same section, same body, later timestamp - must be idempotent.
	update.Timestamp = update.Timestamp.Add(time.Minute)
	result, err := store.Apply(update)
	if err != nil {
		t.Fatalf("second apply: %v", err)
	}
	if !result.Idempotent {
		t.Fatalf("repeated identical update must be reported as idempotent")
	}
}

func TestApplyCompactionDropsOldestSection(t *testing.T) {
	// Budget tight enough that three medium-length sections cannot fit.
	store := newTestStore(t, 200)
	base := baseUpdate()

	first := base
	first.SectionLabel = "handoff_note"
	first.Body = strings.Repeat("a", 80)
	first.Timestamp = base.Timestamp

	second := base
	second.SectionLabel = "open_question"
	second.Body = strings.Repeat("b", 80)
	second.Timestamp = base.Timestamp.Add(time.Minute)

	third := base
	third.SectionLabel = "last_action"
	third.Body = strings.Repeat("c", 80)
	third.Timestamp = base.Timestamp.Add(2 * time.Minute)

	if _, err := store.Apply(first); err != nil {
		t.Fatalf("first: %v", err)
	}
	if _, err := store.Apply(second); err != nil {
		t.Fatalf("second: %v", err)
	}
	result, err := store.Apply(third)
	if err != nil {
		t.Fatalf("third: %v", err)
	}
	// Expect at least the oldest section (handoff_note) to have been dropped.
	if len(result.DroppedSections) == 0 || result.DroppedSections[0] != "handoff_note" {
		t.Fatalf("expected handoff_note dropped, got %v", result.DroppedSections)
	}
	for _, section := range result.Snapshot.Sections {
		if section.Label == "handoff_note" {
			t.Fatalf("handoff_note should have been compacted out")
		}
	}
	if composedRuneLen(result.Snapshot.Sections) > store.RuneBudget {
		t.Fatalf("compaction did not respect budget")
	}
}

func TestApplyRejectsMissingIdentities(t *testing.T) {
	store := newTestStore(t, 0)

	noAgent := baseUpdate()
	noAgent.AgentID = ""
	if _, err := store.Apply(noAgent); err == nil {
		t.Fatalf("expected error for missing agent_id")
	}

	noWorkspace := baseUpdate()
	noWorkspace.WorkspaceID = ""
	if _, err := store.Apply(noWorkspace); err == nil {
		t.Fatalf("expected error for missing workspace_id")
	}

	badLabel := baseUpdate()
	badLabel.SectionLabel = "Not Valid Label"
	if _, err := store.Apply(badLabel); err == nil {
		t.Fatalf("expected error for invalid section_label")
	}

	emptyBody := baseUpdate()
	emptyBody.Body = "   "
	if _, err := store.Apply(emptyBody); err == nil {
		t.Fatalf("expected error for empty body")
	}
}

func TestAgentIsolationInSameWorkspace(t *testing.T) {
	store := newTestStore(t, 0)
	updateA := baseUpdate()
	updateA.AgentID = "maestro"
	updateB := baseUpdate()
	updateB.AgentID = "walter"
	updateB.Body = "Independent walter note."
	if _, err := store.Apply(updateA); err != nil {
		t.Fatalf("apply A: %v", err)
	}
	if _, err := store.Apply(updateB); err != nil {
		t.Fatalf("apply B: %v", err)
	}
	loadedA, err := store.Load(updateA.WorkspaceID, updateA.AgentID)
	if err != nil {
		t.Fatalf("load A: %v", err)
	}
	loadedB, err := store.Load(updateB.WorkspaceID, updateB.AgentID)
	if err != nil {
		t.Fatalf("load B: %v", err)
	}
	if loadedA.Sections[0].Body == loadedB.Sections[0].Body {
		t.Fatalf("agent isolation violated: bodies should differ")
	}
	if loadedA.AgentID == loadedB.AgentID {
		t.Fatalf("agent identities must not cross")
	}
}

func TestWorkspaceIsolationForSameAgent(t *testing.T) {
	store := newTestStore(t, 0)
	updateA := baseUpdate()
	updateA.WorkspaceID = "workspace-a"
	updateB := baseUpdate()
	updateB.WorkspaceID = "workspace-b"
	updateB.Body = "Body only visible inside workspace-b."
	if _, err := store.Apply(updateA); err != nil {
		t.Fatalf("apply A: %v", err)
	}
	if _, err := store.Apply(updateB); err != nil {
		t.Fatalf("apply B: %v", err)
	}
	loadedA, err := store.Load("workspace-a", updateA.AgentID)
	if err != nil {
		t.Fatalf("load A: %v", err)
	}
	loadedB, err := store.Load("workspace-b", updateB.AgentID)
	if err != nil {
		t.Fatalf("load B: %v", err)
	}
	if loadedA.Sections[0].Body == loadedB.Sections[0].Body {
		t.Fatalf("workspace isolation violated: bodies should differ")
	}
	if loadedA.WorkspaceID == loadedB.WorkspaceID {
		t.Fatalf("workspace identities must not cross")
	}
}

func TestLoadReportsMissingSnapshotExplicitly(t *testing.T) {
	store := newTestStore(t, 0)
	_, err := store.Load("workspace-a", "maestro")
	if !errors.Is(err, ErrNoSnapshot) {
		t.Fatalf("expected ErrNoSnapshot, got %v", err)
	}
}

func TestApplyRejectsNonMonotonicTimestamp(t *testing.T) {
	store := newTestStore(t, 0)
	update := baseUpdate()
	if _, err := store.Apply(update); err != nil {
		t.Fatalf("first apply: %v", err)
	}
	older := baseUpdate()
	older.SectionLabel = "handoff_note"
	older.Body = "older body"
	older.Timestamp = update.Timestamp.Add(-time.Hour)
	if _, err := store.Apply(older); !errors.Is(err, errNonMonotonicTimestamp) {
		t.Fatalf("expected non-monotonic timestamp rejection, got %v", err)
	}
}
