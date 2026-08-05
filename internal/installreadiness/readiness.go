// Package installreadiness verifies the local identities created by first
// installation, workspace initialization and runtime adapter installation.
// It is deliberately read-only and never treats configuration as native hook
// observation or capability qualification.
package installreadiness

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
	"runtime"
	"strings"

	baseruntime "github.com/agentic-os-brasil/bcg-brasil-agentic-os/bundles/base/runtime"
	baseskills "github.com/agentic-os-brasil/bcg-brasil-agentic-os/bundles/base/skills"
	datapracticeskills "github.com/agentic-os-brasil/bcg-brasil-agentic-os/bundles/data-practice/skills"
	engineeringcoreskills "github.com/agentic-os-brasil/bcg-brasil-agentic-os/bundles/engineering-core/skills"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/agentorchestration"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/agentscaffold"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installtx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ownerctx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/runtimeprojection"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/skillsindex"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspaceagent"
)

const maximumConfigurationBytes = 1 << 20

// Options contains only identities already selected by installation and
// onboarding. The product CLI supplies its own executable path and version;
// they are not caller-selected flags on the readiness command.
type Options struct {
	Runtime          string
	WorkspacePath    string
	DataRoot         string
	ExecutablePath   string
	CLIVersion       string
	CapabilityTracks []string
	TargetOS         string
	TargetArch       string
}

// Report is the structured post-install handoff. Ready means only that the
// configured local identities agree; NativeObservation and CapabilityState
// prevent that result from being mistaken for native runtime qualification.
type Report struct {
	SchemaVersion     int                `json:"schema_version"`
	State             string             `json:"state"`
	Ready             bool               `json:"ready"`
	Runtime           string             `json:"runtime"`
	WorkspacePath     string             `json:"workspace_path,omitempty"`
	WorkspaceID       string             `json:"workspace_id,omitempty"`
	DataRoot          string             `json:"data_root,omitempty"`
	InstalledCLI      string             `json:"installed_cli,omitempty"`
	CLIVersion        string             `json:"cli_version,omitempty"`
	EvidenceClass     string             `json:"evidence_class"`
	NativeObservation string             `json:"native_observation"`
	CapabilityState   string             `json:"capability_state"`
	Checks            []Check            `json:"checks"`
	Lifecycle         []LifecycleBinding `json:"lifecycle,omitempty"`
}

type Check struct {
	ID      string `json:"id"`
	State   string `json:"state"`
	Message string `json:"message"`
}

type LifecycleBinding struct {
	SemanticEvent   string `json:"semantic_event"`
	NativeEvent     string `json:"native_event"`
	Command         string `json:"command"`
	CapabilityID    string `json:"capability_id"`
	CapabilityState string `json:"capability_state"`
	Configured      bool   `json:"configured"`
	AdapterObserved bool   `json:"adapter_observed"`
	NativeQualified bool   `json:"native_qualified"`
}

type projectionManifest struct {
	SchemaVersion   int               `json:"schema_version"`
	Runtime         string            `json:"runtime"`
	OrientationPath string            `json:"orientation_path"`
	OrientationHash string            `json:"orientation_hash"`
	SkillHashes     map[string]string `json:"skill_hashes"`
	PolicyPath      string            `json:"policy_path"`
	PolicyHash      string            `json:"policy_hash"`
}

type expectedBinding struct {
	semantic, native, commandSuffix, capability string
	asynchronous                                bool
}

var expectedBindings = map[string][]expectedBinding{
	"claude": {
		{semantic: "session_start", native: "SessionStart", commandSuffix: "claude session-start", capability: "session_start"},
		{semantic: "context_inject", native: "UserPromptSubmit", commandSuffix: "claude context-injection", capability: "context_injection"},
		{semantic: "pre_action_guard", native: "PreToolUse", commandSuffix: "claude pre-action-guard", capability: "pre_action_guard"},
		{semantic: "post_action_observe", native: "PostToolUse", commandSuffix: "claude post-action-receipt", capability: "post_action_observe", asynchronous: true},
		{semantic: "stop_finalize", native: "Stop", commandSuffix: "claude stop-finalization", capability: "stop_finalize", asynchronous: true},
	},
	"codex": {
		{semantic: "session_start", native: "SessionStart", commandSuffix: "session-start --runtime codex", capability: "session_start"},
		{semantic: "context_inject", native: "UserPromptSubmit", commandSuffix: "codex context-injection", capability: "context_injection"},
		{semantic: "pre_action_guard", native: "PreToolUse", commandSuffix: "codex pre-action-guard", capability: "pre_action_guard"},
		{semantic: "post_action_observe", native: "PostToolUse", commandSuffix: "codex post-action-receipt", capability: "post_action_observe"},
		{semantic: "stop_finalize", native: "Stop", commandSuffix: "codex stop-finalization", capability: "stop_finalize"},
	},
}

// Verify performs no writes and invokes no external process. It stops at the
// first failed trust boundary and returns the partial structured report.
func Verify(options Options) (Report, error) {
	runtimeName := strings.TrimSpace(options.Runtime)
	if runtimeName == "" {
		runtimeName = "codex"
	}
	if _, supported := expectedBindings[runtimeName]; !supported {
		return Report{SchemaVersion: 1, State: "failed", Runtime: runtimeName}, fmt.Errorf("unsupported readiness runtime %q", runtimeName)
	}
	report := Report{
		SchemaVersion: 1, State: "failed", Runtime: runtimeName,
		EvidenceClass: "configured", NativeObservation: "not_observed", CapabilityState: "unavailable",
		Checks: []Check{},
	}
	fail := func(id string, err error) (Report, error) {
		report.Checks = append(report.Checks, Check{ID: id, State: "fail", Message: err.Error()})
		return report, fmt.Errorf("%s: %w", id, err)
	}
	pass := func(id, message string) {
		report.Checks = append(report.Checks, Check{ID: id, State: "pass", Message: message})
	}

	workspacePath, err := canonicalDirectory(options.WorkspacePath, "workspace")
	if err != nil {
		return fail("workspace_path", err)
	}
	report.WorkspacePath = workspacePath
	pass("workspace_path", "workspace is a canonical non-symlink directory")

	dataRoot, err := canonicalDirectory(options.DataRoot, "owner-data root")
	if err != nil {
		return fail("install_state", err)
	}
	report.DataRoot = dataRoot
	if _, err := regularFile(dataRoot, filepath.Join("config", "install-state.json"), maximumConfigurationBytes, false); err != nil {
		return fail("install_state", fmt.Errorf("installed state is not a protected regular file: %w", err))
	}
	state, err := installtx.ReadState(dataRoot)
	if err != nil {
		return fail("install_state", fmt.Errorf("read installed state: %w", err))
	}
	pass("install_state", "schema-versioned installed state is valid")

	targetOS := options.TargetOS
	if targetOS == "" {
		targetOS = runtime.GOOS
	}
	targetArch := options.TargetArch
	if targetArch == "" {
		targetArch = runtime.GOARCH
	}
	managedRoot, err := canonicalDirectory(state.ManagedRoot, "managed root")
	if err != nil {
		return fail("installed_cli", err)
	}
	cliName := "bcgos"
	if targetOS == "windows" {
		cliName += ".exe"
	}
	expectedCLI := filepath.Join(managedRoot, "bin", cliName)
	executable, err := canonicalRegular(options.ExecutablePath, "installed CLI", 1<<30, true)
	if err != nil {
		return fail("installed_cli", err)
	}
	if !samePath(expectedCLI, executable) || !samePath(state.ManagedRoot, managedRoot) {
		return fail("installed_cli", errors.New("current executable is not managed-root/bin/bcgos"))
	}
	if state.TargetOS != targetOS || state.TargetArch != targetArch {
		return fail("installed_cli", errors.New("installed target does not match the running CLI platform"))
	}
	if strings.TrimSpace(options.CLIVersion) == "" || options.CLIVersion != state.CLIVersion {
		return fail("installed_cli", errors.New("running CLI version does not match installed state"))
	}
	report.InstalledCLI, report.CLIVersion = executable, state.CLIVersion
	pass("installed_cli", "running CLI path and version match installed state")

	for _, relative := range []string{filepath.Join(".bcgos", "workspace.json"), filepath.Join("brain", "README.md")} {
		if _, err := regularFile(workspacePath, relative, maximumConfigurationBytes, false); err != nil {
			return fail("workspace_identity", err)
		}
	}
	inspection, err := workspace.Inspect(workspacePath, dataRoot)
	if err != nil {
		return fail("workspace_identity", err)
	}
	if inspection.State != "ready" || inspection.MetadataStatus != "valid" || inspection.WorkspacePath != workspacePath || inspection.WorkspaceID == "" {
		return fail("workspace_identity", fmt.Errorf("initialized workspace is not ready: state=%s metadata=%s", inspection.State, inspection.MetadataStatus))
	}
	report.WorkspaceID = inspection.WorkspaceID
	pass("workspace_identity", "workspace manifest is bound to this canonical path")

	if err := verifyOrchestrationState(workspacePath); err != nil {
		return fail("orchestration_state", err)
	}
	pass("orchestration_state", "durable orchestration state is present, valid and protected")
	owner, err := ownerctx.Inspect(dataRoot)
	if err != nil || !owner.Initialized {
		if err == nil {
			err = errors.New("owner context is not initialized")
		}
		return fail("owner_context", err)
	}
	pass("owner_context", "owner facets and onboarding registry are initialized")
	workspaceAgent, err := workspaceagent.Inspect(dataRoot, inspection.WorkspaceID)
	if err != nil || !workspaceAgent.Initialized {
		if err == nil {
			err = errors.New("workspace agent is not initialized")
		}
		return fail("workspace_agent", err)
	}
	pass("workspace_agent", "workspace agent state and dossier are initialized")
	agentID := agentscaffold.WorkspaceRequest(inspection.WorkspaceID).AgentID
	agentStub, err := agentscaffold.Inspect(dataRoot, agentID)
	if err != nil || !agentStub.Initialized {
		if err == nil {
			err = errors.New("workspace agent scaffold is not initialized")
		}
		return fail("agent_scaffold", err)
	}
	pass("agent_scaffold", "signed workspace-agent scaffold is initialized")

	if err := verifyProjection(runtimeName, workspacePath, options.CapabilityTracks); err != nil {
		return fail("runtime_projection", err)
	}
	pass("runtime_projection", "the managed "+runtimeName+" projection matches the active embedded bundle")

	lifecycle, err := verifyHooks(runtimeName, workspacePath, executable, targetOS)
	if err != nil {
		return fail("runtime_hooks", err)
	}
	pass("runtime_hooks", "all five workspace-local "+runtimeName+" lifecycle commands point to the installed CLI")

	if err := bindCapabilities(runtimeName, &lifecycle); err != nil {
		return fail("lifecycle_capabilities", err)
	}
	report.Lifecycle = lifecycle
	pass("lifecycle_capabilities", "canonical lifecycle capabilities remain configured and unavailable without native observation")
	report.State, report.Ready = "verified", true
	return report, nil
}

func verifyOrchestrationState(workspacePath string) error {
	path, err := regularFile(workspacePath, filepath.Join(".bcgos", "maestro-orchestration-state.json"), agentorchestration.MaximumDurableStateBytes, false)
	if err != nil {
		return err
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.Mode().Perm()&0o077 != 0 {
		return errors.New("orchestration state must be owner-only (0600 or stricter)")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	snapshot, err := agentorchestration.DecodeStateSnapshot(body)
	if err != nil {
		return fmt.Errorf("decode durable orchestration state: %w", err)
	}
	return agentorchestration.ValidateStateSnapshot(snapshot)
}

func verifyProjection(runtimeName, workspacePath string, tracks []string) error {
	orientation, skillsRoot, runtimeLabel := "AGENTS.md", filepath.Join(".codex", "skills"), "Codex"
	if runtimeName == "claude" {
		orientation, skillsRoot, runtimeLabel = "CLAUDE.md", filepath.Join(".claude", "skills"), "Claude Code"
	}
	for _, relative := range []string{orientation, runtimeprojection.ManifestRelativePath, runtimeprojection.PolicyRelativePath, skillsRoot} {
		if strings.HasSuffix(relative, "skills") {
			if _, err := canonicalDirectory(filepath.Join(workspacePath, relative), runtimeLabel+" skills root"); err != nil {
				return err
			}
			continue
		}
		if _, err := regularFile(workspacePath, relative, maximumConfigurationBytes, false); err != nil {
			return err
		}
	}
	manifestBody, err := os.ReadFile(filepath.Join(workspacePath, runtimeprojection.ManifestRelativePath))
	if err != nil {
		return err
	}
	var manifest projectionManifest
	if err := decodeStrict(manifestBody, &manifest); err != nil {
		return fmt.Errorf("decode runtime projection manifest: %w", err)
	}
	if manifest.SchemaVersion != runtimeprojection.SchemaVersion || manifest.Runtime != runtimeName ||
		manifest.OrientationPath != orientation || manifest.PolicyPath != runtimeprojection.PolicyRelativePath {
		return errors.New("runtime projection manifest has a mismatched " + runtimeLabel + " path identity")
	}
	policy, catalog, err := runtimeprojection.PolicyForTracks(tracks)
	if err != nil {
		return fmt.Errorf("resolve active capability tracks: %w", err)
	}
	policyBody, err := json.MarshalIndent(policy, "", "  ")
	if err != nil {
		return err
	}
	policyBody = append(policyBody, '\n')
	if manifest.PolicyHash != digest(policyBody) {
		return errors.New("runtime projection policy identity does not match active capability tracks")
	}
	installedPolicy, err := os.ReadFile(filepath.Join(workspacePath, runtimeprojection.PolicyRelativePath))
	if err != nil || !bytes.Equal(installedPolicy, policyBody) {
		return errors.New("selection-scoped skill policy was modified")
	}

	expectedHashes := make(map[string]string, len(catalog.Skills))
	for _, skill := range catalog.Skills {
		body, err := managedSkill(skill.ID)
		if err != nil {
			return err
		}
		expectedHashes[skill.ID] = digest(body)
		path, err := regularFile(workspacePath, filepath.Join(skillsRoot, skill.ID, "SKILL.md"), maximumConfigurationBytes, false)
		if err != nil {
			return err
		}
		installed, err := os.ReadFile(path)
		if err != nil || digest(installed) != expectedHashes[skill.ID] {
			return fmt.Errorf("managed skill %s was modified", skill.ID)
		}
	}
	if !equalHashes(manifest.SkillHashes, expectedHashes) {
		return errors.New("runtime projection skill identities do not match the active embedded bundle")
	}

	expectedOrientation, err := renderOrientation(runtimeName, catalog.Skills)
	if err != nil {
		return err
	}
	orientationBody, err := os.ReadFile(filepath.Join(workspacePath, orientation))
	if err != nil {
		return err
	}
	managedBlock, err := exactManagedBlock(string(orientationBody))
	if err != nil {
		return err
	}
	expectedBlock, err := exactManagedBlock(expectedOrientation)
	if err != nil {
		return err
	}
	if managedBlock != expectedBlock || manifest.OrientationHash != digest([]byte(strings.TrimSpace(expectedBlock))) {
		return errors.New("managed " + orientation + " orientation does not match the active embedded bundle")
	}
	return nil
}

func verifyHooks(runtimeName, workspacePath, executable, targetOS string) ([]LifecycleBinding, error) {
	configRelativePath := filepath.Join(".codex", "hooks.json")
	if runtimeName == "claude" {
		configRelativePath = filepath.Join(".claude", "settings.local.json")
	}
	path, err := regularFile(workspacePath, configRelativePath, maximumConfigurationBytes, false)
	if err != nil {
		return nil, err
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var config map[string]any
	if err := decodeStrict(body, &config); err != nil {
		return nil, fmt.Errorf("decode %s hook configuration: %w", runtimeName, err)
	}
	hooks, ok := config["hooks"].(map[string]any)
	if !ok {
		return nil, errors.New(runtimeName + " hook configuration hooks must be an object")
	}
	expected := expectedBindings[runtimeName]
	expectedByNative := make(map[string]expectedBinding, len(expected))
	for _, binding := range expected {
		expectedByNative[binding.native] = binding
	}
	for event, groupsValue := range hooks {
		groups, ok := groupsValue.([]any)
		if !ok {
			return nil, fmt.Errorf("%s hook event %s must be a list", runtimeName, event)
		}
		if _, expected := expectedByNative[event]; expected {
			continue
		}
		for _, command := range ownedCommands(groups, event) {
			if command != "" {
				return nil, fmt.Errorf("Maestro-owned hook is attached to unexpected event %s", event)
			}
		}
	}
	result := make([]LifecycleBinding, 0, len(expected))
	for _, binding := range expected {
		groupsValue, exists := hooks[binding.native]
		if !exists {
			return nil, fmt.Errorf("missing Maestro-owned %s %s hook", runtimeName, binding.native)
		}
		groups, ok := groupsValue.([]any)
		if !ok {
			return nil, fmt.Errorf("%s hook event %s must be a list", runtimeName, binding.native)
		}
		expectedCommand := quoteCommandPath(targetOS, executable) + " hook " + binding.commandSuffix +
			" --adapter-source maestro --orchestration-state .bcgos/maestro-orchestration-state.json " + quoteCommandPath(targetOS, workspacePath)
		owned := ownedHookEntries(groups, binding.native)
		if len(owned) != 1 {
			return nil, fmt.Errorf("%s hook event %s has %d Maestro-owned entries, want exactly one", runtimeName, binding.native, len(owned))
		}
		entry := owned[0]
		if entry["type"] != "command" || entry["command"] != expectedCommand || entry["timeout"] != float64(2) {
			return nil, fmt.Errorf("%s hook event %s does not match the installed CLI contract", runtimeName, binding.native)
		}
		if binding.asynchronous {
			if len(entry) != 4 || entry["async"] != true {
				return nil, fmt.Errorf("%s hook event %s must remain asynchronous", runtimeName, binding.native)
			}
		} else if len(entry) != 3 {
			return nil, fmt.Errorf("%s hook event %s must remain synchronous", runtimeName, binding.native)
		}
		result = append(result, LifecycleBinding{
			SemanticEvent: binding.semantic, NativeEvent: binding.native, Command: expectedCommand,
			CapabilityID: binding.capability,
		})
	}
	return result, nil
}

func bindCapabilities(runtimeName string, bindings *[]LifecycleBinding) error {
	manifest, err := baseruntime.Manifest()
	if err != nil {
		return fmt.Errorf("load canonical capability manifest: %w", err)
	}
	byID := make(map[string]int, len(*bindings))
	for index, binding := range *bindings {
		byID[binding.CapabilityID] = index
	}
	seen := map[string]bool{}
	for _, capability := range manifest.Capabilities {
		index, wanted := byID[capability.ID]
		if !wanted {
			continue
		}
		if seen[capability.ID] {
			return fmt.Errorf("duplicate lifecycle capability %s", capability.ID)
		}
		seen[capability.ID] = true
		contract, exists := capability.Runtimes[runtimeName]
		binding := &(*bindings)[index]
		if !exists || capability.SemanticEvent != binding.SemanticEvent || contract.State != "unavailable" ||
			!contract.Configured || contract.AdapterObserved || contract.NativeQualified || contract.Reason == "" {
			return fmt.Errorf("%s lifecycle capability %s is not fail-closed and configuration-only", runtimeName, capability.ID)
		}
		binding.CapabilityState = contract.State
		binding.Configured = contract.Configured
		binding.AdapterObserved = contract.AdapterObserved
		binding.NativeQualified = contract.NativeQualified
	}
	if len(seen) != len(*bindings) {
		return errors.New("canonical lifecycle capability manifest is incomplete")
	}
	return nil
}

func ownedHookEntries(groups []any, event string) []map[string]any {
	var result []map[string]any
	for _, groupValue := range groups {
		group, ok := groupValue.(map[string]any)
		if !ok {
			return append(result, map[string]any{"invalid": event})
		}
		entries, ok := group["hooks"].([]any)
		if !ok {
			return append(result, map[string]any{"invalid": event})
		}
		for _, entryValue := range entries {
			entry, ok := entryValue.(map[string]any)
			if !ok {
				return append(result, map[string]any{"invalid": event})
			}
			command, _ := entry["command"].(string)
			if isMaestroOwned(command) {
				result = append(result, entry)
			}
		}
	}
	return result
}

func ownedCommands(groups []any, event string) []string {
	entries := ownedHookEntries(groups, event)
	result := make([]string, 0, len(entries))
	for _, entry := range entries {
		command, _ := entry["command"].(string)
		result = append(result, command)
	}
	return result
}

func isMaestroOwned(command string) bool {
	return strings.Contains(command, "--adapter-source maestro") ||
		strings.Contains(command, "--orchestration-state .bcgos/maestro-orchestration-state.json") ||
		strings.HasPrefix(strings.TrimSpace(command), "bcgos hook ")
}

func renderOrientation(runtimeName string, skills []skillsindex.Skill) (string, error) {
	template := string(baseruntime.OrientationTemplate())
	if !strings.Contains(template, "{{SKILLS_BLOCK}}") || !strings.Contains(template, "{{RUNTIME}}") || !strings.Contains(template, "{{RUNTIME_ID}}") {
		return "", errors.New("orientation template is missing required placeholders")
	}
	var block strings.Builder
	block.WriteString("<!-- BCGOS:INSTALLED-SKILLS:BEGIN -->\n")
	skillsRoot, runtimeLabel := ".codex/skills", "Codex"
	if runtimeName == "claude" {
		skillsRoot, runtimeLabel = ".claude/skills", "Claude Code"
	}
	for _, skill := range skills {
		fmt.Fprintf(&block, "- `$%s` — %s; usar quando: %s; fonte: `%s/%s/SKILL.md`\n", skill.ID, skill.DisplayName, skill.Trigger, skillsRoot, skill.ID)
	}
	block.WriteString("<!-- BCGOS:INSTALLED-SKILLS:END -->")
	body := strings.ReplaceAll(template, "{{RUNTIME}}", runtimeLabel)
	body = strings.ReplaceAll(body, "{{RUNTIME_ID}}", runtimeName)
	body = strings.ReplaceAll(body, "{{SKILLS_BLOCK}}", block.String())
	return runtimeprojection.OrientationBegin + "\n" + strings.TrimSpace(body) + "\n" + runtimeprojection.OrientationEnd + "\n", nil
}

func managedSkill(id string) ([]byte, error) {
	if body, err := baseskills.Skill(id); err == nil {
		return body, nil
	}
	if body, err := engineeringcoreskills.Skill(id); err == nil {
		return body, nil
	}
	return datapracticeskills.Skill(id)
}

func exactManagedBlock(body string) (string, error) {
	if strings.Count(body, runtimeprojection.OrientationBegin) != 1 || strings.Count(body, runtimeprojection.OrientationEnd) != 1 {
		return "", errors.New("AGENTS.md must contain exactly one managed orientation block")
	}
	start := strings.Index(body, runtimeprojection.OrientationBegin)
	end := strings.Index(body, runtimeprojection.OrientationEnd)
	if start < 0 || end < start {
		return "", errors.New("AGENTS.md managed orientation markers are invalid")
	}
	end += len(runtimeprojection.OrientationEnd)
	return body[start:end], nil
}

func equalHashes(left, right map[string]string) bool {
	if len(left) != len(right) {
		return false
	}
	for key, value := range right {
		if left[key] != value || len(value) != sha256.Size*2 {
			return false
		}
	}
	return true
}

func decodeStrict(body []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}

func canonicalDirectory(path, label string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", label, err)
	}
	absolute = filepath.Clean(absolute)
	info, err := os.Lstat(absolute)
	if err != nil {
		return "", fmt.Errorf("inspect %s: %w", label, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return "", fmt.Errorf("%s must be a non-symlink directory", label)
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", fmt.Errorf("resolve %s physical path: %w", label, err)
	}
	if !samePath(absolute, resolved) {
		return "", fmt.Errorf("%s must use its canonical path without symlinked components", label)
	}
	return absolute, nil
}

func canonicalRegular(path, label string, maximum int64, executable bool) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", label, err)
	}
	absolute = filepath.Clean(absolute)
	info, err := os.Lstat(absolute)
	if err != nil {
		return "", fmt.Errorf("inspect %s: %w", label, err)
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maximum {
		return "", fmt.Errorf("%s must be a bounded regular file", label)
	}
	if executable && runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0 {
		return "", fmt.Errorf("%s must be executable", label)
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", fmt.Errorf("resolve %s physical path: %w", label, err)
	}
	if !samePath(absolute, resolved) {
		return "", fmt.Errorf("%s must not use symlinked path components", label)
	}
	return absolute, nil
}

func regularFile(root, relative string, maximum int64, executable bool) (string, error) {
	if filepath.IsAbs(relative) || filepath.Clean(relative) != relative || relative == "." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", errors.New("managed path is not a safe workspace-relative path")
	}
	path := filepath.Join(root, relative)
	rel, err := filepath.Rel(root, path)
	if err != nil || rel != relative {
		return "", errors.New("managed path escaped its canonical root")
	}
	return canonicalRegular(path, relative, maximum, executable)
}

func quoteCommandPath(platform, path string) string {
	if platform == "windows" {
		return `"` + strings.ReplaceAll(path, `"`, `\"`) + `"`
	}
	return "'" + strings.ReplaceAll(path, "'", "'\"'\"'") + "'"
}

func samePath(left, right string) bool {
	left, right = filepath.Clean(left), filepath.Clean(right)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(left, right)
	}
	return left == right
}

func digest(body []byte) string {
	value := sha256.Sum256(body)
	return hex.EncodeToString(value[:])
}
