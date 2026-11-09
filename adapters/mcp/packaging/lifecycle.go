package packaging

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
)

const passiveExecutionBoundary = "Configured lifecycle hooks read bounded committed memory and record typed observations only. Native enforcement, approval, execution completion and automatic memory maintenance remain unavailable."

type LifecycleCapability struct {
	Profile         string `json:"profile"`
	State           string `json:"state"`
	NativeExecution string `json:"native_execution"`
	Enforcement     string `json:"enforcement"`
	RequiredShell   string `json:"required_shell,omitempty"`
}

func lifecycleCapability(target Target) *LifecycleCapability {
	c := &LifecycleCapability{Profile: "lifecycle_context", State: "configured", NativeExecution: "not_observed", Enforcement: "unavailable"}
	if target.OS == "windows" {
		c.RequiredShell = "powershell"
	}
	return c
}

type lifecyclePacketBinding struct {
	UserHome  string `json:"user_home"`
	Workspace string `json:"workspace"`
}

func lifecycleBinding(o Options) *lifecyclePacketBinding {
	if !o.LifecycleContext {
		return nil
	}
	return &lifecyclePacketBinding{UserHome: o.Binding.UserHome, Workspace: o.Binding.WorkspacePath}
}
func (b lifecyclePacketBinding) validate(stable *stablePacketBinding) error {
	for _, p := range []string{b.UserHome, b.Workspace} {
		if !filepath.IsAbs(p) || filepath.Clean(p) != p || strings.ContainsAny(p, "\x00\r\n") {
			return errors.New("exact local lifecycle selection required")
		}
	}
	if stable != nil && (stable.UserHome != b.UserHome || stable.Workspace != b.Workspace) {
		return errors.New("lifecycle selection differs from stable client binding")
	}
	return nil
}
func lifecycleLocalMetadata(files map[string][]byte, platform string, b *lifecyclePacketBinding) error {
	if err := b.validate(nil); err != nil {
		return err
	}
	var manifest map[string]any
	if err := json.Unmarshal(files[".claude-plugin/plugin.json"], &manifest); err != nil {
		return err
	}
	delete(manifest, "userConfig")
	body, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	files[".claude-plugin/plugin.json"] = append(body, '\n')
	config := map[string]any{"mcpServers": map[string]any{"maestro": map[string]any{"type": "stdio", "command": "${CLAUDE_PLUGIN_ROOT}/scripts/" + binaryName(platform), "args": []string{"serve", "--packet", "--product-root", "${CLAUDE_PLUGIN_ROOT}", "--user-home", b.UserHome, "--workspace", b.Workspace}}}}
	body, err = json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	files[".mcp.json"] = append(body, '\n')
	files["README-PACKET.md"] = []byte("# Local Maestro lifecycle packet\n\nThis explicitly prepared Code packet binds the participant's exact Owner directory and Workspace. MCP and its three passive hooks use the same selection. Regenerate explicitly to change it. Keep this local packet out of shared distribution: it contains local paths but no private memory. Managed packet relocation remains supported. Startup never initializes or repairs. Configuration does not qualify a native host or enforcement.\n")
	return nil
}
func lifecycleProfileFiles(platform string, b *lifecyclePacketBinding, stable *stablePacketBinding) (map[string][]byte, error) {
	if b == nil {
		return nil, errors.New("lifecycle selection unavailable")
	}
	if err := b.validate(stable); err != nil {
		return nil, err
	}
	hooks := map[string]any{}
	for _, event := range []string{"SessionStart", "UserPromptSubmit", "Stop"} {
		args := []string{"lifecycle-hook", "--event", event, "--packet", "--product-root", "PLUGIN_ROOT_ENV", "--user-home", b.UserHome, "--workspace", b.Workspace}
		executable := ""
		if stable != nil {
			name := "maestro-install"
			if platform == "win32" {
				name += ".exe"
			}
			executable = filepath.Join(stable.ManagedRoot, name)
			args = []string{"stable-hook", "--event", event, "--product-root", stable.ManagedRoot, "--user-home", stable.UserHome, "--workspace", stable.Workspace, "--carrier-id", stable.Carrier.ID}
		}
		quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }
		command := "exec " + quote(executable)
		if stable == nil {
			command = "exec \"$CLAUDE_PLUGIN_ROOT/scripts/" + binaryName(platform) + "\""
		}
		if platform == "win32" {
			quote = func(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }
			command = "$ErrorActionPreference='Stop'; & " + quote(executable)
			if stable == nil {
				command = "$ErrorActionPreference='Stop'; & \"$env:CLAUDE_PLUGIN_ROOT\\scripts\\" + binaryName(platform) + "\""
			}
		}
		for _, arg := range args {
			if arg == "PLUGIN_ROOT_ENV" && stable == nil {
				if platform == "win32" {
					command += " \"$env:CLAUDE_PLUGIN_ROOT\""
				} else {
					command += " \"$CLAUDE_PLUGIN_ROOT\""
				}
			} else {
				command += " " + quote(arg)
			}
		}
		if platform == "win32" {
			command += "; exit $LASTEXITCODE"
		}
		h := map[string]any{"type": "command", "command": command, "timeout": 5}
		if platform == "win32" {
			h["shell"] = "powershell"
		}
		if event == "Stop" {
			h["async"] = true
		}
		hooks[event] = []any{map[string]any{"hooks": []any{h}}}
	}
	body, err := json.MarshalIndent(map[string]any{"hooks": hooks}, "", "  ")
	return map[string][]byte{"hooks/hooks.json": append(body, '\n')}, err
}
