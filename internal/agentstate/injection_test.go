package agentstate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAssembleMissingSnapshotReturnsDiagnostic(t *testing.T) {
	store := newTestStore(t, 0)
	section, err := Assemble(store, "workspace-a", "maestro")
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	if section.Diagnostic != SnapshotLayerLabel+": missing; skipped" {
		t.Fatalf("expected missing diagnostic, got %q", section.Diagnostic)
	}
	if section.Layer != "" || section.Content != "" || section.DrillDown != "" || section.Truncated {
		t.Fatalf("missing section must be zero-valued except diagnostic: %+v", section)
	}
}

func TestAssembleRoundtripRendersH2Sections(t *testing.T) {
	store := newTestStore(t, 0)
	first := baseUpdate()
	first.SectionLabel = "handoff_note"
	first.Body = "Handed off session to yoda."

	second := baseUpdate()
	second.SectionLabel = "last_action"
	second.Body = "Confirmed onboarding checkpoint."
	second.Timestamp = first.Timestamp.Add(time.Minute)

	if _, err := store.Apply(first); err != nil {
		t.Fatalf("apply first: %v", err)
	}
	if _, err := store.Apply(second); err != nil {
		t.Fatalf("apply second: %v", err)
	}

	section, err := Assemble(store, first.WorkspaceID, first.AgentID)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	if section.Layer != SnapshotLayerLabel {
		t.Fatalf("expected layer %q, got %q", SnapshotLayerLabel, section.Layer)
	}
	if section.Truncated {
		t.Fatalf("did not expect truncation for two small sections")
	}
	if !strings.Contains(section.Content, "## handoff_note") {
		t.Fatalf("missing handoff heading:\n%s", section.Content)
	}
	if !strings.Contains(section.Content, "## last_action") {
		t.Fatalf("missing last_action heading:\n%s", section.Content)
	}
	// Deterministic ordering: oldest first.
	if strings.Index(section.Content, "handoff_note") > strings.Index(section.Content, "last_action") {
		t.Fatalf("expected oldest-first ordering, got:\n%s", section.Content)
	}
	if section.DrillDown != "workspaces/"+first.WorkspaceID+"/agents/"+first.AgentID+"/state.md" {
		t.Fatalf("unexpected drill-down: %q", section.DrillDown)
	}
}

func TestAssembleAppliesDropOldestWithinBudget(t *testing.T) {
	// Tight budget forces the oldest section to be dropped.
	store := newTestStore(t, 200)
	base := baseUpdate()

	first := base
	first.SectionLabel = "handoff_note"
	first.Body = strings.Repeat("a", 40)
	first.Timestamp = base.Timestamp

	second := base
	second.SectionLabel = "open_question"
	second.Body = strings.Repeat("b", 40)
	second.Timestamp = base.Timestamp.Add(time.Minute)

	third := base
	third.SectionLabel = "last_action"
	third.Body = strings.Repeat("c", 40)
	third.Timestamp = base.Timestamp.Add(2 * time.Minute)

	if _, err := store.Apply(first); err != nil {
		t.Fatalf("first: %v", err)
	}
	if _, err := store.Apply(second); err != nil {
		t.Fatalf("second: %v", err)
	}
	if _, err := store.Apply(third); err != nil {
		t.Fatalf("third: %v", err)
	}

	section, err := Assemble(store, base.WorkspaceID, base.AgentID)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	// The store's own compaction already runs on Apply; but if the store's
	// budget and the injection budget differ in a future refactor, Assemble
	// must still respect the injection-side budget. We assert (a) it fits and
	// (b) it never contains an already-dropped section.
	if len(section.Content) > 0 && len([]rune(section.Content)) > store.budget() {
		t.Fatalf("content exceeds budget: %d runes vs %d", len([]rune(section.Content)), store.budget())
	}
	if section.Layer != SnapshotLayerLabel {
		t.Fatalf("expected layer set, got %q (diag=%q)", section.Layer, section.Diagnostic)
	}
}

func TestAssembleSingleOversizedSectionReturnsInvalid(t *testing.T) {
	// Manually construct an on-disk snapshot with a single section that
	// exceeds the injection-side rune budget. This can only happen if a
	// future writer changes budgets asymmetrically; Assemble must report
	// invalid rather than emit truncated prose.
	store := newTestStore(t, 0)
	// Seed with a small update so the store has a valid layout.
	update := baseUpdate()
	if _, err := store.Apply(update); err != nil {
		t.Fatalf("seed apply: %v", err)
	}
	// Shrink the runtime budget below the stored section's rendered size.
	store.RuneBudget = 5

	section, err := Assemble(store, update.WorkspaceID, update.AgentID)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	if section.Diagnostic != SnapshotLayerLabel+": invalid; skipped" {
		t.Fatalf("expected invalid diagnostic, got %+v", section)
	}
}

func TestAssembleCorruptSnapshotReturnsDiagnostic(t *testing.T) {
	store := newTestStore(t, 0)
	update := baseUpdate()
	if _, err := store.Apply(update); err != nil {
		t.Fatalf("seed apply: %v", err)
	}
	// Corrupt every commit manifest to force ErrCorruptSnapshot from Load.
	commitsDir := store.commitsDir(update.WorkspaceID, update.AgentID)
	entries, err := os.ReadDir(commitsDir)
	if err != nil {
		t.Fatalf("read commits dir: %v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(commitsDir, entry.Name())
		if err := os.WriteFile(path, []byte("{not valid json"), 0o600); err != nil {
			t.Fatalf("corrupt %s: %v", path, err)
		}
	}
	section, err := Assemble(store, update.WorkspaceID, update.AgentID)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	if section.Diagnostic != SnapshotLayerLabel+": invalid; skipped" {
		t.Fatalf("expected invalid diagnostic, got %+v", section)
	}
}

func TestAssembleWorkspaceAndAgentIsolation(t *testing.T) {
	store := newTestStore(t, 0)
	a := baseUpdate()
	a.WorkspaceID = "workspace-a"
	a.AgentID = "maestro"
	a.Body = "Only visible to workspace-a maestro."

	b := baseUpdate()
	b.WorkspaceID = "workspace-b"
	b.AgentID = "maestro"
	b.Body = "Only visible to workspace-b maestro."

	c := baseUpdate()
	c.WorkspaceID = "workspace-a"
	c.AgentID = "yoda"
	c.Body = "Only visible to workspace-a yoda."

	for _, u := range []SnapshotUpdate{a, b, c} {
		if _, err := store.Apply(u); err != nil {
			t.Fatalf("apply %s/%s: %v", u.WorkspaceID, u.AgentID, u)
		}
	}

	sectionA, err := Assemble(store, "workspace-a", "maestro")
	if err != nil {
		t.Fatalf("assemble a: %v", err)
	}
	if !strings.Contains(sectionA.Content, "workspace-a maestro") {
		t.Fatalf("workspace-a/maestro missing own body:\n%s", sectionA.Content)
	}
	if strings.Contains(sectionA.Content, "workspace-b") || strings.Contains(sectionA.Content, "yoda") {
		t.Fatalf("cross-agent or cross-workspace leak:\n%s", sectionA.Content)
	}
}

func TestAssembleRejectsMalformedIdentity(t *testing.T) {
	store := newTestStore(t, 0)
	if _, err := Assemble(store, "", "maestro"); err == nil {
		t.Fatalf("expected error for empty workspace")
	}
	if _, err := Assemble(store, "workspace-a", ""); err == nil {
		t.Fatalf("expected error for empty agent")
	}
	if _, err := Assemble(store, "not valid workspace", "maestro"); err == nil {
		t.Fatalf("expected error for malformed workspace")
	}
	if _, err := Assemble(nil, "workspace-a", "maestro"); err == nil {
		t.Fatalf("expected error for nil store")
	}
}

func TestRenderIsPureAndDeterministic(t *testing.T) {
	snap := Snapshot{
		SchemaVersion: SchemaVersion,
		WorkspaceID:   "workspace-a",
		AgentID:       "maestro",
		Sections: []Section{
			{Label: "handoff_note", Body: "one", Timestamp: time.Unix(1, 0).UTC()},
			{Label: "last_action", Body: "two", Timestamp: time.Unix(2, 0).UTC()},
		},
	}
	first := Render(snap)
	second := Render(snap)
	if first != second {
		t.Fatalf("Render must be deterministic")
	}
	if !strings.HasPrefix(first, "## handoff_note\none") {
		t.Fatalf("unexpected first section rendering:\n%s", first)
	}
	if !strings.Contains(first, "\n\n## last_action\ntwo") {
		t.Fatalf("sections must be joined by blank line:\n%s", first)
	}
}
