package agentstate

import (
	"errors"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

// SnapshotLayerLabel identifies the agent context snapshot when it is
// appended after the canonical `lifetime -> L3 -> L2 -> L1` memory order.
// Spec 052 §Injection reserves this exact label; runtimes must not remap it.
const SnapshotLayerLabel = "agent-snapshot"

// InjectableSection is the runtime-neutral projection produced by Assemble.
// Its shape mirrors internal/memory.ContextSection so callers can append it
// directly to their existing context bundle without a package dependency
// from memory to agentstate.
type InjectableSection struct {
	// Layer is always SnapshotLayerLabel when Diagnostic is empty. When the
	// section is absent (missing or invalid snapshot) Layer is empty and
	// Diagnostic carries the human-readable reason instead.
	Layer string

	// Content is the rendered, budget-respecting prose body of the snapshot.
	// Empty when Diagnostic is set.
	Content string

	// DrillDown is a slash-separated relative pointer to the versioned
	// snapshot body, matching the memory bundle's DrillDown convention. It
	// is empty when Diagnostic is set.
	DrillDown string

	// Truncated is true when deterministic compaction dropped one or more
	// whole sections during rendering. It is false when Diagnostic is set.
	Truncated bool

	// Diagnostic is empty for a successful assembly. On missing or invalid
	// snapshots it carries the same shape memory.AssembleContext uses:
	// "agent-snapshot: missing; skipped" or "agent-snapshot: invalid; skipped".
	Diagnostic string
}

// Assemble returns the injectable agent-snapshot section for the exact
// (workspace, agent) pair. It never falls back to another agent, another
// workspace or a raw body: a missing or invalid snapshot yields an empty
// section with a diagnostic, never a synthesized substitute.
//
// The returned error is reserved for structural validation failures on the
// inputs (empty store, empty identity, malformed identity). Absent or
// corrupt on-disk snapshots are reported through Diagnostic, matching the
// memory bundle's behavior for missing layers.
func Assemble(store *Store, workspaceID, agentID string) (InjectableSection, error) {
	if store == nil {
		return InjectableSection{}, errors.New("agentstate: store is required")
	}
	if err := validateIdentity("workspace_id", workspaceID); err != nil {
		return InjectableSection{}, err
	}
	if err := validateIdentity("agent_id", agentID); err != nil {
		return InjectableSection{}, err
	}
	snapshot, err := store.Load(workspaceID, agentID)
	if errors.Is(err, ErrNoSnapshot) {
		return InjectableSection{Diagnostic: SnapshotLayerLabel + ": missing; skipped"}, nil
	}
	if errors.Is(err, ErrCorruptSnapshot) {
		return InjectableSection{Diagnostic: SnapshotLayerLabel + ": invalid; skipped"}, nil
	}
	if err != nil {
		return InjectableSection{}, err
	}
	if snapshot.SchemaVersion != SchemaVersion ||
		snapshot.WorkspaceID != workspaceID ||
		snapshot.AgentID != agentID ||
		len(snapshot.Sections) == 0 {
		return InjectableSection{Diagnostic: SnapshotLayerLabel + ": invalid; skipped"}, nil
	}
	budget := store.budget()
	content, truncated := renderSnapshot(snapshot, budget)
	if content == "" {
		return InjectableSection{Diagnostic: SnapshotLayerLabel + ": invalid; skipped"}, nil
	}
	return InjectableSection{
		Layer:     SnapshotLayerLabel,
		Content:   content,
		DrillDown: snapshotDrillDown(workspaceID, agentID),
		Truncated: truncated,
	}, nil
}

// Render is a pure formatter that returns the prose projection of a snapshot
// without touching the store. Sections are emitted in the same order they
// are stored (oldest to newest), joined by a blank line. Each section is
// prefixed with a Markdown H2 heading matching its label so downstream
// consumers can parse or display it consistently.
//
// Render never truncates. Callers that must respect a budget use Assemble,
// which invokes deterministic drop-oldest compaction before rendering.
func Render(snapshot Snapshot) string {
	if len(snapshot.Sections) == 0 {
		return ""
	}
	labels := make([]string, 0, len(snapshot.Sections))
	seen := make(map[string]struct{}, len(snapshot.Sections))
	sorted := append([]Section(nil), snapshot.Sections...)
	// Preserve store order but guard against accidental duplicates: the store
	// contract already enforces uniqueness, but the renderer is defensive.
	for _, section := range sorted {
		if _, dup := seen[section.Label]; dup {
			continue
		}
		seen[section.Label] = struct{}{}
		labels = append(labels, section.Label)
	}
	var builder strings.Builder
	for i, label := range labels {
		if i > 0 {
			builder.WriteString("\n\n")
		}
		section := findSection(sorted, label)
		builder.WriteString("## ")
		builder.WriteString(label)
		builder.WriteByte('\n')
		builder.WriteString(strings.TrimRight(section.Body, "\n"))
	}
	return builder.String()
}

// renderSnapshot applies deterministic drop-oldest compaction to fit the
// rendered content within the rune budget. It never truncates mid-section
// and never reorders surviving sections.
func renderSnapshot(snapshot Snapshot, budget int) (string, bool) {
	if budget <= 0 {
		return "", false
	}
	sections := append([]Section(nil), snapshot.Sections...)
	// Sections are stored oldest first; drop-oldest = drop from the front.
	// Stable sort by Timestamp to be defensive if the store ever changes.
	sort.SliceStable(sections, func(i, j int) bool {
		return sections[i].Timestamp.Before(sections[j].Timestamp)
	})
	truncated := false
	for {
		rendered := Render(Snapshot{
			SchemaVersion: snapshot.SchemaVersion,
			WorkspaceID:   snapshot.WorkspaceID,
			AgentID:       snapshot.AgentID,
			Sections:      sections,
			UpdatedAt:     snapshot.UpdatedAt,
		})
		if utf8.RuneCountInString(rendered) <= budget {
			return rendered, truncated
		}
		if len(sections) <= 1 {
			// Even the newest section alone exceeds the budget. This is a
			// contract violation on the writer side; Assemble reports it as
			// invalid rather than emit truncated prose.
			return "", false
		}
		sections = sections[1:]
		truncated = true
	}
}

func findSection(sections []Section, label string) Section {
	for _, s := range sections {
		if s.Label == label {
			return s
		}
	}
	return Section{}
}

// snapshotDrillDown returns the slash-separated relative path to the
// currently active snapshot file for a given (workspace, agent) pair. It
// intentionally matches the on-disk layout documented in spec 052 so callers
// can present it identically to a memory-bundle drill-down.
func snapshotDrillDown(workspaceID, agentID string) string {
	return filepath.ToSlash(filepath.Join("workspaces", workspaceID, "agents", agentID, "state.md"))
}
