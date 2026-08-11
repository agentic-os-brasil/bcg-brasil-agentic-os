// Package adaptercfg manages only the Maestro-owned local hook entry in a
// workspace runtime configuration. It preserves unrelated user configuration.
package adaptercfg

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/claudeagents"
)

const adapterSourceMarker = "--adapter-source maestro"
const orchestrationStateMarker = "--orchestration-state .bcgos/maestro-orchestration-state.json"

var (
	installClaudeAgents   = claudeagents.Install
	uninstallClaudeAgents = claudeagents.Uninstall
	writeRuntimeConfig    = write
	excludeLocalConfig    = ensureLocalConfigExcluded
)

var managedClaudeAgentFiles = []string{
	"client-account-agent.md",
	"case-agent.md",
	"walter.md",
	"darwin.md",
	"pa-expert.md",
}

type Status struct {
	Runtime string `json:"runtime"`
	Path    string `json:"path"`
	State   string `json:"state"`
}

type binding struct {
	NativeEvent string
	Command     string
	Async       bool
}

type stateFileSnapshot struct {
	path   string
	exists bool
	mode   os.FileMode
	body   []byte
}

type stateDirectorySnapshot struct {
	path   string
	exists bool
}

// StateSnapshot captures every local surface owned by adapter installation:
// the runtime settings, Git exclude entry and the exact managed Claude agent
// files. CLI transactions use it to roll an adapter back when a later runtime
// projection fails.
type StateSnapshot struct {
	files       []stateFileSnapshot
	directories []stateDirectorySnapshot
}

// CaptureState is read-only and refuses symlinked transaction surfaces.
func CaptureState(runtimeName, workspace string) (StateSnapshot, error) {
	configPath, err := target(runtimeName, workspace)
	if err != nil {
		return StateSnapshot{}, err
	}
	excludePath, err := LocalConfigExcludePath(runtimeName, workspace)
	if err != nil {
		return StateSnapshot{}, err
	}
	paths := []string{configPath}
	if excludePath != "" {
		paths = append(paths, excludePath)
	}
	directories := []string{filepath.Dir(configPath)}
	if runtimeName == "claude" {
		agentsRoot := filepath.Join(workspace, ".claude", "agents")
		directories = append(directories, agentsRoot)
		for _, name := range managedClaudeAgentFiles {
			paths = append(paths, filepath.Join(agentsRoot, name))
		}
	}
	snapshot := StateSnapshot{}
	for _, path := range paths {
		file, err := captureStateFile(path)
		if err != nil {
			return StateSnapshot{}, err
		}
		snapshot.files = append(snapshot.files, file)
	}
	for _, path := range directories {
		directory, err := captureStateDirectory(path)
		if err != nil {
			return StateSnapshot{}, err
		}
		snapshot.directories = append(snapshot.directories, directory)
	}
	return snapshot, nil
}

func captureStateFile(path string) (stateFileSnapshot, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return stateFileSnapshot{path: path}, nil
	}
	if err != nil {
		return stateFileSnapshot{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return stateFileSnapshot{}, fmt.Errorf("refusing to snapshot non-regular adapter surface %s", path)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return stateFileSnapshot{}, err
	}
	return stateFileSnapshot{path: path, exists: true, mode: info.Mode().Perm(), body: body}, nil
}

func captureStateDirectory(path string) (stateDirectorySnapshot, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return stateDirectorySnapshot{path: path}, nil
	}
	if err != nil {
		return stateDirectorySnapshot{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return stateDirectorySnapshot{}, fmt.Errorf("refusing to snapshot non-directory adapter surface %s", path)
	}
	return stateDirectorySnapshot{path: path, exists: true}, nil
}

// Restore returns every captured file to its prior bytes and removes only
// transaction-created empty directories. User-owned neighboring files remain
// untouched.
func (snapshot StateSnapshot) Restore() error {
	var restoreErrors []error
	for _, file := range snapshot.files {
		if !file.exists {
			if err := os.Remove(file.path); err != nil && !errors.Is(err, os.ErrNotExist) {
				restoreErrors = append(restoreErrors, err)
			}
			continue
		}
		if err := writeBytes(file.path, file.body, file.mode); err != nil {
			restoreErrors = append(restoreErrors, err)
		}
	}
	for index := len(snapshot.directories) - 1; index >= 0; index-- {
		directory := snapshot.directories[index]
		if directory.exists {
			continue
		}
		if err := os.Remove(directory.path); err != nil && !errors.Is(err, os.ErrNotExist) && !errors.Is(err, syscall.ENOTEMPTY) {
			restoreErrors = append(restoreErrors, err)
		}
	}
	return errors.Join(restoreErrors...)
}

func rollbackState(snapshot StateSnapshot, operation string, original error) error {
	if restoreErr := snapshot.Restore(); restoreErr != nil {
		return fmt.Errorf("%s failed and rollback failed: %w (original: %v)", operation, restoreErr, original)
	}
	return original
}

// ValidateInstall performs the read-only checks used by Install. Callers that
// coordinate another local transaction can use it to fail closed before either
// side writes.
func ValidateInstall(runtimeName, workspace, executable string) error {
	if runtimeName == "claude" {
		if err := claudeagents.ValidateInstall(workspace); err != nil {
			return err
		}
	}
	path, err := target(runtimeName, workspace)
	if err != nil {
		return err
	}
	bindings, err := bindingsFor(runtimeName, executable, workspace)
	if err != nil {
		return err
	}
	config, err := read(path)
	if err != nil {
		return err
	}
	hooks, err := hooksMap(config)
	if err != nil {
		return err
	}
	for _, binding := range bindings {
		groups, err := groupsForEvent(hooks, binding.NativeEvent)
		if err != nil {
			return err
		}
		if err := validateEventGroups(binding.NativeEvent, groups); err != nil {
			return err
		}
	}
	return rejectTrackedConfig(workspace, path)
}

// ValidateUninstall performs the read-only checks used by Uninstall.
func ValidateUninstall(runtimeName, workspace string) error {
	if runtimeName == "claude" {
		if err := claudeagents.ValidateUninstall(workspace); err != nil {
			return err
		}
	}
	path, err := target(runtimeName, workspace)
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	config, err := read(path)
	if err != nil {
		return err
	}
	hooks, err := hooksMap(config)
	if err != nil {
		return err
	}
	bindings, err := bindingsFor(runtimeName, "bcgos", workspace)
	if err != nil {
		return err
	}
	for _, binding := range bindings {
		groups, err := groupsForEvent(hooks, binding.NativeEvent)
		if err != nil {
			return err
		}
		if err := validateEventGroups(binding.NativeEvent, groups); err != nil {
			return err
		}
	}
	return nil
}

// LocalConfigExcludePath returns the Git exclude file touched by installation,
// or an empty path when the workspace is not a Git checkout.
func LocalConfigExcludePath(runtimeName, workspace string) (string, error) {
	_, err := target(runtimeName, workspace)
	if err != nil {
		return "", err
	}
	gitDir, err := gitDirForWorkspace(workspace)
	if err != nil {
		return "", err
	}
	if gitDir == "" {
		return "", nil
	}
	return filepath.Join(gitDir, "info", "exclude"), nil
}

// Install writes the owned workspace-local lifecycle bindings for the given
// released CLI executable. The executable is explicit so an installed hook
// never depends on a consultant's PATH or shell profile.
func Install(runtimeName, workspace, executable string) (Status, error) {
	path, err := target(runtimeName, workspace)
	if err != nil {
		return Status{}, err
	}
	bindings, err := bindingsFor(runtimeName, executable, workspace)
	if err != nil {
		return Status{}, err
	}
	config, err := read(path)
	if err != nil {
		return Status{}, err
	}
	hooks, err := hooksMap(config)
	if err != nil {
		return Status{}, err
	}
	// Validate every existing target group before any configuration or Git
	// exclusion write.
	for _, binding := range bindings {
		groups, err := groupsForEvent(hooks, binding.NativeEvent)
		if err != nil {
			return Status{}, err
		}
		if err := validateEventGroups(binding.NativeEvent, groups); err != nil {
			return Status{}, err
		}
	}
	if err := rejectTrackedConfig(workspace, path); err != nil {
		return Status{}, err
	}
	snapshot, err := CaptureState(runtimeName, workspace)
	if err != nil {
		return Status{}, err
	}
	if runtimeName == "claude" {
		if _, err := installClaudeAgents(workspace); err != nil {
			return Status{}, rollbackState(snapshot, "adapter install", err)
		}
	}
	changed := false
	for _, binding := range bindings {
		groups, _ := groupsForEvent(hooks, binding.NativeEvent)
		updated, bindingChanged := updateOwnedEventHook(groups, runtimeName, binding.NativeEvent, binding.Command, binding.Async)
		if bindingChanged {
			hooks[binding.NativeEvent] = updated
			changed = true
		}
	}
	if err := excludeLocalConfig(workspace, path); err != nil {
		return Status{}, rollbackState(snapshot, "adapter install", err)
	}
	if !changed {
		return Status{Runtime: runtimeName, Path: path, State: "installed"}, nil
	}
	config["hooks"] = hooks
	if err := writeRuntimeConfig(path, config); err != nil {
		return Status{}, rollbackState(snapshot, "adapter install", err)
	}
	return Status{Runtime: runtimeName, Path: path, State: "installed"}, nil
}

func Uninstall(runtimeName, workspace string) (Status, error) {
	if runtimeName == "claude" {
		if err := claudeagents.ValidateUninstall(workspace); err != nil {
			return Status{}, err
		}
	}
	path, err := target(runtimeName, workspace)
	if err != nil {
		return Status{}, err
	}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		snapshot, snapshotErr := CaptureState(runtimeName, workspace)
		if snapshotErr != nil {
			return Status{}, snapshotErr
		}
		if runtimeName == "claude" {
			if _, removeErr := uninstallClaudeAgents(workspace); removeErr != nil {
				return Status{}, rollbackState(snapshot, "adapter uninstall", removeErr)
			}
		}
		return Status{Runtime: runtimeName, Path: path, State: "absent"}, nil
	} else if err != nil {
		return Status{}, err
	}
	config, err := read(path)
	if err != nil {
		return Status{}, err
	}
	hooks, err := hooksMap(config)
	if err != nil {
		return Status{}, err
	}
	bindings, err := bindingsFor(runtimeName, "bcgos", workspace)
	if err != nil {
		return Status{}, err
	}
	for _, binding := range bindings {
		groups, err := groupsForEvent(hooks, binding.NativeEvent)
		if err != nil {
			return Status{}, err
		}
		if err := validateEventGroups(binding.NativeEvent, groups); err != nil {
			return Status{}, err
		}
		filtered := removeOwnedHooks(groups, runtimeName, binding.NativeEvent)
		if len(filtered) == 0 {
			delete(hooks, binding.NativeEvent)
		} else {
			hooks[binding.NativeEvent] = filtered
		}
	}
	snapshot, err := CaptureState(runtimeName, workspace)
	if err != nil {
		return Status{}, err
	}
	config["hooks"] = hooks
	if err := writeRuntimeConfig(path, config); err != nil {
		return Status{}, rollbackState(snapshot, "adapter uninstall", err)
	}
	if runtimeName == "claude" {
		if _, err := uninstallClaudeAgents(workspace); err != nil {
			return Status{}, rollbackState(snapshot, "adapter uninstall", err)
		}
	}
	return Status{Runtime: runtimeName, Path: path, State: "removed"}, nil
}

func Inspect(runtimeName, workspace string) (Status, error) {
	path, err := target(runtimeName, workspace)
	if err != nil {
		return Status{}, err
	}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return Status{Runtime: runtimeName, Path: path, State: "absent"}, nil
	} else if err != nil {
		return Status{}, err
	}
	config, err := read(path)
	if err != nil {
		return Status{}, err
	}
	hooks, err := hooksMap(config)
	if err != nil {
		return Status{}, err
	}
	bindings, err := bindingsFor(runtimeName, "bcgos", workspace)
	if err != nil {
		return Status{}, err
	}
	for _, binding := range bindings {
		groups, err := groupsForEvent(hooks, binding.NativeEvent)
		if err != nil {
			return Status{}, err
		}
		if err := validateEventGroups(binding.NativeEvent, groups); err != nil {
			return Status{}, err
		}
	}
	if hasOwnedBindings(config, runtimeName) {
		if runtimeName == "claude" {
			agents, err := claudeagents.Inspect(workspace)
			if err != nil {
				return Status{}, err
			}
			if agents.State != "installed" {
				return Status{Runtime: runtimeName, Path: path, State: "partial"}, nil
			}
		}
		return Status{Runtime: runtimeName, Path: path, State: "installed"}, nil
	}
	return Status{Runtime: runtimeName, Path: path, State: "absent"}, nil
}

func target(runtimeName, workspace string) (string, error) {
	if runtimeName == "claude" {
		return filepath.Join(workspace, ".claude", "settings.local.json"), nil
	}
	if runtimeName == "codex" {
		return filepath.Join(workspace, ".codex", "hooks.json"), nil
	}
	return "", fmt.Errorf("unsupported runtime %q", runtimeName)
}

func commandFor(runtimeName, executable string) (string, error) {
	bindings, err := bindingsFor(runtimeName, executable)
	if err != nil {
		return "", err
	}
	return bindings[0].Command, nil
}

func bindingsFor(runtimeName, executable string, workspacePath ...string) ([]binding, error) {
	if strings.TrimSpace(executable) == "" {
		return nil, errors.New("adapter executable must not be empty")
	}
	abs, err := filepath.Abs(executable)
	if err != nil {
		return nil, fmt.Errorf("resolve adapter executable: %w", err)
	}
	prefix := quoteCommandPath(abs) + " hook "
	markers := adapterSourceMarker + " " + orchestrationStateMarker
	workspaceArgument := ""
	if len(workspacePath) > 0 && strings.TrimSpace(workspacePath[0]) != "" {
		workspace, err := filepath.Abs(filepath.Clean(workspacePath[0]))
		if err != nil {
			return nil, fmt.Errorf("resolve adapter workspace: %w", err)
		}
		workspaceArgument = " " + quoteCommandPath(workspace)
	}
	switch runtimeName {
	case "codex":
		return []binding{
			{NativeEvent: "SessionStart", Command: prefix + "session-start --runtime codex " + markers + workspaceArgument},
			{NativeEvent: "UserPromptSubmit", Command: prefix + "codex context-injection " + markers + workspaceArgument},
			{NativeEvent: "PreToolUse", Command: prefix + "codex pre-action-guard " + markers + workspaceArgument},
			{NativeEvent: "PostToolUse", Command: prefix + "codex post-action-receipt " + markers + workspaceArgument},
			{NativeEvent: "Stop", Command: prefix + "codex stop-finalization " + markers + workspaceArgument},
		}, nil
	case "claude":
		return []binding{
			{NativeEvent: "SessionStart", Command: prefix + "claude session-start " + markers + workspaceArgument},
			{NativeEvent: "UserPromptSubmit", Command: prefix + "claude context-injection " + markers + workspaceArgument},
			{NativeEvent: "PreToolUse", Command: prefix + "claude pre-action-guard " + markers + workspaceArgument},
			{NativeEvent: "PostToolUse", Command: prefix + "claude post-action-receipt " + markers + workspaceArgument, Async: true},
			// Stop is synchronous because an incomplete strategic agent route must
			// be able to return Claude's native blocking decision.
			{NativeEvent: "Stop", Command: prefix + "claude stop-finalization " + markers + workspaceArgument},
			{NativeEvent: "SubagentStart", Command: prefix + "claude subagent-start " + markers + workspaceArgument},
			{NativeEvent: "SubagentStop", Command: prefix + "claude subagent-stop " + markers + workspaceArgument},
		}, nil
	default:
		return nil, fmt.Errorf("unsupported runtime %q", runtimeName)
	}
}

func quoteCommandPath(path string) string {
	return quoteCommandPathFor(runtime.GOOS, path)
}

func quoteCommandPathFor(platform, path string) string {
	if platform == "windows" {
		return `"` + strings.ReplaceAll(path, `"`, `\"`) + `"`
	}
	return "'" + strings.ReplaceAll(path, "'", "'\"'\"'") + "'"
}

func read(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]any{}, nil
		}
		return nil, err
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parse runtime configuration %s: %w", path, err)
	}
	return config, nil
}

func write(path string, config map[string]any) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return writeBytes(path, data, 0o600)
}

func writeBytes(path string, data []byte, defaultMode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	mode := defaultMode
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".bcgos-adapter-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(mode); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
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

func hooksMap(config map[string]any) (map[string]any, error) {
	if value, exists := config["hooks"]; exists {
		if hooks, ok := value.(map[string]any); ok {
			return hooks, nil
		}
		return nil, errors.New("runtime configuration hooks must be an object")
	}
	return map[string]any{}, nil
}

func sessionGroups(hooks map[string]any) ([]any, error) {
	return groupsForEvent(hooks, "SessionStart")
}

func groupsForEvent(hooks map[string]any, event string) ([]any, error) {
	value, exists := hooks[event]
	if !exists {
		return nil, nil
	}
	groups, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("runtime configuration %s must be a list", event)
	}
	return groups, nil
}

func validateGroups(groups []any) error {
	return validateEventGroups("SessionStart", groups)
}

func validateEventGroups(event string, groups []any) error {
	for _, group := range groups {
		entry, ok := group.(map[string]any)
		if !ok {
			return fmt.Errorf("runtime configuration %s group must be an object", event)
		}
		entries, ok := entry["hooks"].([]any)
		if !ok {
			return fmt.Errorf("runtime configuration %s hooks must be a list", event)
		}
		for _, raw := range entries {
			if _, ok := raw.(map[string]any); !ok {
				return errors.New("runtime configuration hook entry must be an object")
			}
		}
	}
	return nil
}

func hasOwnedCommand(config map[string]any, runtimeName string) bool {
	hooks, err := hooksMap(config)
	if err != nil {
		return false
	}
	groups, err := sessionGroups(hooks)
	if err != nil {
		return false
	}
	for _, group := range groups {
		entry, ok := group.(map[string]any)
		if !ok {
			continue
		}
		entries, _ := entry["hooks"].([]any)
		for _, raw := range entries {
			hook, ok := raw.(map[string]any)
			if ok && isOwnedCommand(runtimeName, hook["command"]) {
				return true
			}
		}
	}
	return false
}

func hasOwnedBindings(config map[string]any, runtimeName string) bool {
	hooks, err := hooksMap(config)
	if err != nil {
		return false
	}
	bindings, err := bindingsFor(runtimeName, "bcgos")
	if err != nil {
		return false
	}
	for _, binding := range bindings {
		found := false
		groups, err := groupsForEvent(hooks, binding.NativeEvent)
		if err != nil || validateEventGroups(binding.NativeEvent, groups) != nil {
			return false
		}
		for _, group := range groups {
			entry := group.(map[string]any)
			for _, raw := range entry["hooks"].([]any) {
				hook := raw.(map[string]any)
				if isOwnedEventCommand(runtimeName, binding.NativeEvent, hook["command"]) &&
					hook["timeout"] == float64(5) &&
					asyncMatches(binding.Async, hook["async"]) {
					found = true
					break
				}
			}
			if found {
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func updateOwnedHook(groups []any, runtimeName, command string) ([]any, bool) {
	return updateOwnedEventHook(groups, runtimeName, "SessionStart", command, false)
}

func updateOwnedEventHook(groups []any, runtimeName, event, command string, asynchronous bool) ([]any, bool) {
	updated := make([]any, 0, len(groups)+1)
	installed := false
	changed := false
	for _, group := range groups {
		entry := group.(map[string]any)
		entries := entry["hooks"].([]any)
		kept := make([]any, 0, len(entries))
		for _, raw := range entries {
			hook := raw.(map[string]any)
			if !isOwnedEventCommand(runtimeName, event, hook["command"]) {
				kept = append(kept, raw)
				continue
			}
			if installed {
				changed = true
				continue
			}
			if hook["command"] != command || hook["timeout"] != float64(5) || !asyncMatches(asynchronous, hook["async"]) {
				changed = true
			}
			kept = append(kept, hookEntryFor(command, asynchronous))
			installed = true
		}
		copy := make(map[string]any, len(entry))
		for key, value := range entry {
			copy[key] = value
		}
		copy["hooks"] = kept
		updated = append(updated, copy)
	}
	if installed {
		return updated, changed
	}
	return append(updated, map[string]any{"hooks": []any{hookEntryFor(command, asynchronous)}}), true
}

func hookEntry(command string) map[string]any {
	return hookEntryFor(command, false)
}

func hookEntryFor(command string, asynchronous bool) map[string]any {
	entry := map[string]any{"type": "command", "command": command, "timeout": 5}
	if asynchronous {
		entry["async"] = true
	}
	return entry
}

func asyncMatches(expected bool, value any) bool {
	actual, _ := value.(bool)
	return actual == expected
}

func isOwnedCommand(runtimeName string, value any) bool {
	return isOwnedEventCommand(runtimeName, "SessionStart", value)
}

func isOwnedEventCommand(runtimeName, event string, value any) bool {
	command, ok := value.(string)
	if !ok {
		return false
	}
	command = strings.TrimSpace(command)
	if runtimeName == "codex" {
		commands := map[string]string{
			"SessionStart":     "session-start --runtime codex",
			"UserPromptSubmit": "codex context-injection",
			"PreToolUse":       "codex pre-action-guard",
			"PostToolUse":      "codex post-action-receipt",
			"Stop":             "codex stop-finalization",
		}
		suffix, exists := commands[event]
		if !exists {
			return false
		}
		return ownedMarkerMatches(command, suffix) ||
			strings.HasSuffix(command, " hook "+suffix+" "+adapterSourceMarker) ||
			command == "bcgos hook "+suffix
	}
	if runtimeName != "claude" {
		return false
	}
	commands := map[string]string{
		"SessionStart":     "claude session-start",
		"UserPromptSubmit": "claude context-injection",
		"PreToolUse":       "claude pre-action-guard",
		"PostToolUse":      "claude post-action-receipt",
		"Stop":             "claude stop-finalization",
		"SubagentStart":    "claude subagent-start",
		"SubagentStop":     "claude subagent-stop",
	}
	suffix, exists := commands[event]
	if !exists {
		return false
	}
	if event == "SessionStart" &&
		(ownedMarkerMatches(command, "session-start --runtime claude") ||
			strings.HasSuffix(command, " hook session-start --runtime claude "+adapterSourceMarker) ||
			command == "bcgos hook session-start --runtime claude") {
		return true
	}
	return ownedMarkerMatches(command, suffix) ||
		strings.HasSuffix(command, " hook "+suffix+" "+adapterSourceMarker) ||
		command == "bcgos hook "+suffix
}

// ownedMarkerMatches accepts the generated absolute workspace argument after
// the governance markers while keeping legacy marker-only commands readable.
// The marker is intentionally required as a contiguous command segment; a
// random mention in an argument cannot establish Maestro ownership.
func ownedMarkerMatches(command, suffix string) bool {
	marker := " hook " + suffix + " " + adapterSourceMarker + " " + orchestrationStateMarker
	index := strings.LastIndex(command, marker)
	if index < 0 {
		return false
	}
	remainder := strings.TrimSpace(command[index+len(marker):])
	if remainder == "" {
		return true
	}
	if len(remainder) < 2 || (remainder[0] != '\'' && remainder[0] != '"') || remainder[len(remainder)-1] != remainder[0] {
		return false
	}
	return strings.TrimSpace(remainder[1:len(remainder)-1]) != ""
}

func removeOwnedHooks(groups []any, runtimeName, event string) []any {
	filtered := make([]any, 0, len(groups))
	for _, group := range groups {
		entry := group.(map[string]any)
		entries := entry["hooks"].([]any)
		kept := make([]any, 0, len(entries))
		for _, raw := range entries {
			hook := raw.(map[string]any)
			if !isOwnedEventCommand(runtimeName, event, hook["command"]) {
				kept = append(kept, raw)
			}
		}
		if len(kept) > 0 {
			copied := make(map[string]any, len(entry))
			for key, value := range entry {
				copied[key] = value
			}
			copied["hooks"] = kept
			filtered = append(filtered, copied)
		}
	}
	return filtered
}

func rejectTrackedConfig(workspace, configPath string) error {
	relative, err := filepath.Rel(workspace, configPath)
	if err != nil {
		return err
	}
	if strings.HasPrefix(relative, "..") {
		return errors.New("adapter configuration must remain inside its workspace")
	}
	gitDir, err := gitDirForWorkspace(workspace)
	if err != nil {
		return err
	}
	if gitDir == "" {
		return nil
	}
	command := exec.Command("git", "-C", workspace, "ls-files", "--error-unmatch", "--", filepath.ToSlash(relative))
	if output, err := command.CombinedOutput(); err == nil {
		return fmt.Errorf("refusing to modify tracked runtime configuration %s; remove it from Git tracking before installing Maestro", filepath.ToSlash(relative))
	} else {
		var exitError *exec.ExitError
		if errors.As(err, &exitError) && exitError.ExitCode() == 1 {
			return nil
		}
		return fmt.Errorf("check whether runtime configuration is tracked: %w (%s)", err, strings.TrimSpace(string(output)))
	}
}

func ensureLocalConfigExcluded(workspace, configPath string) error {
	relative, err := filepath.Rel(workspace, configPath)
	if err != nil {
		return err
	}
	if strings.HasPrefix(relative, "..") {
		return errors.New("adapter configuration must remain inside its workspace")
	}
	gitDir, err := gitDirForWorkspace(workspace)
	if err != nil {
		return err
	}
	if gitDir == "" {
		return nil
	}
	excludePath := filepath.Join(gitDir, "info", "exclude")
	pattern := "/" + filepath.ToSlash(relative)
	data, err := os.ReadFile(excludePath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == pattern {
			return nil
		}
	}
	if len(data) > 0 && data[len(data)-1] != '\n' {
		data = append(data, '\n')
	}
	data = append(data, []byte(pattern+"\n")...)
	return writeBytes(excludePath, data, 0o600)
}

func gitDirForWorkspace(workspace string) (string, error) {
	marker := filepath.Join(workspace, ".git")
	info, err := os.Stat(marker)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return marker, nil
	}
	data, err := os.ReadFile(marker)
	if err != nil {
		return "", err
	}
	line := strings.TrimSpace(string(data))
	const prefix = "gitdir: "
	if !strings.HasPrefix(line, prefix) {
		return "", fmt.Errorf("unsupported .git file in %s", workspace)
	}
	gitDir := strings.TrimSpace(strings.TrimPrefix(line, prefix))
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(workspace, gitDir)
	}
	commonDir, err := os.ReadFile(filepath.Join(gitDir, "commondir"))
	if errors.Is(err, os.ErrNotExist) {
		return gitDir, nil
	}
	if err != nil {
		return "", err
	}
	common := strings.TrimSpace(string(commonDir))
	if !filepath.IsAbs(common) {
		common = filepath.Join(gitDir, common)
	}
	return common, nil
}
