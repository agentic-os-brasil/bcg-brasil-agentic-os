// Package execution owns the workspace-scoped, local execution ledger.
// It materializes resumable agent work without becoming a business task system.
package execution

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

type ItemState string

const (
	StateReady      ItemState = "ready"
	StateRunning    ItemState = "running"
	StatePaused     ItemState = "paused"
	StateEvaluating ItemState = "evaluating"
	StateCompleted  ItemState = "completed"
	StateCancelled  ItemState = "cancelled"
)

type AttemptState string

const (
	AttemptActive      AttemptState = "active"
	AttemptInterrupted AttemptState = "interrupted"
	AttemptCompleted   AttemptState = "completed"
	AttemptCancelled   AttemptState = "cancelled"
)

type CriterionType string

const (
	CriterionArtifactSnapshot CriterionType = "artifact_snapshot"
	CriterionCommandCheck     CriterionType = "command_check"
)

const AuthorityLocalExecution = "local_execution"

const ActivePointerPath = "bcgos://execution/active"

const (
	ActivePointerAvailable   = "available"
	ActivePointerUnavailable = "unavailable"
	ActivePointerAmbiguous   = "ambiguous"
)

var (
	ErrRevisionConflict     = errors.New("execution item state revision conflict")
	ErrAttemptConflict      = errors.New("execution item attempt conflict")
	ErrContractChanged      = errors.New("execution item contract changed")
	ErrConfirmationRequired = errors.New("explicit confirmation is required")
	ErrItemBusy             = errors.New("execution item is busy")
	ErrActiveItemAmbiguous  = errors.New("multiple active execution items require an explicit item ID")
	idPattern               = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,95}$`)
	workspaceIDPattern      = regexp.MustCompile(`^[a-f0-9]{32}$`)
	logicalRefPattern       = regexp.MustCompile(`^bcgos://[A-Za-z0-9/_.-]+$`)
)

const (
	MaximumCheckpointSummaryBytes  = 768
	MaximumCheckpointNextStepBytes = 768
	MaximumCheckpointBlockerBytes  = 256
	MaximumCheckpointArtifactRefs  = 8
	MaximumNextProjectionBytes     = 2048
)

type Criterion struct {
	ID        string        `json:"id"`
	Type      CriterionType `json:"type"`
	TargetRef string        `json:"target_ref,omitempty"`
	Command   []string      `json:"command,omitempty"`
}

type Contract struct {
	SchemaVersion       int         `json:"schema_version"`
	ContractVersion     int         `json:"contract_version"`
	ItemID              string      `json:"item_id"`
	WorkspaceID         string      `json:"workspace_id"`
	AuthorityKind       string      `json:"authority_kind"`
	Objective           string      `json:"objective"`
	InitialNextStep     string      `json:"initial_next_step"`
	Criteria            []Criterion `json:"criteria"`
	AllowedRefs         []string    `json:"allowed_refs"`
	RequireWalterReview bool        `json:"require_walter_review"`
	WalterPublicKey     string      `json:"walter_public_key,omitempty"`
	CreatedAt           time.Time   `json:"created_at"`
}

type State struct {
	SchemaVersion   int       `json:"schema_version"`
	ItemID          string    `json:"item_id"`
	WorkspaceID     string    `json:"workspace_id"`
	State           ItemState `json:"state"`
	StateRevision   int       `json:"state_revision"`
	ContractVersion int       `json:"contract_version"`
	ContractSHA256  string    `json:"contract_sha256"`
	ActiveAttemptID string    `json:"active_attempt_id,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type Attempt struct {
	SchemaVersion int          `json:"schema_version"`
	AttemptID     string       `json:"attempt_id"`
	ItemID        string       `json:"item_id"`
	WorkspaceID   string       `json:"workspace_id"`
	State         AttemptState `json:"state"`
	StartedAt     time.Time    `json:"started_at"`
}

// Checkpoint is private, bounded handoff context. It may contain professional
// content, so it is never copied into Transition or injected automatically.
type Checkpoint struct {
	SchemaVersion int       `json:"schema_version"`
	CheckpointID  string    `json:"checkpoint_id"`
	ItemID        string    `json:"item_id"`
	WorkspaceID   string    `json:"workspace_id"`
	AttemptID     string    `json:"attempt_id"`
	Summary       string    `json:"summary"`
	NextStep      string    `json:"next_step"`
	Blocker       string    `json:"blocker,omitempty"`
	ArtifactRefs  []string  `json:"artifact_refs"`
	CreatedAt     time.Time `json:"created_at"`
}

// Transition is intentionally metadata-only. Contract and professional content
// never enter transition history.
type Transition struct {
	SchemaVersion int       `json:"schema_version"`
	ItemID        string    `json:"item_id"`
	WorkspaceID   string    `json:"workspace_id"`
	AttemptID     string    `json:"attempt_id,omitempty"`
	State         ItemState `json:"state"`
	StateRevision int       `json:"state_revision"`
	OccurredAt    time.Time `json:"occurred_at"`
}

// Revision is the immutable commit boundary for one execution mutation.
// Projections such as state.json may be regenerated from the newest valid
// revision after an interrupted write.
type Revision struct {
	SchemaVersion int                  `json:"schema_version"`
	State         State                `json:"state"`
	Attempt       *Attempt             `json:"attempt,omitempty"`
	Checkpoint    *Checkpoint          `json:"checkpoint,omitempty"`
	Evidence      *EvidenceReceipt     `json:"evidence,omitempty"`
	ToolCall      *ToolCallReceipt     `json:"tool_call,omitempty"`
	WalterReview  *WalterReviewReceipt `json:"walter_review,omitempty"`
	Transition    Transition           `json:"transition"`
}

type CreateInput struct {
	WorkspaceID         string
	Objective           string
	InitialNextStep     string
	Criteria            []Criterion
	AllowedRefs         []string
	RequireWalterReview bool
	WalterPublicKey     string
}

type CheckpointInput struct {
	ExpectedRevision int
	AttemptID        string
	Summary          string
	NextStep         string
	Blocker          string
	ArtifactRefs     []string
}

type Item struct {
	Contract   Contract         `json:"contract"`
	State      State            `json:"state"`
	Attempt    *Attempt         `json:"attempt,omitempty"`
	Checkpoint *Checkpoint      `json:"checkpoint,omitempty"`
	Evidence   *EvidenceReceipt `json:"evidence,omitempty"`
}

type Export struct {
	Contract      Contract              `json:"contract"`
	State         State                 `json:"state"`
	Attempt       *Attempt              `json:"attempt,omitempty"`
	Checkpoint    *Checkpoint           `json:"checkpoint,omitempty"`
	Evidences     []EvidenceReceipt     `json:"evidences,omitempty"`
	ToolCalls     []ToolCallReceipt     `json:"tool_calls,omitempty"`
	WalterReviews []WalterReviewReceipt `json:"walter_reviews,omitempty"`
	Transitions   []Transition          `json:"transitions"`
}

// NextProjection is the only execution body designed for bounded model
// rehydration. It intentionally excludes the objective and completion contract.
type NextProjection struct {
	SchemaVersion int        `json:"schema_version"`
	ItemID        string     `json:"item_id"`
	State         ItemState  `json:"state"`
	StateRevision int        `json:"state_revision"`
	AttemptID     string     `json:"attempt_id,omitempty"`
	Summary       string     `json:"summary,omitempty"`
	NextStep      string     `json:"next_step"`
	Blocker       string     `json:"blocker,omitempty"`
	ArtifactRefs  []string   `json:"artifact_refs,omitempty"`
	CheckpointAt  *time.Time `json:"checkpoint_at,omitempty"`
}

type MutationReceipt struct {
	SchemaVersion int       `json:"schema_version"`
	ItemID        string    `json:"item_id"`
	WorkspaceID   string    `json:"workspace_id"`
	State         ItemState `json:"state"`
	StateRevision int       `json:"state_revision"`
	AttemptID     string    `json:"attempt_id,omitempty"`
	CheckpointID  string    `json:"checkpoint_id,omitempty"`
}

// ActivePointer is a metadata-only capability pointer. It intentionally omits
// the active item and attempt identities; an authorized caller resolves the
// bounded handoff later through `bcgos work next --active`.
type ActivePointer struct {
	Path      string `json:"path,omitempty"`
	Available bool   `json:"available"`
	State     string `json:"state"`
}

type Store struct {
	Root       string
	Now        func() time.Time
	NewID      func(kind string) (string, error)
	FaultPoint func(point string) error
}

func (store Store) Create(input CreateInput) (Item, error) {
	if err := validateStoreRoot(store.Root); err != nil {
		return Item{}, err
	}
	if err := validateCreateInput(input); err != nil {
		return Item{}, err
	}
	itemID, err := store.newID("item")
	if err != nil {
		return Item{}, err
	}
	if err := validateID("item", itemID); err != nil {
		return Item{}, err
	}
	now := store.now()
	contract := Contract{
		SchemaVersion:       1,
		ContractVersion:     1,
		ItemID:              itemID,
		WorkspaceID:         input.WorkspaceID,
		AuthorityKind:       AuthorityLocalExecution,
		Objective:           strings.TrimSpace(input.Objective),
		InitialNextStep:     strings.TrimSpace(input.InitialNextStep),
		Criteria:            append([]Criterion(nil), input.Criteria...),
		AllowedRefs:         append([]string(nil), input.AllowedRefs...),
		RequireWalterReview: input.RequireWalterReview,
		WalterPublicKey:     strings.TrimSpace(input.WalterPublicKey),
		CreatedAt:           now,
	}
	digest, err := contractDigest(contract)
	if err != nil {
		return Item{}, err
	}
	state := State{
		SchemaVersion: 1, ItemID: itemID, WorkspaceID: input.WorkspaceID,
		State: StateReady, StateRevision: 1, ContractVersion: 1,
		ContractSHA256: digest, CreatedAt: now, UpdatedAt: now,
	}
	transition := Transition{
		SchemaVersion: 1, ItemID: itemID, WorkspaceID: input.WorkspaceID,
		State: StateReady, StateRevision: 1, OccurredAt: now,
	}
	revision := Revision{SchemaVersion: 1, State: state, Transition: transition}
	if err := validateContract(contract); err != nil {
		return Item{}, err
	}
	if err := validateState(state); err != nil {
		return Item{}, err
	}
	if err := validateNextProjection(nextProjection(contract, state, nil)); err != nil {
		return Item{}, fmt.Errorf("initial next-action projection: %w", err)
	}

	itemsRoot := store.itemsRoot(input.WorkspaceID)
	stagingRoot := filepath.Join(store.executionRoot(input.WorkspaceID), ".transactions")
	if err := os.MkdirAll(stagingRoot, 0o700); err != nil {
		return Item{}, err
	}
	staging, err := os.MkdirTemp(stagingRoot, "create-")
	if err != nil {
		return Item{}, err
	}
	if err := os.Chmod(staging, 0o700); err != nil {
		_ = os.RemoveAll(staging)
		return Item{}, err
	}
	defer os.RemoveAll(staging)
	if err := writeNewJSON(filepath.Join(staging, "contract.v1.json"), contract); err != nil {
		return Item{}, err
	}
	if err := writeNewJSON(filepath.Join(staging, "state.json"), state); err != nil {
		return Item{}, err
	}
	if err := writeNewJSON(filepath.Join(staging, "revisions", revisionName(1)), revision); err != nil {
		return Item{}, err
	}
	if err := os.MkdirAll(itemsRoot, 0o700); err != nil {
		return Item{}, err
	}
	destination := filepath.Join(itemsRoot, itemID)
	if err := durableRename(staging, destination); err != nil {
		return Item{}, err
	}
	return Item{Contract: contract, State: state}, nil
}

func (store Store) Start(workspaceID, itemID string, expectedRevision int) (Item, error) {
	if err := store.validateItemInput(workspaceID, itemID); err != nil {
		return Item{}, err
	}
	unlock, err := store.lock(workspaceID, itemID)
	if err != nil {
		return Item{}, err
	}
	defer unlock()

	item, err := store.inspectUnlocked(workspaceID, itemID)
	if err != nil {
		return Item{}, err
	}
	if item.State.StateRevision != expectedRevision {
		return Item{}, ErrRevisionConflict
	}
	if item.State.State != StateReady {
		return Item{}, fmt.Errorf("execution item must be ready to start, got %s", item.State.State)
	}
	attemptID, err := store.newID("attempt")
	if err != nil {
		return Item{}, err
	}
	if err := validateID("attempt", attemptID); err != nil {
		return Item{}, err
	}
	now := store.now()
	attempt := Attempt{
		SchemaVersion: 1, AttemptID: attemptID, ItemID: itemID,
		WorkspaceID: workspaceID, State: AttemptActive, StartedAt: now,
	}
	if err := validateAttempt(attempt); err != nil {
		return Item{}, err
	}
	state := item.State
	state.State = StateRunning
	state.StateRevision++
	state.ActiveAttemptID = attemptID
	state.UpdatedAt = now
	transition := Transition{
		SchemaVersion: 1, ItemID: itemID, WorkspaceID: workspaceID,
		AttemptID: attemptID, State: StateRunning,
		StateRevision: state.StateRevision, OccurredAt: now,
	}
	revision := Revision{SchemaVersion: 1, State: state, Attempt: &attempt, Transition: transition}
	if err := validateRevision(revision); err != nil {
		return Item{}, err
	}
	if err := store.fault("before_revision_commit"); err != nil {
		return Item{}, err
	}
	if err := writeImmutableRevision(filepath.Join(store.itemRoot(workspaceID, itemID), "revisions", revisionName(state.StateRevision)), revision); err != nil {
		return Item{}, err
	}
	if err := store.fault("after_revision_commit"); err != nil {
		return Item{}, err
	}
	if err := writeAtomicJSON(filepath.Join(store.itemRoot(workspaceID, itemID), "state.json"), state); err != nil {
		return Item{}, err
	}
	if err := store.fault("after_state_projection"); err != nil {
		return Item{}, err
	}
	return Item{Contract: item.Contract, State: state, Attempt: &attempt}, nil
}

func (store Store) Checkpoint(workspaceID, itemID string, input CheckpointInput) (Item, error) {
	if err := store.validateItemInput(workspaceID, itemID); err != nil {
		return Item{}, err
	}
	if err := validateCheckpointInput(input); err != nil {
		return Item{}, err
	}
	unlock, err := store.lock(workspaceID, itemID)
	if err != nil {
		return Item{}, err
	}
	defer unlock()

	item, err := store.inspectUnlocked(workspaceID, itemID)
	if err != nil {
		return Item{}, err
	}
	if item.State.StateRevision != input.ExpectedRevision {
		return Item{}, ErrRevisionConflict
	}
	if item.State.State != StateRunning {
		return Item{}, fmt.Errorf("execution item must be running to checkpoint, got %s", item.State.State)
	}
	if item.Attempt == nil || item.Attempt.State != AttemptActive ||
		item.State.ActiveAttemptID != input.AttemptID || item.Attempt.AttemptID != input.AttemptID {
		return Item{}, ErrAttemptConflict
	}
	allowed := make(map[string]bool, len(item.Contract.AllowedRefs))
	for _, reference := range item.Contract.AllowedRefs {
		allowed[reference] = true
	}
	for _, reference := range input.ArtifactRefs {
		if !allowed[reference] {
			return Item{}, fmt.Errorf("checkpoint artifact reference %q is not allowed by the execution contract", reference)
		}
	}
	checkpointID, err := store.newID("checkpoint")
	if err != nil {
		return Item{}, err
	}
	if err := validateID("checkpoint", checkpointID); err != nil {
		return Item{}, err
	}
	now := store.now()
	checkpoint := Checkpoint{
		SchemaVersion: 1, CheckpointID: checkpointID, ItemID: itemID,
		WorkspaceID: workspaceID, AttemptID: input.AttemptID,
		Summary: strings.TrimSpace(input.Summary), NextStep: strings.TrimSpace(input.NextStep),
		Blocker:      strings.TrimSpace(input.Blocker),
		ArtifactRefs: append([]string(nil), input.ArtifactRefs...), CreatedAt: now,
	}
	if err := validateCheckpoint(checkpoint); err != nil {
		return Item{}, err
	}
	state := item.State
	state.StateRevision++
	state.UpdatedAt = now
	transition := Transition{
		SchemaVersion: 1, ItemID: itemID, WorkspaceID: workspaceID,
		AttemptID: input.AttemptID, State: StateRunning,
		StateRevision: state.StateRevision, OccurredAt: now,
	}
	revision := Revision{
		SchemaVersion: 1, State: state, Attempt: item.Attempt,
		Checkpoint: &checkpoint, Transition: transition,
	}
	if err := validateRevision(revision); err != nil {
		return Item{}, err
	}
	projection := nextProjection(item.Contract, state, &checkpoint)
	if err := validateNextProjection(projection); err != nil {
		return Item{}, err
	}
	if err := store.commitRevision(workspaceID, itemID, revision); err != nil {
		return Item{}, err
	}
	return Item{Contract: item.Contract, State: state, Attempt: item.Attempt, Checkpoint: &checkpoint}, nil
}

func (store Store) Pause(workspaceID, itemID string, expectedRevision int, attemptID string) (Item, error) {
	if err := store.validateItemInput(workspaceID, itemID); err != nil {
		return Item{}, err
	}
	if err := validateID("attempt", attemptID); err != nil {
		return Item{}, err
	}
	unlock, err := store.lock(workspaceID, itemID)
	if err != nil {
		return Item{}, err
	}
	defer unlock()

	item, err := store.inspectUnlocked(workspaceID, itemID)
	if err != nil {
		return Item{}, err
	}
	if item.State.StateRevision != expectedRevision {
		return Item{}, ErrRevisionConflict
	}
	if item.State.State != StateRunning {
		return Item{}, fmt.Errorf("execution item must be running to pause, got %s", item.State.State)
	}
	if item.Attempt == nil || item.Attempt.State != AttemptActive ||
		item.State.ActiveAttemptID != attemptID || item.Attempt.AttemptID != attemptID {
		return Item{}, ErrAttemptConflict
	}
	if item.Checkpoint == nil || item.Checkpoint.AttemptID != attemptID {
		return Item{}, errors.New("current attempt must create a checkpoint before pause")
	}
	now := store.now()
	attempt := *item.Attempt
	attempt.State = AttemptInterrupted
	state := item.State
	state.State = StatePaused
	state.StateRevision++
	state.ActiveAttemptID = ""
	state.UpdatedAt = now
	transition := Transition{
		SchemaVersion: 1, ItemID: itemID, WorkspaceID: workspaceID,
		AttemptID: attemptID, State: StatePaused,
		StateRevision: state.StateRevision, OccurredAt: now,
	}
	revision := Revision{
		SchemaVersion: 1, State: state, Attempt: &attempt,
		Checkpoint: item.Checkpoint, Transition: transition,
	}
	if err := validateRevision(revision); err != nil {
		return Item{}, err
	}
	if err := store.commitRevision(workspaceID, itemID, revision); err != nil {
		return Item{}, err
	}
	return Item{Contract: item.Contract, State: state, Attempt: &attempt, Checkpoint: item.Checkpoint}, nil
}

func (store Store) Resume(workspaceID, itemID string, expectedRevision int) (Item, error) {
	if err := store.validateItemInput(workspaceID, itemID); err != nil {
		return Item{}, err
	}
	unlock, err := store.lock(workspaceID, itemID)
	if err != nil {
		return Item{}, err
	}
	defer unlock()

	item, err := store.inspectUnlocked(workspaceID, itemID)
	if err != nil {
		return Item{}, err
	}
	if item.State.StateRevision != expectedRevision {
		return Item{}, ErrRevisionConflict
	}
	if item.State.State != StatePaused {
		return Item{}, fmt.Errorf("execution item must be paused to resume, got %s", item.State.State)
	}
	if item.Checkpoint == nil || item.Attempt == nil || item.Attempt.State != AttemptInterrupted {
		return Item{}, errors.New("paused execution item is missing its checkpoint or interrupted attempt")
	}
	attemptID, err := store.newID("attempt")
	if err != nil {
		return Item{}, err
	}
	if err := validateID("attempt", attemptID); err != nil {
		return Item{}, err
	}
	now := store.now()
	attempt := Attempt{
		SchemaVersion: 1, AttemptID: attemptID, ItemID: itemID,
		WorkspaceID: workspaceID, State: AttemptActive, StartedAt: now,
	}
	state := item.State
	state.State = StateRunning
	state.StateRevision++
	state.ActiveAttemptID = attemptID
	state.UpdatedAt = now
	transition := Transition{
		SchemaVersion: 1, ItemID: itemID, WorkspaceID: workspaceID,
		AttemptID: attemptID, State: StateRunning,
		StateRevision: state.StateRevision, OccurredAt: now,
	}
	revision := Revision{
		SchemaVersion: 1, State: state, Attempt: &attempt,
		Checkpoint: item.Checkpoint, Transition: transition,
	}
	if err := validateRevision(revision); err != nil {
		return Item{}, err
	}
	if err := store.commitRevision(workspaceID, itemID, revision); err != nil {
		return Item{}, err
	}
	return Item{Contract: item.Contract, State: state, Attempt: &attempt, Checkpoint: item.Checkpoint}, nil
}

func (store Store) Next(workspaceID, itemID string) (NextProjection, error) {
	item, err := store.Inspect(workspaceID, itemID)
	if err != nil {
		return NextProjection{}, err
	}
	projection := nextProjection(item.Contract, item.State, item.Checkpoint)
	if err := validateNextProjection(projection); err != nil {
		return NextProjection{}, err
	}
	return projection, nil
}

func (store Store) NextActive(workspaceID string) (NextProjection, error) {
	active, err := store.activeItemIDs(workspaceID)
	if err != nil {
		return NextProjection{}, err
	}
	if len(active) == 0 {
		return NextProjection{}, errors.New("no active execution item was found")
	}
	if len(active) > 1 {
		return NextProjection{}, ErrActiveItemAmbiguous
	}
	return store.Next(workspaceID, active[0])
}

// ActivePointer reports whether the workspace has one safely resolvable
// running or paused item without exposing its identity or execution content.
func (store Store) ActivePointer(workspaceID string) (ActivePointer, error) {
	active, err := store.activeItemIDs(workspaceID)
	if err != nil {
		return ActivePointer{}, err
	}
	switch len(active) {
	case 0:
		return ActivePointer{State: ActivePointerUnavailable}, nil
	case 1:
		return ActivePointer{Path: ActivePointerPath, Available: true, State: ActivePointerAvailable}, nil
	default:
		return ActivePointer{State: ActivePointerAmbiguous}, nil
	}
}

func (store Store) activeItemIDs(workspaceID string) ([]string, error) {
	if err := validateStoreRoot(store.Root); err != nil {
		return nil, err
	}
	if err := validateID("workspace", workspaceID); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(store.itemsRoot(workspaceID))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	active := make([]string, 0, 1)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if err := validateID("item", entry.Name()); err != nil {
			continue
		}
		item, err := store.Inspect(workspaceID, entry.Name())
		if err != nil {
			return nil, fmt.Errorf("inspect active execution item %s: %w", entry.Name(), err)
		}
		if item.State.State == StateRunning || item.State.State == StatePaused {
			active = append(active, entry.Name())
		}
	}
	sort.Strings(active)
	return active, nil
}

func Receipt(item Item) MutationReceipt {
	receipt := MutationReceipt{
		SchemaVersion: 1,
		ItemID:        item.State.ItemID,
		WorkspaceID:   item.State.WorkspaceID,
		State:         item.State.State,
		StateRevision: item.State.StateRevision,
		AttemptID:     item.State.ActiveAttemptID,
	}
	if item.Checkpoint != nil {
		receipt.CheckpointID = item.Checkpoint.CheckpointID
	}
	return receipt
}

func (store Store) Inspect(workspaceID, itemID string) (Item, error) {
	if err := store.validateItemInput(workspaceID, itemID); err != nil {
		return Item{}, err
	}
	return store.inspectUnlocked(workspaceID, itemID)
}

func (store Store) inspectUnlocked(workspaceID, itemID string) (Item, error) {
	root := store.itemRoot(workspaceID, itemID)
	var contract Contract
	if err := readStrictJSON(filepath.Join(root, "contract.v1.json"), &contract); err != nil {
		return Item{}, err
	}
	if err := validateContract(contract); err != nil {
		return Item{}, err
	}
	revision, err := latestValidRevision(root)
	if err != nil {
		return Item{}, err
	}
	state := revision.State
	if contract.ItemID != itemID || state.ItemID != itemID || contract.WorkspaceID != workspaceID || state.WorkspaceID != workspaceID {
		return Item{}, errors.New("execution item does not match requested workspace and item")
	}
	digest, err := contractDigest(contract)
	if err != nil {
		return Item{}, err
	}
	if digest != state.ContractSHA256 {
		return Item{}, ErrContractChanged
	}
	item := Item{
		Contract: contract, State: state, Attempt: revision.Attempt,
		Checkpoint: revision.Checkpoint, Evidence: revision.Evidence,
	}
	if item.Attempt != nil && (item.Attempt.ItemID != itemID || item.Attempt.WorkspaceID != workspaceID) {
		return Item{}, errors.New("attempt does not match execution item")
	}
	return item, nil
}

func (store Store) Export(workspaceID, itemID string) (Export, error) {
	item, err := store.Inspect(workspaceID, itemID)
	if err != nil {
		return Export{}, err
	}
	transitions, err := store.transitions(workspaceID, itemID)
	if err != nil {
		return Export{}, err
	}
	evidences, err := store.evidences(workspaceID, itemID)
	if err != nil {
		return Export{}, err
	}
	toolCalls, err := store.toolCalls(workspaceID, itemID)
	if err != nil {
		return Export{}, err
	}
	walterReviews, err := store.walterReviews(workspaceID, itemID)
	if err != nil {
		return Export{}, err
	}
	return Export{
		Contract: item.Contract, State: item.State, Attempt: item.Attempt,
		Checkpoint: item.Checkpoint, Evidences: evidences, ToolCalls: toolCalls,
		WalterReviews: walterReviews, Transitions: transitions,
	}, nil
}

func (store Store) Delete(workspaceID, itemID string, expectedRevision int, confirmed bool) error {
	if !confirmed {
		return ErrConfirmationRequired
	}
	if err := store.validateItemInput(workspaceID, itemID); err != nil {
		return err
	}
	unlock, err := store.lock(workspaceID, itemID)
	if err != nil {
		return err
	}
	defer unlock()
	item, err := store.inspectUnlocked(workspaceID, itemID)
	if err != nil {
		return err
	}
	if item.State.StateRevision != expectedRevision {
		return ErrRevisionConflict
	}
	if item.State.State == StateRunning || item.State.State == StateEvaluating {
		return fmt.Errorf("execution item must be paused, ready, cancelled or completed before deletion")
	}
	now := store.now()
	tombstoneState := item.State
	tombstoneState.State = StateCancelled
	tombstoneState.StateRevision++
	tombstoneState.ActiveAttemptID = ""
	tombstoneState.UpdatedAt = now
	tombstoneTransition := Transition{
		SchemaVersion: 1, ItemID: itemID, WorkspaceID: workspaceID,
		State: StateCancelled, StateRevision: tombstoneState.StateRevision,
		OccurredAt: now,
	}
	tombstone := Revision{SchemaVersion: 1, State: tombstoneState, Transition: tombstoneTransition}
	if err := validateRevision(tombstone); err != nil {
		return err
	}
	if err := writeImmutableRevision(filepath.Join(store.itemRoot(workspaceID, itemID), "revisions", revisionName(tombstoneState.StateRevision)), tombstone); err != nil {
		return fmt.Errorf("delete lost execution revision compare-and-swap: %w", err)
	}
	root := store.itemRoot(workspaceID, itemID)
	trashRoot := filepath.Join(store.executionRoot(workspaceID), ".trash")
	if err := os.MkdirAll(trashRoot, 0o700); err != nil {
		return err
	}
	target := filepath.Join(trashRoot, itemID+"-"+hex.EncodeToString(randomBytes(6)))
	if err := durableRename(root, target); err != nil {
		return err
	}
	return os.RemoveAll(target)
}

func (store Store) transitions(workspaceID, itemID string) ([]Transition, error) {
	root := filepath.Join(store.itemRoot(workspaceID, itemID), "revisions")
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(left, right int) bool { return entries[left].Name() < entries[right].Name() })
	transitions := make([]Transition, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		var revision Revision
		if err := readStrictJSON(filepath.Join(root, entry.Name()), &revision); err != nil {
			return nil, err
		}
		if err := validateRevision(revision); err != nil {
			return nil, err
		}
		transition := revision.Transition
		if err := validateTransition(transition); err != nil {
			return nil, err
		}
		if transition.WorkspaceID != workspaceID || transition.ItemID != itemID {
			return nil, errors.New("transition does not match execution item")
		}
		transitions = append(transitions, transition)
	}
	return transitions, nil
}

func (store Store) lock(workspaceID, itemID string) (func(), error) {
	path := filepath.Join(store.itemRoot(workspaceID, itemID), ".mutation.lock")
	token := hex.EncodeToString(randomBytes(12))
	ownerPath := filepath.Join(path, "owner-"+token)
	acquire := func() error {
		if err := os.Mkdir(path, 0o700); err != nil {
			return err
		}
		file, err := os.OpenFile(ownerPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			_ = os.Remove(path)
			return err
		}
		if err := file.Sync(); err != nil {
			_ = file.Close()
			_ = os.Remove(ownerPath)
			_ = os.Remove(path)
			return err
		}
		if err := file.Close(); err != nil {
			_ = os.Remove(ownerPath)
			_ = os.Remove(path)
			return err
		}
		return nil
	}
	err := acquire()
	if errors.Is(err, os.ErrExist) {
		info, statErr := os.Stat(path)
		if statErr != nil || store.now().Sub(info.ModTime()) <= 2*time.Minute {
			return nil, ErrItemBusy
		}
		stalePath := path + ".stale-" + token
		if renameErr := durableRename(path, stalePath); renameErr != nil {
			return nil, ErrItemBusy
		}
		defer os.RemoveAll(stalePath)
		err = acquire()
	}
	if err != nil {
		return nil, err
	}
	return func() {
		if err := os.Remove(ownerPath); err != nil {
			return
		}
		_ = os.Remove(path)
	}, nil
}

func (store Store) validateItemInput(workspaceID, itemID string) error {
	if err := validateStoreRoot(store.Root); err != nil {
		return err
	}
	if err := validateID("workspace", workspaceID); err != nil {
		return err
	}
	return validateID("item", itemID)
}

func (store Store) executionRoot(workspaceID string) string {
	return filepath.Join(store.Root, "workspaces", workspaceID, "execution")
}

func (store Store) itemsRoot(workspaceID string) string {
	return filepath.Join(store.executionRoot(workspaceID), "items")
}

func (store Store) itemRoot(workspaceID, itemID string) string {
	return filepath.Join(store.itemsRoot(workspaceID), itemID)
}

func (store Store) now() time.Time {
	if store.Now != nil {
		return store.Now().UTC()
	}
	return time.Now().UTC()
}

func (store Store) newID(kind string) (string, error) {
	if store.NewID != nil {
		return store.NewID(kind)
	}
	bytes := make([]byte, 12)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return kind + "-" + hex.EncodeToString(bytes), nil
}

func (store Store) fault(point string) error {
	if store.FaultPoint == nil {
		return nil
	}
	return store.FaultPoint(point)
}

func (store Store) commitRevision(workspaceID, itemID string, revision Revision) error {
	if err := store.fault("before_revision_commit"); err != nil {
		return err
	}
	path := filepath.Join(
		store.itemRoot(workspaceID, itemID),
		"revisions",
		revisionName(revision.State.StateRevision),
	)
	if err := writeImmutableRevision(path, revision); err != nil {
		return err
	}
	if err := store.fault("after_revision_commit"); err != nil {
		return err
	}
	if err := writeAtomicJSON(filepath.Join(store.itemRoot(workspaceID, itemID), "state.json"), revision.State); err != nil {
		return err
	}
	return store.fault("after_state_projection")
}

func validateStoreRoot(root string) error {
	if strings.TrimSpace(root) == "" {
		return errors.New("execution store root is required")
	}
	return nil
}

func validateCreateInput(input CreateInput) error {
	if err := validateID("workspace", input.WorkspaceID); err != nil {
		return err
	}
	if err := validateWalterContractSettings(input.RequireWalterReview, input.WalterPublicKey); err != nil {
		return err
	}
	objective := strings.TrimSpace(input.Objective)
	if objective == "" || len(objective) > 4096 {
		return errors.New("execution objective must contain 1 to 4096 bytes")
	}
	next := strings.TrimSpace(input.InitialNextStep)
	if next == "" || len(next) > 2048 {
		return errors.New("initial next step must contain 1 to 2048 bytes")
	}
	if len(input.Criteria) == 0 {
		return errors.New("at least one completion criterion is required")
	}
	allowed := make(map[string]bool, len(input.AllowedRefs))
	for _, reference := range input.AllowedRefs {
		if len(reference) > 512 || !logicalRefPattern.MatchString(reference) {
			return fmt.Errorf("invalid logical reference %q", reference)
		}
		allowed[reference] = true
	}
	seen := make(map[string]bool)
	for _, criterion := range input.Criteria {
		if err := validateID("criterion", criterion.ID); err != nil {
			return err
		}
		if seen[criterion.ID] {
			return fmt.Errorf("duplicate completion criterion %q", criterion.ID)
		}
		seen[criterion.ID] = true
		if criterion.Type != CriterionArtifactSnapshot && criterion.Type != CriterionCommandCheck {
			return fmt.Errorf("unsupported completion criterion type %q", criterion.Type)
		}
		switch criterion.Type {
		case CriterionArtifactSnapshot:
			if criterion.TargetRef == "" || !allowed[criterion.TargetRef] || len(criterion.Command) != 0 {
				return fmt.Errorf("artifact criterion %q requires one allowed target_ref and no command", criterion.ID)
			}
			if _, err := workspaceRelativePath(criterion.TargetRef); err != nil {
				return fmt.Errorf("artifact criterion %q: %w", criterion.ID, err)
			}
		case CriterionCommandCheck:
			if criterion.TargetRef != "" || !allowedCommand(criterion.Command) {
				return fmt.Errorf("command criterion %q contains an unsupported command", criterion.ID)
			}
		}
	}
	return nil
}

func validateCheckpointInput(input CheckpointInput) error {
	if input.ExpectedRevision < 1 {
		return errors.New("checkpoint expected revision must be positive")
	}
	if err := validateID("attempt", input.AttemptID); err != nil {
		return err
	}
	summary := strings.TrimSpace(input.Summary)
	if summary == "" || len(summary) > MaximumCheckpointSummaryBytes {
		return fmt.Errorf("checkpoint summary must contain 1 to %d bytes", MaximumCheckpointSummaryBytes)
	}
	next := strings.TrimSpace(input.NextStep)
	if next == "" || len(next) > MaximumCheckpointNextStepBytes {
		return fmt.Errorf("checkpoint next step must contain 1 to %d bytes", MaximumCheckpointNextStepBytes)
	}
	if len(strings.TrimSpace(input.Blocker)) > MaximumCheckpointBlockerBytes {
		return fmt.Errorf("checkpoint blocker must contain at most %d bytes", MaximumCheckpointBlockerBytes)
	}
	if len(input.ArtifactRefs) > MaximumCheckpointArtifactRefs {
		return fmt.Errorf("checkpoint may contain at most %d artifact references", MaximumCheckpointArtifactRefs)
	}
	seen := make(map[string]bool, len(input.ArtifactRefs))
	for _, reference := range input.ArtifactRefs {
		if len(reference) > 512 || !logicalRefPattern.MatchString(reference) {
			return fmt.Errorf("invalid logical reference %q", reference)
		}
		if seen[reference] {
			return fmt.Errorf("duplicate checkpoint artifact reference %q", reference)
		}
		seen[reference] = true
	}
	return nil
}

func validateContract(contract Contract) error {
	if contract.SchemaVersion != 1 || contract.ContractVersion != 1 || contract.AuthorityKind != AuthorityLocalExecution || contract.CreatedAt.IsZero() {
		return errors.New("invalid execution contract header")
	}
	if contract.ItemID == "" {
		return errors.New("execution contract item ID is required")
	}
	return validateCreateInput(CreateInput{
		WorkspaceID: contract.WorkspaceID, Objective: contract.Objective,
		InitialNextStep: contract.InitialNextStep, Criteria: contract.Criteria,
		AllowedRefs:         contract.AllowedRefs,
		RequireWalterReview: contract.RequireWalterReview,
		WalterPublicKey:     contract.WalterPublicKey,
	})
}

func validateState(state State) error {
	if state.SchemaVersion != 1 || state.ContractVersion != 1 || state.StateRevision < 1 || state.CreatedAt.IsZero() || state.UpdatedAt.IsZero() {
		return errors.New("invalid execution state header")
	}
	if err := validateID("workspace", state.WorkspaceID); err != nil {
		return err
	}
	if err := validateID("item", state.ItemID); err != nil {
		return err
	}
	switch state.State {
	case StateReady, StateRunning, StatePaused, StateEvaluating, StateCompleted, StateCancelled:
	default:
		return fmt.Errorf("invalid execution item state %q", state.State)
	}
	if len(state.ContractSHA256) != 64 {
		return errors.New("invalid execution contract digest")
	}
	if state.ActiveAttemptID != "" {
		if err := validateID("attempt", state.ActiveAttemptID); err != nil {
			return err
		}
	}
	return nil
}

func validateAttempt(attempt Attempt) error {
	if attempt.SchemaVersion != 1 || attempt.StartedAt.IsZero() {
		return errors.New("invalid execution attempt header")
	}
	for kind, id := range map[string]string{"workspace": attempt.WorkspaceID, "item": attempt.ItemID, "attempt": attempt.AttemptID} {
		if err := validateID(kind, id); err != nil {
			return err
		}
	}
	switch attempt.State {
	case AttemptActive, AttemptInterrupted, AttemptCompleted, AttemptCancelled:
	default:
		return fmt.Errorf("invalid execution attempt state %q", attempt.State)
	}
	return nil
}

func validateCheckpoint(checkpoint Checkpoint) error {
	if checkpoint.SchemaVersion != 1 || checkpoint.CreatedAt.IsZero() {
		return errors.New("invalid execution checkpoint header")
	}
	for kind, id := range map[string]string{
		"workspace":  checkpoint.WorkspaceID,
		"item":       checkpoint.ItemID,
		"attempt":    checkpoint.AttemptID,
		"checkpoint": checkpoint.CheckpointID,
	} {
		if err := validateID(kind, id); err != nil {
			return err
		}
	}
	return validateCheckpointInput(CheckpointInput{
		ExpectedRevision: 1,
		AttemptID:        checkpoint.AttemptID,
		Summary:          checkpoint.Summary,
		NextStep:         checkpoint.NextStep,
		Blocker:          checkpoint.Blocker,
		ArtifactRefs:     checkpoint.ArtifactRefs,
	})
}

func validateTransition(transition Transition) error {
	if transition.SchemaVersion != 1 || transition.StateRevision < 1 || transition.OccurredAt.IsZero() {
		return errors.New("invalid execution transition header")
	}
	for kind, id := range map[string]string{"workspace": transition.WorkspaceID, "item": transition.ItemID} {
		if err := validateID(kind, id); err != nil {
			return err
		}
	}
	if transition.AttemptID != "" {
		if err := validateID("attempt", transition.AttemptID); err != nil {
			return err
		}
	}
	switch transition.State {
	case StateReady, StateRunning, StatePaused, StateEvaluating, StateCompleted, StateCancelled:
	default:
		return fmt.Errorf("invalid execution transition state %q", transition.State)
	}
	return nil
}

func validateRevision(revision Revision) error {
	if revision.SchemaVersion != 1 {
		return errors.New("invalid execution revision header")
	}
	if err := validateState(revision.State); err != nil {
		return err
	}
	if err := validateTransition(revision.Transition); err != nil {
		return err
	}
	if revision.State.ItemID != revision.Transition.ItemID ||
		revision.State.WorkspaceID != revision.Transition.WorkspaceID ||
		revision.State.State != revision.Transition.State ||
		revision.State.StateRevision != revision.Transition.StateRevision {
		return errors.New("execution revision state and transition do not match")
	}
	if revision.Checkpoint != nil {
		if err := validateCheckpoint(*revision.Checkpoint); err != nil {
			return err
		}
		if revision.Checkpoint.ItemID != revision.State.ItemID ||
			revision.Checkpoint.WorkspaceID != revision.State.WorkspaceID {
			return errors.New("execution revision checkpoint does not match state")
		}
	}
	if revision.Evidence != nil {
		if err := validateEvidenceReceipt(*revision.Evidence); err != nil {
			return err
		}
		if revision.Evidence.ItemID != revision.State.ItemID ||
			revision.Evidence.WorkspaceID != revision.State.WorkspaceID ||
			revision.Evidence.AttemptID != revision.Transition.AttemptID {
			return errors.New("execution revision evidence does not match state")
		}
	}
	if revision.ToolCall != nil {
		if revision.Evidence != nil {
			return errors.New("execution revision cannot contain both evidence and a tool call")
		}
		if revision.WalterReview != nil {
			return errors.New("execution revision cannot contain multiple mutation receipts")
		}
		if err := validateToolCallReceipt(*revision.ToolCall); err != nil {
			return err
		}
		if revision.ToolCall.ItemID != revision.State.ItemID ||
			revision.ToolCall.WorkspaceID != revision.State.WorkspaceID ||
			revision.ToolCall.AttemptID != revision.Transition.AttemptID {
			return errors.New("execution revision tool call does not match state")
		}
		if revision.State.State != StateRunning {
			return errors.New("execution tool call revision requires a running item")
		}
	}
	if revision.WalterReview != nil {
		if revision.Evidence != nil {
			return errors.New("execution revision cannot contain multiple mutation receipts")
		}
		if err := validateWalterReviewReceipt(*revision.WalterReview); err != nil {
			return err
		}
		if revision.WalterReview.ItemID != revision.State.ItemID ||
			revision.WalterReview.WorkspaceID != revision.State.WorkspaceID ||
			revision.WalterReview.AttemptID != revision.Transition.AttemptID ||
			revision.WalterReview.RecordedRevision != revision.State.StateRevision {
			return errors.New("execution Walter review does not match state")
		}
		if revision.State.State != StateRunning {
			return errors.New("Walter review revision requires a running item")
		}
	}
	if revision.Attempt == nil {
		if revision.State.ActiveAttemptID != "" || revision.Transition.AttemptID != "" {
			return errors.New("execution revision is missing its active attempt")
		}
		return nil
	}
	if err := validateAttempt(*revision.Attempt); err != nil {
		return err
	}
	if revision.Attempt.AttemptID != revision.Transition.AttemptID ||
		revision.Attempt.ItemID != revision.State.ItemID ||
		revision.Attempt.WorkspaceID != revision.State.WorkspaceID {
		return errors.New("execution revision attempt does not match state")
	}
	switch revision.State.State {
	case StateRunning:
		if revision.Attempt.State != AttemptActive || revision.Attempt.AttemptID != revision.State.ActiveAttemptID {
			return errors.New("running execution revision requires its active attempt")
		}
	case StatePaused:
		if revision.Attempt.State != AttemptInterrupted || revision.State.ActiveAttemptID != "" || revision.Checkpoint == nil {
			return errors.New("paused execution revision requires an interrupted attempt and checkpoint")
		}
	case StateCompleted:
		if revision.Attempt.State != AttemptCompleted || revision.State.ActiveAttemptID != "" {
			return errors.New("completed execution revision requires its completed attempt")
		}
	default:
		if revision.State.ActiveAttemptID != "" {
			return errors.New("non-running execution revision cannot retain an active attempt")
		}
	}
	return nil
}

func nextProjection(contract Contract, state State, checkpoint *Checkpoint) NextProjection {
	projection := NextProjection{
		SchemaVersion: 1,
		ItemID:        state.ItemID,
		State:         state.State,
		StateRevision: state.StateRevision,
		AttemptID:     state.ActiveAttemptID,
		NextStep:      contract.InitialNextStep,
	}
	if checkpoint != nil {
		projection.Summary = checkpoint.Summary
		projection.NextStep = checkpoint.NextStep
		projection.Blocker = checkpoint.Blocker
		projection.ArtifactRefs = append([]string(nil), checkpoint.ArtifactRefs...)
		checkpointAt := checkpoint.CreatedAt
		projection.CheckpointAt = &checkpointAt
	}
	return projection
}

func validateNextProjection(projection NextProjection) error {
	if projection.SchemaVersion != 1 || projection.StateRevision < 1 || strings.TrimSpace(projection.NextStep) == "" {
		return errors.New("invalid next-action projection")
	}
	if err := validateID("item", projection.ItemID); err != nil {
		return err
	}
	body, err := json.Marshal(projection)
	if err != nil {
		return err
	}
	if len(body)+1 > MaximumNextProjectionBytes {
		return fmt.Errorf("next-action projection exceeds %d bytes", MaximumNextProjectionBytes)
	}
	return nil
}

func validateID(kind, id string) error {
	if kind == "workspace" && !workspaceIDPattern.MatchString(id) {
		return fmt.Errorf("invalid opaque workspace ID")
	}
	if !idPattern.MatchString(id) {
		return fmt.Errorf("invalid %s ID %q", kind, id)
	}
	return nil
}

func contractDigest(contract Contract) (string, error) {
	body, err := json.Marshal(contract)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:]), nil
}

func revisionName(revision int) string {
	return fmt.Sprintf("%020d.json", revision)
}

func latestValidRevision(itemRoot string) (Revision, error) {
	root := filepath.Join(itemRoot, "revisions")
	entries, err := os.ReadDir(root)
	if err != nil {
		return Revision{}, err
	}
	sort.Slice(entries, func(left, right int) bool { return entries[left].Name() > entries[right].Name() })
	var invalid bool
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		var revision Revision
		if err := readStrictJSON(filepath.Join(root, entry.Name()), &revision); err != nil {
			invalid = true
			continue
		}
		if err := validateRevision(revision); err != nil {
			invalid = true
			continue
		}
		if entry.Name() != revisionName(revision.State.StateRevision) {
			invalid = true
			continue
		}
		return revision, nil
	}
	if invalid {
		return Revision{}, errors.New("execution revisions exist but none is valid")
	}
	return Revision{}, os.ErrNotExist
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
			return errors.New("execution file contains multiple JSON values")
		}
		return err
	}
	return nil
}

func writeNewJSON(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func writeImmutableRevision(path string, value Revision) error {
	directory := filepath.Dir(path)
	info, err := os.Stat(directory)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("execution revision parent is not a directory")
	}
	if _, err := os.Stat(path); err == nil {
		return os.ErrExist
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	file, err := os.CreateTemp(directory, ".revision-")
	if err != nil {
		return err
	}
	temp := file.Name()
	defer os.Remove(temp)
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return err
	}
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return durablePublishNoClobber(temp, path)
}

func writeAtomicJSON(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".state-")
	if err != nil {
		return err
	}
	temp := file.Name()
	defer os.Remove(temp)
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return err
	}
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return durableReplace(temp, path)
}

func randomBytes(size int) []byte {
	value := make([]byte, size)
	if _, err := rand.Read(value); err != nil {
		now := sha256.Sum256([]byte(time.Now().UTC().Format(time.RFC3339Nano)))
		copy(value, now[:])
	}
	return value
}

// ValidateSchemaFile wires the published contract into executable validation.
func ValidateSchemaFile(path string) error {
	var schema map[string]any
	if err := readStrictJSON(path, &schema); err != nil {
		return err
	}
	if schema["$schema"] != "https://json-schema.org/draft/2020-12/schema" {
		return errors.New("execution state schema must use JSON Schema draft 2020-12")
	}
	if schema["$id"] != "urn:bcg-brasil-agentic-os:schema:execution-state:v1" {
		return errors.New("execution state schema has an unexpected identifier")
	}
	definitions, ok := schema["$defs"].(map[string]any)
	if !ok {
		return errors.New("execution state schema is missing definitions")
	}
	for _, name := range []string{"criterion", "evidence"} {
		definition, ok := definitions[name].(map[string]any)
		if !ok {
			return fmt.Errorf("execution state schema is missing %s", name)
		}
		variants, ok := definition["oneOf"].([]any)
		if !ok || len(variants) != 2 {
			return fmt.Errorf("execution state schema %s must have two discriminated variants", name)
		}
	}
	body, err := json.Marshal(schema)
	if err != nil {
		return err
	}
	for _, required := range []string{
		`["go","version"]`, `["go","test","./..."]`, `["go","vet","./..."]`,
		`"tool_sha256"`, `"artifact_snapshot"`, `"command_check"`,
		`"tool_call"`, `"started"`, `"succeeded"`, `"unavailable"`,
		`"walter_review"`, `"require_walter_review"`, `"approved"`, `"rejected"`,
	} {
		if !bytes.Contains(body, []byte(required)) {
			return fmt.Errorf("execution state schema is missing exact contract %s", required)
		}
	}
	return nil
}
