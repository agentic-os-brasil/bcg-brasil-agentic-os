package packaging

import (
	"encoding/json"
	"strings"
)

type LifecycleCapability struct {
	Profile         string `json:"profile"`
	State           string `json:"state"`
	NativeExecution string `json:"native_execution"`
	Enforcement     string `json:"enforcement"`
	RequiredShell   string `json:"required_shell,omitempty"`
}

func lifecycleCapability(o Options) *LifecycleCapability {
	if !o.LifecycleContext {
		return nil
	}
	c := &LifecycleCapability{Profile: "lifecycle_context", State: "configured", NativeExecution: "not_observed", Enforcement: "unavailable"}
	if o.target.OS == "windows" {
		c.RequiredShell = "powershell"
	}
	return c
}

func lifecycleProfileFiles(o Options) (map[string][]byte, error) {
	hooks := map[string]any{}
	for _, event := range []string{"SessionStart", "UserPromptSubmit", "Stop"} {
		arguments := append([]string{"lifecycle-hook", "--event", event, "--product-root", o.Binding.ProductRoot, "--user-home", o.Binding.UserHome, "--workspace", o.Binding.WorkspacePath}, installedArgs(o.Binding)...)
		command := `exec "$CLAUDE_PLUGIN_ROOT/scripts/maestro-mcp"`
		quote := func(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'" }
		if o.target.OS == "windows" {
			command = `$ErrorActionPreference='Stop'; & "$env:CLAUDE_PLUGIN_ROOT\scripts\maestro-mcp.exe"`
			quote = func(value string) string { return "'" + strings.ReplaceAll(value, "'", "''") + "'" }
		}
		for _, argument := range arguments {
			command += " " + quote(argument)
		}
		if o.target.OS == "windows" {
			command += "; exit $LASTEXITCODE"
		}
		handler := map[string]any{"type": "command", "command": command, "timeout": 5}
		if o.target.OS == "windows" {
			handler["shell"] = "powershell"
		}
		if event == "Stop" {
			handler["async"] = true
		}
		hooks[event] = []any{map[string]any{"hooks": []any{handler}}}
	}
	body, err := json.MarshalIndent(map[string]any{"hooks": hooks}, "", "  ")
	if err != nil {
		return nil, err
	}
	return map[string][]byte{"hooks/hooks.json": append(body, '\n')}, nil
}
