//go:build maestro_experimental_installed_seed

package packaging

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"regexp"
)

// This renderer consumes already authenticated construction metadata, or
// reconstructs a recipe for inspection. It never turns metadata into authority.
func canonicalNativeProfile(platform, binding string, stable *stablePacketBinding) (map[string][]byte, error) {
	if stable == nil {
		return nil, errors.New("experimental full native profile requires the authenticated stable binding")
	}
	if err := stable.validate(); err != nil {
		return nil, err
	}
	if binding != "" && !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(binding) {
		return nil, errors.New("exact native selection digest required")
	}
	files, err := nativeProfilePayload(platform, binding)
	if err != nil {
		return nil, err
	}
	var config map[string]any
	if err := json.Unmarshal(files["hooks/hooks.json"], &config); err != nil {
		return nil, err
	}
	launcher := "maestro-install"
	if platform == "win32" {
		launcher += ".exe"
	}
	for event, rows := range config["hooks"].(map[string]any) {
		for _, row := range rows.([]any) {
			for _, raw := range row.(map[string]any)["hooks"].([]any) {
				hook := raw.(map[string]any)
				hook["command"] = filepath.Join(stable.ManagedRoot, launcher)
				hook["args"] = []string{"stable-native-hook", "--event", event, "--binding-sha256", binding, "--product-root", stable.ManagedRoot, "--user-home", stable.UserHome, "--workspace", stable.Workspace, "--carrier-id", stable.Carrier.ID}
			}
		}
	}
	body, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return nil, err
	}
	files["hooks/hooks.json"] = append(body, '\n')
	return files, nil
}
