// Non-production tracer bullet for issue #429; keep off the release path.
package main

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const markerName = "maestro-spike-denied.txt"

//go:embed compare-shells.sh
var compareBash []byte

//go:embed compare-shells.ps1
var comparePowerShell []byte

type event struct {
	HookEventName string `json:"hook_event_name"`
	Cwd           string `json:"cwd"`
	ToolName      string `json:"tool_name"`
	ToolInput     struct {
		FilePath     string `json:"file_path"`
		NotebookPath string `json:"notebook_path"`
	} `json:"tool_input"`
}

func run(args []string, in io.Reader, out, diagnostic io.Writer) int {
	if err := execute(args, in, out); err != nil {
		fmt.Fprintln(diagnostic, "maestro-spike:", err)
		return 2
	}
	return 0
}

func execute(args []string, in io.Reader, out io.Writer) error {
	if len(args) == 2 && args[0] == "build" {
		return build(args[1], out)
	}
	if len(args) != 1 {
		return errors.New("use build <new-directory>, session-start, guard or delegation")
	}
	if args[0] == "session-start" {
		root := os.Getenv("CLAUDE_PLUGIN_ROOT")
		if !filepath.IsAbs(root) {
			return errors.New("CLAUDE_PLUGIN_ROOT must be absolute")
		}
		cmd := exec.Command(filepath.Join(root, "scripts", executableName("maestro-runtime", runtime.GOOS)), "preflight")
		result, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("runtime preflight: %w: %s", err, result)
		}
		fmt.Fprintln(out, "MAESTRO_SPIKE revision=0.0.1; non-production; guard marker="+markerName+"; preflight="+strings.TrimSpace(string(result)))
		return nil
	}
	if args[0] != "guard" && args[0] != "delegation" {
		return errors.New("unknown mode")
	}
	body, err := io.ReadAll(io.LimitReader(in, 1024*1024+1))
	if err != nil {
		return err
	}
	if len(body) > 1024*1024 {
		return errors.New("hook input exceeds 1 MiB")
	}
	var ev event
	if err := json.Unmarshal(body, &ev); err != nil {
		return fmt.Errorf("invalid hook input: %w", err)
	}
	if ev.HookEventName != "PreToolUse" {
		return errors.New("expected PreToolUse event")
	}
	if args[0] == "delegation" {
		if ev.ToolName != "Agent" && ev.ToolName != "Task" {
			return errors.New("expected Agent or Task")
		}
		data := os.Getenv("CLAUDE_PLUGIN_DATA")
		if !filepath.IsAbs(data) {
			return errors.New("CLAUDE_PLUGIN_DATA must be absolute")
		}
		if err := os.MkdirAll(data, 0700); err != nil {
			return err
		}
		f, err := os.OpenFile(filepath.Join(data, "delegation.jsonl"), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0600)
		if err != nil {
			return err
		}
		err = json.NewEncoder(f).Encode(map[string]string{"tool_name": ev.ToolName})
		return errors.Join(err, f.Close())
	}
	if ev.ToolName != "Edit" && ev.ToolName != "MultiEdit" && ev.ToolName != "Write" && ev.ToolName != "NotebookEdit" {
		return errors.New("unexpected guarded tool")
	}
	path := ev.ToolInput.FilePath
	if ev.ToolName == "NotebookEdit" {
		path = ev.ToolInput.NotebookPath
	}
	if strings.TrimSpace(path) == "" || !filepath.IsAbs(ev.Cwd) {
		return errors.New("guard requires a path and absolute cwd")
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(ev.Cwd, path)
	}
	path = filepath.Clean(path)
	if strings.EqualFold(filepath.Base(path), markerName) {
		return errors.New("write denied to spike marker")
	}
	// shortcut: this probes one marker, replace with the product guard before production packaging.
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		if strings.EqualFold(filepath.Base(resolved), markerName) {
			return errors.New("write denied through marker symlink")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("resolve guarded path: %w", err)
	} else if info, statErr := os.Lstat(path); statErr == nil && info.Mode()&os.ModeSymlink != 0 {
		return errors.New("cannot resolve guarded symlink")
	}
	return nil
}

func executableName(name, goos string) string {
	if goos == "windows" {
		return name + ".exe"
	}
	return name
}

func pluginHooks(goos string) map[string]any {
	hook := func(mode string) []any {
		if goos == "windows" {
			return []any{map[string]any{"type": "command", "command": "${CLAUDE_PLUGIN_ROOT}/scripts/maestro-spike-hook.exe", "args": []string{mode}}}
		}
		return []any{map[string]string{"type": "command", "command": "\"${CLAUDE_PLUGIN_ROOT}/scripts/maestro-spike-hook\" " + mode}}
	}
	return map[string]any{"hooks": map[string]any{
		"SessionStart": []any{map[string]any{"hooks": hook("session-start")}},
		"PreToolUse":   []any{map[string]any{"matcher": "^(Edit|MultiEdit|Write|NotebookEdit)$", "hooks": hook("guard")}, map[string]any{"matcher": "^(Agent|Task)$", "hooks": hook("delegation")}},
	}}
}

func shellComparisonHooks() map[string]any {
	config := pluginHooks("windows")
	for _, groups := range config["hooks"].(map[string]any) {
		for _, raw := range groups.([]any) {
			group := raw.(map[string]any)
			mode := group["hooks"].([]any)[0].(map[string]any)["args"].([]string)[0]
			group["hooks"] = []any{
				map[string]any{"type": "command", "shell": "bash", "command": "bash \"${CLAUDE_PLUGIN_ROOT}/scripts/compare-shells.sh\" " + mode},
				map[string]any{"type": "command", "shell": "powershell", "command": "& \"${CLAUDE_PLUGIN_ROOT}/scripts/compare-shells.ps1\" " + mode},
			}
		}
	}
	return config
}

func build(destination string, out io.Writer) error {
	destination, err := filepath.Abs(destination)
	if err != nil {
		return err
	}
	if err := os.Mkdir(destination, 0700); err != nil {
		return fmt.Errorf("destination must be new (nothing overwritten): %w", err)
	}
	plugin := filepath.Join(destination, "versions", "0.0.1", "maestro-spike")
	for _, dir := range []string{filepath.Join(destination, ".claude-plugin"), filepath.Join(plugin, ".claude-plugin"), filepath.Join(plugin, "hooks"), filepath.Join(plugin, "skills", "probe"), filepath.Join(plugin, "scripts")} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	manifest := map[string]any{"name": "maestro-spike", "version": "0.0.1", "description": "Spike descartável da distribuição Maestro; fora de produção"}
	marketplace := map[string]any{"name": "maestro-spike", "owner": map[string]string{"name": "Maestro"}, "plugins": []any{map[string]string{"name": "maestro-spike", "source": "./versions/0.0.1/maestro-spike", "description": "Spike descartável"}}}
	hooks := pluginHooks(runtime.GOOS)
	manifests := map[string]any{filepath.Join(destination, ".claude-plugin", "marketplace.json"): marketplace, filepath.Join(plugin, ".claude-plugin", "plugin.json"): manifest, filepath.Join(plugin, "hooks", "hooks.json"): hooks}
	if runtime.GOOS == "windows" {
		manifests[filepath.Join(plugin, "hooks", "compare-shells.json")] = shellComparisonHooks()
		for name, body := range map[string][]byte{"compare-shells.sh": compareBash, "compare-shells.ps1": comparePowerShell} {
			if err := os.WriteFile(filepath.Join(plugin, "scripts", name), body, 0755); err != nil {
				return err
			}
		}
	}
	for path, value := range manifests {
		body, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(path, append(body, '\n'), 0644); err != nil {
			return err
		}
	}
	skill := "---\nname: probe\ndescription: Execute somente no teste assistido do spike Maestro para identificar a revisão carregada.\ndisable-model-invocation: true\n---\n\nResponda exatamente `MAESTRO_SPIKE_SKILL revision=0.0.1`.\n"
	if err := os.WriteFile(filepath.Join(plugin, "skills", "probe", "SKILL.md"), []byte(skill), 0644); err != nil {
		return err
	}
	for target, source := range map[string]string{"maestro-runtime": "./cmd/maestro-runtime", "maestro-spike-hook": "./dev/spikes/plugin429"} {
		cmd := exec.Command("go", "build", "-o", filepath.Join(plugin, "scripts", executableName(target, runtime.GOOS)), source)
		cmd.Stdout, cmd.Stderr = out, out
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("build %s (partial output retained): %w", target, err)
		}
	}
	fmt.Fprintln(out, destination)
	return nil
}

func main() { os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr)) }
