package execution

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

type ToolCallState string

const (
	ToolCallStarted     ToolCallState = "started"
	ToolCallSucceeded   ToolCallState = "succeeded"
	ToolCallFailed      ToolCallState = "failed"
	ToolCallUnavailable ToolCallState = "unavailable"
)

var canonicalToolCallAgents = map[string]struct{}{
	"claude":       {},
	"codex":        {},
	"maestro-core": {},
}

var canonicalToolCallTools = map[string]struct{}{
	"browser":          {},
	"filesystem-read":  {},
	"filesystem-write": {},
	"git":              {},
	"github":           {},
	"other":            {},
	"shell":            {},
	"skill":            {},
	"subagent":         {},
	"web":              {},
}

// ToolCallReceipt is an intentionally content-free breadcrumb. Agent adapters
// may record lifecycle metadata, but never prompts, arguments, output or errors.
type ToolCallReceipt struct {
	SchemaVersion int           `json:"schema_version"`
	ToolCallID    string        `json:"tool_call_id"`
	ItemID        string        `json:"item_id"`
	WorkspaceID   string        `json:"workspace_id"`
	AttemptID     string        `json:"attempt_id"`
	AgentID       string        `json:"agent_id"`
	ToolID        string        `json:"tool_id"`
	State         ToolCallState `json:"state"`
	StartedAt     time.Time     `json:"started_at"`
	FinishedAt    *time.Time    `json:"finished_at,omitempty"`
}

type ToolCallStartInput struct {
	ExpectedRevision int
	AttemptID        string
	AgentID          string
	ToolID           string
}

type ToolCallFinishInput struct {
	ExpectedRevision int
	AttemptID        string
	ToolCallID       string
	Outcome          ToolCallState
}

type ToolCallResult struct {
	Item    Item
	Receipt ToolCallReceipt
}

type ToolCallMutationReceipt struct {
	SchemaVersion int           `json:"schema_version"`
	ItemID        string        `json:"item_id"`
	WorkspaceID   string        `json:"workspace_id"`
	StateRevision int           `json:"state_revision"`
	AttemptID     string        `json:"attempt_id"`
	ToolCallID    string        `json:"tool_call_id"`
	AgentID       string        `json:"agent_id"`
	ToolID        string        `json:"tool_id"`
	State         ToolCallState `json:"state"`
	StartedAt     time.Time     `json:"started_at"`
	FinishedAt    *time.Time    `json:"finished_at,omitempty"`
}

func ToolCallReceiptOutput(result ToolCallResult) ToolCallMutationReceipt {
	return ToolCallMutationReceipt{
		SchemaVersion: 1,
		ItemID:        result.Receipt.ItemID,
		WorkspaceID:   result.Receipt.WorkspaceID,
		StateRevision: result.Item.State.StateRevision,
		AttemptID:     result.Receipt.AttemptID,
		ToolCallID:    result.Receipt.ToolCallID,
		AgentID:       result.Receipt.AgentID,
		ToolID:        result.Receipt.ToolID,
		State:         result.Receipt.State,
		StartedAt:     result.Receipt.StartedAt,
		FinishedAt:    result.Receipt.FinishedAt,
	}
}

func (store Store) StartToolCall(workspaceID, itemID string, input ToolCallStartInput) (ToolCallResult, error) {
	if err := store.validateItemInput(workspaceID, itemID); err != nil {
		return ToolCallResult{}, err
	}
	for kind, id := range map[string]string{"attempt": input.AttemptID, "agent": input.AgentID, "tool": input.ToolID} {
		if err := validateID(kind, id); err != nil {
			return ToolCallResult{}, err
		}
	}
	if _, ok := canonicalToolCallAgents[input.AgentID]; !ok {
		return ToolCallResult{}, fmt.Errorf("unsupported canonical agent ID %q", input.AgentID)
	}
	if _, ok := canonicalToolCallTools[input.ToolID]; !ok {
		return ToolCallResult{}, fmt.Errorf("unsupported canonical tool ID %q", input.ToolID)
	}
	if input.ExpectedRevision < 1 {
		return ToolCallResult{}, errors.New("tool call expected revision must be positive")
	}
	unlock, err := store.lock(workspaceID, itemID)
	if err != nil {
		return ToolCallResult{}, err
	}
	defer unlock()
	item, err := store.inspectUnlocked(workspaceID, itemID)
	if err != nil {
		return ToolCallResult{}, err
	}
	if item.State.StateRevision != input.ExpectedRevision {
		return ToolCallResult{}, ErrRevisionConflict
	}
	if item.State.State != StateRunning || item.Attempt == nil ||
		item.Attempt.State != AttemptActive || item.State.ActiveAttemptID != input.AttemptID ||
		item.Attempt.AttemptID != input.AttemptID {
		return ToolCallResult{}, ErrAttemptConflict
	}
	callID, err := store.newID("tool-call")
	if err != nil {
		return ToolCallResult{}, err
	}
	if err := validateID("tool-call", callID); err != nil {
		return ToolCallResult{}, err
	}
	now := store.now()
	receipt := ToolCallReceipt{
		SchemaVersion: 1, ToolCallID: callID, ItemID: itemID,
		WorkspaceID: workspaceID, AttemptID: input.AttemptID,
		AgentID: input.AgentID, ToolID: input.ToolID,
		State: ToolCallStarted, StartedAt: now,
	}
	return store.commitToolCall(item, receipt, now)
}

func (store Store) FinishToolCall(workspaceID, itemID string, input ToolCallFinishInput) (ToolCallResult, error) {
	if err := store.validateItemInput(workspaceID, itemID); err != nil {
		return ToolCallResult{}, err
	}
	for kind, id := range map[string]string{"attempt": input.AttemptID, "tool-call": input.ToolCallID} {
		if err := validateID(kind, id); err != nil {
			return ToolCallResult{}, err
		}
	}
	if input.ExpectedRevision < 1 {
		return ToolCallResult{}, errors.New("tool call expected revision must be positive")
	}
	if input.Outcome != ToolCallSucceeded && input.Outcome != ToolCallFailed && input.Outcome != ToolCallUnavailable {
		return ToolCallResult{}, fmt.Errorf("invalid terminal tool call outcome %q", input.Outcome)
	}
	unlock, err := store.lock(workspaceID, itemID)
	if err != nil {
		return ToolCallResult{}, err
	}
	defer unlock()
	item, err := store.inspectUnlocked(workspaceID, itemID)
	if err != nil {
		return ToolCallResult{}, err
	}
	if item.State.StateRevision != input.ExpectedRevision {
		return ToolCallResult{}, ErrRevisionConflict
	}
	if item.State.State != StateRunning || item.Attempt == nil ||
		item.Attempt.State != AttemptActive || item.State.ActiveAttemptID != input.AttemptID ||
		item.Attempt.AttemptID != input.AttemptID {
		return ToolCallResult{}, ErrAttemptConflict
	}
	latest, err := store.latestToolCalls(workspaceID, itemID)
	if err != nil {
		return ToolCallResult{}, err
	}
	receipt, ok := latest[input.ToolCallID]
	if !ok {
		return ToolCallResult{}, fmt.Errorf("tool call %q was not found", input.ToolCallID)
	}
	if receipt.AttemptID != input.AttemptID {
		return ToolCallResult{}, ErrAttemptConflict
	}
	if receipt.State != ToolCallStarted {
		return ToolCallResult{}, errors.New("tool call is already terminal")
	}
	now := store.now()
	receipt.State = input.Outcome
	receipt.FinishedAt = &now
	return store.commitToolCall(item, receipt, now)
}

func (store Store) commitToolCall(item Item, receipt ToolCallReceipt, now time.Time) (ToolCallResult, error) {
	if err := validateToolCallReceipt(receipt); err != nil {
		return ToolCallResult{}, err
	}
	state := item.State
	state.StateRevision++
	state.UpdatedAt = now
	transition := Transition{
		SchemaVersion: 1, ItemID: state.ItemID, WorkspaceID: state.WorkspaceID,
		AttemptID: receipt.AttemptID, State: StateRunning,
		StateRevision: state.StateRevision, OccurredAt: now,
	}
	revision := Revision{
		SchemaVersion: 1, State: state, Attempt: item.Attempt,
		Checkpoint: item.Checkpoint, ToolCall: &receipt, Transition: transition,
	}
	if err := validateRevision(revision); err != nil {
		return ToolCallResult{}, err
	}
	if err := store.commitRevision(state.WorkspaceID, state.ItemID, revision); err != nil {
		return ToolCallResult{}, err
	}
	resultItem := Item{
		Contract: item.Contract, State: state, Attempt: item.Attempt,
		Checkpoint: item.Checkpoint,
	}
	return ToolCallResult{Item: resultItem, Receipt: receipt}, nil
}

func (store Store) latestToolCalls(workspaceID, itemID string) (map[string]ToolCallReceipt, error) {
	calls, err := store.toolCalls(workspaceID, itemID)
	if err != nil {
		return nil, err
	}
	result := make(map[string]ToolCallReceipt)
	for _, call := range calls {
		result[call.ToolCallID] = call
	}
	return result, nil
}

func (store Store) toolCalls(workspaceID, itemID string) ([]ToolCallReceipt, error) {
	root := filepath.Join(store.itemRoot(workspaceID, itemID), "revisions")
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(left, right int) bool { return entries[left].Name() < entries[right].Name() })
	result := make([]ToolCallReceipt, 0)
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		var revision Revision
		if err := readStrictJSON(filepath.Join(root, entry.Name()), &revision); err != nil {
			return nil, err
		}
		if err := validateRevision(revision); err != nil || entry.Name() != revisionName(revision.State.StateRevision) {
			return nil, errors.New("execution tool call belongs to an invalid revision")
		}
		if revision.ToolCall != nil {
			result = append(result, *revision.ToolCall)
		}
	}
	return result, nil
}

func validateToolCallReceipt(receipt ToolCallReceipt) error {
	if receipt.SchemaVersion != 1 || receipt.StartedAt.IsZero() {
		return errors.New("invalid tool call receipt header")
	}
	for kind, id := range map[string]string{
		"workspace": receipt.WorkspaceID, "item": receipt.ItemID,
		"attempt": receipt.AttemptID, "tool-call": receipt.ToolCallID,
		"agent": receipt.AgentID, "tool": receipt.ToolID,
	} {
		if err := validateID(kind, id); err != nil {
			return err
		}
	}
	if _, ok := canonicalToolCallAgents[receipt.AgentID]; !ok {
		return fmt.Errorf("unsupported canonical agent ID %q", receipt.AgentID)
	}
	if _, ok := canonicalToolCallTools[receipt.ToolID]; !ok {
		return fmt.Errorf("unsupported canonical tool ID %q", receipt.ToolID)
	}
	switch receipt.State {
	case ToolCallStarted:
		if receipt.FinishedAt != nil {
			return errors.New("started tool call cannot have a finish time")
		}
	case ToolCallSucceeded, ToolCallFailed, ToolCallUnavailable:
		if receipt.FinishedAt == nil || receipt.FinishedAt.Before(receipt.StartedAt) {
			return errors.New("terminal tool call requires a valid finish time")
		}
	default:
		return fmt.Errorf("invalid tool call state %q", receipt.State)
	}
	return nil
}
