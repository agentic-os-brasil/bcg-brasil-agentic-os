package longrun

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const stateSchemaVersion = 1

var ErrMonotonicAnchorUnavailable = errors.New("long-running monotonic anchor is unavailable")
var ErrGoalCommitInProgress = errors.New("long-running goal commit is already in progress")

// MonotonicAnchor is outside Store.Root. A secure host adapter (keychain, OS
// credential service or append-only service) owns it so a directory restore
// cannot replace both the state and its accepted event head.
type MonotonicAnchor interface {
	Load(goalID string) (AnchorRecord, error)
	Store(head AnchorRecord) error
}

// Store owns the user-local integrity key and durable transition log. A saved
// snapshot is only a cache: Load verifies every receipt and replays the events
// through Goal before returning state after context compaction or restart.
type Store struct {
	Root   string
	Anchor MonotonicAnchor
}

type persistedGoal struct {
	SchemaVersion          int                    `json:"schema_version"`
	ID                     string                 `json:"id"`
	Contract               DoneContract           `json:"contract"`
	Status                 Status                 `json:"status"`
	Phase                  string                 `json:"phase,omitempty"`
	Evidence               []Evidence             `json:"evidence"`
	Breadcrumbs            []Breadcrumb           `json:"breadcrumbs"`
	NeedsFreshWalterReview bool                   `json:"needs_fresh_walter_review"`
	WorkspaceReady         bool                   `json:"workspace_ready"`
	SpecialistReturned     bool                   `json:"specialist_returned"`
	CompletedDeliverables  []string               `json:"completed_deliverables"`
	BlockerRefs            []string               `json:"blocker_refs"`
	LedgerRevision         int                    `json:"ledger_revision"`
	WalterApprovedLedger   int                    `json:"walter_approved_ledger"`
	CompletionAudit        *CompletionAudit       `json:"completion_audit,omitempty"`
	Delegations            []SpecialistWorkPacket `json:"delegations"`
	Events                 []GoalEvent            `json:"events"`
}

// AnchorRecord is a separately committed monotonic pointer to the accepted event
// head. A valid signed prefix is therefore not a valid recovery state.
type AnchorRecord struct {
	GoalID   string `json:"goal_id"`
	Sequence int    `json:"sequence"`
	Terminal bool   `json:"terminal"`
	MAC      string `json:"mac"`
}

func (store Store) Create(goal *Goal) error {
	if err := store.validateRoot(); err != nil || goal == nil || !goal.validForPersistence() || len(goal.events) == 0 {
		return errors.New("invalid long-running goal store create")
	}
	return store.withGoalLock(goal.ID(), func() error {
		if err := os.MkdirAll(filepath.Dir(store.statePath(goal.ID())), 0o700); err != nil {
			return err
		}
		if _, err := os.Stat(store.statePath(goal.ID())); err == nil {
			return os.ErrExist
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		key, err := store.ensureKey()
		if err != nil {
			return err
		}
		// Anchor first: a stale writer is rejected before it can overwrite state.
		if err := store.writeAnchor(goal, key); err != nil {
			return err
		}
		return writeNewJSON(store.statePath(goal.ID()), snapshotWithReceipts(goal, key))
	})
}
func (store Store) Save(goal *Goal) error {
	if err := store.validateRoot(); err != nil || goal == nil || !goal.validForPersistence() {
		return errors.New("invalid long-running goal store save")
	}
	return store.withGoalLock(goal.ID(), func() error {
		if _, err := os.Stat(store.statePath(goal.ID())); err != nil {
			return err
		}
		key, err := store.loadKey()
		if err != nil {
			return err
		}
		if err := store.writeAnchor(goal, key); err != nil {
			return err
		}
		return writeAtomicJSON(store.statePath(goal.ID()), snapshotWithReceipts(goal, key))
	})
}
func (store Store) Load(id string) (*Goal, error) {
	if err := store.validateRoot(); err != nil || !idPattern.MatchString(id) {
		return nil, errors.New("invalid long-running goal store load")
	}
	key, err := store.loadKey()
	if err != nil {
		return nil, err
	}
	var persisted persistedGoal
	if err := readStrictJSON(store.statePath(id), &persisted); err != nil {
		return nil, err
	}
	if persisted.SchemaVersion != stateSchemaVersion || persisted.ID != id || !validContract(persisted.Contract) || len(persisted.Events) == 0 || !verifyEvents(persisted.Events, key) {
		return nil, errors.New("invalid persisted long-running goal receipts")
	}
	head, err := store.readAnchor(id, key)
	if err != nil || head.Sequence != len(persisted.Events) || head.Terminal != (persisted.Status == Completed) {
		return nil, errors.New("invalid long-running goal head")
	}
	goal, err := NewGoal(persisted.ID, persisted.Contract)
	if err != nil {
		return nil, err
	}
	for _, signed := range persisted.Events {
		event := signed
		event.MAC = ""
		if err := replay(goal, event); err != nil {
			return nil, errors.New("invalid persisted long-running transition")
		}
	}
	if !samePersistedState(persisted, snapshot(goal)) {
		return nil, errors.New("persisted long-running state does not match its receipts")
	}
	return goal, nil
}
func (store Store) statePath(id string) string {
	return filepath.Join(store.Root, "goals", id, "state.json")
}
func (store Store) keyPath() string           { return filepath.Join(store.Root, ".longrun-integrity-key") }
func (store Store) lockPath(id string) string { return filepath.Join(store.Root, "locks", id+".lock") }
func (store Store) validateRoot() error {
	if strings.TrimSpace(store.Root) == "" {
		return errors.New("long-running goal store root is required")
	}
	return nil
}
func (store Store) withGoalLock(id string, operation func() error) error {
	if err := os.MkdirAll(filepath.Dir(store.lockPath(id)), 0o700); err != nil {
		return err
	}
	lock, err := os.OpenFile(store.lockPath(id), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		return ErrGoalCommitInProgress
	}
	if err != nil {
		return err
	}
	if err := lock.Close(); err != nil {
		_ = os.Remove(store.lockPath(id))
		return err
	}
	defer os.Remove(store.lockPath(id))
	return operation()
}
func (store Store) ensureKey() ([]byte, error) {
	if key, err := store.loadKey(); err == nil {
		return key, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if err := os.MkdirAll(store.Root, 0o700); err != nil {
		return nil, err
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(store.keyPath(), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return store.loadKey()
		}
		return nil, err
	}
	if _, err := file.WriteString(hex.EncodeToString(key)); err != nil {
		_ = file.Close()
		return nil, err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return nil, err
	}
	if err := file.Close(); err != nil {
		return nil, err
	}
	return key, nil
}
func (store Store) loadKey() ([]byte, error) {
	encoded, err := os.ReadFile(store.keyPath())
	if err != nil {
		return nil, err
	}
	key, err := hex.DecodeString(strings.TrimSpace(string(encoded)))
	if err != nil || len(key) != 32 {
		return nil, errors.New("invalid long-running integrity key")
	}
	return key, nil
}
func (store Store) writeAnchor(goal *Goal, key []byte) error {
	if store.Anchor == nil {
		return ErrMonotonicAnchorUnavailable
	}
	head := AnchorRecord{GoalID: goal.ID(), Sequence: len(goal.events), Terminal: goal.status == Completed}
	head.MAC = headMAC(head, key)
	return store.Anchor.Store(head)
}
func (store Store) readAnchor(id string, key []byte) (AnchorRecord, error) {
	if store.Anchor == nil {
		return AnchorRecord{}, ErrMonotonicAnchorUnavailable
	}
	head, err := store.Anchor.Load(id)
	if err != nil {
		return AnchorRecord{}, err
	}
	if head.GoalID != id || head.Sequence < 1 || !hmac.Equal([]byte(head.MAC), []byte(headMAC(head, key))) {
		return AnchorRecord{}, errors.New("invalid long-running goal head")
	}
	return head, nil
}

func snapshotWithReceipts(goal *Goal, key []byte) persistedGoal {
	state := snapshot(goal)
	state.Events = signEvents(goal.Events(), key)
	return state
}
func snapshot(goal *Goal) persistedGoal {
	delegations := make([]SpecialistWorkPacket, 0, len(goal.delegations))
	for _, delegation := range goal.delegations {
		delegations = append(delegations, delegation)
	}
	sort.Slice(delegations, func(i, j int) bool { return delegations[i].DelegationID < delegations[j].DelegationID })
	completed := make([]string, 0, len(goal.completedDeliverables))
	for id := range goal.completedDeliverables {
		completed = append(completed, id)
	}
	sort.Strings(completed)
	blockers := make([]string, 0, len(goal.blockerRefs))
	for ref := range goal.blockerRefs {
		blockers = append(blockers, ref)
	}
	sort.Strings(blockers)
	var audit *CompletionAudit
	if goal.completionAudit != nil {
		audit = cloneAudit(goal.completionAudit)
	}
	return persistedGoal{SchemaVersion: stateSchemaVersion, ID: goal.id, Contract: cloneContract(goal.contract), Status: goal.status, Phase: goal.phase, Evidence: append([]Evidence(nil), goal.evidence...), Breadcrumbs: append([]Breadcrumb(nil), goal.breadcrumbs...), NeedsFreshWalterReview: goal.needsFreshWalterReview, WorkspaceReady: goal.workspaceReady, SpecialistReturned: goal.specialistReturned, CompletedDeliverables: completed, BlockerRefs: blockers, LedgerRevision: goal.ledgerRevision, WalterApprovedLedger: goal.walterApprovedLedger, CompletionAudit: audit, Delegations: delegations, Events: cloneEvents(goal.events)}
}
func signEvents(events []GoalEvent, key []byte) []GoalEvent {
	signed := cloneEvents(events)
	for index := range signed {
		signed[index].MAC = eventMAC(signed[index], key)
	}
	return signed
}
func verifyEvents(events []GoalEvent, key []byte) bool {
	for index, event := range events {
		if event.Sequence != index+1 || event.MAC == "" || !hmac.Equal([]byte(event.MAC), []byte(eventMAC(event, key))) {
			return false
		}
	}
	return true
}
func eventMAC(event GoalEvent, key []byte) string {
	event.MAC = ""
	encoded, _ := json.Marshal(event)
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(encoded)
	return hex.EncodeToString(mac.Sum(nil))
}
func headMAC(head AnchorRecord, key []byte) string {
	head.MAC = ""
	encoded, _ := json.Marshal(head)
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(encoded)
	return hex.EncodeToString(mac.Sum(nil))
}
func samePersistedState(left, right persistedGoal) bool {
	left.Events, right.Events = nil, nil
	encodedLeft, _ := json.Marshal(left)
	encodedRight, _ := json.Marshal(right)
	return hmac.Equal(encodedLeft, encodedRight)
}
func (goal *Goal) validForPersistence() bool { return goal != nil && !goal.invalidForPersistence() }
func (goal *Goal) invalidForPersistence() bool {
	if !idPattern.MatchString(goal.id) || !validContract(goal.contract) || !validPersistedStatus(goal.status) || (goal.status != Draft && !idPattern.MatchString(goal.phase)) || goal.delegations == nil || goal.completedDeliverables == nil || goal.blockerRefs == nil || goal.ledgerRevision < 0 || goal.walterApprovedLedger < 0 {
		return true
	}
	for _, evidence := range goal.evidence {
		if !validEvidence(evidence) || goal.evidenceCount(evidence.ID) != 1 {
			return true
		}
	}
	return false
}
func (goal *Goal) evidenceCount(id string) int {
	count := 0
	for _, evidence := range goal.evidence {
		if evidence.ID == id {
			count++
		}
	}
	return count
}
func validPersistedStatus(status Status) bool {
	return status == Draft || status == Active || status == AwaitingWalter || status == AwaitingHuman || status == Completed || status == Blocked
}
func validAction(action Action) bool {
	return action == "" || action == ActionReturnToWorkspace || action == ActionComposeAdvancement || action == ActionRequestWalter || action == ActionRequestHuman || action == ActionCompletionAudit
}
func readStrictJSON(path string, target any) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("long-running state contains multiple JSON values")
		}
		return err
	}
	return nil
}
func writeNewJSON(path string, value any) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if err := json.NewEncoder(file).Encode(value); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}
func writeAtomicJSON(path string, value any) error {
	temporary, err := os.CreateTemp(filepath.Dir(path), ".longrun-*.json")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := json.NewEncoder(temporary).Encode(value); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryName, path)
}
func ValidateSchemaFile(path string) error {
	var schema map[string]any
	if err := readStrictJSON(path, &schema); err != nil {
		return err
	}
	if schema["$schema"] != "https://json-schema.org/draft/2020-12/schema" || schema["$id"] != "urn:bcg-brasil-agentic-os:schema:long-running-goal:v1" {
		return errors.New("invalid long-running goal state schema")
	}
	return nil
}
