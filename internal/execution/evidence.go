package execution

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type EvidenceOutcome string

const (
	EvidencePassed       EvidenceOutcome = "passed"
	EvidenceFailed       EvidenceOutcome = "failed"
	maximumArtifactBytes                 = 100 << 20
)

var ErrCompletionUnsatisfied = errors.New("execution completion contract is not satisfied")

type EvidenceReceipt struct {
	SchemaVersion  int             `json:"schema_version"`
	EvidenceID     string          `json:"evidence_id"`
	ItemID         string          `json:"item_id"`
	WorkspaceID    string          `json:"workspace_id"`
	AttemptID      string          `json:"attempt_id"`
	CriterionID    string          `json:"criterion_id"`
	Type           CriterionType   `json:"type"`
	Outcome        EvidenceOutcome `json:"outcome"`
	TargetRef      string          `json:"target_ref,omitempty"`
	ArtifactSHA256 string          `json:"artifact_sha256,omitempty"`
	ArtifactBytes  int64           `json:"artifact_bytes,omitempty"`
	CommandSHA256  string          `json:"command_sha256,omitempty"`
	ToolSHA256     string          `json:"tool_sha256,omitempty"`
	ExitCode       *int            `json:"exit_code,omitempty"`
	ObservedAt     time.Time       `json:"observed_at"`
}

type EvidenceInput struct {
	WorkspaceRoot    string
	ExpectedRevision int
	AttemptID        string
	CriterionID      string
}

type CompletionInput struct {
	WorkspaceRoot    string
	ExpectedRevision int
	AttemptID        string
}

type EvidenceResult struct {
	Item    Item
	Receipt EvidenceReceipt
}

type EvidenceReceiptOutput struct {
	SchemaVersion  int             `json:"schema_version"`
	ItemID         string          `json:"item_id"`
	WorkspaceID    string          `json:"workspace_id"`
	State          ItemState       `json:"state"`
	StateRevision  int             `json:"state_revision"`
	AttemptID      string          `json:"attempt_id"`
	EvidenceID     string          `json:"evidence_id"`
	CriterionID    string          `json:"criterion_id"`
	Type           CriterionType   `json:"type"`
	Outcome        EvidenceOutcome `json:"outcome"`
	TargetRef      string          `json:"target_ref,omitempty"`
	ArtifactSHA256 string          `json:"artifact_sha256,omitempty"`
	ArtifactBytes  int64           `json:"artifact_bytes,omitempty"`
	CommandSHA256  string          `json:"command_sha256,omitempty"`
	ToolSHA256     string          `json:"tool_sha256,omitempty"`
	ExitCode       *int            `json:"exit_code,omitempty"`
}

func EvidenceMutationReceipt(result EvidenceResult) EvidenceReceiptOutput {
	return EvidenceReceiptOutput{
		SchemaVersion:  1,
		ItemID:         result.Item.State.ItemID,
		WorkspaceID:    result.Item.State.WorkspaceID,
		State:          result.Item.State.State,
		StateRevision:  result.Item.State.StateRevision,
		AttemptID:      result.Receipt.AttemptID,
		EvidenceID:     result.Receipt.EvidenceID,
		CriterionID:    result.Receipt.CriterionID,
		Type:           result.Receipt.Type,
		Outcome:        result.Receipt.Outcome,
		TargetRef:      result.Receipt.TargetRef,
		ArtifactSHA256: result.Receipt.ArtifactSHA256,
		ArtifactBytes:  result.Receipt.ArtifactBytes,
		CommandSHA256:  result.Receipt.CommandSHA256,
		ToolSHA256:     result.Receipt.ToolSHA256,
		ExitCode:       result.Receipt.ExitCode,
	}
}

func (store Store) CollectEvidence(workspaceID, itemID string, input EvidenceInput) (EvidenceResult, error) {
	if err := store.validateItemInput(workspaceID, itemID); err != nil {
		return EvidenceResult{}, err
	}
	if strings.TrimSpace(input.WorkspaceRoot) == "" || input.ExpectedRevision < 1 {
		return EvidenceResult{}, errors.New("workspace root and expected revision are required")
	}
	if err := validateID("attempt", input.AttemptID); err != nil {
		return EvidenceResult{}, err
	}
	if err := validateID("criterion", input.CriterionID); err != nil {
		return EvidenceResult{}, err
	}
	unlock, err := store.lock(workspaceID, itemID)
	if err != nil {
		return EvidenceResult{}, err
	}
	item, err := store.inspectUnlocked(workspaceID, itemID)
	if err != nil {
		unlock()
		return EvidenceResult{}, err
	}
	if item.State.StateRevision != input.ExpectedRevision {
		unlock()
		return EvidenceResult{}, ErrRevisionConflict
	}
	if item.State.State != StateRunning || item.Attempt == nil || item.Attempt.State != AttemptActive ||
		item.State.ActiveAttemptID != input.AttemptID || item.Attempt.AttemptID != input.AttemptID {
		unlock()
		return EvidenceResult{}, ErrAttemptConflict
	}
	criterion, err := findCriterion(item.Contract, input.CriterionID)
	if err != nil {
		unlock()
		return EvidenceResult{}, err
	}
	evidenceID, err := store.newID("evidence")
	if err != nil {
		unlock()
		return EvidenceResult{}, err
	}
	now := store.now()
	unlock()

	receipt, err := witnessCriterion(input.WorkspaceRoot, item, criterion, evidenceID, now)
	if err != nil {
		return EvidenceResult{}, err
	}
	unlock, err = store.lock(workspaceID, itemID)
	if err != nil {
		return EvidenceResult{}, err
	}
	defer unlock()
	current, err := store.inspectUnlocked(workspaceID, itemID)
	if err != nil {
		return EvidenceResult{}, err
	}
	if current.State.StateRevision != input.ExpectedRevision {
		return EvidenceResult{}, ErrRevisionConflict
	}
	if current.State.State != StateRunning || current.Attempt == nil ||
		current.State.ActiveAttemptID != input.AttemptID || current.Attempt.AttemptID != input.AttemptID {
		return EvidenceResult{}, ErrAttemptConflict
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
		Checkpoint: item.Checkpoint, Evidence: &receipt, Transition: transition,
	}
	if err := validateRevision(revision); err != nil {
		return EvidenceResult{}, err
	}
	if err := store.commitRevision(workspaceID, itemID, revision); err != nil {
		return EvidenceResult{}, err
	}
	resultItem := Item{
		Contract: item.Contract, State: state, Attempt: item.Attempt,
		Checkpoint: item.Checkpoint, Evidence: &receipt,
	}
	return EvidenceResult{Item: resultItem, Receipt: receipt}, nil
}

func (store Store) Complete(workspaceID, itemID string, input CompletionInput) (Item, error) {
	if err := store.validateItemInput(workspaceID, itemID); err != nil {
		return Item{}, err
	}
	unlock, err := store.lock(workspaceID, itemID)
	if err != nil {
		return Item{}, err
	}
	item, err := store.inspectUnlocked(workspaceID, itemID)
	if err != nil {
		unlock()
		return Item{}, err
	}
	if item.State.StateRevision != input.ExpectedRevision {
		unlock()
		return Item{}, ErrRevisionConflict
	}
	if item.State.State != StateRunning || item.Attempt == nil ||
		item.State.ActiveAttemptID != input.AttemptID || item.Attempt.AttemptID != input.AttemptID {
		unlock()
		return Item{}, ErrAttemptConflict
	}
	evidence, err := store.latestEvidence(workspaceID, itemID)
	if err != nil {
		unlock()
		return Item{}, err
	}
	unlock()

	for _, criterion := range item.Contract.Criteria {
		prior, ok := evidence[criterion.ID]
		if !ok || prior.Outcome != EvidencePassed {
			return Item{}, fmt.Errorf("%w: criterion %s has no passing receipt", ErrCompletionUnsatisfied, criterion.ID)
		}
		current, err := witnessCriterion(input.WorkspaceRoot, item, criterion, prior.EvidenceID, store.now())
		if err != nil || current.Outcome != EvidencePassed ||
			current.ArtifactSHA256 != prior.ArtifactSHA256 ||
			current.CommandSHA256 != prior.CommandSHA256 ||
			current.ToolSHA256 != prior.ToolSHA256 {
			return Item{}, fmt.Errorf("%w: criterion %s is no longer valid", ErrCompletionUnsatisfied, criterion.ID)
		}
	}
	unlock, err = store.lock(workspaceID, itemID)
	if err != nil {
		return Item{}, err
	}
	defer unlock()
	current, err := store.inspectUnlocked(workspaceID, itemID)
	if err != nil {
		return Item{}, err
	}
	if current.State.StateRevision != input.ExpectedRevision {
		return Item{}, ErrRevisionConflict
	}
	if current.State.State != StateRunning || current.Attempt == nil ||
		current.State.ActiveAttemptID != input.AttemptID || current.Attempt.AttemptID != input.AttemptID {
		return Item{}, ErrAttemptConflict
	}
	now := store.now()
	attempt := *item.Attempt
	attempt.State = AttemptCompleted
	state := item.State
	state.State = StateCompleted
	state.StateRevision++
	state.ActiveAttemptID = ""
	state.UpdatedAt = now
	transition := Transition{
		SchemaVersion: 1, ItemID: itemID, WorkspaceID: workspaceID,
		AttemptID: input.AttemptID, State: StateCompleted,
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

func witnessCriterion(workspaceRoot string, item Item, criterion Criterion, evidenceID string, now time.Time) (EvidenceReceipt, error) {
	receipt := EvidenceReceipt{
		SchemaVersion: 1, EvidenceID: evidenceID, ItemID: item.State.ItemID,
		WorkspaceID: item.State.WorkspaceID, AttemptID: item.Attempt.AttemptID,
		CriterionID: criterion.ID, Type: criterion.Type, Outcome: EvidencePassed, ObservedAt: now,
	}
	switch criterion.Type {
	case CriterionArtifactSnapshot:
		path, err := resolveWorkspaceArtifact(workspaceRoot, criterion.TargetRef)
		if err != nil {
			return EvidenceReceipt{}, err
		}
		digest, size, err := hashFile(path)
		if err != nil {
			return EvidenceReceipt{}, err
		}
		receipt.TargetRef = criterion.TargetRef
		receipt.ArtifactSHA256 = digest
		receipt.ArtifactBytes = size
	case CriterionCommandCheck:
		digest, err := commandDigest(criterion.Command)
		if err != nil {
			return EvidenceReceipt{}, err
		}
		receipt.CommandSHA256 = digest
		exitCode, toolDigest, err := runValidatedCommand(workspaceRoot, criterion.Command)
		if err != nil {
			return EvidenceReceipt{}, err
		}
		receipt.ExitCode = &exitCode
		receipt.ToolSHA256 = toolDigest
		if exitCode != 0 {
			receipt.Outcome = EvidenceFailed
		}
	default:
		return EvidenceReceipt{}, errors.New("unsupported evidence criterion")
	}
	return receipt, validateEvidenceReceipt(receipt)
}

func (store Store) latestEvidence(workspaceID, itemID string) (map[string]EvidenceReceipt, error) {
	evidences, err := store.evidences(workspaceID, itemID)
	if err != nil {
		return nil, err
	}
	result := make(map[string]EvidenceReceipt)
	for _, evidence := range evidences {
		result[evidence.CriterionID] = evidence
	}
	return result, nil
}

func (store Store) evidences(workspaceID, itemID string) ([]EvidenceReceipt, error) {
	root := filepath.Join(store.itemRoot(workspaceID, itemID), "revisions")
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	result := make([]EvidenceReceipt, 0)
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		var revision Revision
		if err := readStrictJSON(filepath.Join(root, entry.Name()), &revision); err != nil {
			return nil, err
		}
		if err := validateRevision(revision); err != nil || entry.Name() != revisionName(revision.State.StateRevision) {
			return nil, errors.New("execution evidence belongs to an invalid revision")
		}
		if revision.Evidence != nil {
			if err := validateEvidenceReceipt(*revision.Evidence); err != nil {
				return nil, err
			}
			result = append(result, *revision.Evidence)
		}
	}
	return result, nil
}

func findCriterion(contract Contract, criterionID string) (Criterion, error) {
	for _, criterion := range contract.Criteria {
		if criterion.ID == criterionID {
			return criterion, nil
		}
	}
	return Criterion{}, fmt.Errorf("completion criterion %q was not found", criterionID)
}

func validateEvidenceReceipt(receipt EvidenceReceipt) error {
	if receipt.SchemaVersion != 1 || receipt.ObservedAt.IsZero() {
		return errors.New("invalid execution evidence header")
	}
	for kind, id := range map[string]string{
		"workspace": receipt.WorkspaceID, "item": receipt.ItemID,
		"attempt": receipt.AttemptID, "criterion": receipt.CriterionID,
		"evidence": receipt.EvidenceID,
	} {
		if err := validateID(kind, id); err != nil {
			return err
		}
	}
	if receipt.Outcome != EvidencePassed && receipt.Outcome != EvidenceFailed {
		return errors.New("invalid evidence outcome")
	}
	if receipt.Type == CriterionArtifactSnapshot {
		if receipt.TargetRef == "" || len(receipt.ArtifactSHA256) != 64 ||
			receipt.CommandSHA256 != "" || receipt.ToolSHA256 != "" || receipt.ExitCode != nil {
			return errors.New("invalid artifact evidence receipt")
		}
	} else if receipt.Type == CriterionCommandCheck {
		if len(receipt.CommandSHA256) != 64 || len(receipt.ToolSHA256) != 64 ||
			receipt.TargetRef != "" || receipt.ArtifactSHA256 != "" || receipt.ExitCode == nil {
			return errors.New("invalid command evidence receipt")
		}
	} else {
		return errors.New("invalid evidence type")
	}
	return nil
}

func allowedCommand(command []string) bool {
	if len(command) == 2 && command[0] == "go" && command[1] == "version" {
		return true
	}
	if len(command) == 3 && command[0] == "go" &&
		(command[1] == "test" || command[1] == "vet") && command[2] == "./..." {
		return true
	}
	return false
}

func runValidatedCommand(workspaceRoot string, command []string) (int, string, error) {
	if !allowedCommand(command) {
		return 0, "", errors.New("command is not allowed for completion evidence")
	}
	root, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return 0, "", err
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return 0, "", errors.New("workspace root is not a readable directory")
	}
	toolPath, err := trustedGoTool()
	if err != nil {
		return 0, "", err
	}
	toolDigest, _, err := hashFile(toolPath)
	if err != nil {
		return 0, "", err
	}
	// Keep the command timeout below the two-minute stale-lock threshold so a
	// healthy witness cannot be taken over while it still owns the mutation.
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cacheRoot, err := os.MkdirTemp("", "bcgos-evidence-go-")
	if err != nil {
		return 0, "", err
	}
	defer os.RemoveAll(cacheRoot)
	cmd := exec.CommandContext(ctx, toolPath, command[1:]...)
	cmd.Dir = root
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	cmd.Env = []string{
		"CGO_ENABLED=0",
		"GOCACHE=" + filepath.Join(cacheRoot, "build"),
		"GOENV=off",
		"GOFLAGS=",
		"GONOSUMDB=*",
		"GOPROXY=off",
		"GOROOT=" + runtime.GOROOT(),
		"GOSUMDB=off",
		"GOTOOLCHAIN=local",
		"HOME=" + filepath.Join(cacheRoot, "home"),
		"PATH=" + filepath.Dir(toolPath),
		"TEMP=" + cacheRoot,
		"TMP=" + cacheRoot,
		"TMPDIR=" + cacheRoot,
	}
	err = cmd.Run()
	if err == nil {
		return 0, toolDigest, nil
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		return exitError.ExitCode(), toolDigest, nil
	}
	return 0, "", err
}

func trustedGoTool() (string, error) {
	if strings.TrimSpace(os.Getenv("GOROOT")) != "" {
		return "", errors.New("command evidence rejects caller-supplied GOROOT")
	}
	name := "go"
	if runtime.GOOS == "windows" {
		name = "go.exe"
	}
	// runtime.GOROOT is compiled into the running CLI and is not sourced from
	// the caller environment. Do not canonicalize it with EvalSymlinks:
	// setup-go installs Windows toolchains behind directory junctions that are
	// executable by the OS but are not reliably resolvable by EvalSymlinks.
	path := filepath.Clean(filepath.Join(runtime.GOROOT(), "bin", name))
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return "", errors.New("trusted Go tool is unavailable")
	}
	return path, nil
}

func commandDigest(command []string) (string, error) {
	body, err := json.Marshal(command)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:]), nil
}

func workspaceRelativePath(reference string) (string, error) {
	const prefix = "bcgos://workspace/"
	if !strings.HasPrefix(reference, prefix) {
		return "", errors.New("artifact target must be a workspace logical reference")
	}
	relative := filepath.FromSlash(strings.TrimPrefix(reference, prefix))
	clean := filepath.Clean(relative)
	if clean == "." || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", errors.New("artifact target escapes the workspace")
	}
	return clean, nil
}

func resolveWorkspaceArtifact(workspaceRoot, reference string) (string, error) {
	relative, err := workspaceRelativePath(reference)
	if err != nil {
		return "", err
	}
	root, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return "", err
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(filepath.Join(resolvedRoot, relative))
	if err != nil {
		return "", err
	}
	inside, err := filepath.Rel(resolvedRoot, resolved)
	if err != nil || inside == ".." || strings.HasPrefix(inside, ".."+string(filepath.Separator)) {
		return "", errors.New("artifact target escapes the workspace")
	}
	return resolved, nil
}

func hashFile(path string) (string, int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return "", 0, err
	}
	if !info.Mode().IsRegular() || info.Size() > maximumArtifactBytes {
		return "", 0, errors.New("artifact evidence requires a regular file up to 100 MiB")
	}
	hash := sha256.New()
	size, err := io.Copy(hash, io.LimitReader(file, maximumArtifactBytes+1))
	if err != nil || size > maximumArtifactBytes {
		return "", 0, errors.New("artifact evidence exceeds the size limit")
	}
	return hex.EncodeToString(hash.Sum(nil)), size, nil
}
