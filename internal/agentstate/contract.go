// Package agentstate implements the per-agent context snapshot defined by
// Spec 052. A snapshot is a bounded, prose-shaped operational note stored
// under the same workspace boundary as memory (Spec 006) and isolated by
// agent identity. It is not a new memory layer and it is not the
// metadata-only breadcrumb tail defined by Spec 047.
package agentstate

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

// DefaultRuneBudget is the bundled default rune budget for a snapshot body.
// It is managed configuration, not an engine constant; callers may override
// it on the Store.
const DefaultRuneBudget = 2048

// SchemaVersion pins the on-disk snapshot layout so future migrations remain
// explicit and reversible.
const SchemaVersion = 1

// SnapshotUpdate is an authenticated post-invocation payload from the agent
// that owns a snapshot. It carries the smallest useful residue from the last
// operation, never raw prompts, raw tool outputs, credentials or
// client-identifying content.
type SnapshotUpdate struct {
	AgentID      string    `json:"agent_id"`
	WorkspaceID  string    `json:"workspace_id"`
	Timestamp    time.Time `json:"timestamp"`
	SectionLabel string    `json:"section_label"`
	Body         string    `json:"body"`
	SourceDigest string    `json:"source_digest"`
}

// Section is one semantic slice of an active snapshot. Sections are the unit
// of deterministic compaction: whole oldest sections are dropped before any
// mid-section truncation is considered.
type Section struct {
	Label        string    `json:"label"`
	Body         string    `json:"body"`
	Timestamp    time.Time `json:"timestamp"`
	SourceDigest string    `json:"source_digest"`
}

// Snapshot is the currently active per-agent operational note for a specific
// workspace/agent pair. It is the observable projection produced by the
// engine; runtimes must never write it directly.
type Snapshot struct {
	SchemaVersion int       `json:"schema_version"`
	WorkspaceID   string    `json:"workspace_id"`
	AgentID       string    `json:"agent_id"`
	Sections      []Section `json:"sections"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// commitManifest is the atomic activation boundary. Readers resolve a
// snapshot only from the newest fully valid commit for the exact
// (workspace, agent) pair.
type commitManifest struct {
	SchemaVersion int       `json:"schema_version"`
	WorkspaceID   string    `json:"workspace_id"`
	AgentID       string    `json:"agent_id"`
	TransactionID string    `json:"transaction_id"`
	ParentCommit  string    `json:"parent_commit,omitempty"`
	CommittedAt   time.Time `json:"committed_at"`
	VersionFile   string    `json:"version_file"`
}

// identityPattern mirrors the workspace identity contract used by
// internal/memory. Agent identities share the same shape so injection
// remains a straightforward `(workspace, agent)` lookup.
var identityPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)

// labelPattern keeps section labels short, structured and safe for storage
// and diagnostics. It is deliberately narrower than the identity pattern.
var labelPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

// ErrNoSnapshot indicates a valid absence of any committed snapshot for the
// requested (workspace, agent) pair.
var ErrNoSnapshot = errors.New("agentstate: no snapshot for workspace/agent")

// ErrCorruptSnapshot indicates that snapshot commit files exist but none is
// structurally valid. Callers must not silently fall back to a raw body.
var ErrCorruptSnapshot = errors.New("agentstate: snapshot commits exist but none is valid")

// Validate checks the structural invariants of a SnapshotUpdate. It never
// inspects the body for content policy; the producing agent is responsible
// for sanitization before submitting the update.
func (update SnapshotUpdate) Validate() error {
	if err := validateIdentity("workspace_id", update.WorkspaceID); err != nil {
		return err
	}
	if err := validateIdentity("agent_id", update.AgentID); err != nil {
		return err
	}
	if update.Timestamp.IsZero() {
		return errors.New("agentstate: snapshot update requires timestamp")
	}
	if !labelPattern.MatchString(update.SectionLabel) {
		return fmt.Errorf("agentstate: invalid section_label %q", update.SectionLabel)
	}
	if strings.TrimSpace(update.Body) == "" {
		return errors.New("agentstate: snapshot update requires non-empty body")
	}
	if strings.TrimSpace(update.SourceDigest) == "" {
		return errors.New("agentstate: snapshot update requires source_digest")
	}
	return nil
}

func validateIdentity(field, value string) error {
	if !identityPattern.MatchString(value) {
		return fmt.Errorf("agentstate: invalid %s %q", field, value)
	}
	return nil
}

// runeLen returns the rune count of a string using UTF-8 decoding. Rune
// budgets are expressed in runes, not bytes, so snapshots stay comparable
// across encodings.
func runeLen(value string) int {
	return utf8.RuneCountInString(value)
}
