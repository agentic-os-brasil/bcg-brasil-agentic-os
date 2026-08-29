// Package workspaceprojection installs Maestro's bounded runtime surface into
// an ordinary Git repository or linked worktree. Managed product and private
// owner state remain outside the checkout.
package workspaceprojection

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
	"sort"
	"strings"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/adaptercfg"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/agentorchestration"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/privatelock"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/runtimeprojection"
)

const (
	SchemaVersion     = 1
	StateEnrolled     = "enrolled"
	StateAbsent       = "absent"
	StateConflict     = "conflict"
	StateRepairNeeded = "repair_required"
	StateRemoved      = "removed"

	ManagedBlockStart    = runtimeprojection.OrientationBegin
	ManagedBlockEnd      = runtimeprojection.OrientationEnd
	maximumJSONBytes     = 1 << 20
	maximumExcludeBytes  = 8 << 20
	maximumSnapshotBytes = 64 << 20
)

type Manager struct {
	ManagedRoot string
	DataRoot    string
	Executable  string
	GitBinary   string
	Clock       func() time.Time

	// FailurePoint exists only for deterministic transaction failure tests.
	// Production callers leave it nil.
	FailurePoint func(string) error
}

type Status struct {
	SchemaVersion int    `json:"schema_version"`
	State         string `json:"state"`
	Runtime       string `json:"runtime"`
	RepositoryID  string `json:"repository_id,omitempty"`
	WorkspaceID   string `json:"workspace_id,omitempty"`
	Reason        string `json:"reason,omitempty"`
	NextAction    string `json:"next_action,omitempty"`
}

type gitIdentity struct {
	RepositoryRoot string
	GitCommonDir   string
	WorktreeRoot   string
	RepositoryID   string
	WorkspaceID    string
}

type projectionManifest struct {
	SchemaVersion  int      `json:"schema_version"`
	Runtime        string   `json:"runtime"`
	RepositoryID   string   `json:"repository_id"`
	WorkspaceID    string   `json:"workspace_id"`
	ManagedRoot    string   `json:"managed_root"`
	DataRoot       string   `json:"data_root"`
	RepositoryRoot string   `json:"repository_root"`
	GitCommonDir   string   `json:"git_common_dir"`
	WorktreeRoot   string   `json:"worktree_root"`
	Executable     string   `json:"executable"`
	ExcludeRules   []string `json:"exclude_rules"`
	ConfigCreated  bool     `json:"config_created"`
	ProjectedAt    string   `json:"projected_at"`
}

type privateBinding struct {
	SchemaVersion    int      `json:"schema_version"`
	Runtime          string   `json:"runtime"`
	RepositoryID     string   `json:"repository_id"`
	WorkspaceID      string   `json:"workspace_id"`
	ManagedRoot      string   `json:"managed_root"`
	DataRoot         string   `json:"data_root"`
	RepositoryRoot   string   `json:"repository_root"`
	GitCommonDir     string   `json:"git_common_dir"`
	WorktreeRoot     string   `json:"worktree_root"`
	Executable       string   `json:"executable"`
	CaseID           string   `json:"case_id,omitempty"`
	ProjectionDigest string   `json:"projection_digest"`
	ExcludeRules     []string `json:"exclude_rules"`
	UpdatedAt        string   `json:"updated_at"`
}

type fileSnapshot struct {
	path   string
	exists bool
	mode   os.FileMode
	body   []byte
}

func manifestRelativePath(runtimeName string) string {
	return filepath.ToSlash(filepath.Join(".bcgos", "workspace-projections", runtimeName+".json"))
}

// Enroll creates or reconciles the exact runtime projection. All preflights
// complete before the first write and the private binding is published last.
func (manager Manager) Enroll(ctx context.Context, runtimeName, target string) (Status, error) {
	return manager.apply(ctx, runtimeName, target, false)
}

// Status inspects the exact checkout and both local authorities without
// mutating the repository, private state or Git exclude file.
func (manager Manager) Status(ctx context.Context, runtimeName, target string) (Status, error) {
	managedRoot, dataRoot, executable, err := manager.authorities()
	if err != nil {
		return Status{}, err
	}
	identity, err := manager.resolveGitIdentity(ctx, target)
	if err != nil {
		return Status{}, err
	}
	preserveTrackedOrientation, err := manager.orientationTracked(ctx, runtimeName, identity.WorktreeRoot)
	if err != nil {
		return Status{}, err
	}
	if err := manager.rejectTrackedProjection(ctx, runtimeName, identity.WorktreeRoot, preserveTrackedOrientation); err != nil {
		return Status{SchemaVersion: SchemaVersion, State: StateConflict, Runtime: runtimeName, RepositoryID: identity.RepositoryID, WorkspaceID: identity.WorkspaceID, Reason: "managed_projection_tracked", NextAction: "remove only Maestro-generated paths from Git tracking, then rerun workspace status"}, nil
	}
	return manager.statusWithIdentity(runtimeName, identity, managedRoot, dataRoot, executable)
}

func (manager Manager) statusWithIdentity(runtimeName string, identity gitIdentity, managedRoot, dataRoot, executable string) (Status, error) {
	status := Status{SchemaVersion: SchemaVersion, State: StateAbsent, Runtime: runtimeName, RepositoryID: identity.RepositoryID, WorkspaceID: identity.WorkspaceID}
	if runtimeName != "claude" && runtimeName != "codex" {
		return Status{}, errors.New("runtime must be claude or codex")
	}
	manifestPath := filepath.Join(identity.WorktreeRoot, manifestRelativePath(runtimeName))
	var manifest projectionManifest
	if err := readJSONStrict(manifestPath, &manifest); errors.Is(err, os.ErrNotExist) {
		return status, nil
	} else if err != nil {
		status.State, status.Reason = StateConflict, "projection_manifest_invalid"
		return status, fmt.Errorf("inspect workspace projection manifest: %w", err)
	}
	if manifest.SchemaVersion != SchemaVersion || manifest.Runtime != runtimeName ||
		manifest.RepositoryID != identity.RepositoryID || manifest.WorkspaceID != identity.WorkspaceID ||
		manifest.RepositoryRoot != identity.RepositoryRoot || manifest.GitCommonDir != identity.GitCommonDir ||
		manifest.WorktreeRoot != identity.WorktreeRoot || manifest.DataRoot != dataRoot {
		status.State, status.Reason = StateConflict, "projection_identity_mismatch"
		return status, nil
	}
	var binding privateBinding
	if err := readJSONStrict(manager.bindingPath(dataRoot, identity.WorkspaceID, runtimeName), &binding); err != nil {
		status.State, status.Reason = StateConflict, "private_binding_missing_or_invalid"
		return status, nil
	}
	if binding.SchemaVersion != SchemaVersion || binding.Runtime != runtimeName ||
		binding.RepositoryID != identity.RepositoryID || binding.WorkspaceID != identity.WorkspaceID ||
		binding.DataRoot != dataRoot || binding.RepositoryRoot != identity.RepositoryRoot ||
		binding.GitCommonDir != identity.GitCommonDir || binding.WorktreeRoot != identity.WorktreeRoot ||
		!validDigest(binding.ProjectionDigest) || !equalStrings(binding.ExcludeRules, manifest.ExcludeRules) {
		status.State, status.Reason = StateConflict, "private_binding_mismatch"
		return status, nil
	}
	projection, err := runtimeprojection.InspectScoped(runtimeName, identity.WorktreeRoot)
	if err != nil || projection.State != "installed" {
		projectionRelative, pathErr := runtimeprojection.ScopedManifestRelativePath(runtimeName)
		if pathErr != nil {
			return Status{}, pathErr
		}
		projectionDigest, digestErr := digestRegularFile(filepath.Join(identity.WorktreeRoot, projectionRelative), 1<<20)
		if err == nil && digestErr == nil && projectionDigest == binding.ProjectionDigest &&
			runtimeprojection.ValidateScopedExistingInstall(runtimeName, identity.WorktreeRoot) == nil {
			status.State, status.Reason = StateRepairNeeded, "managed_core_updated"
			status.NextAction = "run bcgos workspace repair with the currently activated Maestro CLI"
			return status, nil
		}
		status.State, status.Reason = StateConflict, "runtime_projection_conflict"
		return status, nil
	}
	adapter, err := adaptercfg.Inspect(runtimeName, identity.WorktreeRoot)
	if err != nil {
		status.State, status.Reason = StateConflict, "runtime_adapter_conflict"
		return status, nil
	}
	if adapter.State != "installed" {
		status.State, status.Reason = StateRepairNeeded, "runtime_adapter_incomplete"
		status.NextAction = "run bcgos workspace repair with the currently activated Maestro CLI"
		return status, nil
	}
	if ok, err := exactExcludesPresent(identity.GitCommonDir, manifest.ExcludeRules); err != nil || !ok {
		status.State, status.Reason = StateConflict, "git_exclude_drift"
		return status, nil
	}
	if manifest.ManagedRoot != managedRoot || manifest.Executable != executable ||
		binding.ManagedRoot != managedRoot || binding.Executable != executable ||
		!configContainsAuthorities(runtimeName, runtimeConfigPath(runtimeName, identity.WorktreeRoot), executable, managedRoot, dataRoot, identity.WorktreeRoot) {
		status.State, status.Reason = StateRepairNeeded, "managed_root_moved"
		status.NextAction = "run bcgos workspace repair with the currently activated Maestro CLI"
		return status, nil
	}
	status.State = StateEnrolled
	return status, nil
}

// Repair is an explicit authority to regenerate an otherwise intact
// projection from the currently executing installed core.
func (manager Manager) Repair(ctx context.Context, runtimeName, target string) (Status, error) {
	status, err := manager.Status(ctx, runtimeName, target)
	if err != nil {
		return status, err
	}
	switch status.State {
	case StateEnrolled:
		return status, nil
	case StateAbsent:
		return status, fmt.Errorf("workspace is not enrolled; no files were changed; next safe action: bcgos workspace enroll --runtime %s <repository-or-worktree>", runtimeName)
	case StateConflict:
		return status, errors.New("workspace projection conflict must be reviewed before repair; no source code or work was lost; next safe action: bcgos workspace status --runtime " + runtimeName + " <repository-or-worktree>")
	case StateRepairNeeded:
		return manager.apply(ctx, runtimeName, target, true)
	default:
		return status, fmt.Errorf("unsupported workspace projection state %q", status.State)
	}
}

// Remove deletes only intact manifest-owned projection files and the exact
// private binding. Git worktree, branch, index and user-authored content are
// never changed.
func (manager Manager) Remove(ctx context.Context, runtimeName, target string) (status Status, returnedErr error) {
	if runtimeName != "claude" && runtimeName != "codex" {
		return Status{}, errors.New("runtime must be claude or codex; no files were changed")
	}
	managedRoot, dataRoot, executable, err := manager.authorities()
	if err != nil {
		return Status{}, err
	}
	identity, err := manager.resolveGitIdentity(ctx, target)
	if err != nil {
		return Status{}, err
	}
	manifestPath := filepath.Join(identity.WorktreeRoot, manifestRelativePath(runtimeName))
	var manifest projectionManifest
	if err := readJSONStrict(manifestPath, &manifest); errors.Is(err, os.ErrNotExist) {
		return Status{SchemaVersion: SchemaVersion, State: StateAbsent, Runtime: runtimeName, RepositoryID: identity.RepositoryID, WorkspaceID: identity.WorkspaceID}, nil
	} else if err != nil {
		return Status{}, err
	}
	current, err := manager.statusWithIdentity(runtimeName, identity, managedRoot, dataRoot, executable)
	if err != nil {
		return current, err
	}
	if current.State == StateConflict {
		return current, errors.New("workspace projection conflict prevents safe removal; no source code or work was lost; next safe action: bcgos workspace status --runtime " + runtimeName + " <repository-or-worktree>")
	}
	if err := runtimeprojection.ValidateScopedUninstall(runtimeName, identity.WorktreeRoot); err != nil {
		return conflict(runtimeName, identity, err)
	}
	if err := adaptercfg.ValidateUninstall(runtimeName, identity.WorktreeRoot); err != nil {
		return conflict(runtimeName, identity, err)
	}
	preserveTrackedOrientation, err := manager.orientationTracked(ctx, runtimeName, identity.WorktreeRoot)
	if err != nil {
		return Status{}, err
	}
	if err := manager.rejectTrackedProjection(ctx, runtimeName, identity.WorktreeRoot, preserveTrackedOrientation); err != nil {
		return conflict(runtimeName, identity, err)
	}
	unlock, err := acquireMutationLock(identity)
	if err != nil {
		return Status{}, fmt.Errorf("acquire workspace projection transaction: %w; no source code or work was lost", err)
	}
	defer func() {
		if unlockErr := unlock(); unlockErr != nil && returnedErr == nil {
			returnedErr = fmt.Errorf("release workspace projection transaction: %w", unlockErr)
		}
	}()
	if err := readJSONStrict(manifestPath, &manifest); errors.Is(err, os.ErrNotExist) {
		return Status{SchemaVersion: SchemaVersion, State: StateAbsent, Runtime: runtimeName, RepositoryID: identity.RepositoryID, WorkspaceID: identity.WorkspaceID}, nil
	} else if err != nil {
		return Status{}, err
	}
	current, err = manager.statusWithIdentity(runtimeName, identity, managedRoot, dataRoot, executable)
	if err != nil {
		return current, err
	}
	if current.State == StateConflict {
		return current, errors.New("workspace projection conflict prevents safe removal; no source code or work was lost; next safe action: bcgos workspace status --runtime " + runtimeName + " <repository-or-worktree>")
	}
	if err := runtimeprojection.ValidateScopedUninstall(runtimeName, identity.WorktreeRoot); err != nil {
		return conflict(runtimeName, identity, err)
	}
	if err := adaptercfg.ValidateUninstall(runtimeName, identity.WorktreeRoot); err != nil {
		return conflict(runtimeName, identity, err)
	}
	if err := manager.rejectTrackedProjection(ctx, runtimeName, identity.WorktreeRoot, preserveTrackedOrientation); err != nil {
		return conflict(runtimeName, identity, err)
	}
	installed, err := runtimeprojection.InstalledScopedManagedPaths(runtimeName, identity.WorktreeRoot)
	if err != nil {
		return Status{}, err
	}
	bindingPath := manager.bindingPath(dataRoot, identity.WorkspaceID, runtimeName)
	managedPaths := append([]string{}, installed...)
	managedPaths = append(managedPaths, manifestPath, filepath.Join(identity.WorktreeRoot, ".bcgos", "maestro-orchestration-state.json"), bindingPath, filepath.Join(identity.GitCommonDir, "info", "exclude"))
	snapshots, err := captureFiles(managedPaths)
	if err != nil {
		return Status{}, err
	}
	adapterSnapshot, err := adaptercfg.CaptureState(runtimeName, identity.WorktreeRoot)
	if err != nil {
		return Status{}, err
	}
	defer func() {
		if returnedErr == nil {
			return
		}
		if restoreErr := errors.Join(restoreFiles(snapshots), adapterSnapshot.Restore()); restoreErr != nil {
			returnedErr = fmt.Errorf("%w; rollback also failed: %v", returnedErr, restoreErr)
		}
	}()
	if _, err := adaptercfg.Uninstall(runtimeName, identity.WorktreeRoot); err != nil {
		return Status{}, err
	}
	if manifest.ConfigCreated {
		if err := removeEmptyRuntimeConfig(runtimeConfigPath(runtimeName, identity.WorktreeRoot)); err != nil {
			return Status{}, err
		}
	}
	if _, err := runtimeprojection.UninstallScoped(runtimeName, identity.WorktreeRoot); err != nil {
		return Status{}, err
	}
	for _, path := range []string{manifestPath, bindingPath} {
		if err := removeRegular(path); err != nil {
			return Status{}, err
		}
	}
	otherRuntime := "claude"
	if runtimeName == "claude" {
		otherRuntime = "codex"
	}
	otherManifest := filepath.Join(identity.WorktreeRoot, manifestRelativePath(otherRuntime))
	siblingPresent := false
	if _, err := os.Lstat(otherManifest); errors.Is(err, os.ErrNotExist) {
		if err := removeRegular(filepath.Join(identity.WorktreeRoot, ".bcgos", "maestro-orchestration-state.json")); err != nil {
			return Status{}, err
		}
	} else if err != nil {
		return Status{}, fmt.Errorf("inspect sibling runtime projection: %w", err)
	} else {
		siblingPresent = true
	}
	referenced, err := manager.referencedExcludeRules(dataRoot, identity.RepositoryID, identity.WorkspaceID, runtimeName)
	if err != nil {
		return Status{}, err
	}
	if siblingPresent {
		referenced["/.bcgos/maestro-orchestration-state.json"] = true
	}
	removable := make([]string, 0, len(manifest.ExcludeRules))
	for _, rule := range manifest.ExcludeRules {
		if !referenced[rule] {
			removable = append(removable, rule)
		}
	}
	if err := removeExactExcludes(identity.GitCommonDir, removable); err != nil {
		return Status{}, err
	}
	return Status{SchemaVersion: SchemaVersion, State: StateRemoved, Runtime: runtimeName, RepositoryID: identity.RepositoryID, WorkspaceID: identity.WorkspaceID}, nil
}

func (manager Manager) apply(ctx context.Context, runtimeName, target string, repairing bool) (status Status, returnedErr error) {
	if runtimeName != "claude" && runtimeName != "codex" {
		return Status{}, fmt.Errorf("runtime must be claude or codex; no files were changed")
	}
	managedRoot, dataRoot, executable, err := manager.authorities()
	if err != nil {
		return Status{}, err
	}
	identity, err := manager.resolveGitIdentity(ctx, target)
	if err != nil {
		return Status{}, err
	}
	preserveTrackedOrientation, err := manager.orientationTracked(ctx, runtimeName, identity.WorktreeRoot)
	if err != nil {
		return Status{}, err
	}
	current, currentErr := manager.statusWithIdentity(runtimeName, identity, managedRoot, dataRoot, executable)
	if currentErr == nil {
		switch current.State {
		case StateEnrolled:
			return current, nil
		case StateRepairNeeded:
			if !repairing {
				return current, fmt.Errorf("workspace projection requires repair (%s); no source code or work was lost; next safe action: bcgos workspace repair --runtime %s <repository-or-worktree>", current.Reason, runtimeName)
			}
		case StateConflict:
			return current, fmt.Errorf("workspace projection has a managed-file conflict; no source code or work was lost; next safe action: review bcgos workspace status before changing managed files")
		}
	} else if !errors.Is(currentErr, os.ErrNotExist) {
		return current, currentErr
	}

	roots := adaptercfg.ScopedRoots{ManagedRoot: managedRoot, DataRoot: dataRoot, WorkspaceRoot: identity.WorktreeRoot}
	if err := validateRuntimeProjection(runtimeName, identity.WorktreeRoot, preserveTrackedOrientation); err != nil {
		return conflict(runtimeName, identity, err)
	}
	if err := adaptercfg.ValidateScopedInstall(runtimeName, identity.WorktreeRoot, executable, roots); err != nil {
		return conflict(runtimeName, identity, err)
	}
	if err := manager.rejectTrackedProjection(ctx, runtimeName, identity.WorktreeRoot, preserveTrackedOrientation); err != nil {
		return conflict(runtimeName, identity, err)
	}
	unlock, err := acquireMutationLock(identity)
	if err != nil {
		return Status{}, fmt.Errorf("acquire workspace projection transaction: %w; no source code or work was lost", err)
	}
	defer func() {
		if unlockErr := unlock(); unlockErr != nil && returnedErr == nil {
			returnedErr = fmt.Errorf("release workspace projection transaction: %w", unlockErr)
		}
	}()
	current, currentErr = manager.statusWithIdentity(runtimeName, identity, managedRoot, dataRoot, executable)
	if currentErr == nil {
		switch current.State {
		case StateEnrolled:
			return current, nil
		case StateRepairNeeded:
			if !repairing {
				return current, fmt.Errorf("workspace projection requires repair (%s); no source code or work was lost; next safe action: bcgos workspace repair --runtime %s <repository-or-worktree>", current.Reason, runtimeName)
			}
		case StateConflict:
			return current, fmt.Errorf("workspace projection has a managed-file conflict; no source code or work was lost; next safe action: review bcgos workspace status before changing managed files")
		}
	} else if !errors.Is(currentErr, os.ErrNotExist) {
		return current, currentErr
	}
	if err := validateRuntimeProjection(runtimeName, identity.WorktreeRoot, preserveTrackedOrientation); err != nil {
		return conflict(runtimeName, identity, err)
	}
	if err := adaptercfg.ValidateScopedInstall(runtimeName, identity.WorktreeRoot, executable, roots); err != nil {
		return conflict(runtimeName, identity, err)
	}
	if err := manager.rejectTrackedProjection(ctx, runtimeName, identity.WorktreeRoot, preserveTrackedOrientation); err != nil {
		return conflict(runtimeName, identity, err)
	}
	var previousManifest projectionManifest
	manifestPath := filepath.Join(identity.WorktreeRoot, manifestRelativePath(runtimeName))
	previousManifestPresent := readJSONStrict(manifestPath, &previousManifest) == nil

	planned, err := plannedRuntimeProjection(runtimeName, identity.WorktreeRoot, preserveTrackedOrientation)
	if err != nil {
		return Status{}, err
	}
	installed, err := runtimeprojection.InstalledScopedManagedPaths(runtimeName, identity.WorktreeRoot)
	if err != nil {
		return Status{}, err
	}
	bindingPath := manager.bindingPath(dataRoot, identity.WorkspaceID, runtimeName)
	managedPaths := append(append([]string{}, planned...), installed...)
	managedPaths = append(managedPaths,
		manifestPath,
		filepath.Join(identity.WorktreeRoot, ".bcgos", "maestro-orchestration-state.json"),
		bindingPath,
		filepath.Join(identity.GitCommonDir, "info", "exclude"),
	)
	snapshots, err := captureFiles(managedPaths)
	if err != nil {
		return Status{}, err
	}
	configExisted, err := regularFileExists(runtimeConfigPath(runtimeName, identity.WorktreeRoot))
	if err != nil {
		return Status{}, err
	}
	adapterSnapshot, err := adaptercfg.CaptureState(runtimeName, identity.WorktreeRoot)
	if err != nil {
		return Status{}, err
	}
	defer func() {
		if returnedErr == nil {
			return
		}
		restoreErr := errors.Join(restoreFiles(snapshots), adapterSnapshot.Restore())
		if restoreErr != nil {
			returnedErr = fmt.Errorf("%w; rollback also failed: %v", returnedErr, restoreErr)
		}
	}()

	if err := agentorchestration.EnsureDurableState(
		filepath.Join(identity.WorktreeRoot, ".bcgos", "maestro-orchestration-state.json"),
		"direct-worktree\x00"+identity.WorkspaceID,
	); err != nil {
		return Status{}, fmt.Errorf("initialize workspace-scoped orchestration state: %w", err)
	}
	if err := manager.fail("after_workspace_state"); err != nil {
		return Status{}, err
	}
	if err := installRuntimeProjection(runtimeName, identity.WorktreeRoot, preserveTrackedOrientation); err != nil {
		return Status{}, err
	}
	if err := manager.fail("after_runtime_projection"); err != nil {
		return Status{}, err
	}
	if _, err := adaptercfg.InstallScoped(runtimeName, identity.WorktreeRoot, executable, roots); err != nil {
		return Status{}, err
	}
	if err := manager.fail("after_adapter"); err != nil {
		return Status{}, err
	}

	rules, err := exactExcludeRules(runtimeName, identity.WorktreeRoot, planned)
	if err != nil {
		return Status{}, err
	}
	if err := ensureExactExcludes(identity.GitCommonDir, rules); err != nil {
		return Status{}, err
	}
	if previousManifestPresent {
		referenced, err := manager.referencedExcludeRules(dataRoot, identity.RepositoryID, identity.WorkspaceID, runtimeName)
		if err != nil {
			return Status{}, err
		}
		currentRules := map[string]bool{}
		for _, rule := range rules {
			currentRules[rule] = true
		}
		retired := []string{}
		for _, rule := range previousManifest.ExcludeRules {
			if !currentRules[rule] && !referenced[rule] {
				retired = append(retired, rule)
			}
		}
		if err := removeExactExcludes(identity.GitCommonDir, retired); err != nil {
			return Status{}, err
		}
	}
	now := time.Now().UTC()
	if manager.Clock != nil {
		now = manager.Clock().UTC()
	}
	manifest := projectionManifest{
		SchemaVersion: SchemaVersion, Runtime: runtimeName,
		RepositoryID: identity.RepositoryID, WorkspaceID: identity.WorkspaceID,
		ManagedRoot: managedRoot, DataRoot: dataRoot, RepositoryRoot: identity.RepositoryRoot,
		GitCommonDir: identity.GitCommonDir, WorktreeRoot: identity.WorktreeRoot,
		Executable: executable, ExcludeRules: rules, ConfigCreated: !configExisted, ProjectedAt: now.Format(time.RFC3339),
	}
	if err := writeJSONAtomic(manifestPath, manifest, 0o600); err != nil {
		return Status{}, err
	}
	if err := manager.fail("after_manifest"); err != nil {
		return Status{}, err
	}
	projectionRelative, err := runtimeprojection.ScopedManifestRelativePath(runtimeName)
	if err != nil {
		return Status{}, err
	}
	projectionDigest, err := digestRegularFile(filepath.Join(identity.WorktreeRoot, projectionRelative), 1<<20)
	if err != nil {
		return Status{}, fmt.Errorf("digest runtime projection manifest: %w", err)
	}
	binding := privateBinding{
		SchemaVersion: SchemaVersion, Runtime: runtimeName,
		RepositoryID: identity.RepositoryID, WorkspaceID: identity.WorkspaceID,
		ManagedRoot: managedRoot, DataRoot: dataRoot, RepositoryRoot: identity.RepositoryRoot,
		GitCommonDir: identity.GitCommonDir, WorktreeRoot: identity.WorktreeRoot,
		Executable: executable, ProjectionDigest: projectionDigest, ExcludeRules: rules, UpdatedAt: now.Format(time.RFC3339),
	}
	if err := writeJSONAtomic(bindingPath, binding, 0o600); err != nil {
		return Status{}, err
	}
	if err := manager.fail("after_private_binding"); err != nil {
		return Status{}, err
	}
	state := StateEnrolled
	if repairing {
		state = StateEnrolled
	}
	return Status{SchemaVersion: SchemaVersion, State: state, Runtime: runtimeName, RepositoryID: identity.RepositoryID, WorkspaceID: identity.WorkspaceID}, nil
}

func conflict(runtimeName string, identity gitIdentity, err error) (Status, error) {
	status := Status{SchemaVersion: SchemaVersion, State: StateConflict, Runtime: runtimeName, RepositoryID: identity.RepositoryID, WorkspaceID: identity.WorkspaceID, Reason: "managed_projection_conflict", NextAction: "review the reported managed-file conflict, preserve your work, then rerun workspace status"}
	return status, fmt.Errorf("%w; no source code or work was lost; next safe action: %s", err, status.NextAction)
}

func acquireMutationLock(identity gitIdentity) (func() error, error) {
	return privatelock.Acquire(filepath.Join(identity.GitCommonDir, "info", "maestro-workspace-projection.lock"))
}

func validateRuntimeProjection(runtimeName, workspace string, preserveTrackedOrientation bool) error {
	if preserveTrackedOrientation {
		return runtimeprojection.ValidateScopedInstallWithoutOrientation(runtimeName, workspace)
	}
	return runtimeprojection.ValidateScopedInstall(runtimeName, workspace)
}

func plannedRuntimeProjection(runtimeName, workspace string, preserveTrackedOrientation bool) ([]string, error) {
	if preserveTrackedOrientation {
		return runtimeprojection.PlannedScopedManagedPathsWithoutOrientation(runtimeName, workspace, nil)
	}
	return runtimeprojection.PlannedScopedManagedPaths(runtimeName, workspace, nil)
}

func installRuntimeProjection(runtimeName, workspace string, preserveTrackedOrientation bool) error {
	if preserveTrackedOrientation {
		_, err := runtimeprojection.InstallScopedWithoutOrientation(runtimeName, workspace)
		return err
	}
	_, err := runtimeprojection.InstallScoped(runtimeName, workspace)
	return err
}

func (manager Manager) orientationTracked(ctx context.Context, runtimeName, workspace string) (bool, error) {
	relative := "AGENTS.md"
	if runtimeName == "claude" {
		relative = "CLAUDE.md"
	}
	gitBinary := strings.TrimSpace(manager.GitBinary)
	var err error
	if gitBinary == "" {
		gitBinary, err = exec.LookPath("git")
		if err != nil {
			return false, errors.New("Git executable is unavailable; no files were changed")
		}
	}
	command := isolatedGitCommand(ctx, gitBinary, "-C", workspace, "ls-files", "--error-unmatch", "--", relative)
	output, err := command.CombinedOutput()
	if err == nil {
		return true, nil
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) && exitError.ExitCode() == 1 {
		return false, nil
	}
	return false, fmt.Errorf("inspect tracked runtime orientation: %w (%s)", err, strings.TrimSpace(string(output)))
}

func (manager Manager) rejectTrackedProjection(ctx context.Context, runtimeName, workspace string, preserveTrackedOrientation bool) error {
	planned, err := plannedRuntimeProjection(runtimeName, workspace, preserveTrackedOrientation)
	if err != nil {
		return err
	}
	installed, err := runtimeprojection.InstalledScopedManagedPaths(runtimeName, workspace)
	if err != nil {
		return err
	}
	paths := append(append([]string{}, planned...), installed...)
	paths = append(paths,
		filepath.Join(workspace, manifestRelativePath(runtimeName)),
		filepath.Join(workspace, ".bcgos", "maestro-orchestration-state.json"),
		runtimeConfigPath(runtimeName, workspace),
	)
	if runtimeName == "claude" {
		for _, id := range []string{"maestro-hub", "client-account-agent", "case-agent", "yoda", "darwin", "pa-expert"} {
			paths = append(paths, filepath.Join(workspace, ".claude", "agents", id+".md"))
		}
	}
	unique := map[string]bool{}
	arguments := []string{"-C", workspace, "ls-files", "--"}
	for _, path := range paths {
		relative, err := filepath.Rel(workspace, path)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return errors.New("managed projection path escaped the worktree")
		}
		relative = filepath.ToSlash(relative)
		if !unique[relative] {
			unique[relative] = true
			arguments = append(arguments, relative)
		}
	}
	gitBinary := strings.TrimSpace(manager.GitBinary)
	if gitBinary == "" {
		gitBinary, err = exec.LookPath("git")
		if err != nil {
			return errors.New("Git executable is unavailable; no files were changed")
		}
	}
	output, err := isolatedGitCommand(ctx, gitBinary, arguments...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("inspect Git tracking for managed projection: %w", err)
	}
	if strings.TrimSpace(string(output)) != "" {
		return errors.New("one or more Maestro-generated projection paths are tracked by Git; no files were changed")
	}
	return nil
}

func (manager Manager) authorities() (string, string, string, error) {
	managedRoot, err := canonicalDirectory("managed root", manager.ManagedRoot)
	if err != nil {
		return "", "", "", err
	}
	dataRoot, err := canonicalDirectory("data root", manager.DataRoot)
	if err != nil {
		return "", "", "", err
	}
	executable, err := canonicalRegularFile("installed CLI", manager.Executable)
	if err != nil {
		return "", "", "", err
	}
	if !inside(managedRoot, executable) {
		return "", "", "", errors.New("installed CLI must remain inside managed root")
	}
	if inside(managedRoot, dataRoot) || inside(dataRoot, managedRoot) {
		return "", "", "", errors.New("managed root and data root must be separate directories")
	}
	return managedRoot, dataRoot, executable, nil
}

func (manager Manager) resolveGitIdentity(ctx context.Context, target string) (gitIdentity, error) {
	if lexicalTraversal(target) {
		return gitIdentity{}, errors.New("repository path contains traversal; no files were changed")
	}
	targetRoot, err := canonicalDirectory("repository target", target)
	if err != nil {
		return gitIdentity{}, err
	}
	gitBinary := strings.TrimSpace(manager.GitBinary)
	if gitBinary == "" {
		gitBinary, err = exec.LookPath("git")
		if err != nil {
			return gitIdentity{}, errors.New("Git executable is unavailable; no files were changed")
		}
	}
	gitBinary, err = filepath.Abs(gitBinary)
	if err != nil {
		return gitIdentity{}, err
	}
	run := func(args ...string) (string, error) {
		command := isolatedGitCommand(ctx, gitBinary, append([]string{"-C", targetRoot}, args...)...)
		output, err := command.CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("resolve Git workspace identity: %w", err)
		}
		return strings.TrimSpace(string(output)), nil
	}
	top, err := run("rev-parse", "--path-format=absolute", "--show-toplevel")
	if err != nil {
		return gitIdentity{}, err
	}
	common, err := run("rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return gitIdentity{}, err
	}
	worktreeRoot, err := canonicalDirectory("Git worktree root", top)
	if err != nil {
		return gitIdentity{}, err
	}
	commonRoot, err := canonicalDirectory("Git common directory", common)
	if err != nil {
		return gitIdentity{}, err
	}
	if err := validateGitMetadataPaths(commonRoot); err != nil {
		return gitIdentity{}, err
	}
	repositoryRoot := worktreeRoot
	if filepath.Base(commonRoot) == ".git" {
		candidate := filepath.Dir(commonRoot)
		if canonical, canonicalErr := canonicalDirectory("Git repository root", candidate); canonicalErr == nil {
			repositoryRoot = canonical
		}
	}
	return gitIdentity{
		RepositoryRoot: repositoryRoot, GitCommonDir: commonRoot, WorktreeRoot: worktreeRoot,
		RepositoryID: opaqueID("repository", commonRoot), WorkspaceID: opaqueID("workspace", worktreeRoot),
	}, nil
}

func isolatedGitCommand(ctx context.Context, executable string, args ...string) *exec.Cmd {
	command := exec.CommandContext(ctx, executable, args...)
	for _, variable := range os.Environ() {
		if strings.HasPrefix(variable, "GIT_INDEX_FILE=") ||
			strings.HasPrefix(variable, "GIT_DIR=") ||
			strings.HasPrefix(variable, "GIT_WORK_TREE=") ||
			strings.HasPrefix(variable, "GIT_PREFIX=") ||
			strings.HasPrefix(variable, "GIT_CONFIG_PARAMETERS=") {
			continue
		}
		command.Env = append(command.Env, variable)
	}
	return command
}

func validateGitMetadataPaths(gitCommonDir string) error {
	infoRoot := filepath.Join(gitCommonDir, "info")
	info, err := os.Lstat(infoRoot)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("Git info directory must be a real directory; no files were changed")
	}
	for _, name := range []string{"exclude", "maestro-workspace-projection.lock"} {
		path := filepath.Join(infoRoot, name)
		entry, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if entry.Mode()&os.ModeSymlink != 0 || !entry.Mode().IsRegular() {
			return fmt.Errorf("Git info/%s must be a regular non-symlink file; no files were changed", name)
		}
		if name == "exclude" && entry.Size() > maximumExcludeBytes {
			return errors.New("Git info/exclude exceeds its bounded size; no files were changed")
		}
	}
	return nil
}

func canonicalDirectory(name, path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("%s is required", name)
	}
	absolute, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", err
	}
	info, err := os.Lstat(absolute)
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return "", fmt.Errorf("%s must be a real directory; refusing symlink or non-directory", name)
	}
	real, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", err
	}
	real, err = filepath.Abs(real)
	if err != nil {
		return "", err
	}
	return real, nil
}

func canonicalRegularFile(name, path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("%s is required", name)
	}
	absolute, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", err
	}
	info, err := os.Lstat(absolute)
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return "", fmt.Errorf("%s must be a regular non-symlink file", name)
	}
	real, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", err
	}
	return filepath.Abs(real)
}

func inside(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func lexicalTraversal(path string) bool {
	path = strings.ReplaceAll(path, "\\", "/")
	for _, part := range strings.Split(path, "/") {
		if part == ".." {
			return true
		}
	}
	return false
}

func opaqueID(kind, value string) string {
	sum := sha256.Sum256([]byte("maestro-" + kind + "-v1\x00" + value))
	return hex.EncodeToString(sum[:16])
}

func digestRegularFile(path string, limit int64) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Size() > limit {
		return "", errors.New("digest target must be a bounded regular non-symlink file")
	}
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, io.LimitReader(file, limit+1)); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func validDigest(value string) bool {
	if len(value) != sha256.Size*2 || strings.ToLower(value) != value {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size
}

func (manager Manager) bindingPath(dataRoot, workspaceID, runtimeName string) string {
	return filepath.Join(dataRoot, "workspaces", workspaceID, "enrollments", runtimeName+".json")
}

func (manager Manager) referencedExcludeRules(dataRoot, repositoryID, workspaceID, runtimeName string) (map[string]bool, error) {
	referenced := map[string]bool{}
	root := filepath.Join(dataRoot, "workspaces")
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return referenced, nil
	}
	if err != nil {
		return nil, err
	}
	if len(entries) > 1024 {
		return nil, errors.New("private workspace binding index exceeds inspection limit")
	}
	for _, entry := range entries {
		if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		for _, siblingRuntime := range []string{"claude", "codex"} {
			if entry.Name() == workspaceID && siblingRuntime == runtimeName {
				continue
			}
			var binding privateBinding
			path := manager.bindingPath(dataRoot, entry.Name(), siblingRuntime)
			if err := readJSONStrict(path, &binding); errors.Is(err, os.ErrNotExist) {
				continue
			} else if err != nil {
				return nil, fmt.Errorf("inspect sibling workspace binding: %w", err)
			}
			if binding.SchemaVersion != SchemaVersion || binding.RepositoryID != repositoryID || binding.Runtime != siblingRuntime {
				continue
			}
			for _, rule := range binding.ExcludeRules {
				if !strings.HasPrefix(rule, "/") || strings.Contains(rule, "..") {
					return nil, errors.New("sibling workspace binding has an unsafe Git exclude rule")
				}
				referenced[rule] = true
			}
		}
	}
	return referenced, nil
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func exactExcludeRules(runtimeName, workspace string, planned []string) ([]string, error) {
	paths := append([]string{}, planned...)
	paths = append(paths,
		filepath.Join(workspace, manifestRelativePath(runtimeName)),
		filepath.Join(workspace, ".bcgos", "maestro-orchestration-state.json"),
	)
	if runtimeName == "claude" {
		paths = append(paths, filepath.Join(workspace, ".claude", "settings.local.json"))
		for _, id := range []string{"maestro-hub", "client-account-agent", "case-agent", "yoda", "darwin", "pa-expert"} {
			paths = append(paths, filepath.Join(workspace, ".claude", "agents", id+".md"))
		}
	} else {
		paths = append(paths, filepath.Join(workspace, ".codex", "hooks.json"))
	}
	unique := map[string]bool{}
	rules := []string{}
	for _, path := range paths {
		relative, err := filepath.Rel(workspace, path)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return nil, errors.New("managed exclude path escaped the worktree")
		}
		rule := "/" + filepath.ToSlash(relative)
		if !unique[rule] {
			unique[rule] = true
			rules = append(rules, rule)
		}
	}
	sort.Strings(rules)
	return rules, nil
}

func ensureExactExcludes(gitCommonDir string, rules []string) error {
	infoRoot := filepath.Join(gitCommonDir, "info")
	if err := os.MkdirAll(infoRoot, 0o700); err != nil {
		return err
	}
	path := filepath.Join(infoRoot, "exclude")
	if info, err := os.Lstat(path); err == nil && (info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular()) {
		return errors.New("Git info/exclude must be a regular non-symlink file")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	body, _, err := readBoundedRegular(path, maximumExcludeBytes)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	existing := map[string]bool{}
	for _, line := range strings.Split(string(body), "\n") {
		existing[strings.TrimSpace(line)] = true
	}
	for _, rule := range rules {
		if existing[rule] {
			continue
		}
		if len(body) > 0 && body[len(body)-1] != '\n' {
			body = append(body, '\n')
		}
		body = append(body, rule...)
		body = append(body, '\n')
		existing[rule] = true
	}
	return writeBytesAtomic(path, body, 0o600)
}

func exactExcludesPresent(gitCommonDir string, rules []string) (bool, error) {
	path := filepath.Join(gitCommonDir, "info", "exclude")
	info, err := os.Lstat(path)
	if err != nil {
		return false, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return false, errors.New("Git info/exclude must be a regular non-symlink file")
	}
	body, _, err := readBoundedRegular(path, maximumExcludeBytes)
	if err != nil {
		return false, err
	}
	existing := map[string]bool{}
	for _, line := range strings.Split(string(body), "\n") {
		existing[strings.TrimSpace(line)] = true
	}
	for _, rule := range rules {
		if !existing[rule] {
			return false, nil
		}
	}
	return true, nil
}

func removeExactExcludes(gitCommonDir string, rules []string) error {
	path := filepath.Join(gitCommonDir, "info", "exclude")
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return errors.New("Git info/exclude must be a regular non-symlink file")
	}
	body, mode, err := readBoundedRegular(path, maximumExcludeBytes)
	if err != nil {
		return err
	}
	owned := map[string]bool{}
	for _, rule := range rules {
		owned[rule] = true
	}
	kept := []string{}
	for _, line := range strings.Split(string(body), "\n") {
		if owned[strings.TrimSpace(line)] {
			continue
		}
		kept = append(kept, line)
	}
	result := strings.Join(kept, "\n")
	if result != "" && !strings.HasSuffix(result, "\n") {
		result += "\n"
	}
	return writeBytesAtomic(path, []byte(result), mode)
}

func runtimeConfigPath(runtimeName, workspace string) string {
	if runtimeName == "claude" {
		return filepath.Join(workspace, ".claude", "settings.local.json")
	}
	return filepath.Join(workspace, ".codex", "hooks.json")
}

func regularFileExists(path string) (bool, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return false, fmt.Errorf("managed path %s must be a regular non-symlink file", path)
	}
	return true, nil
}

func configContainsAuthorities(runtimeName, path string, values ...string) bool {
	var config map[string]any
	if readJSONStrict(path, &config) != nil {
		return false
	}
	hooks, ok := config["hooks"].(map[string]any)
	if !ok {
		return false
	}
	commands := []string{}
	for event, rawGroups := range hooks {
		groups, _ := rawGroups.([]any)
		for _, rawGroup := range groups {
			group, _ := rawGroup.(map[string]any)
			entries, _ := group["hooks"].([]any)
			for _, rawEntry := range entries {
				entry, _ := rawEntry.(map[string]any)
				if command, ok := entry["command"].(string); ok && adaptercfg.IsOwnedEventCommand(runtimeName, event, command) {
					commands = append(commands, command)
				}
			}
		}
	}
	if len(commands) == 0 {
		return false
	}
	for _, command := range commands {
		for _, value := range values {
			if !strings.Contains(command, value) {
				return false
			}
		}
	}
	return true
}

func removeEmptyRuntimeConfig(path string) error {
	var config map[string]any
	if err := readJSONStrict(path, &config); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	hooks, ok := config["hooks"].(map[string]any)
	if len(config) != 1 || !ok || len(hooks) != 0 {
		return nil
	}
	return removeRegular(path)
}

func removeRegular(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("refusing to remove non-regular managed path %s", path)
	}
	return os.Remove(path)
}

func readJSONStrict(path string, target any) error {
	body, _, err := readBoundedRegular(path, maximumJSONBytes)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("JSON contains trailing content")
		}
		return err
	}
	return nil
}

func captureFiles(paths []string) ([]fileSnapshot, error) {
	unique := map[string]bool{}
	snapshots := []fileSnapshot{}
	var total int64
	for _, path := range paths {
		if unique[path] {
			continue
		}
		unique[path] = true
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			snapshots = append(snapshots, fileSnapshot{path: path})
			continue
		}
		if err != nil {
			return nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return nil, fmt.Errorf("refusing to snapshot non-regular managed path %s", path)
		}
		if info.Size() > maximumExcludeBytes || total+info.Size() > maximumSnapshotBytes {
			return nil, fmt.Errorf("managed transaction snapshot exceeds its bounded size at %s", path)
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		total += int64(len(body))
		snapshots = append(snapshots, fileSnapshot{path: path, exists: true, mode: info.Mode().Perm(), body: body})
	}
	return snapshots, nil
}

func readBoundedRegular(path string, limit int64) ([]byte, os.FileMode, error) {
	if err := rejectExistingParentSymlinks(path); err != nil {
		return nil, 0, err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, 0, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Size() > limit {
		return nil, 0, fmt.Errorf("bounded authority must be a regular non-symlink file: %s", path)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer file.Close()
	body, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, 0, err
	}
	if int64(len(body)) > limit {
		return nil, 0, fmt.Errorf("bounded authority exceeds its limit: %s", path)
	}
	return body, info.Mode().Perm(), nil
}

func restoreFiles(snapshots []fileSnapshot) error {
	var restoreErrors []error
	for index := len(snapshots) - 1; index >= 0; index-- {
		snapshot := snapshots[index]
		if !snapshot.exists {
			if err := os.Remove(snapshot.path); err != nil && !errors.Is(err, os.ErrNotExist) {
				restoreErrors = append(restoreErrors, err)
			}
			continue
		}
		if err := writeBytesAtomic(snapshot.path, snapshot.body, snapshot.mode); err != nil {
			restoreErrors = append(restoreErrors, err)
		}
	}
	return errors.Join(restoreErrors...)
}

func writeJSONAtomic(path string, value any, mode os.FileMode) error {
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return writeBytesAtomic(path, append(body, '\n'), mode)
}

func writeBytesAtomic(path string, body []byte, mode os.FileMode) error {
	if err := rejectExistingParentSymlinks(path); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return fmt.Errorf("refusing to replace non-regular path %s", path)
		}
		mode = info.Mode().Perm()
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".maestro-workspace-*.tmp")
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

func rejectExistingParentSymlinks(path string) error {
	absolute, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return err
	}
	parent := filepath.Dir(absolute)
	volume := filepath.VolumeName(parent)
	current := string(filepath.Separator)
	remaining := strings.TrimPrefix(parent, current)
	if volume != "" {
		current = volume + string(filepath.Separator)
		remaining = strings.TrimPrefix(strings.TrimPrefix(parent, volume), string(filepath.Separator))
	}
	for _, component := range strings.Split(remaining, string(filepath.Separator)) {
		if component == "" || component == "." {
			continue
		}
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("managed authority parent must be a real directory: %s", current)
		}
	}
	return nil
}

func (manager Manager) fail(point string) error {
	if manager.FailurePoint == nil {
		return nil
	}
	return manager.FailurePoint(point)
}

// Runtime-specific executable suffix is exposed only to keep packaging and
// tests consistent without teaching callers platform path rules.
func ExecutableName() string {
	if runtime.GOOS == "windows" {
		return "bcgos.exe"
	}
	return "bcgos"
}
