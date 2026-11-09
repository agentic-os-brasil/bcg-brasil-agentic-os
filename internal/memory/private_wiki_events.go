package memory

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/privatewiki"
)

const privateWikiCohort = "private-memory-wiki-v1"

var ErrPrivateWikiCohort = errors.New("private_wiki_checkpoint_cohort_unavailable")

type privateWikiSourceEvent struct {
	SchemaVersion int       `json:"schema_version"`
	ID            string    `json:"id"`
	WorkspaceID   string    `json:"workspace_id"`
	Cohort        string    `json:"cohort"`
	Reason        string    `json:"reason"`
	Generation    uint64    `json:"denial_generation"`
	SourceCommit  string    `json:"source_commit,omitempty"`
	PolicyVersion string    `json:"policy_version"`
	PolicyDigest  string    `json:"policy_digest"`
	CreatedAt     time.Time `json:"created_at"`
	Recovered     bool      `json:"recovered"`
}

type privateWikiCohortState struct {
	Cohort         string                  `json:"cohort"`
	DenialEvent    *privateWikiSourceEvent `json:"denial_event,omitempty"`
	RetiredThrough uint64                  `json:"retired_through"`
}

func (engine *Engine) newPrivateWikiEvent(workspace, reason, commit string, generation uint64, at time.Time, recovered bool) privateWikiSourceEvent {
	b, _ := json.Marshal(engine.Policy)
	return privateWikiSourceEvent{SchemaVersion: 1, ID: memoryRandomID(), WorkspaceID: workspace, Cohort: privateWikiCohort, Reason: reason, Generation: generation, SourceCommit: commit, PolicyVersion: "memory-source-events-v1", PolicyDigest: privatewiki.Digest(b), CreatedAt: at.UTC(), Recovered: recovered}
}

func validPrivateWikiEvent(e privateWikiSourceEvent, workspace string) bool {
	if e.SchemaVersion != 1 || len(e.ID) != 32 || !isHex(e.ID) || e.WorkspaceID != workspace || e.Cohort != privateWikiCohort || e.PolicyVersion != "memory-source-events-v1" || !privatewiki.ValidID(e.PolicyDigest) || e.CreatedAt.IsZero() {
		return false
	}
	if e.Recovered {
		if e.Reason != "reconcile_requested" {
			return false
		}
	} else if e.Reason != "memory_commit_activated" && e.Reason != "access_revoked" {
		return false
	}
	if e.SourceCommit != "" && (filepath.Base(e.SourceCommit) != e.SourceCommit || !strings.HasSuffix(e.SourceCommit, ".json") || strings.HasPrefix(e.SourceCommit, ".") || len(e.SourceCommit) > 192) {
		return false
	}
	return true
}

func validatePrivateWikiCohort(b visibilityBarrier) error {
	if b.PrivateWiki == nil {
		return nil
	}
	w := b.PrivateWiki
	if w.Cohort != privateWikiCohort || w.RetiredThrough > b.Generation {
		return ErrPrivateWikiCohort
	}
	if b.Generation == 0 {
		if w.DenialEvent != nil || w.RetiredThrough != 0 {
			return ErrPrivateWikiCohort
		}
		return nil
	}
	if w.DenialEvent == nil || !validPrivateWikiEvent(*w.DenialEvent, b.WorkspaceID) || w.DenialEvent.Generation != b.Generation || w.DenialEvent.SourceCommit != "" || w.DenialEvent.Reason == "memory_commit_activated" {
		return ErrPrivateWikiCohort
	}
	return nil
}

// The caller holds the canonical activation lock. The permanent v2 requirement
// precedes the checkpoint, so interruption cannot reopen legacy history. Partial
// protected state is unavailable rather than repaired from guessed defaults.
func (engine *Engine) preparePrivateWikiCohort(workspace string) (visibilityBarrier, error) {
	requirement, required, err := engine.readVisibilityRequirement(workspace)
	if err != nil {
		return visibilityBarrier{}, err
	}
	b, err := engine.readVisibility(workspace)
	if err != nil {
		return visibilityBarrier{}, err
	}
	if nativeErr := engine.privateWikiNativeAvailable(workspace); nativeErr != nil {
		if b.PrivateWiki != nil {
			return visibilityBarrier{}, nativeErr
		}
		// Ordinary unprotected memory keeps its existing schema and no wiki IO.
		// A retained derivative cannot be reclassified by removing its marker.
		if _, err := memoryReadDir(engine.privateWikiRoot(workspace)); !errors.Is(err, os.ErrNotExist) {
			return visibilityBarrier{}, nativeErr
		}
		return b, nil
	}
	custody, err := engine.holdPrivateWikiWorkspace(workspace)
	if err != nil {
		return visibilityBarrier{}, err
	}
	defer custody()
	if b.PrivateWiki == nil {
		// An established protected derivative cannot be reclassified as legacy by
		// dropping both checkpoint extension and marker version.
		if _, err := memoryReadDir(engine.privateWikiRoot(workspace)); err == nil || !errors.Is(err, os.ErrNotExist) {
			return visibilityBarrier{}, ErrPrivateWikiCohort
		}
		b.PrivateWiki = &privateWikiCohortState{Cohort: privateWikiCohort}
		if !required {
			b.LegacyThrough = b.Revision
		}
		if b.Generation > 0 {
			event := engine.newPrivateWikiEvent(workspace, "reconcile_requested", "", b.Generation, engine.Now().UTC(), true)
			b.PrivateWiki.DenialEvent = &event
		}
	}
	if validateVisibilityState(workspace, b) != nil {
		return visibilityBarrier{}, ErrPrivateWikiCohort
	}
	if required && requirement.SchemaVersion == 2 {
		return b, nil
	}
	directory := filepath.Join(engine.workspaceRoot(workspace), ".visibility")
	if memoryMkdirAll(directory, 0700) != nil {
		return visibilityBarrier{}, ErrPrivateWikiCohort
	}
	body, _ := json.Marshal(b)
	checkpoint := filepath.Join(directory, "checkpoint.json")
	_, readErr := memoryReadFile(checkpoint, maximumVisibilityCheckpointBytes)
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return visibilityBarrier{}, ErrPrivateWikiCohort
	}
	marker, _ := json.Marshal(visibilityCheckpointRequirement{SchemaVersion: 2, WorkspaceID: workspace})
	if memoryPublish(filepath.Join(engine.workspaceRoot(workspace), ".visibility-checkpoint-required"), marker, !required) != nil {
		return visibilityBarrier{}, ErrPrivateWikiCohort
	}
	if err := engine.fault("after_visibility_checkpoint_required"); err != nil {
		return visibilityBarrier{}, err
	}
	if memoryPublish(checkpoint, body, errors.Is(readErr, os.ErrNotExist)) != nil {
		return visibilityBarrier{}, ErrPrivateWikiCohort
	}
	if err := engine.fault("wiki_after_checkpoint_upgrade"); err != nil {
		return visibilityBarrier{}, err
	}
	return b, nil
}

// Denial is already durable before cleanup. A failed cleanup keeps the pending
// generation in source authority and can never make an old view readable.
func (engine *Engine) retirePrivateWiki(workspace string) error {
	custody, err := engine.holdPrivateWikiWorkspace(workspace)
	if err != nil {
		return ErrPrivateWikiRetirement
	}
	defer custody()
	b, err := engine.preparePrivateWikiCohort(workspace)
	if err != nil {
		return err
	}
	if b.PrivateWiki == nil || b.Generation == 0 || b.PrivateWiki.RetiredThrough == b.Generation {
		return nil
	}
	if engine.fault("wiki_before_retirement") != nil {
		return ErrPrivateWikiRetirement
	}
	if privateWikiRetire(engine.privateWikiRoot(workspace)) != nil {
		return ErrPrivateWikiRetirement
	}
	if engine.fault("wiki_after_retirement") != nil {
		return ErrPrivateWikiRetirement
	}
	after, err := engine.readVisibility(workspace)
	if err != nil || after.Generation != b.Generation || after.Revision != b.Revision || after.PrivateWiki == nil {
		return ErrPrivateWikiRetirement
	}
	after.PrivateWiki.RetiredThrough = after.Generation
	body, _ := json.Marshal(after)
	if memoryPublish(filepath.Join(engine.workspaceRoot(workspace), ".visibility", "checkpoint.json"), body, false) != nil {
		return ErrPrivateWikiRetirement
	}
	return nil
}

func (engine *Engine) privateWikiEventPath(workspace, commit string) string {
	return filepath.Join(engine.workspaceRoot(workspace), ".wiki-outbox", commit)
}

func (engine *Engine) publishPrivateWikiCommitIntent(workspace, commit string, manifest CommitManifest) error {
	if engine.privateWikiNativeAvailable(workspace) != nil {
		return nil
	}
	custody, err := engine.holdPrivateWikiWorkspace(workspace)
	if err != nil {
		return err
	}
	defer custody()
	if filepath.Base(commit) != commit || len(commit) > 192 || strings.HasPrefix(commit, ".") {
		return ErrPrivateWikiCohort
	}
	directory := filepath.Dir(engine.privateWikiEventPath(workspace, commit))
	if memoryMkdirAll(directory, 0700) != nil {
		return ErrPrivateWikiCohort
	}
	entries, err := memoryReadDir(directory)
	if err != nil || len(entries) >= 4096 {
		return ErrCaptureCapacity
	}
	event := engine.newPrivateWikiEvent(workspace, "memory_commit_activated", commit, manifest.VisibilityRevision, manifest.CommittedAt, false)
	if err := engine.fault("wiki_before_commit_intent"); err != nil {
		return err
	}
	body, _ := json.Marshal(event)
	if memoryPublish(engine.privateWikiEventPath(workspace, commit), body, true) != nil {
		return ErrPrivateWikiCohort
	}
	return engine.fault("wiki_after_commit_intent")
}

func (engine *Engine) recoverPrivateWikiEvent(workspace string, snapshot privateWikiSnapshot) (bool, error) {
	path := engine.privateWikiEventPath(workspace, snapshot.commitName)
	body, err := memoryReadFile(path, 4096)
	if err == nil {
		var e privateWikiSourceEvent
		if decodeMemoryJSON(body, &e) != nil || !validPrivateWikiEvent(e, workspace) || e.SourceCommit != snapshot.commitName || e.Generation != snapshot.input.DenialGeneration || e.Reason == "access_revoked" || !e.Recovered && !e.CreatedAt.Equal(snapshot.input.CommittedAt) {
			return false, ErrPrivateWikiCohort
		}
		return false, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return false, ErrPrivateWikiCohort
	}
	if memoryMkdirAll(filepath.Dir(path), 0700) != nil {
		return false, ErrPrivateWikiCohort
	}
	event := engine.newPrivateWikiEvent(workspace, "reconcile_requested", snapshot.commitName, snapshot.input.DenialGeneration, engine.Now().UTC(), true)
	body, _ = json.Marshal(event)
	if memoryPublish(path, body, true) != nil {
		return false, ErrPrivateWikiCohort
	}
	return true, nil
}
