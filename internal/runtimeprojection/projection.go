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
	"os"
	"path/filepath"
	"sort"
	"strings"

	baseagents "github.com/agentic-os-brasil/bcg-brasil-agentic-os/bundles/base/agents"
	baseruntime "github.com/agentic-os-brasil/bcg-brasil-agentic-os/bundles/base/runtime"
	baseskills "github.com/agentic-os-brasil/bcg-brasil-agentic-os/bundles/base/skills"
	bundlecatalog "github.com/agentic-os-brasil/bcg-brasil-agentic-os/bundles/catalog"
	datapracticeskills "github.com/agentic-os-brasil/bcg-brasil-agentic-os/bundles/data-practice/skills"
	engineeringcoreskills "github.com/agentic-os-brasil/bcg-brasil-agentic-os/bundles/engineering-core/skills"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/skillpolicy"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/skillsindex"
)

const (
	SchemaVersion        = 1
	OrientationBegin     = "<!-- BCGOS:MAESTRO-ORIENTATION:BEGIN -->"
	OrientationEnd       = "<!-- BCGOS:MAESTRO-ORIENTATION:END -->"
	ManifestRelativePath = ".bcgos/runtime-projection.json"
	PolicyRelativePath   = ".bcgos/agent-skill-policy.json"
)

type Status struct {
	Runtime         string   `json:"runtime"`
	State           string   `json:"state"`
	OrientationPath string   `json:"orientation_path"`
	SkillsRoot      string   `json:"skills_root"`
	ManifestPath    string   `json:"manifest_path"`
	PolicyPath      string   `json:"policy_path"`
	SkillCount      int      `json:"skill_count"`
	Conflicts       []string `json:"conflicts,omitempty"`
	Reason          string   `json:"reason,omitempty"`
}

type manifest struct {
	SchemaVersion   int               `json:"schema_version"`
	Runtime         string            `json:"runtime"`
	OrientationPath string            `json:"orientation_path"`
	OrientationHash string            `json:"orientation_hash"`
	SkillHashes     map[string]string `json:"skill_hashes"`
	PolicyPath      string            `json:"policy_path,omitempty"`
	PolicyHash      string            `json:"policy_hash,omitempty"`
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

// ValidateInstall performs the projection preflight without writing files.
// It is used by the CLI to coordinate projection and adapter configuration.
func ValidateInstall(runtimeName, workspace string) error {
	return ValidateInstallForTracks(runtimeName, workspace, nil)
}

func ValidateInstallForTracks(runtimeName, workspace string, tracks []string) error {
	layout, err := layout(runtimeName, workspace)
	if err != nil {
		return err
	}
	for _, relative := range []string{layout.orientation, layout.root, ManifestRelativePath, PolicyRelativePath} {
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
	old, err := readManifest(filepath.Join(workspace, ManifestRelativePath))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if old.Runtime != "" && old.Runtime != runtimeName {
		return fmt.Errorf("workspace already has a %s runtime projection", old.Runtime)
	}
	if conflicts := preflight(workspace, layout, contents, hashes, digest(policyBody), old); len(conflicts) > 0 {
		return fmt.Errorf("runtime projection has conflicts: %s", strings.Join(conflicts, ", "))
	}
	return nil
}

// ValidateUninstall performs the projection preflight without removing files.
func ValidateUninstall(runtimeName, workspace string) error {
	status, err := Inspect(runtimeName, workspace)
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
	layout, err := layout(runtimeName, workspace)
	if err != nil {
		return Status{}, err
	}
	for _, relative := range []string{layout.orientation, layout.root, ManifestRelativePath, PolicyRelativePath} {
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
	old, err := readManifest(filepath.Join(workspace, ManifestRelativePath))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return Status{}, err
	}
	if old.Runtime != "" && old.Runtime != runtimeName {
		return Status{}, fmt.Errorf("workspace already has a %s runtime projection", old.Runtime)
	}
	if conflicts := preflight(workspace, layout, contents, hashes, policyHash, old); len(conflicts) > 0 {
		return Status{Runtime: runtimeName, State: "conflict", OrientationPath: filepath.Join(workspace, layout.orientation), SkillsRoot: filepath.Join(workspace, layout.root), ManifestPath: filepath.Join(workspace, ManifestRelativePath), PolicyPath: filepath.Join(workspace, PolicyRelativePath), SkillCount: len(contents), Conflicts: conflicts, Reason: "existing user files were preserved; no projection files were changed"}, fmt.Errorf("runtime projection has conflicts: %s", strings.Join(conflicts, ", "))
	}
	paths := []string{filepath.Join(workspace, layout.orientation), filepath.Join(workspace, ManifestRelativePath), filepath.Join(workspace, PolicyRelativePath)}
	for id := range contents {
		paths = append(paths, filepath.Join(workspace, layout.root, id, "SKILL.md"))
	}
	for id := range old.SkillHashes {
		if _, current := contents[id]; !current {
			paths = append(paths, filepath.Join(workspace, layout.root, id, "SKILL.md"))
		}
	}
	snapshots, err := snapshotFiles(paths)
	if err != nil {
		return Status{}, err
	}
	rollback := func(cause error) error {
		if restoreErr := restoreFiles(snapshots); restoreErr != nil {
			return fmt.Errorf("%w (projection rollback failed: %v)", cause, restoreErr)
		}
		return cause
	}
	if err := writeOrientation(filepath.Join(workspace, layout.orientation), orientation); err != nil {
		return Status{}, rollback(err)
	}
	for id, body := range contents {
		if err := writeManagedFile(filepath.Join(workspace, layout.root, id, "SKILL.md"), body); err != nil {
			return Status{}, rollback(fmt.Errorf("write installed skill %s: %w", id, err))
		}
	}
	if err := writeManagedFile(filepath.Join(workspace, PolicyRelativePath), policyBody); err != nil {
		return Status{}, rollback(fmt.Errorf("write selection-scoped skill policy: %w", err))
	}
	for id, expected := range old.SkillHashes {
		if _, current := contents[id]; current {
			continue
		}
		path := filepath.Join(workspace, layout.root, id, "SKILL.md")
		body, readErr := os.ReadFile(path)
		if readErr == nil && digest(body) == expected {
			if err := os.Remove(path); err != nil {
				return Status{}, rollback(fmt.Errorf("remove retired managed skill %s: %w", id, err))
			}
			_ = os.Remove(filepath.Dir(path))
		}
	}
	newManifest := manifest{SchemaVersion: SchemaVersion, Runtime: runtimeName, OrientationPath: layout.orientation, OrientationHash: orientationDigest(orientation), SkillHashes: hashes, PolicyPath: PolicyRelativePath, PolicyHash: policyHash}
	if err := writeJSON(filepath.Join(workspace, ManifestRelativePath), newManifest); err != nil {
		return Status{}, rollback(fmt.Errorf("write runtime projection manifest: %w", err))
	}
	return Status{Runtime: runtimeName, State: "installed", OrientationPath: filepath.Join(workspace, layout.orientation), SkillsRoot: filepath.Join(workspace, layout.root), ManifestPath: filepath.Join(workspace, ManifestRelativePath), PolicyPath: filepath.Join(workspace, PolicyRelativePath), SkillCount: len(contents)}, nil
}

func Inspect(runtimeName, workspace string) (Status, error) {
	layout, err := layout(runtimeName, workspace)
	if err != nil {
		return Status{}, err
	}
	for _, relative := range []string{layout.orientation, layout.root, ManifestRelativePath, PolicyRelativePath} {
		if err := rejectSymlinkComponents(workspace, relative); err != nil {
			return Status{}, err
		}
	}
	path := filepath.Join(workspace, ManifestRelativePath)
	current, err := readManifest(path)
	if errors.Is(err, os.ErrNotExist) {
		return Status{Runtime: runtimeName, State: "absent", OrientationPath: filepath.Join(workspace, layout.orientation), SkillsRoot: filepath.Join(workspace, layout.root), ManifestPath: path}, nil
	}
	if err != nil {
		return Status{}, err
	}
	if current.Runtime != runtimeName {
		return Status{}, fmt.Errorf("runtime projection manifest belongs to %s", current.Runtime)
	}
	conflicts := []string{}
	orientation, err := os.ReadFile(filepath.Join(workspace, layout.orientation))
	if err != nil {
		conflicts = append(conflicts, layout.orientation)
	} else if !orientationMatchesManifest(string(orientation), current.OrientationHash) {
		conflicts = append(conflicts, layout.orientation)
	}
	for id, expected := range current.SkillHashes {
		relative := filepath.Join(layout.root, id, "SKILL.md")
		if err := rejectSymlinkComponents(workspace, relative); err != nil {
			conflicts = append(conflicts, relative)
			continue
		}
		body, err := os.ReadFile(filepath.Join(workspace, relative))
		if err != nil || digest(body) != expected {
			conflicts = append(conflicts, relative)
		}
	}
	policyPath := filepath.Join(workspace, PolicyRelativePath)
	if current.PolicyPath != PolicyRelativePath || current.PolicyHash == "" {
		conflicts = append(conflicts, policyPath)
	} else if body, readErr := os.ReadFile(policyPath); readErr != nil || digest(body) != current.PolicyHash {
		conflicts = append(conflicts, policyPath)
	}
	sort.Strings(conflicts)
	state := "installed"
	if len(conflicts) > 0 {
		state = "conflict"
	}
	return Status{Runtime: runtimeName, State: state, OrientationPath: filepath.Join(workspace, layout.orientation), SkillsRoot: filepath.Join(workspace, layout.root), ManifestPath: path, PolicyPath: policyPath, SkillCount: len(current.SkillHashes), Conflicts: conflicts}, nil
}

func Uninstall(runtimeName, workspace string) (Status, error) {
	layout, err := layout(runtimeName, workspace)
	if err != nil {
		return Status{}, err
	}
	for _, relative := range []string{layout.orientation, layout.root, ManifestRelativePath, PolicyRelativePath} {
		if err := rejectSymlinkComponents(workspace, relative); err != nil {
			return Status{}, err
		}
	}
	manifestPath := filepath.Join(workspace, ManifestRelativePath)
	current, err := readManifest(manifestPath)
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
	policyPath := filepath.Join(workspace, PolicyRelativePath)
	paths := []string{orientationPath, manifestPath, policyPath}
	for id := range current.SkillHashes {
		paths = append(paths, filepath.Join(workspace, layout.root, id, "SKILL.md"))
	}
	snapshots, err := snapshotFiles(paths)
	if err != nil {
		return Status{}, err
	}
	rollback := func(cause error) error {
		if restoreErr := restoreFiles(snapshots); restoreErr != nil {
			return fmt.Errorf("%w (projection rollback failed: %v)", cause, restoreErr)
		}
		return cause
	}
	orientation, err := os.ReadFile(orientationPath)
	if err != nil {
		return Status{}, rollback(err)
	}
	if !orientationMatchesManifest(string(orientation), current.OrientationHash) {
		return Status{Runtime: runtimeName, State: "conflict", OrientationPath: orientationPath, SkillsRoot: filepath.Join(workspace, layout.root), ManifestPath: manifestPath, SkillCount: len(current.SkillHashes), Conflicts: []string{layout.orientation}, Reason: "managed orientation was changed or its markers are missing; no projection files were removed"}, errors.New("managed orientation was changed or its markers are missing")
	}
	var conflicts []string
	for id, expected := range current.SkillHashes {
		path := filepath.Join(workspace, layout.root, id, "SKILL.md")
		body, readErr := os.ReadFile(path)
		if readErr != nil || digest(body) != expected {
			conflicts = append(conflicts, path)
		}
	}
	if current.PolicyPath != PolicyRelativePath || current.PolicyHash == "" {
		conflicts = append(conflicts, policyPath)
	} else if body, readErr := os.ReadFile(policyPath); readErr != nil || digest(body) != current.PolicyHash {
		conflicts = append(conflicts, policyPath)
	}
	if len(conflicts) > 0 {
		sort.Strings(conflicts)
		return Status{Runtime: runtimeName, State: "conflict", OrientationPath: orientationPath, SkillsRoot: filepath.Join(workspace, layout.root), ManifestPath: manifestPath, SkillCount: len(current.SkillHashes), Conflicts: conflicts, Reason: "modified skill files were preserved"}, errors.New("modified managed skill files were preserved")
	}
	updated, err := removeOrientationBlock(string(orientation))
	if err != nil {
		return Status{}, rollback(err)
	}
	if strings.TrimSpace(updated) == "" {
		if err := os.Remove(orientationPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return Status{}, rollback(err)
		}
	} else if err := writeManagedFile(orientationPath, []byte(updated)); err != nil {
		return Status{}, rollback(err)
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
	if info, err := os.Lstat(absolute); err != nil {
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

func catalogForTracks(tracks []string) (skillsindex.Catalog, error) {
	base, err := baseskills.Catalog()
	if err != nil {
		return skillsindex.Catalog{}, fmt.Errorf("load managed skills catalog: %w", err)
	}
	if len(tracks) == 0 {
		return base, nil
	}
	catalog, err := bundlecatalog.Catalog()
	if err != nil {
		return skillsindex.Catalog{}, err
	}
	plan, err := catalog.PlanForTracks(tracks)
	if err != nil {
		return skillsindex.Catalog{}, err
	}
	for _, bundle := range plan.Bundles {
		var optional skillsindex.Catalog
		var loadErr error
		switch bundle.ID {
		case "engineering-core":
			optional, loadErr = engineeringcoreskills.Catalog()
		case "data-practice":
			optional, loadErr = datapracticeskills.Catalog()
		default:
			continue
		}
		if loadErr != nil {
			return skillsindex.Catalog{}, fmt.Errorf("load %s skills catalog: %w", bundle.ID, loadErr)
		}
		base.Skills = append(base.Skills, optional.Skills...)
	}
	sort.Slice(base.Skills, func(left, right int) bool { return base.Skills[left].ID < base.Skills[right].ID })
	if err := base.Validate(); err != nil {
		return skillsindex.Catalog{}, fmt.Errorf("validate activated skills catalog: %w", err)
	}
	return base, nil
}

// PolicyForTracks composes the immutable base policy with exactly the optional
// methods activated by the confirmed track plan. The returned policy compiles
// only against the corresponding active catalog, so unselected bundle methods
// remain denied even if their source is embedded in the release.
func PolicyForTracks(tracks []string) (skillpolicy.Policy, skillsindex.Catalog, error) {
	active, err := catalogForTracks(tracks)
	if err != nil {
		return skillpolicy.Policy{}, skillsindex.Catalog{}, err
	}
	base, err := baseskills.Catalog()
	if err != nil {
		return skillpolicy.Policy{}, skillsindex.Catalog{}, err
	}
	policy, err := skillpolicy.Parse(bytes.NewReader(baseskills.AgentSkillPolicy()))
	if err != nil {
		return skillpolicy.Policy{}, skillsindex.Catalog{}, fmt.Errorf("parse base agent skill policy: %w", err)
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
		return skillpolicy.Policy{}, skillsindex.Catalog{}, err
	}
	agents, err := baseagents.Catalog()
	if err != nil {
		return skillpolicy.Policy{}, skillsindex.Catalog{}, err
	}
	if _, err := skillpolicy.Compile(policy, active, agents); err != nil {
		return skillpolicy.Policy{}, skillsindex.Catalog{}, fmt.Errorf("compile selection-scoped agent skill policy: %w", err)
	}
	return policy, active, nil
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
	if body, err := engineeringcoreskills.Skill(id); err == nil {
		return body, nil
	}
	return datapracticeskills.Skill(id)
}

func renderOrientation(layout runtimeLayout, catalog skillsindex.Catalog) (string, error) {
	template := string(baseruntime.OrientationTemplate())
	if !strings.Contains(template, "{{SKILLS_BLOCK}}") || !strings.Contains(template, "{{RUNTIME}}") || !strings.Contains(template, "{{RUNTIME_ID}}") {
		return "", errors.New("orientation template is missing required placeholders")
	}
	var block strings.Builder
	block.WriteString("<!-- BCGOS:INSTALLED-SKILLS:BEGIN -->\n")
	for _, skill := range catalog.Skills {
		fmt.Fprintf(&block, "- `$%s` — %s; usar quando: %s; fonte: `%s/%s/SKILL.md`\n", skill.ID, skill.DisplayName, skill.Trigger, layout.root, skill.ID)
	}
	block.WriteString("<!-- BCGOS:INSTALLED-SKILLS:END -->")
	body := strings.ReplaceAll(template, "{{RUNTIME}}", layout.runtimeName)
	body = strings.ReplaceAll(body, "{{RUNTIME_ID}}", runtimeID(layout))
	body = strings.ReplaceAll(body, "{{SKILLS_BLOCK}}", block.String())
	return OrientationBegin + "\n" + strings.TrimSpace(body) + "\n" + OrientationEnd + "\n", nil
}

func runtimeID(layout runtimeLayout) string {
	if layout.runtimeName == "Claude Code" {
		return "claude"
	}
	return "codex"
}

func preflight(workspace string, layout runtimeLayout, contents map[string][]byte, hashes map[string]string, policyHash string, old manifest) []string {
	var conflicts []string
	orientationPath := filepath.Join(workspace, layout.orientation)
	currentOrientation, orientationErr := os.ReadFile(orientationPath)
	if old.Runtime != "" {
		if orientationErr != nil || !orientationMatchesManifest(string(currentOrientation), old.OrientationHash) {
			conflicts = append(conflicts, orientationPath)
		}
	} else if orientationErr == nil && (strings.Contains(string(currentOrientation), OrientationBegin) || strings.Contains(string(currentOrientation), OrientationEnd)) {
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
			current, readErr := os.ReadFile(path)
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
		body, err := os.ReadFile(path)
		if err == nil && digest(body) != expected {
			conflicts = append(conflicts, path)
		}
	}
	policyPath := filepath.Join(workspace, PolicyRelativePath)
	policyBody, policyErr := os.ReadFile(policyPath)
	if old.Runtime == "" || old.PolicyHash == "" {
		// A legacy projection without policy ownership may be upgraded only when
		// the policy path is absent. An existing file belongs to the user.
		if policyErr == nil || !errors.Is(policyErr, os.ErrNotExist) {
			conflicts = append(conflicts, policyPath)
		}
	} else if old.PolicyPath != PolicyRelativePath || policyErr != nil {
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
	current, err := os.ReadFile(path)
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
		return strings.TrimRight(current, "\r\n") + "\n\n" + generated, nil
	}
	end += len(OrientationEnd)
	return current[:start] + strings.TrimSpace(generated) + current[end:], nil
}

func removeOrientationBlock(current string) (string, error) {
	start, end := strings.Index(current, OrientationBegin), strings.Index(current, OrientationEnd)
	if start == -1 || end < start {
		return "", errors.New("orientation markers are missing")
	}
	end += len(OrientationEnd)
	return strings.TrimSpace(current[:start]+current[end:]) + "\n", nil
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
	body, err := os.ReadFile(path)
	if err != nil {
		return manifest{}, err
	}
	var value manifest
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return manifest{}, fmt.Errorf("decode runtime projection manifest: %w", err)
	}
	if value.SchemaVersion != SchemaVersion || value.Runtime == "" || value.OrientationPath == "" || value.OrientationHash == "" || len(value.SkillHashes) == 0 {
		return manifest{}, errors.New("runtime projection manifest is invalid")
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
		if value.PolicyPath != PolicyRelativePath || len(value.PolicyHash) != sha256.Size*2 {
			return manifest{}, errors.New("runtime projection manifest has an invalid skill policy identity")
		}
		if _, err := hex.DecodeString(value.PolicyHash); err != nil || strings.ToLower(value.PolicyHash) != value.PolicyHash {
			return manifest{}, errors.New("runtime projection manifest has an invalid skill policy digest")
		}
	}
	return value, nil
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
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("refusing to snapshot symlink %s", path)
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		snapshots = append(snapshots, fileSnapshot{path: path, exists: true, mode: info.Mode().Perm(), body: body})
	}
	return snapshots, nil
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
