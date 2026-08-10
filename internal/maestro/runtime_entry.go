package maestro

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"time"
)

type DispatchBoundaryReceipt struct {
	SchemaVersion        int    `json:"schema_version"`
	PlanDigest           string `json:"plan_digest"`
	ChainDigest          string `json:"chain_digest"`
	PacketDigest         string `json:"packet_digest"`
	PromptDigest         string `json:"prompt_digest"`
	DraftDigest          string `json:"draft_digest"`
	AccountConsultation  bool   `json:"account_consultation_required"`
	WalterRequired       bool   `json:"walter_required"`
	HistoryCount         int    `json:"history_count"`
	DispatchID           string `json:"dispatch_id"`
	DurableDispatchEpoch uint64 `json:"durable_dispatch_epoch"`
	BindingChainDigest   string `json:"binding_chain_digest"`
	State                Stage  `json:"state"`
	Outcome              string `json:"outcome"`
	OrchestrationStage   Stage  `json:"orchestration_stage,omitempty"`
	AgentEventCount      int    `json:"agent_event_count,omitempty"`
}

type DispatchRecoveryMarker struct {
	SchemaVersion int       `json:"schema_version"`
	PlanDigest    string    `json:"plan_digest"`
	DispatchID    string    `json:"dispatch_id,omitempty"`
	ArtifactKind  string    `json:"artifact_kind"`
	TargetRef     string    `json:"target_ref"`
	Reason        string    `json:"reason"`
	RecordedAt    time.Time `json:"recorded_at"`
}

const (
	RecoveryArtifactChainState      = "chain_state"
	RecoveryArtifactDispatchReceipt = "dispatch_receipt"
)

// DispatchBoundaryState is a metadata-only local CAS record. It proves that
// Maestro prepared and closed one dispatch boundary; it is not native-agent
// authentication and never contains credentials.
type DispatchBoundaryState struct {
	SchemaVersion      int       `json:"schema_version"`
	InstallationDigest string    `json:"installation_digest"`
	DispatchID         string    `json:"dispatch_id"`
	OwnerDigest        string    `json:"owner_digest"`
	SessionDigest      string    `json:"session_digest"`
	ReceiptName        string    `json:"receipt_name"`
	PromptDigest       string    `json:"prompt_digest"`
	PacketDigest       string    `json:"packet_digest"`
	DraftDigest        string    `json:"draft_digest"`
	Epoch              uint64    `json:"epoch"`
	PlanDigest         string    `json:"plan_digest"`
	ChainDigest        string    `json:"chain_digest"`
	BindingChainDigest string    `json:"binding_chain_digest"`
	Status             string    `json:"status"`
	ActiveSpokeCount   int       `json:"active_spoke_count"`
	StartedAt          time.Time `json:"started_at"`
	FinishedAt         time.Time `json:"finished_at"`
	StateDigest        string    `json:"state_digest"`
}

type dispatchCurrentPointer struct {
	SchemaVersion int    `json:"schema_version"`
	Epoch         uint64 `json:"epoch"`
	ReceiptName   string `json:"receipt_name"`
	PointerDigest string `json:"pointer_digest"`
}

type DispatchBoundaryInput struct {
	Root         string
	OwnerID      string
	SessionID    string
	DispatchID   string
	PromptDigest string
	PacketDigest string
	DraftDigest  string
	Plan         Plan
	Chain        ChainState
}

var syncChainDirectoryFunc = syncChainDirectory
var syncDispatchDirectoryFunc = syncChainDirectory

type dispatchBoundaryStateUnknownError struct {
	err error
}

func (err *dispatchBoundaryStateUnknownError) Error() string {
	return err.err.Error()
}

func (err *dispatchBoundaryStateUnknownError) Unwrap() error {
	return err.err
}

// DispatchBoundaryStateUnknown reports that a receipt replacement crossed
// the rename boundary but neither the new nor prepared bytes could be made
// durable. Callers must preserve prompt/chain artifacts for recovery.
func DispatchBoundaryStateUnknown(err error) bool {
	var unknown *dispatchBoundaryStateUnknownError
	return errors.As(err, &unknown)
}

func (input DispatchBoundaryInput) PersistDispatchBoundary() (DispatchBoundaryState, error) {
	if err := input.Plan.Validate(); err != nil {
		return DispatchBoundaryState{}, err
	}
	if input.Chain.PlanDigest != input.Plan.PlanDigest {
		return DispatchBoundaryState{}, errors.New("dispatch boundary chain is not bound to plan")
	}
	if err := validateDispatchIdentity(input.OwnerID, input.SessionID, input.DispatchID); err != nil {
		return DispatchBoundaryState{}, err
	}
	if !validLowerDigest(input.PromptDigest) || !validLowerDigest(input.PacketDigest) || !validLowerDigest(input.DraftDigest) {
		return DispatchBoundaryState{}, errors.New("dispatch occurrence content digests are invalid")
	}
	bindingDigest := orderedBindingDigest(input.Plan)
	directory := filepath.Join(input.Root, "owner", "maestro", "dispatch")
	receiptsDirectory := filepath.Join(directory, "receipts")
	if err := ensurePrivateChainTree(input.Root, directory); err != nil {
		return DispatchBoundaryState{}, err
	}
	if err := ensurePrivateChainTree(input.Root, receiptsDirectory); err != nil {
		return DispatchBoundaryState{}, err
	}
	unlock, err := acquireChainLock(directory)
	if err != nil {
		return DispatchBoundaryState{}, err
	}
	defer func() { _ = unlock() }()
	chainDigestValue := chainDigest(input.Chain)
	ownerDigest := SHA256Hex(input.OwnerID)
	sessionDigest := SHA256Hex(input.SessionID)
	receipts, maxEpoch, err := scanDispatchReceipts(receiptsDirectory)
	if err != nil {
		return DispatchBoundaryState{}, err
	}
	priorPointer, priorPointerBody, err := readDispatchCurrentPointer(directory, receiptsDirectory)
	if err != nil {
		return DispatchBoundaryState{}, err
	}
	var prior *DispatchBoundaryState
	for index := range receipts {
		if receipts[index].DispatchID == input.DispatchID {
			candidate := receipts[index]
			prior = &candidate
			break
		}
	}
	if prior != nil {
		if prior.InstallationDigest != installationDigest(input.Root) || prior.OwnerDigest != ownerDigest || prior.SessionDigest != sessionDigest {
			return DispatchBoundaryState{}, errors.New("dispatch occurrence is bound to another owner or session")
		}
		if prior.PlanDigest != input.Plan.PlanDigest || prior.ChainDigest != chainDigestValue || prior.BindingChainDigest != bindingDigest || prior.PromptDigest != input.PromptDigest || prior.PacketDigest != input.PacketDigest || prior.DraftDigest != input.DraftDigest {
			return DispatchBoundaryState{}, errors.New("dispatch occurrence was reused with different content")
		}
		if prior.Status != "prepared" && prior.Status != "finished" {
			return DispatchBoundaryState{}, errors.New("dispatch occurrence has an invalid lifecycle state")
		}
		if priorPointer == nil || priorPointer.Epoch < prior.Epoch {
			repairedPointer := newDispatchCurrentPointer(*prior)
			if err := persistDispatchCurrentPointer(directory, filepath.Join(directory, "current.json"), repairedPointer, priorPointerBody); err != nil {
				return DispatchBoundaryState{}, fmt.Errorf("dispatch occurrence exists but current pointer recovery failed: %w", err)
			}
		} else if priorPointer.Epoch == prior.Epoch && priorPointer.ReceiptName != prior.ReceiptName {
			return DispatchBoundaryState{}, errors.New("durable dispatch current pointer conflicts with occurrence receipt")
		}
		return *prior, nil
	}
	epoch := uint64(1)
	if maxEpoch > 0 {
		epoch = maxEpoch + 1
	}
	if priorPointer != nil {
		priorState, err := readDispatchReceipt(receiptsDirectory, priorPointer.ReceiptName)
		if err != nil {
			return DispatchBoundaryState{}, err
		}
		if priorState.InstallationDigest != installationDigest(input.Root) || priorState.ActiveSpokeCount != 0 || priorState.Status != "finished" {
			return DispatchBoundaryState{}, errors.New("durable dispatch current receipt is invalid")
		}
	}
	state := newDispatchBoundaryState(input.Root, input.OwnerID, input.SessionID, input.DispatchID, input.PromptDigest, input.PacketDigest, input.DraftDigest, epoch, input.Plan, input.Chain, bindingDigest)
	receiptPath := filepath.Join(receiptsDirectory, state.ReceiptName)
	if err := persistDispatchReceipt(receiptsDirectory, receiptPath, state); err != nil {
		return DispatchBoundaryState{}, err
	}
	pointer := newDispatchCurrentPointer(state)
	if err := persistDispatchCurrentPointer(directory, filepath.Join(directory, "current.json"), pointer, priorPointerBody); err != nil {
		if markerErr := PersistDispatchRecoveryMarker(input.Root, DispatchRecoveryMarker{SchemaVersion: 1, PlanDigest: state.PlanDigest, DispatchID: input.DispatchID, ArtifactKind: RecoveryArtifactDispatchReceipt, TargetRef: receiptPath, Reason: "dispatch occurrence pointer durability failure"}); markerErr != nil {
			return DispatchBoundaryState{}, fmt.Errorf("dispatch pointer durability failed: %w; occurrence recovery marker failed: %v", err, markerErr)
		}
		return DispatchBoundaryState{}, err
	}
	return state, nil
}

// FinalizeDispatchBoundary closes a previously prepared occurrence only after
// the caller has committed the corresponding owner-local prompt/observation
// side effects. A failed replacement restores the prepared receipt, so a
// caller can retry without manufacturing a finished occurrence.
func (input DispatchBoundaryInput) FinalizeDispatchBoundary(prepared DispatchBoundaryState) (DispatchBoundaryState, error) {
	if err := input.Plan.Validate(); err != nil {
		return DispatchBoundaryState{}, err
	}
	if input.Chain.PlanDigest != input.Plan.PlanDigest || prepared.PlanDigest != input.Plan.PlanDigest {
		return DispatchBoundaryState{}, errors.New("dispatch finalization chain is not bound to plan")
	}
	if err := validateDispatchIdentity(input.OwnerID, input.SessionID, input.DispatchID); err != nil {
		return DispatchBoundaryState{}, err
	}
	if !validLowerDigest(input.PromptDigest) || !validLowerDigest(input.PacketDigest) || !validLowerDigest(input.DraftDigest) {
		return DispatchBoundaryState{}, errors.New("dispatch occurrence content digests are invalid")
	}
	directory := filepath.Join(input.Root, "owner", "maestro", "dispatch")
	receiptsDirectory := filepath.Join(directory, "receipts")
	if err := ensurePrivateChainTree(input.Root, directory); err != nil {
		return DispatchBoundaryState{}, err
	}
	if err := ensurePrivateChainTree(input.Root, receiptsDirectory); err != nil {
		return DispatchBoundaryState{}, err
	}
	unlock, err := acquireChainLock(directory)
	if err != nil {
		return DispatchBoundaryState{}, err
	}
	defer func() { _ = unlock() }()
	receipts, _, err := scanDispatchReceipts(receiptsDirectory)
	if err != nil {
		return DispatchBoundaryState{}, err
	}
	current, _, err := readDispatchCurrentPointer(directory, receiptsDirectory)
	if err != nil || current == nil {
		if err == nil {
			err = errors.New("prepared dispatch has no current pointer")
		}
		return DispatchBoundaryState{}, err
	}
	var existing *DispatchBoundaryState
	for index := range receipts {
		if receipts[index].DispatchID == input.DispatchID {
			candidate := receipts[index]
			existing = &candidate
			break
		}
	}
	if existing == nil || existing.Epoch != prepared.Epoch || existing.ReceiptName != prepared.ReceiptName || existing.StateDigest != prepared.StateDigest {
		return DispatchBoundaryState{}, errors.New("prepared dispatch receipt is stale or missing")
	}
	if existing.InstallationDigest != installationDigest(input.Root) || existing.OwnerDigest != SHA256Hex(input.OwnerID) || existing.SessionDigest != SHA256Hex(input.SessionID) || existing.PromptDigest != input.PromptDigest || existing.PacketDigest != input.PacketDigest || existing.DraftDigest != input.DraftDigest || existing.ChainDigest != chainDigest(input.Chain) || existing.BindingChainDigest != orderedBindingDigest(input.Plan) {
		return DispatchBoundaryState{}, errors.New("prepared dispatch receipt content binding is invalid")
	}
	if existing.Status == "finished" {
		return *existing, nil
	}
	if existing.Status != "prepared" || current.Epoch < existing.Epoch || (current.Epoch == existing.Epoch && current.ReceiptName != existing.ReceiptName) {
		return DispatchBoundaryState{}, errors.New("prepared dispatch is not the current fenced occurrence")
	}
	finished := *existing
	finished.Status = "finished"
	finished.FinishedAt = time.Now().UTC()
	finished.StateDigest = dispatchBoundaryStateDigest(finished)
	receiptPath := filepath.Join(receiptsDirectory, finished.ReceiptName)
	previousBody, err := os.ReadFile(receiptPath)
	if err != nil {
		return DispatchBoundaryState{}, err
	}
	if err := replaceDispatchReceipt(receiptsDirectory, receiptPath, finished, previousBody); err != nil {
		if DispatchBoundaryStateUnknown(err) {
			markerErr := PersistDispatchRecoveryMarker(input.Root, DispatchRecoveryMarker{SchemaVersion: 1, PlanDigest: finished.PlanDigest, DispatchID: finished.DispatchID, ArtifactKind: RecoveryArtifactDispatchReceipt, TargetRef: receiptPath, Reason: "dispatch finalization receipt state is unknown"})
			if markerErr != nil {
				return DispatchBoundaryState{}, fmt.Errorf("dispatch finalization state is unknown: %w; recovery marker failed: %v", err, markerErr)
			}
			return DispatchBoundaryState{}, fmt.Errorf("dispatch finalization state is unknown; recovery marker recorded: %w", err)
		}
		return DispatchBoundaryState{}, err
	}
	return finished, nil
}

func validateDispatchIdentity(ownerID, sessionID, dispatchID string) error {
	if ownerID == "" || sessionID == "" || len(ownerID) > 128 || len(sessionID) > 128 || strings.ContainsAny(ownerID+sessionID, `/\\`) {
		return errors.New("dispatch owner or session identity is invalid")
	}
	if !validClosedDispatchID(dispatchID) {
		return errors.New("dispatch occurrence identity is invalid")
	}
	return nil
}

func validClosedDispatchID(value string) bool {
	if len(value) == 64 {
		for _, char := range value {
			if !(char >= '0' && char <= '9' || char >= 'a' && char <= 'f') {
				return false
			}
		}
		return true
	}
	if len(value) < 1 || len(value) > 128 || value == "." || value == ".." {
		return false
	}
	for index, char := range value {
		if !(char >= 'a' && char <= 'z' || char >= '0' && char <= '9' || char == '-' || char == '_' || char == '.') || index == 0 && char == '.' {
			return false
		}
	}
	return true
}

func validLowerDigest(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, char := range value {
		if !(char >= '0' && char <= '9' || char >= 'a' && char <= 'f') {
			return false
		}
	}
	return true
}

func orderedBindingDigest(plan Plan) string {
	type invocationStep struct {
		Phase   string       `json:"phase"`
		Binding AgentBinding `json:"binding"`
	}
	steps := make([]invocationStep, 0, len(plan.Bindings)+2)
	appendBinding := func(phase, role, scopeKind, scopeID string) {
		for _, binding := range plan.Bindings {
			if binding.Role == role && binding.ScopeKind == scopeKind && binding.ScopeID == scopeID {
				steps = append(steps, invocationStep{Phase: phase, Binding: binding})
				return
			}
		}
	}
	if plan.CaseEntry == CaseEntryAccountFirst {
		appendBinding("account_framing", "client_account_agent", "account", plan.AccountScopeID)
	}
	if plan.ScopeKind != "" {
		appendBinding("case_execution", "case_agent", plan.ScopeKind, plan.ScopeID)
	}
	if plan.CaseEntry == CaseEntryAccountFirst && plan.RequiresAccountValidation {
		appendBinding("account_validation", "client_account_agent", "account", plan.AccountScopeID)
	}
	if plan.RequiresWalter {
		appendBinding("walter_review", "reviewer", "review", "review")
	}
	if plan.Action == ActionGamma {
		appendBinding("gamma_quality", "quality_guardian", "workspace", plan.ScopeID)
	}
	if len(steps) == 0 {
		for _, binding := range plan.Bindings {
			steps = append(steps, invocationStep{Phase: "selected", Binding: binding})
		}
	}
	body, _ := json.Marshal(struct {
		PlanDigest string           `json:"plan_digest"`
		Steps      []invocationStep `json:"steps"`
	}{PlanDigest: plan.PlanDigest, Steps: steps})
	return SHA256Hex(string(body))
}

func chainDigest(chain ChainState) string {
	body, _ := json.Marshal(chain)
	return SHA256Hex(string(body))
}

func installationDigest(root string) string {
	abs, _ := filepath.Abs(filepath.Clean(root))
	return SHA256Hex(abs)
}

func newDispatchBoundaryState(root, ownerID, sessionID, dispatchID, promptDigest, packetDigest, draftDigest string, epoch uint64, plan Plan, chain ChainState, bindingDigest string) DispatchBoundaryState {
	now := time.Now().UTC()
	state := DispatchBoundaryState{SchemaVersion: 1, InstallationDigest: installationDigest(root), DispatchID: dispatchID, OwnerDigest: SHA256Hex(ownerID), SessionDigest: SHA256Hex(sessionID), ReceiptName: fmt.Sprintf("%020d-%s.json", epoch, dispatchID), PromptDigest: promptDigest, PacketDigest: packetDigest, DraftDigest: draftDigest, Epoch: epoch, PlanDigest: plan.PlanDigest, ChainDigest: chainDigest(chain), BindingChainDigest: bindingDigest, Status: "prepared", ActiveSpokeCount: 0, StartedAt: now}
	state.StateDigest = dispatchBoundaryStateDigest(state)
	return state
}

func validDispatchBoundaryState(state DispatchBoundaryState) bool {
	statusValid := state.Status == "prepared" || state.Status == "finished"
	finishedAtValid := (state.Status == "prepared" && state.FinishedAt.IsZero()) || (state.Status == "finished" && !state.FinishedAt.IsZero())
	return state.SchemaVersion == 1 && state.Epoch > 0 && validClosedDispatchID(state.DispatchID) && state.ReceiptName == canonicalDispatchReceiptName(state.Epoch, state.DispatchID) && safeReceiptName(state.ReceiptName) && validLowerDigest(state.InstallationDigest) && validLowerDigest(state.OwnerDigest) && validLowerDigest(state.SessionDigest) && validLowerDigest(state.PromptDigest) && validLowerDigest(state.PacketDigest) && validLowerDigest(state.DraftDigest) && validLowerDigest(state.PlanDigest) && validLowerDigest(state.ChainDigest) && validLowerDigest(state.BindingChainDigest) && statusValid && state.ActiveSpokeCount == 0 && !state.StartedAt.IsZero() && finishedAtValid && state.StateDigest == dispatchBoundaryStateDigest(state)
}

func dispatchBoundaryStateDigest(state DispatchBoundaryState) string {
	digestValue := state.StateDigest
	state.StateDigest = ""
	body, _ := json.Marshal(state)
	state.StateDigest = digestValue
	return SHA256Hex(string(body))
}

func newDispatchCurrentPointer(state DispatchBoundaryState) dispatchCurrentPointer {
	pointer := dispatchCurrentPointer{SchemaVersion: 1, Epoch: state.Epoch, ReceiptName: state.ReceiptName}
	pointer.PointerDigest = dispatchCurrentPointerDigest(pointer)
	return pointer
}

func dispatchCurrentPointerDigest(pointer dispatchCurrentPointer) string {
	digest := pointer.PointerDigest
	pointer.PointerDigest = ""
	body, _ := json.Marshal(pointer)
	pointer.PointerDigest = digest
	return SHA256Hex(string(body))
}

func validDispatchCurrentPointer(pointer dispatchCurrentPointer) bool {
	return pointer.SchemaVersion == 1 && pointer.Epoch > 0 && safeReceiptName(pointer.ReceiptName) && pointer.PointerDigest == dispatchCurrentPointerDigest(pointer)
}

func canonicalDispatchReceiptName(epoch uint64, dispatchID string) string {
	return fmt.Sprintf("%020d-%s.json", epoch, dispatchID)
}

func safeReceiptName(name string) bool {
	if len(name) < 27 || filepath.Base(name) != name || !strings.HasSuffix(name, ".json") || name[20] != '-' {
		return false
	}
	for _, char := range name[:20] {
		if char < '0' || char > '9' {
			return false
		}
	}
	return validClosedDispatchID(strings.TrimSuffix(name[21:], ".json"))
}

func readDispatchCurrentPointer(directory, receiptsDirectory string) (*dispatchCurrentPointer, []byte, error) {
	path := filepath.Join(directory, "current.json")
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, nil, errors.New("durable dispatch current pointer is not a regular private file")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	var pointer dispatchCurrentPointer
	if err := json.Unmarshal(body, &pointer); err != nil || !validDispatchCurrentPointer(pointer) {
		return nil, nil, errors.New("durable dispatch current pointer is invalid")
	}
	receipt, err := readDispatchReceipt(receiptsDirectory, pointer.ReceiptName)
	if err != nil {
		return nil, nil, err
	}
	if pointer.Epoch != receipt.Epoch || pointer.ReceiptName != canonicalDispatchReceiptName(receipt.Epoch, receipt.DispatchID) {
		return nil, nil, errors.New("durable dispatch current pointer is not bound to its receipt")
	}
	return &pointer, append([]byte(nil), body...), nil
}

func readDispatchReceipt(directory, name string) (DispatchBoundaryState, error) {
	if !safeReceiptName(name) {
		return DispatchBoundaryState{}, errors.New("durable dispatch receipt name is invalid")
	}
	path := filepath.Join(directory, name)
	info, err := os.Lstat(path)
	if err != nil {
		return DispatchBoundaryState{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return DispatchBoundaryState{}, errors.New("durable dispatch receipt is not a regular private file")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return DispatchBoundaryState{}, err
	}
	var state DispatchBoundaryState
	if json.Unmarshal(body, &state) != nil || !validDispatchBoundaryState(state) || state.ReceiptName != name || name != canonicalDispatchReceiptName(state.Epoch, state.DispatchID) {
		return DispatchBoundaryState{}, errors.New("durable dispatch receipt is invalid")
	}
	return state, nil
}

func scanDispatchReceipts(directory string) ([]DispatchBoundaryState, uint64, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, 0, err
	}
	receipts := make([]DispatchBoundaryState, 0, len(entries))
	var maxEpoch uint64
	seenEpoch := make(map[uint64]struct{}, len(entries))
	seenDispatch := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !safeReceiptName(entry.Name()) {
			return nil, 0, errors.New("durable dispatch receipts directory contains an unexpected entry")
		}
		state, err := readDispatchReceipt(directory, entry.Name())
		if err != nil {
			return nil, 0, err
		}
		if _, exists := seenEpoch[state.Epoch]; exists {
			return nil, 0, errors.New("durable dispatch receipts contain a duplicate epoch")
		}
		if _, exists := seenDispatch[state.DispatchID]; exists {
			return nil, 0, errors.New("durable dispatch receipts contain a duplicate dispatch occurrence")
		}
		seenEpoch[state.Epoch] = struct{}{}
		seenDispatch[state.DispatchID] = struct{}{}
		if state.Epoch > maxEpoch {
			maxEpoch = state.Epoch
		}
		receipts = append(receipts, state)
	}
	return receipts, maxEpoch, nil
}

func persistDispatchReceipt(directory, path string, state DispatchBoundaryState) error {
	body, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return errors.New("durable dispatch receipt target is not a regular private file")
		}
		return errors.New("durable dispatch receipt already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	temporary, err := os.CreateTemp(directory, ".dispatch-receipt-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(append(body, '\n')); err != nil {
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
	if err := os.Rename(temporaryPath, path); err != nil {
		return err
	}
	return syncDispatchDirectoryFunc(directory)
}

func replaceDispatchReceipt(directory, path string, state DispatchBoundaryState, previous []byte) error {
	body, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	if info, err := os.Lstat(path); err != nil {
		return err
	} else if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return errors.New("durable dispatch receipt target is not a regular private file")
	}
	write := func(contents []byte) error {
		temporary, err := os.CreateTemp(directory, ".dispatch-receipt-replace-*")
		if err != nil {
			return err
		}
		temporaryPath := temporary.Name()
		defer os.Remove(temporaryPath)
		if err := temporary.Chmod(0o600); err != nil {
			_ = temporary.Close()
			return err
		}
		if _, err := temporary.Write(append(contents, '\n')); err != nil {
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
		return os.Rename(temporaryPath, path)
	}
	if err := write(body); err != nil {
		return err
	}
	if err := syncDispatchDirectoryFunc(directory); err != nil {
		restoreErr := write(bytesTrimTrailingNewline(previous))
		if restoreErr == nil {
			restoreErr = syncDispatchDirectoryFunc(directory)
		}
		if restoreErr != nil {
			return &dispatchBoundaryStateUnknownError{err: fmt.Errorf("dispatch receipt finalization durability failed and prepared receipt restore failed: %w", restoreErr)}
		}
		return fmt.Errorf("dispatch receipt finalization durability failed; prepared receipt restored: %w", err)
	}
	return nil
}

func bytesTrimTrailingNewline(body []byte) []byte {
	return []byte(strings.TrimSuffix(string(body), "\n"))
}

func persistDispatchCurrentPointer(directory, path string, pointer dispatchCurrentPointer, previous []byte) error {
	body, err := json.MarshalIndent(pointer, "", "  ")
	if err != nil {
		return err
	}
	write := func(contents []byte) error {
		temporary, err := os.CreateTemp(directory, ".dispatch-current-*")
		if err != nil {
			return err
		}
		temporaryPath := temporary.Name()
		defer os.Remove(temporaryPath)
		if err := temporary.Chmod(0o600); err != nil {
			_ = temporary.Close()
			return err
		}
		if _, err := temporary.Write(append(contents, '\n')); err != nil {
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
		return os.Rename(temporaryPath, path)
	}
	if err := write(body); err != nil {
		return err
	}
	if err := syncDispatchDirectoryFunc(directory); err != nil {
		if len(previous) > 0 {
			if restoreErr := write([]byte(strings.TrimSuffix(string(previous), "\n"))); restoreErr != nil {
				return fmt.Errorf("dispatch pointer durability failed and previous pointer restore failed: %w", err)
			} else if restoreErr = syncDispatchDirectoryFunc(directory); restoreErr != nil {
				return fmt.Errorf("dispatch pointer durability failed; previous pointer restore fsync failed: %v", restoreErr)
			}
		} else {
			_ = os.Remove(path)
			_ = syncDispatchDirectoryFunc(directory)
		}
		return fmt.Errorf("dispatch pointer durability failed: %w", err)
	}
	return nil
}

func PersistChainState(root string, state ChainState) (string, error) {
	if !validLowerDigest(state.PlanDigest) {
		return "", errors.New("cannot persist a chain without a plan digest")
	}
	body, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return "", err
	}
	storedBody := append(append([]byte(nil), body...), '\n')
	directory := filepath.Join(root, "owner", "maestro", "chains")
	if err := ensurePrivateChainTree(root, directory); err != nil {
		return "", err
	}
	path := filepath.Join(directory, state.PlanDigest+".json")
	unlock, err := acquireChainLock(directory)
	if err != nil {
		return "", err
	}
	defer func() { _ = unlock() }()
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return "", errors.New("chain state target is not a regular private file")
		}
		current, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		if string(current) == string(storedBody) {
			return path, nil
		}
		var previous ChainState
		if err := json.Unmarshal(current, &previous); err != nil {
			return "", fmt.Errorf("decode existing chain state: %w", err)
		}
		if previous.PlanDigest != state.PlanDigest || !chainStateProgresses(previous, state) {
			return "", errors.New("chain state conflict for plan digest")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	temporary, err := os.CreateTemp(directory, ".chain-*")
	if err != nil {
		return "", err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return "", err
	}
	if _, err := temporary.Write(storedBody); err != nil {
		_ = temporary.Close()
		return "", err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return "", err
	}
	if err := temporary.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return "", err
	}
	if err := syncChainDirectoryFunc(directory); err != nil {
		cleanupErr := os.Remove(path)
		if cleanupErr == nil {
			cleanupErr = syncChainDirectoryFunc(directory)
		}
		if cleanupErr != nil {
			markerErr := PersistDispatchRecoveryMarker(root, DispatchRecoveryMarker{SchemaVersion: 1, PlanDigest: state.PlanDigest, ArtifactKind: RecoveryArtifactChainState, TargetRef: path, Reason: "chain post-rename durability failure"})
			if markerErr != nil {
				return "", fmt.Errorf("chain durability failed: %v; cleanup failed: %v; recovery marker failed: %w", err, cleanupErr, markerErr)
			}
			return "", fmt.Errorf("chain durability failed; recovery marker recorded: %w", err)
		}
		return "", fmt.Errorf("chain durability failed after rename; chain cleanup completed: %w", err)
	}
	return path, nil
}

// chainStateProgresses permits an evidence-bearing retry to replace the
// metadata-only planning state written by an earlier dispatch of the same
// immutable plan. The previous receipt tail must remain an exact prefix; a
// divergent or regressive write is still rejected as a conflict.
func chainStateProgresses(previous, next ChainState) bool {
	if len(next.Receipts) < len(previous.Receipts) {
		return false
	}
	for index := range previous.Receipts {
		if !reflect.DeepEqual(previous.Receipts[index], next.Receipts[index]) {
			return false
		}
	}
	if len(next.Receipts) == len(previous.Receipts) {
		return false
	}
	return true
}

func RemoveChainState(root string, state ChainState) error {
	if !validLowerDigest(state.PlanDigest) {
		return errors.New("cannot remove a chain without a plan digest")
	}
	directory := filepath.Join(root, "owner", "maestro", "chains")
	if err := ensurePrivateChainTree(root, directory); err != nil {
		return err
	}
	unlock, err := acquireChainLock(directory)
	if err != nil {
		return err
	}
	defer func() { _ = unlock() }()
	path := filepath.Join(directory, state.PlanDigest+".json")
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return errors.New("chain state target is not a regular private file")
	}
	if err := os.Remove(path); err != nil {
		return err
	}
	return syncChainDirectoryFunc(directory)
}

func PersistDispatchRecoveryMarker(root string, marker DispatchRecoveryMarker) error {
	if !validLowerDigest(marker.PlanDigest) || marker.Reason == "" || marker.TargetRef == "" {
		return errors.New("dispatch recovery marker is incomplete")
	}
	switch marker.ArtifactKind {
	case RecoveryArtifactChainState:
		if marker.DispatchID != "" {
			return errors.New("chain recovery marker cannot carry a dispatch occurrence")
		}
	case RecoveryArtifactDispatchReceipt:
		if !validClosedDispatchID(marker.DispatchID) {
			return errors.New("dispatch recovery marker has an invalid occurrence")
		}
	default:
		return errors.New("dispatch recovery marker has an invalid artifact kind")
	}
	directory := filepath.Join(root, "owner", "maestro", "recovery")
	if err := ensurePrivateChainTree(root, directory); err != nil {
		return err
	}
	if marker.RecordedAt.IsZero() {
		marker.RecordedAt = time.Now().UTC()
	}
	body, err := json.MarshalIndent(marker, "", "  ")
	if err != nil {
		return err
	}
	markerName := marker.PlanDigest + ".json"
	if marker.ArtifactKind == RecoveryArtifactDispatchReceipt {
		markerName = marker.PlanDigest + "-" + SHA256Hex(marker.ArtifactKind + "\x00" + marker.PlanDigest + "\x00" + marker.DispatchID)[:32] + ".json"
	}
	path := filepath.Join(directory, markerName)
	unlock, err := acquireChainLock(directory)
	if err != nil {
		return err
	}
	defer func() { _ = unlock() }()
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return errors.New("dispatch recovery marker target is not a regular private file")
		}
		existing, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var prior DispatchRecoveryMarker
		if json.Unmarshal(existing, &prior) == nil && prior.SchemaVersion == marker.SchemaVersion && prior.PlanDigest == marker.PlanDigest && prior.DispatchID == marker.DispatchID && prior.ArtifactKind == marker.ArtifactKind && prior.TargetRef == marker.TargetRef && prior.Reason == marker.Reason {
			return nil
		}
		return errors.New("unresolved dispatch recovery marker already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	temporary, err := os.CreateTemp(directory, ".recovery-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(append(body, '\n')); err != nil {
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
	if err := os.Rename(temporaryPath, path); err != nil {
		return err
	}
	return syncChainDirectoryFunc(directory)
}

func ensurePrivateChainTree(root, directory string) error {
	root, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return err
	}
	if err := validatePrivateRootAncestors(root); err != nil {
		return err
	}
	rootInfo, err := os.Lstat(root)
	if err != nil {
		return err
	}
	if rootInfo.Mode()&os.ModeSymlink != 0 || !rootInfo.IsDir() {
		return errors.New("chain state root is not a private directory")
	}
	relative, err := filepath.Rel(root, directory)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return errors.New("chain state directory escapes owner root")
	}
	current := root
	for _, part := range splitRelativePath(relative) {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			if err := os.Mkdir(current, 0o700); err != nil {
				return err
			}
			info, err = os.Lstat(current)
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return errors.New("chain state parent is not a private directory")
		}
	}
	return nil
}

func validatePrivateRootAncestors(root string) error {
	current := filepath.Dir(root)
	for {
		info, err := os.Lstat(current)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			resolved, err := filepath.EvalSymlinks(current)
			if err != nil {
				return err
			}
			approvedDarwinSystemLink := runtime.GOOS == "darwin" && resolved == filepath.Join(string(filepath.Separator), "private", strings.TrimPrefix(current, string(filepath.Separator)))
			if !approvedDarwinSystemLink {
				return errors.New("chain state root has an unapproved symlinked ancestor")
			}
		}
		parent := filepath.Dir(current)
		if parent == current {
			return nil
		}
		current = parent
	}
}

func splitRelativePath(value string) []string {
	if value == "." || value == "" {
		return nil
	}
	parts := []string{}
	for value != "." && value != string(filepath.Separator) {
		parts = append([]string{filepath.Base(value)}, parts...)
		value = filepath.Dir(value)
	}
	return parts
}

func acquireChainLock(directory string) (func() error, error) {
	lockPath := filepath.Join(directory, ".lock")
	deadline := time.Now().Add(2 * time.Second)
	for {
		before, err := os.Lstat(lockPath)
		missing := errors.Is(err, os.ErrNotExist)
		if err != nil && !missing {
			return nil, err
		}
		if !missing && (before.Mode()&os.ModeSymlink != 0 || !before.Mode().IsRegular()) {
			return nil, errors.New("chain state lock is not a private regular file")
		}
		flags := os.O_RDWR
		if missing {
			flags |= os.O_CREATE | os.O_EXCL
		}
		file, err := os.OpenFile(lockPath, flags, 0o600)
		if errors.Is(err, os.ErrExist) || errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err == nil {
			opened, statErr := file.Stat()
			if statErr != nil {
				_ = file.Close()
				return nil, statErr
			}
			after, statErr := os.Lstat(lockPath)
			if statErr != nil || after.Mode()&os.ModeSymlink != 0 || !after.Mode().IsRegular() || !os.SameFile(opened, after) {
				_ = file.Close()
				return nil, errors.New("chain state lock changed during secure open")
			}
			if err := tryLockDispatchFile(file); err == nil {
				return func() error {
					unlockErr := unlockDispatchFile(file)
					closeErr := file.Close()
					if unlockErr != nil {
						return unlockErr
					}
					return closeErr
				}, nil
			} else if !errors.Is(err, errDispatchLockBusy) {
				_ = file.Close()
				return nil, err
			}
			_ = file.Close()
		} else {
			return nil, err
		}
		if time.Now().After(deadline) {
			return nil, errors.New("chain state lock is busy")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func syncChainDirectory(directory string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	file, err := os.Open(directory)
	if err != nil {
		return err
	}
	defer file.Close()
	return file.Sync()
}
