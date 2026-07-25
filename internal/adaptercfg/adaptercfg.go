// Package adaptercfg manages only the Maestro-owned local hook entry in a
// workspace runtime configuration. It preserves unrelated user configuration.
package adaptercfg

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type Status struct {
	Runtime string `json:"runtime"`
	Path    string `json:"path"`
	State   string `json:"state"`
}

func Install(runtime, workspace string) (Status, error) {
	path, command, err := target(runtime, workspace)
	if err != nil {
		return Status{}, err
	}
	config, err := read(path)
	if err != nil {
		return Status{}, err
	}
	if hasCommand(config, command) {
		return Status{Runtime: runtime, Path: path, State: "installed"}, nil
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
	hooks["SessionStart"] = append(groups, map[string]any{"hooks": []any{map[string]any{
		"type": "command", "command": command, "timeout": 2,
	}}})
	config["hooks"] = hooks
	if err := write(path, config); err != nil {
		return Status{}, err
	}
	return Status{Runtime: runtime, Path: path, State: "installed"}, nil
}

func Uninstall(runtime, workspace string) (Status, error) {
	path, command, err := target(runtime, workspace)
	if err != nil {
		return Status{}, err
	}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return Status{Runtime: runtime, Path: path, State: "absent"}, nil
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
			if hook["command"] != command {
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
	return Status{Runtime: runtime, Path: path, State: "removed"}, nil
}

func Inspect(runtime, workspace string) (Status, error) {
	path, command, err := target(runtime, workspace)
	if err != nil {
		return Status{}, err
	}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return Status{Runtime: runtime, Path: path, State: "absent"}, nil
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
	if hasCommand(config, command) {
		return Status{Runtime: runtime, Path: path, State: "installed"}, nil
	}
	return Status{Runtime: runtime, Path: path, State: "absent"}, nil
}

func target(runtime, workspace string) (string, string, error) {
	if runtime == "claude" {
		return filepath.Join(workspace, ".claude", "settings.local.json"), "bcgos hook session-start --runtime claude", nil
	}
	if runtime == "codex" {
		return filepath.Join(workspace, ".codex", "hooks.json"), "bcgos hook session-start --runtime codex", nil
	}
	return "", "", fmt.Errorf("unsupported runtime %q", runtime)
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
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	temporary, err := os.CreateTemp(filepath.Dir(path), ".bcgos-adapter-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
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

func hasCommand(config map[string]any, command string) bool {
	hooks, err := hooksMap(config)
	if err != nil {
		return false
	}
	groups, err := sessionGroups(hooks)
	if err != nil {
		return false
	}
	for _, group := range groups {
		if groupHasCommand(group, command) {
			return true
		}
	}
	return false
}

func groupHasCommand(group any, command string) bool {
	entry, ok := group.(map[string]any)
	if !ok {
		return false
	}
	entries, _ := entry["hooks"].([]any)
	for _, raw := range entries {
		hook, ok := raw.(map[string]any)
		if ok && hook["command"] == command {
			return true
		}
	}
	return false
}
