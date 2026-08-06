// Package workspacemigration updates only the managed Maestro surface inside
// an existing workspace. It is intentionally separate from external import
// and from managed-core activation.
package workspacemigration

import (
	"bytes"
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

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/adaptercfg"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/runtimeprojection"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
)

const (
	SchemaVersion           = 1
	ExpectedWorkspaceSchema = 1
	MaxSnapshotFiles        = 128
	MaxSnapshotFileSize     = 512 << 10
	MaxSnapshotBytes        = 4 << 20
	StableBootstrapper      = "stable-bootstrapper"
)

var versionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
var digestPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)
var planIDPattern = regexp.MustCompile(`^[a-f0-9]{32}$`)

type State string

const (
	StateValid       State = "valid"
	StateLegacy      State = "legacy"
	StateIncomplete  State = "incomplete"
	StateInvalid     State = "invalid"
	StateUnavailable State = "unavailable"
)

// PlanOptions describes the exact target that the already-activated core is
// expected to provide. It contains no external-import behavior.
type PlanOptions struct {
	WorkspacePath    string
	DataRoot         string
	Runtime          string
	Executable       string
	TargetRelease    string
	TargetBundle     string
	CapabilityTracks []string
}

type Inspection struct {
	State                   State  `json:"state"`
	WorkspacePath           string `json:"workspace_path"`
	WorkspaceID             string `json:"workspace_id,omitempty"`
	WorkspaceSchemaVersion  int    `json:"workspace_schema_version"`
	Runtime                 string `json:"runtime"`
	ProjectionState         string `json:"projection_state"`
	AdapterState            string `json:"adapter_state"`
	ProjectionSchemaVersion int    `json:"projection_schema_version"`
	SourceDigest            string `json:"source_digest"`
	Reason                  string `json:"reason,omitempty"`
}

// Plan is immutable after it is staged. State transitions are represented by
// Confirmation, Execution and Receipt files, never by rewriting Plan.
type Plan struct {
	SchemaVersion            int      `json:"schema_version"`
	ID                       string   `json:"id"`
	State                    string   `json:"state"`
	Execution                string   `json:"execution"`
	Reason                   string   `json:"reason,omitempty"`
	WorkspacePath            string   `json:"workspace_path"`
	WorkspaceID              string   `json:"workspace_id"`
	Runtime                  string   `json:"runtime"`
	SourceState              State    `json:"source_state"`
	SourceWorkspaceSchema    int      `json:"source_workspace_schema"`
	SourceProjectionState    string   `json:"source_projection_state"`
	SourceAdapterState       string   `json:"source_adapter_state"`
	SourceDigest             string   `json:"source_digest"`
	ExpectedWorkspaceSchema  int      `json:"expected_workspace_schema"`
	ExpectedProjectionSchema int      `json:"expected_projection_schema"`
	ExpectedRelease          string   `json:"expected_release"`
	ExpectedBundle           string   `json:"expected_bundle"`
	Executable               string   `json:"executable"`
	CapabilityTracks         []string `json:"capability_tracks,omitempty"`
	ManagedPaths             []string `json:"managed_paths"`
	ConfirmationRequired     bool     `json:"confirmation_required"`
	SnapshotMaxFiles         int      `json:"snapshot_max_files"`
	SnapshotMaxFileSize      int64    `json:"snapshot_max_file_size"`
	SnapshotMaxBytes         int64    `json:"snapshot_max_bytes"`
}

type CoreActivation struct {
	Authority     string `json:"authority"`
	Activated     bool   `json:"activated"`
	Release       string `json:"release"`
	BundleVersion string `json:"bundle_version"`
	ManagedRoot   string `json:"managed_root"`
	StateDigest   string `json:"state_digest"`
}

type Confirmation struct {
	SchemaVersion   int            `json:"schema_version"`
	PlanID          string         `json:"plan_id"`
	Core            CoreActivation `json:"core"`
	SnapshotPath    string         `json:"snapshot_path"`
	WorkspaceDigest string         `json:"workspace_digest"`
	ConfirmedAt     time.Time      `json:"confirmed_at"`
}

type Receipt struct {
	SchemaVersion int       `json:"schema_version"`
	PlanID        string    `json:"plan_id"`
	State         string    `json:"state"`
	Restored      bool      `json:"restored"`
	Reason        string    `json:"reason,omitempty"`
	CompletedAt   time.Time `json:"completed_at"`
}

type snapshot struct {
	SchemaVersion   int             `json:"schema_version"`
	PlanID          string          `json:"plan_id"`
	WorkspacePath   string          `json:"workspace_path"`
	Runtime         string          `json:"runtime"`
	WorkspaceDigest string          `json:"workspace_digest"`
	TotalBytes      int64           `json:"total_bytes"`
	Entries         []snapshotEntry `json:"entries"`
}

type snapshotEntry struct {
	Path   string `json:"path"`
	Exists bool   `json:"exists"`
	Mode   uint32 `json:"mode,omitempty"`
	Digest string `json:"digest,omitempty"`
	Body   []byte `json:"body,omitempty"`
}

type execution struct {
	SchemaVersion int       `json:"schema_version"`
	PlanID        string    `json:"plan_id"`
	State         string    `json:"state"`
	SnapshotPath  string    `json:"snapshot_path"`
	StartedAt     time.Time `json:"started_at"`
}

// Status is the read-only CLI/status surface. Execution is deliberately
// unavailable unless the caller can provide bootstrapper authority.
type Status struct {
	SchemaVersion int    `json:"schema_version"`
	Capability    string `json:"capability"`
	State         string `json:"state"`
	Execution     string `json:"execution"`
	Reason        string `json:"reason"`
}

var installAdapter = adaptercfg.Install
var installProjection = runtimeprojection.InstallForTracks
var removeExecutionMarkerFunc = removeExecutionMarker

var errExecutionUnavailable = errors.New("workspace migration execution is unavailable until stable-bootstrapper authority wiring is active")

func CapabilityStatus() Status {
	return Status{SchemaVersion: SchemaVersion, Capability: "workspace_migration", State: "pending_core_activation", Execution: "unavailable", Reason: "post-bootstrap workspace target and authenticated core-activation authority are not wired into bcgos update"}
}

func Inspect(options PlanOptions) (Inspection, error) {
	path, err := normalizePath(options.WorkspacePath)
	if err != nil {
		return Inspection{}, err
	}
	if options.DataRoot == "" {
		return Inspection{}, errors.New("workspace migration data root is required")
	}
	if options.Runtime != "claude" && options.Runtime != "codex" {
		return Inspection{}, errors.New("workspace migration runtime must be claude or codex")
	}
	workspaceInspection, err := workspace.Inspect(path, options.DataRoot)
	if err != nil {
		return Inspection{}, err
	}
	adapter, adapterErr := adaptercfg.Inspect(options.Runtime, path)
	projection, projectionErr := runtimeprojection.Inspect(options.Runtime, path)
	result := Inspection{
		WorkspacePath: path, WorkspaceID: workspaceInspection.WorkspaceID,
		WorkspaceSchemaVersion: ExpectedWorkspaceSchema, Runtime: options.Runtime,
		ProjectionSchemaVersion: runtimeprojection.SchemaVersion,
		ProjectionState:         "invalid", AdapterState: "invalid",
	}
	if adapterErr == nil {
		result.AdapterState = adapter.State
	}
	if projectionErr == nil {
		result.ProjectionState = projection.State
	}
	result.SourceDigest, err = sourceDigest(options)
	if err != nil {
		return Inspection{}, err
	}
	switch {
	case workspaceInspection.State == "ready" || workspaceInspection.State == "warning":
		result.State = StateValid
	case workspaceInspection.State == "incomplete":
		result.State = StateIncomplete
	case workspaceInspection.State == "invalid":
		result.State = StateInvalid
	default:
		if hasManagedMarker(path) || result.AdapterState == "installed" || result.ProjectionState != "absent" {
			result.State = StateLegacy
		} else {
			result.State = StateIncomplete
		}
	}
	if adapterErr != nil {
		result.Reason = adapterErr.Error()
	} else if projectionErr != nil {
		result.Reason = projectionErr.Error()
	} else if result.State != StateValid {
		result.Reason = fmt.Sprintf("workspace is %s and cannot be upgraded implicitly", result.State)
	}
	return result, nil
}

func BuildPlan(options PlanOptions) (Plan, error) {
	options.CapabilityTracks = sortedUnique(options.CapabilityTracks)
	if !versionPattern.MatchString(options.TargetRelease) || !versionPattern.MatchString(options.TargetBundle) {
		return Plan{}, errors.New("workspace migration target release and bundle must be canonical versions")
	}
	if strings.TrimSpace(options.Executable) == "" {
		return Plan{}, errors.New("workspace migration executable is required")
	}
	inspection, err := Inspect(options)
	if err != nil {
		return Plan{}, err
	}
	plan := Plan{
		SchemaVersion: SchemaVersion, State: "available", Execution: "available",
		WorkspacePath: inspection.WorkspacePath, WorkspaceID: inspection.WorkspaceID,
		Runtime: options.Runtime, SourceState: inspection.State,
		SourceWorkspaceSchema: inspection.WorkspaceSchemaVersion,
		SourceProjectionState: inspection.ProjectionState,
		SourceAdapterState:    inspection.AdapterState, SourceDigest: inspection.SourceDigest,
		ExpectedWorkspaceSchema:  ExpectedWorkspaceSchema,
		ExpectedProjectionSchema: runtimeprojection.SchemaVersion,
		ExpectedRelease:          options.TargetRelease, ExpectedBundle: options.TargetBundle,
		Executable: options.Executable, CapabilityTracks: options.CapabilityTracks,
		ConfirmationRequired: true, SnapshotMaxFiles: MaxSnapshotFiles,
		SnapshotMaxFileSize: MaxSnapshotFileSize, SnapshotMaxBytes: MaxSnapshotBytes,
	}
	if inspection.State != StateValid {
		plan.Execution, plan.Reason = "unavailable", fmt.Sprintf("workspace is %s; migration fails closed", inspection.State)
	} else if err := validatePreflight(plan); err != nil {
		plan.Execution, plan.Reason = "unavailable", err.Error()
	}
	managed, err := managedPaths(options)
	if err != nil {
		return Plan{}, err
	}
	plan.ManagedPaths = managed
	plan.ID, err = planID(plan)
	if err != nil {
		return Plan{}, err
	}
	return plan, ValidatePlan(plan)
}

func StagePlan(dataRoot string, plan Plan) error {
	if err := ValidatePlan(plan); err != nil {
		return err
	}
	return writeJSON(planPath(dataRoot, plan.ID), plan, 0o600)
}

// Confirm is intentionally unavailable until the stable bootstrapper supplies
// a trusted activation verifier. CoreActivation is a wire shape only; its
// caller-provided fields cannot establish authentication.
func Confirm(dataRoot, id string, core CoreActivation, now time.Time) (Confirmation, error) {
	return Confirmation{}, errExecutionUnavailable
}

func confirmInternal(dataRoot, id string, core CoreActivation, now time.Time) (Confirmation, error) {
	plan, err := loadPlan(dataRoot, id)
	if err != nil {
		return Confirmation{}, err
	}
	if plan.Execution != "available" {
		return Confirmation{}, fmt.Errorf("workspace migration execution is %s: %s", plan.Execution, plan.Reason)
	}
	if err := validateCore(plan, core); err != nil {
		return Confirmation{}, err
	}
	current, err := Inspect(PlanOptions{WorkspacePath: plan.WorkspacePath, DataRoot: dataRoot, Runtime: plan.Runtime, Executable: plan.Executable, CapabilityTracks: plan.CapabilityTracks})
	if err != nil {
		return Confirmation{}, err
	}
	if current.SourceDigest != plan.SourceDigest || current.WorkspaceID != plan.WorkspaceID || current.State != StateValid {
		return Confirmation{}, errors.New("workspace migration plan is stale or workspace is no longer valid")
	}
	preflightErr := validatePreflight(plan)
	if preflightErr != nil {
		return Confirmation{}, preflightErr
	}
	entries, total, err := makeSnapshotEntries(plan)
	if err != nil {
		return Confirmation{}, err
	}
	root := planRoot(dataRoot, plan.ID)
	snapshotPath := filepath.Join(root, "snapshot.json")
	if err := writeJSON(snapshotPath, snapshot{SchemaVersion: SchemaVersion, PlanID: plan.ID, WorkspacePath: plan.WorkspacePath, Runtime: plan.Runtime, WorkspaceDigest: current.SourceDigest, TotalBytes: total, Entries: entries}, 0o600); err != nil {
		return Confirmation{}, err
	}
	confirmation := Confirmation{SchemaVersion: SchemaVersion, PlanID: plan.ID, Core: core, SnapshotPath: snapshotPath, WorkspaceDigest: current.SourceDigest, ConfirmedAt: now.UTC()}
	if confirmation.ConfirmedAt.IsZero() || confirmation.ConfirmedAt.Location() != time.UTC {
		return Confirmation{}, errors.New("workspace migration confirmation time must be UTC")
	}
	if err := writeJSON(filepath.Join(root, "confirmation.json"), confirmation, 0o600); err != nil {
		return Confirmation{}, err
	}
	return confirmation, nil
}

// Apply is intentionally unavailable until the stable bootstrapper supplies
// a trusted activation verifier and safe managed-target primitives.
func Apply(dataRoot, id string, core CoreActivation, now time.Time) (Receipt, error) {
	return Receipt{}, errExecutionUnavailable
}

func applyInternal(dataRoot, id string, core CoreActivation, now time.Time) (Receipt, error) {
	plan, err := loadPlan(dataRoot, id)
	if err != nil {
		return Receipt{}, err
	}
	root := planRoot(dataRoot, id)
	if receipt, receiptErr := readReceipt(filepath.Join(root, "receipt.json"), plan.ID); receiptErr == nil {
		if receipt.State == "applied" {
			if err := removeExecutionMarkerFunc(root); err != nil {
				return receipt, err
			}
			return receipt, nil
		}
	} else if !errors.Is(receiptErr, os.ErrNotExist) {
		return Receipt{}, receiptErr
	}
	if existing, readErr := readExecution(filepath.Join(root, "execution.json")); readErr == nil && existing.State == "applying" {
		recovered, recoverErr := recoverInternal(dataRoot, id, now)
		if recoverErr != nil {
			return Receipt{}, recoverErr
		}
		if recovered.State == "applied" {
			return recovered, nil
		}
	} else if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return Receipt{}, readErr
	}
	confirmation, err := readConfirmation(filepath.Join(root, "confirmation.json"))
	if err != nil {
		return Receipt{}, err
	}
	if confirmation.PlanID != plan.ID || confirmation.SnapshotPath != filepath.Join(root, "snapshot.json") || confirmation.WorkspaceDigest != plan.SourceDigest {
		return Receipt{}, errors.New("workspace migration confirmation does not match its plan")
	}
	if err := validateCore(plan, core); err != nil {
		return Receipt{}, err
	}
	current, err := Inspect(PlanOptions{WorkspacePath: plan.WorkspacePath, DataRoot: dataRoot, Runtime: plan.Runtime, Executable: plan.Executable, CapabilityTracks: plan.CapabilityTracks})
	if err != nil {
		return Receipt{}, err
	}
	if current.SourceDigest != confirmation.WorkspaceDigest {
		return Receipt{}, errors.New("workspace changed after migration confirmation; no files were changed")
	}
	if err := validatePreflight(plan); err != nil {
		return Receipt{}, err
	}
	if _, err := readSnapshotForPlan(confirmation.SnapshotPath, plan); err != nil {
		return Receipt{}, err
	}
	if err := writeJSON(filepath.Join(root, "execution.json"), execution{SchemaVersion: SchemaVersion, PlanID: plan.ID, State: "applying", SnapshotPath: confirmation.SnapshotPath, StartedAt: now.UTC()}, 0o600); err != nil {
		return Receipt{}, err
	}
	fail := func(cause error) (Receipt, error) {
		restoreErr := restoreSnapshotForPlan(confirmation.SnapshotPath, plan)
		receipt := Receipt{SchemaVersion: SchemaVersion, PlanID: plan.ID, State: "rolled_back", Restored: restoreErr == nil, Reason: cause.Error(), CompletedAt: now.UTC()}
		if restoreErr != nil {
			receipt.Reason += "; snapshot restore failed: " + restoreErr.Error()
		}
		_ = writeJSON(filepath.Join(root, "receipt.json"), receipt, 0o600)
		_ = os.Remove(filepath.Join(root, "execution.json"))
		if restoreErr != nil {
			return receipt, errors.Join(cause, restoreErr)
		}
		return receipt, cause
	}
	if _, err := installAdapter(plan.Runtime, plan.WorkspacePath, plan.Executable); err != nil {
		return fail(fmt.Errorf("install managed adapter: %w", err))
	}
	if _, err := installProjection(plan.Runtime, plan.WorkspacePath, plan.CapabilityTracks); err != nil {
		return fail(fmt.Errorf("install managed runtime projection: %w", err))
	}
	if err := validateReadiness(plan, dataRoot); err != nil {
		return fail(err)
	}
	receipt := Receipt{SchemaVersion: SchemaVersion, PlanID: plan.ID, State: "applied", Restored: false, CompletedAt: now.UTC()}
	if err := writeJSON(filepath.Join(root, "receipt.json"), receipt, 0o600); err != nil {
		return fail(fmt.Errorf("write workspace migration receipt: %w", err))
	}
	if err := removeExecutionMarkerFunc(root); err != nil {
		return receipt, fmt.Errorf("workspace migration applied but terminalization failed: %w", err)
	}
	return receipt, nil
}

// Recover is intentionally unavailable on the public surface until the
// bootstrapper owns the safe restore authority.
func Recover(dataRoot, id string, now time.Time) (Receipt, error) {
	return Receipt{}, errExecutionUnavailable
}

func recoverInternal(dataRoot, id string, now time.Time) (Receipt, error) {
	if !planIDPattern.MatchString(id) {
		return Receipt{}, errors.New("workspace migration plan ID is invalid")
	}
	plan, err := loadPlan(dataRoot, id)
	if err != nil {
		return Receipt{}, err
	}
	root := planRoot(dataRoot, id)
	if receipt, receiptErr := readReceipt(filepath.Join(root, "receipt.json"), id); receiptErr == nil {
		if err := removeExecutionMarkerFunc(root); err != nil {
			return receipt, err
		}
		return receipt, nil
	} else if !errors.Is(receiptErr, os.ErrNotExist) {
		return Receipt{}, receiptErr
	}
	execution, err := readExecution(filepath.Join(root, "execution.json"))
	if errors.Is(err, os.ErrNotExist) {
		return Receipt{SchemaVersion: SchemaVersion, PlanID: id, State: "no_interrupted_execution", CompletedAt: now.UTC()}, nil
	}
	if err != nil {
		return Receipt{}, err
	}
	if execution.PlanID != plan.ID || execution.SnapshotPath != filepath.Join(root, "snapshot.json") || execution.State != "applying" {
		return Receipt{}, errors.New("workspace migration execution marker is invalid")
	}
	if err := restoreSnapshotForPlan(execution.SnapshotPath, plan); err != nil {
		return Receipt{}, fmt.Errorf("restore interrupted workspace migration: %w", err)
	}
	receipt := Receipt{SchemaVersion: SchemaVersion, PlanID: id, State: "rolled_back", Restored: true, Reason: "interrupted execution restored from bounded snapshot", CompletedAt: now.UTC()}
	if err := writeJSON(filepath.Join(root, "receipt.json"), receipt, 0o600); err != nil {
		return Receipt{}, err
	}
	if err := removeExecutionMarkerFunc(root); err != nil {
		return receipt, fmt.Errorf("workspace migration rollback completed but terminalization failed: %w", err)
	}
	return receipt, nil
}

func ValidatePlan(plan Plan) error {
	if plan.SchemaVersion != SchemaVersion || !planIDPattern.MatchString(plan.ID) || plan.State != "available" || plan.Runtime != "claude" && plan.Runtime != "codex" || plan.WorkspacePath == "" || plan.WorkspaceID == "" || plan.ExpectedWorkspaceSchema != ExpectedWorkspaceSchema || plan.ExpectedProjectionSchema != runtimeprojection.SchemaVersion || !versionPattern.MatchString(plan.ExpectedRelease) || !versionPattern.MatchString(plan.ExpectedBundle) || !digestPattern.MatchString(plan.SourceDigest) || len(plan.ManagedPaths) == 0 || len(plan.ManagedPaths) > MaxSnapshotFiles || !plan.ConfirmationRequired || plan.SnapshotMaxFiles != MaxSnapshotFiles || plan.SnapshotMaxFileSize != MaxSnapshotFileSize || plan.SnapshotMaxBytes != MaxSnapshotBytes {
		return errors.New("workspace migration plan contract is invalid")
	}
	seenPaths := make(map[string]bool, len(plan.ManagedPaths))
	for _, path := range plan.ManagedPaths {
		if filepath.Clean(path) != path || !filepath.IsAbs(path) || seenPaths[path] {
			return errors.New("workspace migration plan managed paths are invalid")
		}
		seenPaths[path] = true
		if _, err := managedPathSafetyRoot(plan.Runtime, plan.WorkspacePath, path); err != nil {
			return err
		}
	}
	expected, err := planID(plan)
	if err != nil {
		return err
	}
	if expected != plan.ID {
		return errors.New("workspace migration plan ID does not match its immutable fields")
	}
	return nil
}

func validateCore(plan Plan, core CoreActivation) error {
	if core.Authority != StableBootstrapper || !core.Activated || core.Release != plan.ExpectedRelease || core.BundleVersion != plan.ExpectedBundle || core.ManagedRoot == "" || !digestPattern.MatchString(core.StateDigest) {
		return errors.New("workspace migration requires active core evidence from the stable bootstrapper")
	}
	return nil
}

func validatePreflight(plan Plan) error {
	if err := adaptercfg.ValidateInstall(plan.Runtime, plan.WorkspacePath, plan.Executable); err != nil {
		return fmt.Errorf("managed adapter preflight failed: %w", err)
	}
	if err := runtimeprojection.ValidateInstallForTracks(plan.Runtime, plan.WorkspacePath, plan.CapabilityTracks); err != nil {
		return fmt.Errorf("managed runtime projection preflight failed: %w", err)
	}
	return nil
}

func validateReadiness(plan Plan, dataRoot string) error {
	inspection, err := Inspect(PlanOptions{WorkspacePath: plan.WorkspacePath, DataRoot: dataRoot, Runtime: plan.Runtime, Executable: plan.Executable, CapabilityTracks: plan.CapabilityTracks})
	if err != nil {
		return err
	}
	if inspection.State != StateValid || inspection.AdapterState != "installed" || inspection.ProjectionState != "installed" {
		return fmt.Errorf("workspace migration readiness is incomplete: workspace=%s adapter=%s projection=%s", inspection.State, inspection.AdapterState, inspection.ProjectionState)
	}
	if _, _, _, err := runtimeprojection.RoutingInputs(plan.Runtime, plan.WorkspacePath); err != nil {
		return fmt.Errorf("workspace migration routing readiness failed: %w", err)
	}
	return nil
}

func makeSnapshotEntries(plan Plan) ([]snapshotEntry, int64, error) {
	paths := append([]string(nil), plan.ManagedPaths...)
	if len(paths) > MaxSnapshotFiles {
		return nil, 0, errors.New("workspace migration snapshot exceeds file limit")
	}
	entries := make([]snapshotEntry, 0, len(paths))
	var total int64
	for _, path := range paths {
		safetyRoot, rootErr := managedPathSafetyRoot(plan.Runtime, plan.WorkspacePath, path)
		if rootErr != nil {
			return nil, 0, rootErr
		}
		if err := ensureNoSymlinkParents(safetyRoot, path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, 0, err
		}
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			entries = append(entries, snapshotEntry{Path: path})
			continue
		}
		if err != nil {
			return nil, 0, err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return nil, 0, fmt.Errorf("managed snapshot target is not a regular file: %s", path)
		}
		if info.Size() > MaxSnapshotFileSize {
			return nil, 0, fmt.Errorf("managed snapshot file exceeds %d bytes: %s", MaxSnapshotFileSize, path)
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return nil, 0, err
		}
		total += int64(len(body))
		if total > MaxSnapshotBytes {
			return nil, 0, errors.New("workspace migration snapshot exceeds byte limit")
		}
		entries = append(entries, snapshotEntry{Path: path, Exists: true, Mode: uint32(info.Mode().Perm()), Digest: digest(body), Body: body})
	}
	return entries, total, nil
}

func restoreSnapshotForPlan(path string, plan Plan) error {
	var value snapshot
	if err := readJSON(path, &value); err != nil {
		return err
	}
	if err := validateSnapshotForPlan(value, plan); err != nil {
		return err
	}
	return restoreSnapshotValue(value)
}

func readSnapshotForPlan(path string, plan Plan) (snapshot, error) {
	var value snapshot
	if err := readJSON(path, &value); err != nil {
		return snapshot{}, err
	}
	if err := validateSnapshotForPlan(value, plan); err != nil {
		return snapshot{}, err
	}
	return value, nil
}

func validateSnapshotForPlan(value snapshot, plan Plan) error {
	if value.SchemaVersion != SchemaVersion || value.PlanID != plan.ID || value.WorkspacePath != plan.WorkspacePath || value.Runtime != plan.Runtime || value.WorkspaceDigest != plan.SourceDigest || len(value.Entries) > MaxSnapshotFiles || value.TotalBytes < 0 || value.TotalBytes > MaxSnapshotBytes {
		return errors.New("workspace migration snapshot is invalid")
	}
	exclude, err := adaptercfg.LocalConfigExcludePath(value.Runtime, value.WorkspacePath)
	if err != nil {
		return err
	}
	expected := make(map[string]bool, len(plan.ManagedPaths))
	for _, path := range plan.ManagedPaths {
		expected[path] = true
	}
	seenEntries := make(map[string]bool, len(value.Entries))
	var total int64
	for _, entry := range value.Entries {
		cleanEntry := filepath.Clean(entry.Path)
		insideWorkspace := pathWithin(value.WorkspacePath, cleanEntry)
		isGitExclude := exclude != "" && cleanEntry == filepath.Clean(exclude)
		if seenEntries[cleanEntry] || cleanEntry != entry.Path || !filepath.IsAbs(entry.Path) || !insideWorkspace && !isGitExclude || !expected[cleanEntry] {
			return errors.New("workspace migration snapshot contains an unmanaged path")
		}
		seenEntries[cleanEntry] = true
		if !entry.Exists {
			if entry.Digest != "" || len(entry.Body) != 0 || entry.Mode != 0 {
				return errors.New("workspace migration snapshot absent entry is invalid")
			}
			continue
		}
		if int64(len(entry.Body)) > MaxSnapshotFileSize || entry.Digest != digest(entry.Body) {
			return errors.New("workspace migration snapshot entry is invalid")
		}
		total += int64(len(entry.Body))
		if total > MaxSnapshotBytes {
			return errors.New("workspace migration snapshot exceeds byte limit")
		}
	}
	if total != value.TotalBytes {
		return errors.New("workspace migration snapshot byte total is invalid")
	}
	return nil
}

func restoreSnapshotValue(value snapshot) error {
	exclude, err := adaptercfg.LocalConfigExcludePath(value.Runtime, value.WorkspacePath)
	if err != nil {
		return err
	}
	for _, entry := range value.Entries {
		root := value.WorkspacePath
		if entry.Path == exclude {
			root = filepath.Dir(filepath.Dir(exclude))
		}
		if err := ensureNoSymlinkParents(root, entry.Path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if !entry.Exists {
			if info, statErr := os.Lstat(entry.Path); statErr == nil && info.Mode()&os.ModeSymlink != 0 {
				return errors.New("workspace migration refuses to remove a symlink during restore")
			}
			if err := os.Remove(entry.Path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
			continue
		}
		if info, statErr := os.Lstat(entry.Path); statErr == nil && info.Mode()&os.ModeSymlink != 0 {
			return errors.New("workspace migration refuses to replace a symlink during restore")
		}
		if err := os.MkdirAll(filepath.Dir(entry.Path), 0o700); err != nil {
			return err
		}
		if err := os.WriteFile(entry.Path, entry.Body, os.FileMode(entry.Mode)); err != nil {
			return err
		}
	}
	return nil
}

func managedPathSafetyRoot(runtimeName, workspacePath, path string) (string, error) {
	if pathWithin(workspacePath, path) {
		return filepath.Clean(workspacePath), nil
	}
	exclude, err := adaptercfg.LocalConfigExcludePath(runtimeName, workspacePath)
	if err != nil {
		return "", err
	}
	if exclude != "" && filepath.Clean(exclude) == filepath.Clean(path) {
		return filepath.Dir(filepath.Dir(filepath.Clean(exclude))), nil
	}
	return "", errors.New("managed path is outside the workspace and approved Git exclude target")
}

func pathWithin(root, path string) bool {
	relative, err := filepath.Rel(filepath.Clean(root), filepath.Clean(path))
	return err == nil && relative != ".." && !filepath.IsAbs(relative) && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

// ensureNoSymlinkParents is a defense-in-depth check for the internal engine.
// The exported execution APIs remain unavailable until the bootstrapper can
// replace this check-and-use sequence with no-follow handles.
func ensureNoSymlinkParents(root, target string) error {
	root = filepath.Clean(root)
	target = filepath.Clean(target)
	if !filepath.IsAbs(root) || !filepath.IsAbs(target) || !pathWithin(root, target) {
		return errors.New("managed path is outside its safety root")
	}
	rootInfo, err := os.Lstat(root)
	if err != nil {
		return err
	}
	if rootInfo.Mode()&os.ModeSymlink != 0 || !rootInfo.IsDir() {
		return errors.New("managed path safety root is not a canonical directory")
	}
	relative, err := filepath.Rel(root, filepath.Dir(target))
	if err != nil {
		return err
	}
	current := root
	if relative == "." {
		return nil
	}
	for _, component := range strings.Split(relative, string(filepath.Separator)) {
		if component == "" || component == "." {
			continue
		}
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("refusing to follow symlink or non-directory parent %s", current)
		}
	}
	return nil
}

func managedPaths(options PlanOptions) ([]string, error) {
	runtimeName, workspacePath := options.Runtime, options.WorkspacePath
	paths := fixedManagedPaths(runtimeName, workspacePath)
	manifestPath := filepath.Join(workspacePath, runtimeprojection.ManifestRelativePath)
	if safetyRoot, rootErr := managedPathSafetyRoot(runtimeName, workspacePath, manifestPath); rootErr != nil {
		return nil, rootErr
	} else if err := ensureNoSymlinkParents(safetyRoot, manifestPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	body, err := os.ReadFile(manifestPath)
	if err == nil {
		var manifest struct {
			SchemaVersion   int               `json:"schema_version"`
			Runtime         string            `json:"runtime"`
			OrientationPath string            `json:"orientation_path"`
			OrientationHash string            `json:"orientation_hash"`
			SkillHashes     map[string]string `json:"skill_hashes"`
			PolicyPath      string            `json:"policy_path,omitempty"`
			PolicyHash      string            `json:"policy_hash,omitempty"`
		}
		decoder := json.NewDecoder(bytes.NewReader(body))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&manifest); err != nil {
			return nil, fmt.Errorf("decode managed projection manifest: %w", err)
		}
		skillsRoot := filepath.Join(workspacePath, ".codex", "skills")
		if runtimeName == "claude" {
			skillsRoot = filepath.Join(workspacePath, ".claude", "skills")
		}
		for id := range manifest.SkillHashes {
			if filepath.Clean(id) != id || id == "." || id == ".." || strings.ContainsAny(id, `/\\`) {
				return nil, fmt.Errorf("managed projection skill ID is unsafe: %q", id)
			}
			paths = append(paths, filepath.Join(skillsRoot, id, "SKILL.md"))
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	planned, err := runtimeprojection.PlannedManagedPaths(runtimeName, workspacePath, options.CapabilityTracks)
	if err != nil {
		return nil, err
	}
	paths = append(paths, planned...)
	if exclude, err := adaptercfg.LocalConfigExcludePath(runtimeName, workspacePath); err != nil {
		return nil, err
	} else if exclude != "" {
		paths = append(paths, exclude)
	}
	paths = uniquePaths(paths)
	sort.Strings(paths)
	return paths, nil
}

func fixedManagedPaths(runtimeName, workspacePath string) []string {
	orientation := "AGENTS.md"
	adapter := filepath.Join(workspacePath, ".codex", "hooks.json")
	if runtimeName == "claude" {
		orientation = "CLAUDE.md"
		adapter = filepath.Join(workspacePath, ".claude", "settings.local.json")
	}
	return []string{filepath.Join(workspacePath, orientation), adapter, filepath.Join(workspacePath, runtimeprojection.ManifestRelativePath), filepath.Join(workspacePath, runtimeprojection.PolicyRelativePath)}
}

func sourceDigest(options PlanOptions) (string, error) {
	paths, err := managedPaths(options)
	if err != nil {
		// A malformed legacy projection still needs a deterministic inspection
		// digest, but it can never become executable. Hash only the fixed
		// managed entry points; do not attempt to interpret untrusted skill IDs.
		paths = fixedManagedPaths(options.Runtime, options.WorkspacePath)
		if planned, plannedErr := runtimeprojection.PlannedManagedPaths(options.Runtime, options.WorkspacePath, options.CapabilityTracks); plannedErr == nil {
			paths = append(paths, planned...)
		}
		if exclude, excludeErr := adaptercfg.LocalConfigExcludePath(options.Runtime, options.WorkspacePath); excludeErr == nil && exclude != "" {
			paths = append(paths, exclude)
		}
		paths = uniquePaths(paths)
	}
	h := sha256.New()
	for _, path := range paths {
		safetyRoot, rootErr := managedPathSafetyRoot(options.Runtime, options.WorkspacePath, path)
		if rootErr != nil {
			return "", rootErr
		}
		if parentErr := ensureNoSymlinkParents(safetyRoot, path); parentErr != nil && !errors.Is(parentErr, os.ErrNotExist) {
			return "", parentErr
		}
		body, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			fmt.Fprintf(h, "%s\x00absent\x00", path)
			continue
		}
		if err != nil {
			return "", err
		}
		fmt.Fprintf(h, "%s\x00", path)
		h.Write(body)
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func uniquePaths(paths []string) []string {
	seen := make(map[string]bool, len(paths))
	unique := make([]string, 0, len(paths))
	for _, path := range paths {
		clean := filepath.Clean(path)
		if !seen[clean] {
			seen[clean] = true
			unique = append(unique, clean)
		}
	}
	return unique
}

func hasManagedMarker(path string) bool {
	for _, relative := range []string{runtimeprojection.ManifestRelativePath, runtimeprojection.PolicyRelativePath} {
		if _, err := os.Stat(filepath.Join(path, relative)); err == nil {
			return true
		}
	}
	return false
}

func normalizePath(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", errors.New("workspace path is required")
	}
	abs, err := filepath.Abs(path)
	return filepath.Clean(abs), err
}

func sortedUnique(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result
}

func planID(plan Plan) (string, error) {
	plan.ID = ""
	body, err := json.Marshal(plan)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:16]), nil
}

func planRoot(dataRoot, id string) string {
	absolute, err := filepath.Abs(filepath.Clean(dataRoot))
	if err != nil {
		absolute = filepath.Clean(dataRoot)
	}
	return filepath.Join(absolute, "updates", "workspace-migrations", id)
}

func planPath(dataRoot, id string) string {
	return filepath.Join(planRoot(dataRoot, id), "plan.json")
}

func loadPlan(dataRoot, id string) (Plan, error) {
	if !planIDPattern.MatchString(id) {
		return Plan{}, errors.New("workspace migration plan ID is invalid")
	}
	var plan Plan
	if err := readJSON(planPath(dataRoot, id), &plan); err != nil {
		return Plan{}, err
	}
	if err := ValidatePlan(plan); err != nil {
		return Plan{}, err
	}
	return plan, nil
}

func writeJSON(path string, value any, mode os.FileMode) error {
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".workspace-migration-")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(mode); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(body); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}

func readJSON(path string, target any) error {
	body, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("workspace migration JSON has trailing content")
		}
		return err
	}
	return nil
}

func readConfirmation(path string) (Confirmation, error) {
	var value Confirmation
	if err := readJSON(path, &value); err != nil {
		return Confirmation{}, err
	}
	if value.SchemaVersion != SchemaVersion || !planIDPattern.MatchString(value.PlanID) || !digestPattern.MatchString(value.WorkspaceDigest) || value.ConfirmedAt.IsZero() || value.ConfirmedAt.Location() != time.UTC {
		return Confirmation{}, errors.New("workspace migration confirmation is invalid")
	}
	return value, nil
}

func readExecution(path string) (execution, error) {
	var value execution
	if err := readJSON(path, &value); err != nil {
		return execution{}, err
	}
	if value.SchemaVersion != SchemaVersion || !planIDPattern.MatchString(value.PlanID) || value.State == "" || value.SnapshotPath == "" {
		return execution{}, errors.New("workspace migration execution is invalid")
	}
	return value, nil
}

func readReceipt(path, planID string) (Receipt, error) {
	var value Receipt
	if err := readJSON(path, &value); err != nil {
		return Receipt{}, err
	}
	if value.SchemaVersion != SchemaVersion || value.PlanID != planID || (value.State != "applied" && value.State != "rolled_back") || value.CompletedAt.IsZero() || value.CompletedAt.Location() != time.UTC {
		return Receipt{}, errors.New("workspace migration receipt is invalid")
	}
	return value, nil
}

func removeExecutionMarker(root string) error {
	path := filepath.Join(root, "execution.json")
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if _, err := os.Lstat(path); err == nil {
		return errors.New("workspace migration execution marker remains after terminalization")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func digest(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}
