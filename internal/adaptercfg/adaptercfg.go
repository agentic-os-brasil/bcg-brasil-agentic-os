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
)

const adapterSourceMarker = "--adapter-source maestro"
const orchestrationStateMarker = "--orchestration-state .bcgos/maestro-orchestration-state.json"

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

// ValidateInstall performs the read-only checks used by Install. Callers that
// coordinate another local transaction can use it to fail closed before either
// side writes.
func ValidateInstall(runtimeName, workspace, executable string) error {
	path, err := target(runtimeName, workspace)
	if err != nil {
		return err
	}
	bindings, err := bindingsFor(runtimeName, executable)
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
	bindings, err := bindingsFor(runtimeName, "bcgos")
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
	bindings, err := bindingsFor(runtimeName, executable)
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
	changed := false
	for _, binding := range bindings {
		groups, _ := groupsForEvent(hooks, binding.NativeEvent)
		updated, bindingChanged := updateOwnedEventHook(groups, runtimeName, binding.NativeEvent, binding.Command, binding.Async)
		if bindingChanged {
			hooks[binding.NativeEvent] = updated
			changed = true
		}
	}
	if err := ensureLocalConfigExcluded(workspace, path); err != nil {
		return Status{}, err
	}
	if !changed {
		return Status{Runtime: runtimeName, Path: path, State: "installed"}, nil
	}
	config["hooks"] = hooks
	if err := write(path, config); err != nil {
		return Status{}, err
	}
	return Status{Runtime: runtimeName, Path: path, State: "installed"}, nil
}

func Uninstall(runtimeName, workspace string) (Status, error) {
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
	bindings, err := bindingsFor(runtimeName, "bcgos")
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
	config["hooks"] = hooks
	if err := write(path, config); err != nil {
		return Status{}, err
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
	bindings, err := bindingsFor(runtimeName, "bcgos")
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

func bindingsFor(runtimeName, executable string) ([]binding, error) {
	if strings.TrimSpace(executable) == "" {
		return nil, errors.New("adapter executable must not be empty")
	}
	abs, err := filepath.Abs(executable)
	if err != nil {
		return nil, fmt.Errorf("resolve adapter executable: %w", err)
	}
	prefix := quoteCommandPath(abs) + " hook "
	markers := adapterSourceMarker + " " + orchestrationStateMarker
	switch runtimeName {
	case "codex":
		return []binding{
			{NativeEvent: "SessionStart", Command: prefix + "session-start --runtime codex " + markers},
			{NativeEvent: "UserPromptSubmit", Command: prefix + "codex context-injection " + markers},
			{NativeEvent: "PreToolUse", Command: prefix + "codex pre-action-guard " + markers},
			{NativeEvent: "PostToolUse", Command: prefix + "codex post-action-receipt " + markers},
			{NativeEvent: "Stop", Command: prefix + "codex stop-finalization " + markers},
		}, nil
	case "claude":
		return []binding{
			{NativeEvent: "SessionStart", Command: prefix + "claude session-start " + markers},
			{NativeEvent: "UserPromptSubmit", Command: prefix + "claude context-injection " + markers},
			{NativeEvent: "PreToolUse", Command: prefix + "claude pre-action-guard " + markers},
			{NativeEvent: "PostToolUse", Command: prefix + "claude post-action-receipt " + markers, Async: true},
			{NativeEvent: "Stop", Command: prefix + "claude stop-finalization " + markers, Async: true},
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
					hook["timeout"] == float64(2) &&
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
			if hook["command"] != command || hook["timeout"] != float64(2) || !asyncMatches(asynchronous, hook["async"]) {
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
	entry := map[string]any{"type": "command", "command": command, "timeout": 2}
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
		return strings.HasSuffix(command, " hook "+suffix+" "+adapterSourceMarker+" "+orchestrationStateMarker) ||
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
	}
	suffix, exists := commands[event]
	if !exists {
		return false
	}
	if event == "SessionStart" &&
		(strings.HasSuffix(command, " hook session-start --runtime claude "+adapterSourceMarker+" "+orchestrationStateMarker) ||
			strings.HasSuffix(command, " hook session-start --runtime claude "+adapterSourceMarker) ||
			command == "bcgos hook session-start --runtime claude") {
		return true
	}
	return strings.HasSuffix(command, " hook "+suffix+" "+adapterSourceMarker+" "+orchestrationStateMarker) ||
		strings.HasSuffix(command, " hook "+suffix+" "+adapterSourceMarker) ||
		command == "bcgos hook "+suffix
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
