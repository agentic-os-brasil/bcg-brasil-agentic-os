// Package runtimeprojection materializes the human-readable Maestro operating
// guide and the installed product skills into a runtime workspace. It owns
// only files marked by its manifest or managed orientation block.
package runtimeprojection

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
	"sort"
	"strings"

	baseagents "github.com/agentic-os-brasil/bcg-brasil-agentic-os/bundles/base/agents"
	baseruntime "github.com/agentic-os-brasil/bcg-brasil-agentic-os/bundles/base/runtime"
	baseskills "github.com/agentic-os-brasil/bcg-brasil-agentic-os/bundles/base/skills"
	bundlecatalog "github.com/agentic-os-brasil/bcg-brasil-agentic-os/bundles/catalog"
	techcoreskills "github.com/agentic-os-brasil/bcg-brasil-agentic-os/bundles/tech-core/skills"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/capabilitybundle"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/skillpolicy"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/skillrouting"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/skillsindex"
)

const (
	SchemaVersion                   = 1
	OrientationBegin                = "<!-- BCGOS:MAESTRO-ORIENTATION:BEGIN -->"
	OrientationEnd                  = "<!-- BCGOS:MAESTRO-ORIENTATION:END -->"
	ManifestRelativePath            = ".bcgos/runtime-projection.json"
	PolicyRelativePath              = ".bcgos/agent-skill-policy.json"
	OrientationModeManaged          = "managed_block"
	OrientationModePreservedTracked = "preserved_tracked"
	OrientationOriginCreated        = "created"
	OrientationOriginExisting       = "existing"
	maximumProjectionFileBytes      = 8 << 20
	maximumProjectionSnapshotBytes  = 64 << 20
)

type Status struct {
	Runtime         string   `json:"runtime"`
	State           string   `json:"state"`
	OrientationMode string   `json:"orientation_mode,omitempty"`
	OrientationPath string   `json:"orientation_path"`
	SkillsRoot      string   `json:"skills_root"`
	ManifestPath    string   `json:"manifest_path"`
	PolicyPath      string   `json:"policy_path"`
	SkillCount      int      `json:"skill_count"`
	Conflicts       []string `json:"conflicts,omitempty"`
	Reason          string   `json:"reason,omitempty"`
}

type manifest struct {
	SchemaVersion     int               `json:"schema_version"`
	Runtime           string            `json:"runtime"`
	OrientationPath   string            `json:"orientation_path"`
	OrientationHash   string            `json:"orientation_hash"`
	OrientationMode   string            `json:"orientation_mode,omitempty"`
	OrientationOrigin string            `json:"orientation_origin,omitempty"`
	SkillHashes       map[string]string `json:"skill_hashes"`
	PolicyPath        string            `json:"policy_path,omitempty"`
	PolicyHash        string            `json:"policy_hash,omitempty"`
}

type fileSnapshot struct {
	path   string
	exists bool
	mode   os.FileMode
	body   []byte
}

type runtimeLayout struct {
	orientation string
	root        string
	runtimeName string
}

type canonicalProjectionContract struct {
	Catalog      skillsindex.Catalog
	Policy       skillpolicy.Policy
	SkillHashes  map[string]string
	PolicyBody   []byte
	PolicyDigest string
}

type projectionPaths struct {
	manifest string
	policy   string
}

var legacyProjectionPaths = projectionPaths{manifest: ManifestRelativePath, policy: PolicyRelativePath}

// ScopedManifestRelativePath returns the runtime-owned manifest used by the
// direct-worktree enrollment flow. The legacy projection API intentionally
// keeps its original single-runtime paths for migration and readiness flows.
func ScopedManifestRelativePath(runtimeName string) (string, error) {
	paths, err := scopedProjectionPaths(runtimeName)
	return paths.manifest, err
}

// ScopedPolicyRelativePath returns the runtime-owned selection policy used by
// the direct-worktree enrollment flow.
func ScopedPolicyRelativePath(runtimeName string) (string, error) {
	paths, err := scopedProjectionPaths(runtimeName)
	return paths.policy, err
}

func scopedProjectionPaths(runtimeName string) (projectionPaths, error) {
	if runtimeName != "claude" && runtimeName != "codex" {
		return projectionPaths{}, errors.New("runtime must be claude or codex")
	}
	return projectionPaths{
		manifest: filepath.ToSlash(filepath.Join(".bcgos", "runtime-projections", runtimeName+".json")),
		policy:   filepath.ToSlash(filepath.Join(".bcgos", "agent-skill-policies", runtimeName+".json")),
	}, nil
}

// ValidateInstall performs the projection preflight without writing files.
// It is used by the CLI to coordinate projection and adapter configuration.
func ValidateInstall(runtimeName, workspace string) error {
	return ValidateInstallForTracks(runtimeName, workspace, nil)
}

func ValidateInstallForTracks(runtimeName, workspace string, tracks []string) error {
	return validateInstallForTracks(runtimeName, workspace, tracks, true, legacyProjectionPaths)
}

// ValidateInstallWithoutOrientation is the direct-worktree preflight used
// when the runtime orientation file is already tracked by Git and must remain
// byte-for-byte user-owned.
func ValidateInstallWithoutOrientation(runtimeName, workspace string) error {
	return validateInstallForTracks(runtimeName, workspace, nil, false, legacyProjectionPaths)
}

// ValidateScopedInstall performs the direct-worktree preflight using a
// runtime-owned manifest and policy so Claude and Codex can coexist.
func ValidateScopedInstall(runtimeName, workspace string) error {
	paths, err := scopedProjectionPaths(runtimeName)
	if err != nil {
		return err
	}
	return validateInstallForTracks(runtimeName, workspace, nil, true, paths)
}

func ValidateScopedInstallWithoutOrientation(runtimeName, workspace string) error {
	paths, err := scopedProjectionPaths(runtimeName)
	if err != nil {
		return err
	}
	return validateInstallForTracks(runtimeName, workspace, nil, false, paths)
}

// ValidateExistingInstall reconciles using the orientation mode pinned in the
// current manifest. It is used to distinguish an intact prior core from local
// co-tamper during explicit repair.
func ValidateExistingInstall(runtimeName, workspace string) error {
	return validateExistingInstall(runtimeName, workspace, legacyProjectionPaths)
}

func ValidateScopedExistingInstall(runtimeName, workspace string) error {
	paths, err := scopedProjectionPaths(runtimeName)
	if err != nil {
		return err
	}
	return validateExistingInstall(runtimeName, workspace, paths)
}

func validateExistingInstall(runtimeName, workspace string, paths projectionPaths) error {
	current, err := readManifestForPolicy(filepath.Join(workspace, paths.manifest), paths.policy)
	if err != nil {
		return err
	}
	return validateInstallForTracks(runtimeName, workspace, nil, orientationManaged(current), paths)
}

func validateInstallForTracks(runtimeName, workspace string, tracks []string, manageOrientation bool, paths projectionPaths) error {
	layout, err := layout(runtimeName, workspace)
	if err != nil {
		return err
	}
	for _, relative := range []string{layout.orientation, layout.root, paths.manifest, paths.policy} {
		if err := rejectSymlinkComponents(workspace, relative); err != nil {
			return err
		}
	}
	catalog, err := catalogForTracks(tracks)
	if err != nil {
		return fmt.Errorf("load managed skills catalog: %w", err)
	}
	contents, hashes, err := skillContents(catalog)
	if err != nil {
		return err
	}
	policyBody, err := policyBodyForTracks(tracks)
	if err != nil {
		return err
	}
	old, err := readManifestForPolicy(filepath.Join(workspace, paths.manifest), paths.policy)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if old.Runtime != "" && old.Runtime != runtimeName {
		return fmt.Errorf("workspace already has a %s runtime projection", old.Runtime)
	}
	if conflicts := preflight(workspace, layout, contents, hashes, digest(policyBody), old, manageOrientation, paths); len(conflicts) > 0 {
		return fmt.Errorf("runtime projection has conflicts: %s", strings.Join(conflicts, ", "))
	}
	return nil
}

// PlannedManagedPaths returns the bounded runtime-projection targets for a
// track selection without reading or writing their contents. Callers that
// coordinate a larger transaction use it to snapshot both the current and
// prospective managed skill set before projection starts.
func PlannedManagedPaths(runtimeName, workspace string, tracks []string) ([]string, error) {
	return plannedManagedPaths(runtimeName, workspace, tracks, true, legacyProjectionPaths)
}

func PlannedManagedPathsWithoutOrientation(runtimeName, workspace string, tracks []string) ([]string, error) {
	return plannedManagedPaths(runtimeName, workspace, tracks, false, legacyProjectionPaths)
}

func PlannedScopedManagedPaths(runtimeName, workspace string, tracks []string) ([]string, error) {
	paths, err := scopedProjectionPaths(runtimeName)
	if err != nil {
		return nil, err
	}
	return plannedManagedPaths(runtimeName, workspace, tracks, true, paths)
}

func PlannedScopedManagedPathsWithoutOrientation(runtimeName, workspace string, tracks []string) ([]string, error) {
	paths, err := scopedProjectionPaths(runtimeName)
	if err != nil {
		return nil, err
	}
	return plannedManagedPaths(runtimeName, workspace, tracks, false, paths)
}

func plannedManagedPaths(runtimeName, workspace string, tracks []string, manageOrientation bool, paths projectionPaths) ([]string, error) {
	layout, err := layout(runtimeName, workspace)
	if err != nil {
		return nil, err
	}
	for _, relative := range []string{layout.orientation, layout.root, paths.manifest, paths.policy} {
		if err := rejectSymlinkComponents(workspace, relative); err != nil {
			return nil, err
		}
	}
	catalog, err := catalogForTracks(tracks)
	if err != nil {
		return nil, err
	}
	managedPaths := []string{
		filepath.Join(workspace, paths.manifest),
		filepath.Join(workspace, paths.policy),
	}
	if manageOrientation {
		managedPaths = append(managedPaths, filepath.Join(workspace, layout.orientation))
	}
	for _, skill := range catalog.Skills {
		managedPaths = append(managedPaths, filepath.Join(workspace, layout.root, skill.ID, "SKILL.md"))
	}
	sort.Strings(managedPaths)
	return managedPaths, nil
}

// InstalledManagedPaths returns the exact manifest-owned projection files for
// transaction coordinators. It includes retired skills from an older intact
// projection so a later adapter failure can restore the complete prior view.
func InstalledManagedPaths(runtimeName, workspace string) ([]string, error) {
	return installedManagedPaths(runtimeName, workspace, legacyProjectionPaths)
}

func InstalledScopedManagedPaths(runtimeName, workspace string) ([]string, error) {
	paths, err := scopedProjectionPaths(runtimeName)
	if err != nil {
		return nil, err
	}
	return installedManagedPaths(runtimeName, workspace, paths)
}

func installedManagedPaths(runtimeName, workspace string, paths projectionPaths) ([]string, error) {
	layout, err := layout(runtimeName, workspace)
	if err != nil {
		return nil, err
	}
	current, err := readManifestForPolicy(filepath.Join(workspace, paths.manifest), paths.policy)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if current.Runtime != runtimeName {
		return nil, fmt.Errorf("runtime projection manifest belongs to %s", current.Runtime)
	}
	managedPaths := []string{
		filepath.Join(workspace, paths.manifest),
		filepath.Join(workspace, paths.policy),
	}
	if orientationManaged(current) {
		managedPaths = append(managedPaths, filepath.Join(workspace, layout.orientation))
	}
	for id := range current.SkillHashes {
		if strings.TrimSpace(id) == "" || id == "." || id == ".." || strings.ContainsAny(id, `/\\`) {
			return nil, fmt.Errorf("runtime projection contains unsafe skill identity %q", id)
		}
		path := filepath.Join(workspace, layout.root, id, "SKILL.md")
		if err := rejectSymlinkComponents(workspace, filepath.Join(layout.root, id, "SKILL.md")); err != nil {
			return nil, err
		}
		managedPaths = append(managedPaths, path)
	}
	sort.Strings(managedPaths)
	return managedPaths, nil
}

// ValidateUninstall performs the projection preflight without removing files.
func ValidateUninstall(runtimeName, workspace string) error {
	return validateUninstall(runtimeName, workspace, legacyProjectionPaths)
}

func ValidateScopedUninstall(runtimeName, workspace string) error {
	paths, err := scopedProjectionPaths(runtimeName)
	if err != nil {
		return err
	}
	return validateUninstall(runtimeName, workspace, paths)
}

func validateUninstall(runtimeName, workspace string, paths projectionPaths) error {
	status, err := inspect(runtimeName, workspace, paths)
	if err != nil {
		return err
	}
	if status.State == "conflict" {
		return fmt.Errorf("runtime projection has conflicts: %s", strings.Join(status.Conflicts, ", "))
	}
	return nil
}

func Install(runtimeName, workspace string) (Status, error) {
	return InstallForTracks(runtimeName, workspace, nil)
}

func InstallForTracks(runtimeName, workspace string, tracks []string) (Status, error) {
	return installForTracks(runtimeName, workspace, tracks, true, legacyProjectionPaths)
}

// InstallWithoutOrientation projects governed skills and policy while leaving
// an already-tracked CLAUDE.md or AGENTS.md byte-for-byte user-owned.
func InstallWithoutOrientation(runtimeName, workspace string) (Status, error) {
	return installForTracks(runtimeName, workspace, nil, false, legacyProjectionPaths)
}

func InstallScoped(runtimeName, workspace string) (Status, error) {
	paths, err := scopedProjectionPaths(runtimeName)
	if err != nil {
		return Status{}, err
	}
	return installForTracks(runtimeName, workspace, nil, true, paths)
}

func InstallScopedWithoutOrientation(runtimeName, workspace string) (Status, error) {
	paths, err := scopedProjectionPaths(runtimeName)
	if err != nil {
		return Status{}, err
	}
	return installForTracks(runtimeName, workspace, nil, false, paths)
}

func installForTracks(runtimeName, workspace string, tracks []string, manageOrientation bool, paths projectionPaths) (Status, error) {
	layout, err := layout(runtimeName, workspace)
	if err != nil {
		return Status{}, err
	}
	for _, relative := range []string{layout.orientation, layout.root, paths.manifest, paths.policy} {
		if err := rejectSymlinkComponents(workspace, relative); err != nil {
			return Status{}, err
		}
	}
	catalog, err := catalogForTracks(tracks)
	if err != nil {
		return Status{}, fmt.Errorf("load managed skills catalog: %w", err)
	}
	contents, hashes, err := skillContents(catalog)
	if err != nil {
		return Status{}, err
	}
	policyBody, err := policyBodyForTracks(tracks)
	if err != nil {
		return Status{}, err
	}
	policyHash := digest(policyBody)
	orientation, err := renderOrientation(layout, catalog)
	if err != nil {
		return Status{}, err
	}
	old, err := readManifestForPolicy(filepath.Join(workspace, paths.manifest), paths.policy)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return Status{}, err
	}
	if old.Runtime != "" && old.Runtime != runtimeName {
		return Status{}, fmt.Errorf("workspace already has a %s runtime projection", old.Runtime)
	}
	orientationOrigin := old.OrientationOrigin
	if manageOrientation && old.Runtime == "" {
		if _, statErr := os.Lstat(filepath.Join(workspace, layout.orientation)); errors.Is(statErr, os.ErrNotExist) {
			orientationOrigin = OrientationOriginCreated
		} else if statErr == nil {
			orientationOrigin = OrientationOriginExisting
		} else {
			return Status{}, statErr
		}
	}
	if conflicts := preflight(workspace, layout, contents, hashes, policyHash, old, manageOrientation, paths); len(conflicts) > 0 {
		return Status{Runtime: runtimeName, State: "conflict", OrientationPath: filepath.Join(workspace, layout.orientation), SkillsRoot: filepath.Join(workspace, layout.root), ManifestPath: filepath.Join(workspace, paths.manifest), PolicyPath: filepath.Join(workspace, paths.policy), SkillCount: len(contents), Conflicts: conflicts, Reason: "existing user files were preserved; no projection files were changed"}, fmt.Errorf("runtime projection has conflicts: %s", strings.Join(conflicts, ", "))
	}
	managedPaths := []string{filepath.Join(workspace, paths.manifest), filepath.Join(workspace, paths.policy)}
	if manageOrientation {
		managedPaths = append(managedPaths, filepath.Join(workspace, layout.orientation))
	}
	for id := range contents {
		managedPaths = append(managedPaths, filepath.Join(workspace, layout.root, id, "SKILL.md"))
	}
	for id := range old.SkillHashes {
		if _, current := contents[id]; !current {
			managedPaths = append(managedPaths, filepath.Join(workspace, layout.root, id, "SKILL.md"))
		}
	}
	snapshots, err := snapshotFiles(managedPaths)
	if err != nil {
		return Status{}, err
	}
	rollback := func(cause error) error {
		if restoreErr := restoreFiles(snapshots); restoreErr != nil {
			return fmt.Errorf("%w (projection rollback failed: %v)", cause, restoreErr)
		}
		return cause
	}
	if manageOrientation {
		if err := writeOrientation(filepath.Join(workspace, layout.orientation), orientation); err != nil {
			return Status{}, rollback(err)
		}
	}
	for id, body := range contents {
		if err := writeManagedFile(filepath.Join(workspace, layout.root, id, "SKILL.md"), body); err != nil {
			return Status{}, rollback(fmt.Errorf("write installed skill %s: %w", id, err))
		}
	}
	if err := writeManagedFile(filepath.Join(workspace, paths.policy), policyBody); err != nil {
		return Status{}, rollback(fmt.Errorf("write selection-scoped skill policy: %w", err))
	}
	for id, expected := range old.SkillHashes {
		if _, current := contents[id]; current {
			continue
		}
		path := filepath.Join(workspace, layout.root, id, "SKILL.md")
		body, readErr := readProjectionFile(path)
		if readErr == nil && digest(body) == expected {
			if err := os.Remove(path); err != nil {
				return Status{}, rollback(fmt.Errorf("remove retired managed skill %s: %w", id, err))
			}
			_ = os.Remove(filepath.Dir(path))
		}
	}
	newManifest := manifest{SchemaVersion: SchemaVersion, Runtime: runtimeName, OrientationPath: layout.orientation, OrientationHash: orientationDigest(orientation), OrientationMode: OrientationModeManaged, OrientationOrigin: orientationOrigin, SkillHashes: hashes, PolicyPath: paths.policy, PolicyHash: policyHash}
	if !manageOrientation {
		newManifest.OrientationHash = ""
		newManifest.OrientationMode = OrientationModePreservedTracked
		newManifest.OrientationOrigin = ""
	}
	if err := writeJSON(filepath.Join(workspace, paths.manifest), newManifest); err != nil {
		return Status{}, rollback(fmt.Errorf("write runtime projection manifest: %w", err))
	}
	return Status{Runtime: runtimeName, State: "installed", OrientationPath: filepath.Join(workspace, layout.orientation), SkillsRoot: filepath.Join(workspace, layout.root), ManifestPath: filepath.Join(workspace, paths.manifest), PolicyPath: filepath.Join(workspace, paths.policy), SkillCount: len(contents)}, nil
}

func Inspect(runtimeName, workspace string) (Status, error) {
	return inspect(runtimeName, workspace, legacyProjectionPaths)
}

func InspectScoped(runtimeName, workspace string) (Status, error) {
	paths, err := scopedProjectionPaths(runtimeName)
	if err != nil {
		return Status{}, err
	}
	return inspect(runtimeName, workspace, paths)
}

func inspect(runtimeName, workspace string, paths projectionPaths) (Status, error) {
	layout, err := layout(runtimeName, workspace)
	if err != nil {
		return Status{}, err
	}
	for _, relative := range []string{layout.orientation, layout.root, paths.manifest, paths.policy} {
		if err := rejectSymlinkComponents(workspace, relative); err != nil {
			return Status{}, err
		}
	}
	path := filepath.Join(workspace, paths.manifest)
	current, err := readManifestForPolicy(path, paths.policy)
	if errors.Is(err, os.ErrNotExist) {
		return Status{Runtime: runtimeName, State: "absent", OrientationPath: filepath.Join(workspace, layout.orientation), SkillsRoot: filepath.Join(workspace, layout.root), ManifestPath: path}, nil
	}
	if err != nil {
		return Status{}, err
	}
	if current.Runtime != runtimeName {
		return Status{}, fmt.Errorf("runtime projection manifest belongs to %s", current.Runtime)
	}
	canonical, canonicalErr := canonicalProjection(current)
	conflicts := projectionConflicts(workspace, layout, current, canonical, canonicalErr, paths)
	policyPath := filepath.Join(workspace, paths.policy)
	sort.Strings(conflicts)
	state := "installed"
	if len(conflicts) > 0 {
		state = "conflict"
	}
	return Status{Runtime: runtimeName, State: state, OrientationMode: current.OrientationMode, OrientationPath: filepath.Join(workspace, layout.orientation), SkillsRoot: filepath.Join(workspace, layout.root), ManifestPath: path, PolicyPath: policyPath, SkillCount: len(current.SkillHashes), Conflicts: conflicts}, nil
}

// SessionOrientation returns the canonical path-free orientation only when a
// tracked runtime instruction file forced enrollment to preserve that file
// byte-for-byte. The caller can then deliver the missing managed orientation
// through SessionStart without duplicating an installed managed block.
func SessionOrientation(runtimeName, workspace string) (string, bool, error) {
	paths, err := scopedProjectionPaths(runtimeName)
	if err != nil {
		return "", false, err
	}
	status, err := inspect(runtimeName, workspace, paths)
	if err != nil {
		return "", false, err
	}
	if status.State != "installed" {
		return "", false, fmt.Errorf("runtime projection is %s", status.State)
	}
	if status.OrientationMode != OrientationModePreservedTracked {
		return "", false, nil
	}
	current, err := readManifestForPolicy(filepath.Join(workspace, paths.manifest), paths.policy)
	if err != nil {
		return "", false, err
	}
	canonical, err := canonicalProjection(current)
	if err != nil {
		return "", false, err
	}
	layout, err := layout(runtimeName, workspace)
	if err != nil {
		return "", false, err
	}
	orientation, err := renderOrientation(layout, canonical.Catalog)
	if err != nil {
		return "", false, err
	}
	return orientation, true, nil
}

// RoutingInputs returns only integrity-checked installed methods, their
// governed selection policy, and runtime-relative pointers. Skill bodies are
// deliberately not returned to the prompt hook.
func RoutingInputs(runtimeName, workspace string) (skillsindex.Catalog, skillpolicy.Policy, []skillrouting.InstalledSkill, error) {
	status, err := Inspect(runtimeName, workspace)
	if err != nil {
		return skillsindex.Catalog{}, skillpolicy.Policy{}, nil, err
	}
	if status.State != "installed" {
		return skillsindex.Catalog{}, skillpolicy.Policy{}, nil, fmt.Errorf("runtime projection is %s", status.State)
	}
	current, err := readManifest(filepath.Join(workspace, ManifestRelativePath))
	if err != nil {
		return skillsindex.Catalog{}, skillpolicy.Policy{}, nil, err
	}
	layout, err := layout(runtimeName, workspace)
	if err != nil {
		return skillsindex.Catalog{}, skillpolicy.Policy{}, nil, err
	}
	canonical, err := canonicalProjection(current)
	if err != nil {
		return skillsindex.Catalog{}, skillpolicy.Policy{}, nil, err
	}
	if conflicts := projectionConflicts(workspace, layout, current, canonical, nil, legacyProjectionPaths); len(conflicts) > 0 {
		return skillsindex.Catalog{}, skillpolicy.Policy{}, nil, fmt.Errorf("runtime projection failed embedded integrity reconciliation: %s", strings.Join(conflicts, ", "))
	}
	installed := make([]skillrouting.InstalledSkill, 0, len(canonical.Catalog.Skills))
	for _, skill := range canonical.Catalog.Skills {
		installed = append(installed, skillrouting.InstalledSkill{ID: skill.ID, Pointer: filepath.ToSlash(filepath.Join(layout.root, skill.ID, "SKILL.md"))})
	}
	return canonical.Catalog, canonical.Policy, installed, nil
}

func Uninstall(runtimeName, workspace string) (Status, error) {
	return uninstall(runtimeName, workspace, legacyProjectionPaths)
}

func UninstallScoped(runtimeName, workspace string) (Status, error) {
	paths, err := scopedProjectionPaths(runtimeName)
	if err != nil {
		return Status{}, err
	}
	return uninstall(runtimeName, workspace, paths)
}

func uninstall(runtimeName, workspace string, paths projectionPaths) (Status, error) {
	layout, err := layout(runtimeName, workspace)
	if err != nil {
		return Status{}, err
	}
	for _, relative := range []string{layout.orientation, layout.root, paths.manifest, paths.policy} {
		if err := rejectSymlinkComponents(workspace, relative); err != nil {
			return Status{}, err
		}
	}
	manifestPath := filepath.Join(workspace, paths.manifest)
	current, err := readManifestForPolicy(manifestPath, paths.policy)
	if errors.Is(err, os.ErrNotExist) {
		return Status{Runtime: runtimeName, State: "absent", OrientationPath: filepath.Join(workspace, layout.orientation), SkillsRoot: filepath.Join(workspace, layout.root), ManifestPath: manifestPath}, nil
	}
	if err != nil {
		return Status{}, err
	}
	if current.Runtime != runtimeName {
		return Status{}, fmt.Errorf("runtime projection manifest belongs to %s", current.Runtime)
	}
	for id := range current.SkillHashes {
		if err := rejectSymlinkComponents(workspace, filepath.Join(layout.root, id, "SKILL.md")); err != nil {
			return Status{Runtime: runtimeName, State: "conflict", OrientationPath: filepath.Join(workspace, layout.orientation), SkillsRoot: filepath.Join(workspace, layout.root), ManifestPath: manifestPath, SkillCount: len(current.SkillHashes), Conflicts: []string{filepath.Join(layout.root, id, "SKILL.md")}, Reason: "managed skill path contains a symlink; no projection files were removed"}, err
		}
	}
	orientationPath := filepath.Join(workspace, layout.orientation)
	policyPath := filepath.Join(workspace, paths.policy)
	managedPaths := []string{manifestPath, policyPath}
	if orientationManaged(current) {
		managedPaths = append(managedPaths, orientationPath)
	}
	for id := range current.SkillHashes {
		managedPaths = append(managedPaths, filepath.Join(workspace, layout.root, id, "SKILL.md"))
	}
	snapshots, err := snapshotFiles(managedPaths)
	if err != nil {
		return Status{}, err
	}
	rollback := func(cause error) error {
		if restoreErr := restoreFiles(snapshots); restoreErr != nil {
			return fmt.Errorf("%w (projection rollback failed: %v)", cause, restoreErr)
		}
		return cause
	}
	var orientation []byte
	if orientationManaged(current) {
		orientation, err = readProjectionFile(orientationPath)
		if err != nil {
			return Status{}, rollback(err)
		}
		if !orientationMatchesManifest(string(orientation), current.OrientationHash) {
			return Status{Runtime: runtimeName, State: "conflict", OrientationPath: orientationPath, SkillsRoot: filepath.Join(workspace, layout.root), ManifestPath: manifestPath, SkillCount: len(current.SkillHashes), Conflicts: []string{layout.orientation}, Reason: "managed orientation was changed or its markers are missing; no projection files were removed"}, errors.New("managed orientation was changed or its markers are missing")
		}
	}
	var conflicts []string
	for id, expected := range current.SkillHashes {
		path := filepath.Join(workspace, layout.root, id, "SKILL.md")
		body, readErr := readProjectionFile(path)
		if readErr != nil || digest(body) != expected {
			conflicts = append(conflicts, path)
		}
	}
	if current.PolicyPath != paths.policy || current.PolicyHash == "" {
		conflicts = append(conflicts, policyPath)
	} else if body, readErr := readProjectionFile(policyPath); readErr != nil || digest(body) != current.PolicyHash {
		conflicts = append(conflicts, policyPath)
	}
	if len(conflicts) > 0 {
		sort.Strings(conflicts)
		return Status{Runtime: runtimeName, State: "conflict", OrientationPath: orientationPath, SkillsRoot: filepath.Join(workspace, layout.root), ManifestPath: manifestPath, SkillCount: len(current.SkillHashes), Conflicts: conflicts, Reason: "modified skill files were preserved"}, errors.New("modified managed skill files were preserved")
	}
	if orientationManaged(current) {
		updated, err := removeOrientationBlock(string(orientation))
		if err != nil {
			return Status{}, rollback(err)
		}
		removeOrientation := current.OrientationOrigin == OrientationOriginCreated && updated == ""
		if current.OrientationOrigin == "" && strings.TrimSpace(updated) == "" {
			removeOrientation = true
		}
		if removeOrientation {
			if err := os.Remove(orientationPath); err != nil && !errors.Is(err, os.ErrNotExist) {
				return Status{}, rollback(err)
			}
		} else if err := writeManagedFile(orientationPath, []byte(updated)); err != nil {
			return Status{}, rollback(err)
		}
	}
	for id := range current.SkillHashes {
		path := filepath.Join(workspace, layout.root, id, "SKILL.md")
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return Status{}, rollback(err)
		}
		_ = os.Remove(filepath.Dir(path))
	}
	_ = os.Remove(filepath.Join(workspace, layout.root))
	if err := os.Remove(policyPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return Status{}, rollback(err)
	}
	if err := os.Remove(manifestPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return Status{}, rollback(err)
	}
	return Status{Runtime: runtimeName, State: "removed", OrientationPath: orientationPath, SkillsRoot: filepath.Join(workspace, layout.root), ManifestPath: manifestPath, PolicyPath: policyPath, SkillCount: len(current.SkillHashes)}, nil
}

func layout(runtimeName, workspace string) (runtimeLayout, error) {
	if runtimeName != "claude" && runtimeName != "codex" {
		return runtimeLayout{}, fmt.Errorf("unsupported runtime %q", runtimeName)
	}
	if strings.TrimSpace(workspace) == "" {
		return runtimeLayout{}, errors.New("workspace is required")
	}
	absolute, err := filepath.Abs(workspace)
	if err != nil {
		return runtimeLayout{}, err
	}
	if info, err := os.Lstat(absolute); errors.Is(err, os.ErrNotExist) {
		// The adapter is also a supported installation entry point. A new
		// workspace may not exist yet, but every existing ancestor must already
		// be a canonical directory so creation cannot be redirected through a
		// symlink. bootstrapAdapterDependencies creates the final path later.
		if err := validateMissingWorkspaceAncestors(absolute); err != nil {
			return runtimeLayout{}, err
		}
	} else if err != nil {
		return runtimeLayout{}, err
	} else if info.Mode()&os.ModeSymlink != 0 {
		return runtimeLayout{}, errors.New("workspace must not be a symlink")
	} else if !info.IsDir() {
		return runtimeLayout{}, errors.New("workspace must be a directory")
	}
	if runtimeName == "claude" {
		return runtimeLayout{orientation: "CLAUDE.md", root: filepath.Join(".claude", "skills"), runtimeName: "Claude Code"}, nil
	}
	return runtimeLayout{orientation: "AGENTS.md", root: filepath.Join(".codex", "skills"), runtimeName: "Codex"}, nil
}

func validateMissingWorkspaceAncestors(path string) error {
	for current := path; ; current = filepath.Dir(current) {
		info, err := os.Lstat(current)
		if err == nil {
			if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
				return errors.New("workspace parent must be a canonical non-symlink directory")
			}
			return nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return errors.New("workspace parent does not exist")
		}
	}
}

func rejectSymlinkComponents(workspace, relative string) error {
	current := workspace
	for _, component := range strings.Split(filepath.Clean(relative), string(filepath.Separator)) {
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
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing to follow symlink in managed path %s", current)
		}
	}
	return nil
}

func skillContents(catalog skillsindex.Catalog) (map[string][]byte, map[string]string, error) {
	contents := make(map[string][]byte, len(catalog.Skills))
	hashes := make(map[string]string, len(catalog.Skills))
	for _, skill := range catalog.Skills {
		body, err := skillBody(skill.ID)
		if err != nil {
			return nil, nil, err
		}
		contents[skill.ID] = body
		hashes[skill.ID] = digest(body)
	}
	return contents, hashes, nil
}

func canonicalProjection(current manifest) (canonicalProjectionContract, error) {
	base, err := baseskills.Catalog()
	if err != nil {
		return canonicalProjectionContract{}, err
	}
	tech, err := techcoreskills.Catalog()
	if err != nil {
		return canonicalProjectionContract{}, err
	}
	for index := range base.Skills {
		base.Skills[index].Bundle = "base"
	}
	for index := range tech.Skills {
		tech.Skills[index].Bundle = "tech-core"
	}
	known := make(map[string]skillsindex.Skill, len(base.Skills)+len(tech.Skills))
	for _, skill := range append(base.Skills, tech.Skills...) {
		known[skill.ID] = skill
	}
	active := skillsindex.Catalog{SchemaVersion: base.SchemaVersion}
	hashes := make(map[string]string, len(current.SkillHashes))
	for id := range current.SkillHashes {
		skill, exists := known[id]
		if !exists {
			return canonicalProjectionContract{}, fmt.Errorf("runtime projection contains skill %q outside embedded governed catalogs", id)
		}
		body, err := skillBody(id)
		if err != nil {
			return canonicalProjectionContract{}, err
		}
		active.Skills = append(active.Skills, skill)
		hashes[id] = digest(body)
	}
	sort.Slice(active.Skills, func(left, right int) bool { return active.Skills[left].ID < active.Skills[right].ID })
	if err := active.Validate(); err != nil {
		return canonicalProjectionContract{}, err
	}
	policy, err := policyForCatalog(active)
	if err != nil {
		return canonicalProjectionContract{}, err
	}
	policyBody, err := json.MarshalIndent(policy, "", "  ")
	if err != nil {
		return canonicalProjectionContract{}, err
	}
	policyBody = append(policyBody, '\n')
	return canonicalProjectionContract{Catalog: active, Policy: policy, SkillHashes: hashes, PolicyBody: policyBody, PolicyDigest: digest(policyBody)}, nil
}

func projectionConflicts(workspace string, layout runtimeLayout, current manifest, canonical canonicalProjectionContract, canonicalErr error, paths projectionPaths) []string {
	conflicts := []string{}
	if orientationManaged(current) {
		orientation, err := readProjectionFile(filepath.Join(workspace, layout.orientation))
		if err != nil || !orientationMatchesManifest(string(orientation), current.OrientationHash) {
			conflicts = append(conflicts, layout.orientation)
		}
	}
	if canonicalErr != nil {
		conflicts = append(conflicts, filepath.Join(workspace, paths.manifest))
		sort.Strings(conflicts)
		return conflicts
	}
	for id, expected := range canonical.SkillHashes {
		relative := filepath.Join(layout.root, id, "SKILL.md")
		if err := rejectSymlinkComponents(workspace, relative); err != nil {
			conflicts = append(conflicts, relative)
			continue
		}
		body, readErr := readProjectionFile(filepath.Join(workspace, relative))
		if readErr != nil || current.SkillHashes[id] != expected || digest(body) != expected {
			conflicts = append(conflicts, relative)
		}
	}
	policyPath := filepath.Join(workspace, paths.policy)
	policyBody, policyErr := readProjectionFile(policyPath)
	if current.PolicyPath != paths.policy || current.PolicyHash != canonical.PolicyDigest || policyErr != nil || !bytes.Equal(policyBody, canonical.PolicyBody) {
		conflicts = append(conflicts, policyPath)
	}
	sort.Strings(conflicts)
	return conflicts
}

func catalogForTracks(tracks []string) (skillsindex.Catalog, error) {
	base, err := baseskills.Catalog()
	if err != nil {
		return skillsindex.Catalog{}, fmt.Errorf("load managed skills catalog: %w", err)
	}
	for index := range base.Skills {
		base.Skills[index].Bundle = "base"
	}
	catalog, err := bundlecatalog.Catalog()
	if err != nil {
		return skillsindex.Catalog{}, err
	}
	var plan capabilitybundle.Plan
	if len(tracks) == 0 {
		plan, err = catalog.DefaultPlan()
	} else {
		plan, err = catalog.PlanForTracks(tracks)
	}
	if err != nil {
		return skillsindex.Catalog{}, err
	}
	for _, bundle := range plan.Bundles {
		var optional skillsindex.Catalog
		var loadErr error
		switch bundle.ID {
		case "tech-core":
			optional, loadErr = techcoreskills.Catalog()
		default:
			continue
		}
		if loadErr != nil {
			return skillsindex.Catalog{}, fmt.Errorf("load %s skills catalog: %w", bundle.ID, loadErr)
		}
		for index := range optional.Skills {
			optional.Skills[index].Bundle = bundle.ID
		}
		base.Skills = append(base.Skills, optional.Skills...)
	}
	sort.Slice(base.Skills, func(left, right int) bool { return base.Skills[left].ID < base.Skills[right].ID })
	if err := base.Validate(); err != nil {
		return skillsindex.Catalog{}, fmt.Errorf("validate activated skills catalog: %w", err)
	}
	return base, nil
}

// PolicyForTracks composes the immutable base policy with included methods and
// exactly the optional methods activated by the confirmed track plan. The
// returned policy compiles only against the corresponding active catalog, so
// future unselected bundle methods remain denied even if their source is
// embedded in the release.
func PolicyForTracks(tracks []string) (skillpolicy.Policy, skillsindex.Catalog, error) {
	active, err := catalogForTracks(tracks)
	if err != nil {
		return skillpolicy.Policy{}, skillsindex.Catalog{}, err
	}
	policy, err := policyForCatalog(active)
	if err != nil {
		return skillpolicy.Policy{}, skillsindex.Catalog{}, err
	}
	return policy, active, nil
}

func policyForCatalog(active skillsindex.Catalog) (skillpolicy.Policy, error) {
	base, err := baseskills.Catalog()
	if err != nil {
		return skillpolicy.Policy{}, err
	}
	policy, err := skillpolicy.Parse(bytes.NewReader(baseskills.AgentSkillPolicy()))
	if err != nil {
		return skillpolicy.Policy{}, fmt.Errorf("parse base agent skill policy: %w", err)
	}
	baseIDs := make(map[string]bool, len(base.Skills))
	for _, skill := range base.Skills {
		baseIDs[skill.ID] = true
	}
	optionalIDs := make([]string, 0, len(active.Skills)-len(base.Skills))
	for _, skill := range active.Skills {
		if !baseIDs[skill.ID] {
			optionalIDs = append(optionalIDs, skill.ID)
		}
	}
	policy, err = skillpolicy.ActivateDirect(policy, "case_agent", optionalIDs)
	if err != nil {
		return skillpolicy.Policy{}, err
	}
	qualityIDs := []string{"coverage-diagnose", "pr-quality-loop", "pr-review", "unit-test-wave"}
	activeIDs := make(map[string]bool, len(active.Skills))
	for _, skill := range active.Skills {
		activeIDs[skill.ID] = true
	}
	selectedQualityIDs := make([]string, 0, len(qualityIDs))
	for _, id := range qualityIDs {
		if activeIDs[id] {
			selectedQualityIDs = append(selectedQualityIDs, id)
		}
	}
	policy, err = skillpolicy.ActivateDirect(policy, "quality_guardian", selectedQualityIDs)
	if err != nil {
		return skillpolicy.Policy{}, err
	}
	agents, err := baseagents.Catalog()
	if err != nil {
		return skillpolicy.Policy{}, err
	}
	if _, err := skillpolicy.Compile(policy, active, agents); err != nil {
		return skillpolicy.Policy{}, fmt.Errorf("compile selection-scoped agent skill policy: %w", err)
	}
	return policy, nil
}

func policyBodyForTracks(tracks []string) ([]byte, error) {
	policy, _, err := PolicyForTracks(tracks)
	if err != nil {
		return nil, err
	}
	body, err := json.MarshalIndent(policy, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(body, '\n'), nil
}

func skillBody(id string) ([]byte, error) {
	if body, err := baseskills.Skill(id); err == nil {
		return body, nil
	}
	return techcoreskills.Skill(id)
}

func renderOrientation(layout runtimeLayout, catalog skillsindex.Catalog) (string, error) {
	template := string(baseruntime.OrientationTemplate())
	if !strings.Contains(template, "{{SKILLS_BLOCK}}") || !strings.Contains(template, "{{RUNTIME}}") || !strings.Contains(template, "{{RUNTIME_ID}}") || !strings.Contains(template, "{{SKILL_PREFIX}}") || !strings.Contains(template, "{{RUNTIME_TRUST_GUIDANCE}}") {
		return "", errors.New("orientation template is missing required placeholders")
	}
	skillPrefix := "/"
	trustGuidance := ""
	if runtimeID(layout) == "codex" {
		skillPrefix = "$"
		trustGuidance = "Na primeira abertura, o Codex exige revisão nativa dos hooks locais. Abra `/hooks`, confira que os comandos apontam para o CLI instalado do Maestro e aprove o conjunto antes de depender das rotinas automáticas. Mudanças posteriores nos hooks exigem nova revisão."
	}
	var block strings.Builder
	block.WriteString("<!-- BCGOS:INSTALLED-SKILLS:BEGIN -->\n")
	for _, skill := range catalog.Skills {
		fmt.Fprintf(&block, "- `%s%s` — %s; usar quando: %s; fonte: `%s/%s/SKILL.md`\n", skillPrefix, skill.ID, skill.DisplayName, skill.Trigger, layout.root, skill.ID)
	}
	block.WriteString("<!-- BCGOS:INSTALLED-SKILLS:END -->")
	body := strings.ReplaceAll(template, "{{RUNTIME}}", layout.runtimeName)
	body = strings.ReplaceAll(body, "{{RUNTIME_ID}}", runtimeID(layout))
	body = strings.ReplaceAll(body, "{{SKILL_PREFIX}}", skillPrefix)
	body = strings.ReplaceAll(body, "{{RUNTIME_TRUST_GUIDANCE}}", trustGuidance)
	body = strings.ReplaceAll(body, "{{SKILLS_BLOCK}}", block.String())
	return OrientationBegin + "\n" + strings.TrimSpace(body) + "\n" + OrientationEnd, nil
}

func runtimeID(layout runtimeLayout) string {
	if layout.runtimeName == "Claude Code" {
		return "claude"
	}
	return "codex"
}

func preflight(workspace string, layout runtimeLayout, contents map[string][]byte, hashes map[string]string, policyHash string, old manifest, manageOrientation bool, paths projectionPaths) []string {
	var conflicts []string
	orientationPath := filepath.Join(workspace, layout.orientation)
	if manageOrientation {
		currentOrientation, orientationErr := readProjectionFile(orientationPath)
		if old.Runtime != "" {
			if !orientationManaged(old) || orientationErr != nil || !orientationMatchesManifest(string(currentOrientation), old.OrientationHash) {
				conflicts = append(conflicts, orientationPath)
			}
		} else if orientationErr == nil && (strings.Contains(string(currentOrientation), OrientationBegin) || strings.Contains(string(currentOrientation), OrientationEnd)) {
			conflicts = append(conflicts, orientationPath)
		}
	} else if old.Runtime != "" && orientationManaged(old) {
		conflicts = append(conflicts, orientationPath)
	}
	for id := range contents {
		relative := filepath.Join(layout.root, id, "SKILL.md")
		path := filepath.Join(workspace, relative)
		if err := rejectSymlinkComponents(workspace, relative); err != nil {
			conflicts = append(conflicts, path)
			continue
		}
		if info, err := os.Lstat(path); err == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				conflicts = append(conflicts, path)
				continue
			}
			current, readErr := readProjectionFile(path)
			if readErr != nil {
				conflicts = append(conflicts, path)
				continue
			}
			currentHash := digest(current)
			oldHash := old.SkillHashes[id]
			if currentHash != hashes[id] && (oldHash == "" || currentHash != oldHash) {
				conflicts = append(conflicts, path)
			}
		}
	}
	for id, expected := range old.SkillHashes {
		if _, current := contents[id]; current {
			continue
		}
		relative := filepath.Join(layout.root, id, "SKILL.md")
		path := filepath.Join(workspace, relative)
		if err := rejectSymlinkComponents(workspace, relative); err != nil {
			conflicts = append(conflicts, path)
			continue
		}
		body, err := readProjectionFile(path)
		if err == nil && digest(body) != expected {
			conflicts = append(conflicts, path)
		}
	}
	policyPath := filepath.Join(workspace, paths.policy)
	policyBody, policyErr := readProjectionFile(policyPath)
	if old.Runtime == "" || old.PolicyHash == "" {
		// A legacy projection without policy ownership may be upgraded only when
		// the policy path is absent. An existing file belongs to the user.
		if policyErr == nil || !errors.Is(policyErr, os.ErrNotExist) {
			conflicts = append(conflicts, policyPath)
		}
	} else if old.PolicyPath != paths.policy || policyErr != nil {
		conflicts = append(conflicts, policyPath)
	} else {
		currentHash := digest(policyBody)
		if currentHash != policyHash && currentHash != old.PolicyHash {
			conflicts = append(conflicts, policyPath)
		}
	}
	sort.Strings(conflicts)
	return conflicts
}

func writeOrientation(path, generated string) error {
	if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing to write orientation symlink %s", path)
	}
	current, err := readProjectionFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return writeManagedFile(path, []byte(generated))
	}
	if err != nil {
		return err
	}
	updated, err := replaceOrientationBlock(string(current), generated)
	if err != nil {
		return err
	}
	return writeManagedFile(path, []byte(updated))
}

func replaceOrientationBlock(current, generated string) (string, error) {
	start, end := strings.Index(current, OrientationBegin), strings.Index(current, OrientationEnd)
	if (start == -1) != (end == -1) || (start >= 0 && end < start) {
		return "", errors.New("orientation has incomplete or inverted Maestro markers")
	}
	if start == -1 {
		return current + generated, nil
	}
	end += len(OrientationEnd)
	return current[:start] + generated + current[end:], nil
}

func removeOrientationBlock(current string) (string, error) {
	start, end := strings.Index(current, OrientationBegin), strings.Index(current, OrientationEnd)
	if start == -1 || end < start {
		return "", errors.New("orientation markers are missing")
	}
	end += len(OrientationEnd)
	return current[:start] + current[end:], nil
}

func orientationDigest(current string) string {
	block, ok := orientationBlock(current)
	if !ok {
		return ""
	}
	return digest([]byte(strings.TrimSpace(block)))
}

func orientationMatchesManifest(current, expected string) bool {
	if orientationDigest(current) == expected {
		return true
	}
	// Accept manifests written by the first projection implementation, which
	// hashed the generated block including its trailing newline. The managed
	// block is still required to be intact; user edits do not match either form.
	block, ok := orientationBlock(current)
	return ok && digest([]byte(strings.TrimSpace(block)+"\n")) == expected
}

func orientationBlock(current string) (string, bool) {
	start, end := strings.Index(current, OrientationBegin), strings.Index(current, OrientationEnd)
	if start == -1 || end < start {
		return "", false
	}
	end += len(OrientationEnd)
	return current[start:end], true
}

func readManifest(path string) (manifest, error) {
	return readManifestForPolicy(path, PolicyRelativePath)
}

func readManifestForPolicy(path, expectedPolicyPath string) (manifest, error) {
	body, err := readProjectionFile(path)
	if err != nil {
		return manifest{}, err
	}
	var value manifest
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return manifest{}, fmt.Errorf("decode runtime projection manifest: %w", err)
	}
	if value.SchemaVersion != SchemaVersion || value.Runtime == "" || value.OrientationPath == "" || len(value.SkillHashes) == 0 {
		return manifest{}, errors.New("runtime projection manifest is invalid")
	}
	if value.OrientationMode == "" {
		value.OrientationMode = OrientationModeManaged
	}
	if value.OrientationMode != OrientationModeManaged && value.OrientationMode != OrientationModePreservedTracked {
		return manifest{}, errors.New("runtime projection manifest has an invalid orientation mode")
	}
	if (value.OrientationMode == OrientationModeManaged && value.OrientationHash == "") ||
		(value.OrientationMode == OrientationModePreservedTracked && value.OrientationHash != "") {
		return manifest{}, errors.New("runtime projection manifest has an invalid orientation identity")
	}
	if value.OrientationOrigin != "" && value.OrientationOrigin != OrientationOriginCreated && value.OrientationOrigin != OrientationOriginExisting {
		return manifest{}, errors.New("runtime projection manifest has an invalid orientation origin")
	}
	if value.OrientationMode == OrientationModePreservedTracked && value.OrientationOrigin != "" {
		return manifest{}, errors.New("preserved orientation cannot have a managed origin")
	}
	for id := range value.SkillHashes {
		if filepath.Clean(id) != id || id == "." || id == ".." || strings.ContainsAny(id, `/\\`) {
			return manifest{}, fmt.Errorf("runtime projection manifest contains unsafe skill ID %q", id)
		}
	}
	if (value.PolicyPath == "") != (value.PolicyHash == "") {
		return manifest{}, errors.New("runtime projection manifest has an incomplete skill policy identity")
	}
	if value.PolicyPath != "" {
		if value.PolicyPath != expectedPolicyPath || len(value.PolicyHash) != sha256.Size*2 {
			return manifest{}, errors.New("runtime projection manifest has an invalid skill policy identity")
		}
		if _, err := hex.DecodeString(value.PolicyHash); err != nil || strings.ToLower(value.PolicyHash) != value.PolicyHash {
			return manifest{}, errors.New("runtime projection manifest has an invalid skill policy digest")
		}
	}
	return value, nil
}

func orientationManaged(value manifest) bool {
	return value.OrientationMode == "" || value.OrientationMode == OrientationModeManaged
}

func writeJSON(path string, value any) error {
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	return writeManagedFile(path, body)
}

func snapshotFiles(paths []string) ([]fileSnapshot, error) {
	snapshots := make([]fileSnapshot, 0, len(paths))
	seen := make(map[string]struct{}, len(paths))
	var total int64
	for _, path := range paths {
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			snapshots = append(snapshots, fileSnapshot{path: path})
			continue
		}
		if err != nil {
			return nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Size() > maximumProjectionFileBytes || total+info.Size() > maximumProjectionSnapshotBytes {
			return nil, fmt.Errorf("refusing unbounded or non-regular projection snapshot %s", path)
		}
		body, err := readProjectionFile(path)
		if err != nil {
			return nil, err
		}
		total += int64(len(body))
		snapshots = append(snapshots, fileSnapshot{path: path, exists: true, mode: info.Mode().Perm(), body: body})
	}
	return snapshots, nil
}

func readProjectionFile(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Size() > maximumProjectionFileBytes {
		return nil, fmt.Errorf("projection authority must be a bounded regular non-symlink file: %s", path)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	body, err := io.ReadAll(io.LimitReader(file, maximumProjectionFileBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maximumProjectionFileBytes {
		return nil, fmt.Errorf("projection authority exceeds its limit: %s", path)
	}
	return body, nil
}

func restoreFiles(snapshots []fileSnapshot) error {
	for _, snapshot := range snapshots {
		if !snapshot.exists {
			if err := os.Remove(snapshot.path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
			continue
		}
		if err := writeManagedFile(snapshot.path, snapshot.body); err != nil {
			return err
		}
		if err := os.Chmod(snapshot.path, snapshot.mode); err != nil {
			return err
		}
	}
	return nil
}

func writeManagedFile(path string, body []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	mode := os.FileMode(0o600)
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing to replace symlink %s", path)
		}
		mode = info.Mode().Perm()
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".bcgos-projection-*")
	if err != nil {
		return err
	}
	temporary := file.Name()
	defer os.Remove(temporary)
	if err := file.Chmod(mode); err != nil {
		file.Close()
		return err
	}
	if _, err := file.Write(body); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(temporary, path)
}

func digest(body []byte) string {
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:])
}
