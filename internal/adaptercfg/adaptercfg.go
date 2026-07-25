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

type Status struct {
	Runtime string `json:"runtime"`
	Path    string `json:"path"`
	State   string `json:"state"`
}

// Install writes one workspace-local Session Start command for the given
// released CLI executable. The executable is explicit so an installed hook
// never depends on a consultant's PATH or shell profile.
func Install(runtimeName, workspace, executable string) (Status, error) {
	path, err := target(runtimeName, workspace)
	if err != nil {
		return Status{}, err
	}
	command, err := commandFor(runtimeName, executable)
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
	groups, err := sessionGroups(hooks)
	if err != nil {
		return Status{}, err
	}
	if err := validateGroups(groups); err != nil {
		return Status{}, err
	}
	updated, changed := updateOwnedHook(groups, runtimeName, command)
	if err := rejectTrackedConfig(workspace, path); err != nil {
		return Status{}, err
	}
	if err := ensureLocalConfigExcluded(workspace, path); err != nil {
		return Status{}, err
	}
	if !changed {
		return Status{Runtime: runtimeName, Path: path, State: "installed"}, nil
	}
	hooks["SessionStart"] = updated
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
	groups, err := sessionGroups(hooks)
	if err != nil {
		return Status{}, err
	}
	if err := validateGroups(groups); err != nil {
		return Status{}, err
	}
	filtered := make([]any, 0, len(groups))
	for _, group := range groups {
		entry := group.(map[string]any)
		entries := entry["hooks"].([]any)
		kept := make([]any, 0, len(entries))
		for _, raw := range entries {
			hook := raw.(map[string]any)
			if !isOwnedCommand(runtimeName, hook["command"]) {
				kept = append(kept, raw)
			}
		}
		if len(kept) > 0 {
			copy := make(map[string]any, len(entry))
			for key, value := range entry {
				copy[key] = value
			}
			copy["hooks"] = kept
			filtered = append(filtered, copy)
		}
	}
	if len(filtered) == 0 {
		delete(hooks, "SessionStart")
	} else {
		hooks["SessionStart"] = filtered
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
	groups, err := sessionGroups(hooks)
	if err != nil {
		return Status{}, err
	}
	if err := validateGroups(groups); err != nil {
		return Status{}, err
	}
	if hasOwnedCommand(config, runtimeName) {
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
	if strings.TrimSpace(executable) == "" {
		return "", errors.New("adapter executable must not be empty")
	}
	abs, err := filepath.Abs(executable)
	if err != nil {
		return "", fmt.Errorf("resolve adapter executable: %w", err)
	}
	return quoteCommandPath(abs) + " hook session-start --runtime " + runtimeName + " " + adapterSourceMarker, nil
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
	value, exists := hooks["SessionStart"]
	if !exists {
		return nil, nil
	}
	groups, ok := value.([]any)
	if !ok {
		return nil, errors.New("runtime configuration SessionStart must be a list")
	}
	return groups, nil
}

func validateGroups(groups []any) error {
	for _, group := range groups {
		entry, ok := group.(map[string]any)
		if !ok {
			return errors.New("runtime configuration SessionStart group must be an object")
		}
		entries, ok := entry["hooks"].([]any)
		if !ok {
			return errors.New("runtime configuration SessionStart hooks must be a list")
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

func updateOwnedHook(groups []any, runtimeName, command string) ([]any, bool) {
	updated := make([]any, 0, len(groups)+1)
	installed := false
	changed := false
	for _, group := range groups {
		entry := group.(map[string]any)
		entries := entry["hooks"].([]any)
		kept := make([]any, 0, len(entries))
		for _, raw := range entries {
			hook := raw.(map[string]any)
			if !isOwnedCommand(runtimeName, hook["command"]) {
				kept = append(kept, raw)
				continue
			}
			if installed {
				changed = true
				continue
			}
			if hook["command"] != command || hook["timeout"] != float64(2) {
				changed = true
			}
			kept = append(kept, hookEntry(command))
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
	return append(updated, map[string]any{"hooks": []any{hookEntry(command)}}), true
}

func hookEntry(command string) map[string]any {
	return map[string]any{"type": "command", "command": command, "timeout": 2}
}

func isOwnedCommand(runtimeName string, value any) bool {
	command, ok := value.(string)
	if !ok {
		return false
	}
	command = strings.TrimSpace(command)
	return strings.HasSuffix(command, " hook session-start --runtime "+runtimeName+" "+adapterSourceMarker) || command == "bcgos hook session-start --runtime "+runtimeName
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
