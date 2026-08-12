package agentstate

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Store manages per-agent context snapshots under a user-local root. It
// follows the same immutable-version + atomic-commit pattern used by
// internal/memory (Spec 006) so readers observe either the previous
// complete commit or the next complete commit, never a partial write.
//
// A Store is safe for use from a single process. Concurrent Apply calls for
// the same (workspace, agent) pair are serialized by the caller; this slice
// deliberately does not add a filesystem lock because snapshot updates run
// after the owning agent's invocation completes and are naturally sequential
// per agent.
type Store struct {
	Root        string
	RuneBudget  int
	Now         func() time.Time
	transaction func() (string, error)
}

// NewStore constructs a Store rooted at the given directory. The rune
// budget defaults to DefaultRuneBudget when non-positive.
func NewStore(root string) *Store {
	return &Store{
		Root:       root,
		RuneBudget: DefaultRuneBudget,
		Now:        time.Now,
	}
}

func (store *Store) budget() int {
	if store.RuneBudget <= 0 {
		return DefaultRuneBudget
	}
	return store.RuneBudget
}

func (store *Store) now() time.Time {
	if store.Now == nil {
		return time.Now()
	}
	return store.Now()
}

func (store *Store) newTransactionID() (string, error) {
	if store.transaction != nil {
		return store.transaction()
	}
	var buffer [16]byte
	if _, err := rand.Read(buffer[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(buffer[:]), nil
}

func (store *Store) validate() error {
	if strings.TrimSpace(store.Root) == "" {
		return errRootRequired
	}
	return nil
}

func (store *Store) agentRoot(workspaceID, agentID string) string {
	return filepath.Join(store.Root, "workspaces", workspaceID, "agents", agentID)
}

func (store *Store) commitsDir(workspaceID, agentID string) string {
	return filepath.Join(store.agentRoot(workspaceID, agentID), "commits")
}

func (store *Store) versionsDir(workspaceID, agentID string) string {
	return filepath.Join(store.agentRoot(workspaceID, agentID), "versions")
}

func (store *Store) statePath(workspaceID, agentID string) string {
	return filepath.Join(store.agentRoot(workspaceID, agentID), "state.md")
}

// Load returns the currently active snapshot for the given (workspace,
// agent) pair. It returns ErrNoSnapshot when nothing has been committed and
// ErrCorruptSnapshot when commits exist but none is structurally valid.
func (store *Store) Load(workspaceID, agentID string) (Snapshot, error) {
	if err := store.validate(); err != nil {
		return Snapshot{}, err
	}
	if err := validateIdentity("workspace_id", workspaceID); err != nil {
		return Snapshot{}, err
	}
	if err := validateIdentity("agent_id", agentID); err != nil {
		return Snapshot{}, err
	}
	manifest, snapshot, err := store.loadActive(workspaceID, agentID)
	if err != nil {
		return Snapshot{}, err
	}
	_ = manifest
	return snapshot, nil
}

func (store *Store) loadActive(workspaceID, agentID string) (commitManifest, Snapshot, error) {
	commitsDir := store.commitsDir(workspaceID, agentID)
	entries, err := os.ReadDir(commitsDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return commitManifest{}, Snapshot{}, ErrNoSnapshot
		}
		return commitManifest{}, Snapshot{}, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		names = append(names, entry.Name())
	}
	if len(names) == 0 {
		return commitManifest{}, Snapshot{}, ErrNoSnapshot
	}
	sort.Sort(sort.Reverse(sort.StringSlice(names)))
	for _, name := range names {
		manifest, snapshot, ok := store.tryReadCommit(commitsDir, name, workspaceID, agentID)
		if ok {
			return manifest, snapshot, nil
		}
	}
	return commitManifest{}, Snapshot{}, ErrCorruptSnapshot
}

func (store *Store) tryReadCommit(commitsDir, name, workspaceID, agentID string) (commitManifest, Snapshot, bool) {
	raw, err := os.ReadFile(filepath.Join(commitsDir, name))
	if err != nil {
		return commitManifest{}, Snapshot{}, false
	}
	var manifest commitManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return commitManifest{}, Snapshot{}, false
	}
	if manifest.SchemaVersion != SchemaVersion {
		return commitManifest{}, Snapshot{}, false
	}
	if manifest.WorkspaceID != workspaceID || manifest.AgentID != agentID {
		return commitManifest{}, Snapshot{}, false
	}
	if strings.TrimSpace(manifest.VersionFile) == "" {
		return commitManifest{}, Snapshot{}, false
	}
	versionPath := filepath.Join(store.versionsDir(workspaceID, agentID), manifest.TransactionID, "snapshot.json")
	if filepath.Base(manifest.VersionFile) != "snapshot.json" {
		return commitManifest{}, Snapshot{}, false
	}
	rawSnapshot, err := os.ReadFile(versionPath)
	if err != nil {
		return commitManifest{}, Snapshot{}, false
	}
	var snapshot Snapshot
	if err := json.Unmarshal(rawSnapshot, &snapshot); err != nil {
		return commitManifest{}, Snapshot{}, false
	}
	if snapshot.SchemaVersion != SchemaVersion {
		return commitManifest{}, Snapshot{}, false
	}
	if snapshot.WorkspaceID != workspaceID || snapshot.AgentID != agentID {
		return commitManifest{}, Snapshot{}, false
	}
	return manifest, snapshot, true
}

// ApplyResult reports the outcome of an Apply call.
type ApplyResult struct {
	Snapshot        Snapshot
	Idempotent      bool
	DroppedSections []string
	TransactionID   string
}

// Apply merges the given update into the active snapshot for the
// (workspace, agent) pair. Idempotent updates (same section, same body as
// the active commit) return without producing a new commit. Updates that
// push the composed body past the rune budget trigger deterministic
// oldest-first section compaction before activation.
func (store *Store) Apply(update SnapshotUpdate) (ApplyResult, error) {
	if err := store.validate(); err != nil {
		return ApplyResult{}, err
	}
	if err := update.Validate(); err != nil {
		return ApplyResult{}, err
	}
	previousManifest, previous, err := store.loadActive(update.WorkspaceID, update.AgentID)
	if err != nil && !errors.Is(err, ErrNoSnapshot) {
		return ApplyResult{}, err
	}
	if err == nil {
		if !previous.UpdatedAt.IsZero() && update.Timestamp.Before(previous.UpdatedAt) {
			return ApplyResult{}, errNonMonotonicTimestamp
		}
		for _, section := range previous.Sections {
			if section.Label == update.SectionLabel && section.Body == update.Body {
				return ApplyResult{Snapshot: previous, Idempotent: true}, nil
			}
		}
	}
	incoming := Section{
		Label:        update.SectionLabel,
		Body:         update.Body,
		Timestamp:    update.Timestamp,
		SourceDigest: update.SourceDigest,
	}
	merged := mergeSection(previous.Sections, incoming)
	compacted, err := compact(merged, store.budget())
	if err != nil {
		return ApplyResult{}, err
	}
	dropped := droppedLabels(merged, compacted)

	transactionID, err := store.newTransactionID()
	if err != nil {
		return ApplyResult{}, err
	}
	snapshot := Snapshot{
		SchemaVersion: SchemaVersion,
		WorkspaceID:   update.WorkspaceID,
		AgentID:       update.AgentID,
		Sections:      compacted,
		UpdatedAt:     update.Timestamp,
	}
	parent := ""
	if err == nil {
		parent = previousManifest.TransactionID
	}
	if err := store.commit(snapshot, transactionID, parent); err != nil {
		return ApplyResult{}, err
	}
	return ApplyResult{
		Snapshot:        snapshot,
		DroppedSections: dropped,
		TransactionID:   transactionID,
	}, nil
}

func droppedLabels(before, after []Section) []string {
	kept := make(map[string]bool, len(after))
	for _, section := range after {
		kept[section.Label] = true
	}
	dropped := make([]string, 0)
	for _, section := range before {
		if !kept[section.Label] {
			dropped = append(dropped, section.Label)
		}
	}
	return dropped
}

func (store *Store) commit(snapshot Snapshot, transactionID, parent string) error {
	versionsDir := store.versionsDir(snapshot.WorkspaceID, snapshot.AgentID)
	commitsDir := store.commitsDir(snapshot.WorkspaceID, snapshot.AgentID)
	agentRoot := store.agentRoot(snapshot.WorkspaceID, snapshot.AgentID)
	stagingDir := filepath.Join(agentRoot, ".transactions", transactionID)

	for _, directory := range []string{versionsDir, commitsDir, filepath.Dir(stagingDir)} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(stagingDir, 0o700); err != nil {
		return err
	}

	snapshotJSON, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return err
	}
	stagedSnapshot := filepath.Join(stagingDir, "snapshot.json")
	if err := writeAtomicFile(stagedSnapshot, snapshotJSON, 0o600); err != nil {
		return err
	}
	body := renderBody(snapshot.Sections)
	stagedState := filepath.Join(stagingDir, "state.md")
	if err := writeAtomicFile(stagedState, []byte(body), 0o600); err != nil {
		return err
	}

	versionDir := filepath.Join(versionsDir, transactionID)
	if err := os.Rename(stagingDir, versionDir); err != nil {
		return err
	}

	committedAt := store.now().UTC()
	manifest := commitManifest{
		SchemaVersion: SchemaVersion,
		WorkspaceID:   snapshot.WorkspaceID,
		AgentID:       snapshot.AgentID,
		TransactionID: transactionID,
		ParentCommit:  parent,
		CommittedAt:   committedAt,
		VersionFile:   filepath.Join("versions", transactionID, "snapshot.json"),
	}
	manifestJSON, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	commitName := fmt.Sprintf("%s-%s.json", committedAt.Format("20060102T150405.000000000Z"), transactionID)
	commitPath := filepath.Join(commitsDir, commitName)
	pendingPath := commitPath + ".pending"
	if err := writeAtomicFile(pendingPath, manifestJSON, 0o600); err != nil {
		return err
	}
	if err := os.Rename(pendingPath, commitPath); err != nil {
		return err
	}
	// Publish the state.md projection last so readers of the currently
	// active projection always see either the previous or the next full
	// version, never a partial write.
	statePath := store.statePath(snapshot.WorkspaceID, snapshot.AgentID)
	if err := writeAtomicFile(statePath, []byte(body), 0o600); err != nil {
		return err
	}
	return nil
}

func writeAtomicFile(target string, payload []byte, mode os.FileMode) error {
	directory := filepath.Dir(target)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(directory, ".write-")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpName) }
	if _, err := tmp.Write(payload); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return err
	}
	if err := os.Rename(tmpName, target); err != nil {
		cleanup()
		return err
	}
	return nil
}
