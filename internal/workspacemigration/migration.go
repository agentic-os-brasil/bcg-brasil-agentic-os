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
	result.SourceDigest, err = sourceDigest(options.Runtime, path)
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

func Confirm(dataRoot, id string, core CoreActivation, now time.Time) (Confirmation, error) {
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

func Apply(dataRoot, id string, core CoreActivation, now time.Time) (Receipt, error) {
	plan, err := loadPlan(dataRoot, id)
	if err != nil {
		return Receipt{}, err
	}
	root := planRoot(dataRoot, id)
	if existing, readErr := readExecution(filepath.Join(root, "execution.json")); readErr == nil && existing.State == "applying" {
		if _, recoverErr := Recover(dataRoot, id, now); recoverErr != nil {
			return Receipt{}, recoverErr
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
	if err := writeJSON(filepath.Join(root, "execution.json"), execution{SchemaVersion: SchemaVersion, PlanID: plan.ID, State: "applying", SnapshotPath: confirmation.SnapshotPath, StartedAt: now.UTC()}, 0o600); err != nil {
		return Receipt{}, err
	}
	fail := func(cause error) (Receipt, error) {
		restoreErr := restoreSnapshot(confirmation.SnapshotPath)
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
	_ = os.Remove(filepath.Join(root, "execution.json"))
	return receipt, nil
}

func Recover(dataRoot, id string, now time.Time) (Receipt, error) {
	if !planIDPattern.MatchString(id) {
		return Receipt{}, errors.New("workspace migration plan ID is invalid")
	}
	root := planRoot(dataRoot, id)
	execution, err := readExecution(filepath.Join(root, "execution.json"))
	if errors.Is(err, os.ErrNotExist) {
		return Receipt{SchemaVersion: SchemaVersion, PlanID: id, State: "no_interrupted_execution", CompletedAt: now.UTC()}, nil
	}
	if err != nil {
		return Receipt{}, err
	}
	if execution.State != "applying" {
		return Receipt{}, errors.New("workspace migration execution marker is invalid")
	}
	if err := restoreSnapshot(execution.SnapshotPath); err != nil {
		return Receipt{}, fmt.Errorf("restore interrupted workspace migration: %w", err)
	}
	receipt := Receipt{SchemaVersion: SchemaVersion, PlanID: id, State: "rolled_back", Restored: true, Reason: "interrupted execution restored from bounded snapshot", CompletedAt: now.UTC()}
	if err := writeJSON(filepath.Join(root, "receipt.json"), receipt, 0o600); err != nil {
		return Receipt{}, err
	}
	_ = os.Remove(filepath.Join(root, "execution.json"))
	return receipt, nil
}

func ValidatePlan(plan Plan) error {
	if plan.SchemaVersion != SchemaVersion || !planIDPattern.MatchString(plan.ID) || plan.State != "available" || plan.Runtime != "claude" && plan.Runtime != "codex" || plan.WorkspacePath == "" || plan.WorkspaceID == "" || plan.ExpectedWorkspaceSchema != ExpectedWorkspaceSchema || plan.ExpectedProjectionSchema != runtimeprojection.SchemaVersion || !versionPattern.MatchString(plan.ExpectedRelease) || !versionPattern.MatchString(plan.ExpectedBundle) || !digestPattern.MatchString(plan.SourceDigest) || !plan.ConfirmationRequired || plan.SnapshotMaxFiles != MaxSnapshotFiles || plan.SnapshotMaxFileSize != MaxSnapshotFileSize || plan.SnapshotMaxBytes != MaxSnapshotBytes {
		return errors.New("workspace migration plan contract is invalid")
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
		return errors.New("workspace migration requires authenticated active core evidence from the stable bootstrapper")
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
	paths, err := managedPaths(plan.Runtime, plan.WorkspacePath)
	if err != nil {
		return nil, 0, err
	}
	if len(paths) > MaxSnapshotFiles {
		return nil, 0, errors.New("workspace migration snapshot exceeds file limit")
	}
	entries := make([]snapshotEntry, 0, len(paths))
	var total int64
	for _, path := range paths {
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

func restoreSnapshot(path string) error {
	var value snapshot
	if err := readJSON(path, &value); err != nil {
		return err
	}
	if value.SchemaVersion != SchemaVersion || value.WorkspacePath == "" || (value.Runtime != "claude" && value.Runtime != "codex") || len(value.Entries) > MaxSnapshotFiles || value.TotalBytes > MaxSnapshotBytes {
		return errors.New("workspace migration snapshot is invalid")
	}
	allowed, skillsRoot, err := managedPathSet(value.Runtime, value.WorkspacePath)
	if err != nil {
		return err
	}
	for _, entry := range value.Entries {
		cleanEntry := filepath.Clean(entry.Path)
		if !allowed[cleanEntry] && !managedSkillPath(cleanEntry, skillsRoot) {
			return errors.New("workspace migration snapshot contains an unmanaged path")
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
		if int64(len(entry.Body)) > MaxSnapshotFileSize || entry.Digest != digest(entry.Body) {
			return errors.New("workspace migration snapshot entry is invalid")
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

func managedPathSet(runtimeName, workspacePath string) (map[string]bool, string, error) {
	paths := map[string]bool{}
	orientation := "AGENTS.md"
	adapter := filepath.Join(workspacePath, ".codex", "hooks.json")
	skillsRoot := filepath.Join(workspacePath, ".codex", "skills")
	if runtimeName == "claude" {
		orientation = "CLAUDE.md"
		adapter = filepath.Join(workspacePath, ".claude", "settings.local.json")
		skillsRoot = filepath.Join(workspacePath, ".claude", "skills")
	}
	paths[filepath.Join(workspacePath, orientation)] = true
	paths[adapter] = true
	paths[filepath.Join(workspacePath, runtimeprojection.ManifestRelativePath)] = true
	paths[filepath.Join(workspacePath, runtimeprojection.PolicyRelativePath)] = true
	// Skill entries are one managed file below the runtime's skill root. The
	// snapshot itself remains the authority for which exact files are restored.
	paths[skillsRoot] = true
	return paths, skillsRoot, nil
}

func managedSkillPath(path, root string) bool {
	relative, err := filepath.Rel(root, path)
	if err != nil || relative == "." || filepath.IsAbs(relative) || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return false
	}
	parts := strings.Split(relative, string(filepath.Separator))
	return len(parts) == 2 && parts[1] == "SKILL.md" && parts[0] != "." && parts[0] != ".."
}

func managedPaths(runtimeName, workspacePath string) ([]string, error) {
	orientation := "AGENTS.md"
	adapter := filepath.Join(workspacePath, ".codex", "hooks.json")
	skillsRoot := filepath.Join(workspacePath, ".codex", "skills")
	if runtimeName == "claude" {
		orientation = "CLAUDE.md"
		adapter = filepath.Join(workspacePath, ".claude", "settings.local.json")
		skillsRoot = filepath.Join(workspacePath, ".claude", "skills")
	}
	paths := []string{filepath.Join(workspacePath, orientation), adapter, filepath.Join(workspacePath, runtimeprojection.ManifestRelativePath), filepath.Join(workspacePath, runtimeprojection.PolicyRelativePath)}
	manifestPath := filepath.Join(workspacePath, runtimeprojection.ManifestRelativePath)
	body, err := os.ReadFile(manifestPath)
	if errors.Is(err, os.ErrNotExist) {
		return paths, nil
	}
	if err != nil {
		return nil, err
	}
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
	for id := range manifest.SkillHashes {
		if filepath.Clean(id) != id || id == "." || id == ".." || strings.ContainsAny(id, `/\\`) {
			return nil, fmt.Errorf("managed projection skill ID is unsafe: %q", id)
		}
		paths = append(paths, filepath.Join(skillsRoot, id, "SKILL.md"))
	}
	sort.Strings(paths)
	return paths, nil
}

func sourceDigest(runtimeName, workspacePath string) (string, error) {
	paths, err := managedPaths(runtimeName, workspacePath)
	if err != nil {
		// A malformed legacy projection still needs a deterministic inspection
		// digest, but it can never become executable. Hash only the fixed
		// managed entry points; do not attempt to interpret untrusted skill IDs.
		paths = []string{
			filepath.Join(workspacePath, "AGENTS.md"),
			filepath.Join(workspacePath, ".codex", "hooks.json"),
			filepath.Join(workspacePath, runtimeprojection.ManifestRelativePath),
			filepath.Join(workspacePath, runtimeprojection.PolicyRelativePath),
		}
		if runtimeName == "claude" {
			paths[0] = filepath.Join(workspacePath, "CLAUDE.md")
			paths[1] = filepath.Join(workspacePath, ".claude", "settings.local.json")
		}
	}
	h := sha256.New()
	for _, path := range paths {
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
	return filepath.Join(dataRoot, "updates", "workspace-migrations", id)
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

func digest(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}
