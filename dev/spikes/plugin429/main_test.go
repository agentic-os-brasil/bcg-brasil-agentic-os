package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	if os.Getenv("MAESTRO_SPIKE_WRAPPER_HELPER") == "1" {
		if len(os.Args) != 2 || os.Args[1] != os.Getenv("MAESTRO_SPIKE_WRAPPER_MODE") {
			os.Exit(3)
		}
		io.Copy(os.Stdout, os.Stdin)
		fmt.Fprintln(os.Stderr, "NATIVE_STDERR")
		if os.Getenv("MAESTRO_SPIKE_WRAPPER_DENY") == "1" {
			os.Exit(2)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestGuardMarkerAndInputFailures(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "maestro-spike-denied.txt")
	if err := os.WriteFile(marker, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tool := range []string{"Edit", "MultiEdit", "Write", "NotebookEdit"} {
		for _, path := range []string{marker, "./maestro-spike-denied.txt", "nested/../maestro-spike-denied.txt"} {
			input, _ := json.Marshal(map[string]any{"hook_event_name": "PreToolUse", "cwd": dir, "tool_name": tool, "tool_input": map[string]string{"file_path": path, "notebook_path": path}})
			var out, diagnostic bytes.Buffer
			if code := run([]string{"guard"}, bytes.NewReader(input), &out, &diagnostic); code != 2 {
				t.Fatalf("%s %s: code=%d output=%s diagnostic=%s", tool, path, code, &out, &diagnostic)
			}
		}
	}
	for _, input := range []string{"{", "{}", "null", `{"hook_event_name":"PreToolUse","tool_name":"Write"}`, strings.Repeat("x", 1024*1024+1)} {
		if code := run([]string{"guard"}, strings.NewReader(input), &bytes.Buffer{}, &bytes.Buffer{}); code != 2 {
			t.Fatalf("bad input returned %d", code)
		}
	}
	body, _ := os.ReadFile(marker)
	if string(body) != "keep" {
		t.Fatal("guard changed the marker")
	}

	t.Run("symlinks", func(t *testing.T) {
		for _, target := range []string{marker, filepath.Join(dir, "missing", markerName)} {
			alias := filepath.Join(t.TempDir(), "alias.txt")
			if err := os.Symlink(target, alias); err != nil {
				if runtime.GOOS == "windows" {
					t.Skipf("symlink creation unavailable on this Windows runner: %v", err)
				}
				t.Fatal(err)
			}
			for _, tool := range []string{"Edit", "MultiEdit", "Write", "NotebookEdit"} {
				input, _ := json.Marshal(map[string]any{"hook_event_name": "PreToolUse", "cwd": dir, "tool_name": tool, "tool_input": map[string]string{"file_path": alias, "notebook_path": alias}})
				var out, diagnostic bytes.Buffer
				if code := run([]string{"guard"}, bytes.NewReader(input), &out, &diagnostic); code != 2 {
					t.Fatalf("%s %s: code=%d output=%s diagnostic=%s", tool, alias, code, &out, &diagnostic)
				}
			}
		}
	})
}

func TestGuardAllowsOtherFile(t *testing.T) {
	input, err := json.Marshal(map[string]any{"hook_event_name": "PreToolUse", "tool_name": "Write", "cwd": t.TempDir(), "tool_input": map[string]string{"file_path": "allowed.txt"}})
	if err != nil {
		t.Fatal(err)
	}
	if code := run([]string{"guard"}, bytes.NewReader(input), &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatalf("code=%d", code)
	}
}

func TestDelegationLogContainsOnlyTool(t *testing.T) {
	t.Setenv("CLAUDE_PLUGIN_DATA", t.TempDir())
	for _, tool := range []string{"Agent", "Task"} {
		input := `{"hook_event_name":"PreToolUse","tool_name":"` + tool + `","tool_input":{"prompt":"do not retain this"}}`
		if code := run([]string{"delegation"}, strings.NewReader(input), &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
			t.Fatalf("code=%d", code)
		}
	}
	body, err := os.ReadFile(filepath.Join(os.Getenv("CLAUDE_PLUGIN_DATA"), "delegation.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "{\"tool_name\":\"Agent\"}\n{\"tool_name\":\"Task\"}\n" {
		t.Fatalf("unexpected log %s", body)
	}
}

func TestBuildRefusesExistingDestination(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "keep.txt")
	if err := os.WriteFile(file, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if code := run([]string{"build", dir}, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{}); code != 2 {
		t.Fatalf("existing destination accepted: %d", code)
	}
	body, err := os.ReadFile(file)
	if err != nil || string(body) != "keep" {
		t.Fatalf("existing file changed: %s %v", body, err)
	}
}

func TestPlatformHookLaunch(t *testing.T) {
	for _, goos := range []string{"windows", "darwin", "linux"} {
		t.Run(goos, func(t *testing.T) {
			body, err := json.Marshal(pluginHooks(goos))
			if err != nil {
				t.Fatal(err)
			}
			var config struct {
				Hooks map[string][]struct {
					Hooks []struct {
						Command string   `json:"command"`
						Args    []string `json:"args"`
					} `json:"hooks"`
				} `json:"hooks"`
			}
			if err := json.Unmarshal(body, &config); err != nil {
				t.Fatal(err)
			}
			modes := map[string]int{}
			for _, groups := range config.Hooks {
				for _, group := range groups {
					for _, hook := range group.Hooks {
						if goos == "windows" {
							if hook.Command != "${CLAUDE_PLUGIN_ROOT}/scripts/maestro-spike-hook.exe" || len(hook.Args) != 1 {
								t.Fatalf("Windows requires native exec form: %+v", hook)
							}
							modes[hook.Args[0]]++
						} else if hook.Args != nil || !strings.HasPrefix(hook.Command, `"${CLAUDE_PLUGIN_ROOT}/scripts/maestro-spike-hook" `) {
							t.Fatalf("changed Unix launch contract: %+v", hook)
						}
					}
				}
			}
			if goos == "windows" && (len(modes) != 3 || modes["session-start"] != 1 || modes["guard"] != 1 || modes["delegation"] != 1) {
				t.Fatalf("missing hook modes: %v", modes)
			}
			want := "maestro-runtime"
			if goos == "windows" {
				want += ".exe"
			}
			if got := executableName("maestro-runtime", goos); got != want {
				t.Fatalf("runtime name=%s want=%s", got, want)
			}
		})
	}
}

func TestShellComparisonManifest(t *testing.T) {
	config := shellComparisonHooks()
	hooks := config["hooks"].(map[string]any)
	for event, groups := range hooks {
		for _, raw := range groups.([]any) {
			group := raw.(map[string]any)
			mode := "session-start"
			if event == "PreToolUse" {
				switch group["matcher"] {
				case "^(Edit|MultiEdit|Write|NotebookEdit)$":
					mode = "guard"
				case "^(Agent|Task)$":
					mode = "delegation"
				default:
					t.Fatalf("unexpected comparison matcher: %v", group["matcher"])
				}
			}
			handlers := group["hooks"].([]any)
			if len(handlers) != 2 {
				t.Fatalf("%s: expected both shells", mode)
			}
			for i, shell := range []string{"bash", "powershell"} {
				handler := handlers[i].(map[string]any)
				if handler["shell"] != shell || handler["args"] != nil || handler["type"] != "command" {
					t.Fatalf("shell form required: %v", handler)
				}
				command := handler["command"].(string)
				if !strings.Contains(command, "${CLAUDE_PLUGIN_ROOT}/scripts/compare-shells.") || !strings.HasSuffix(command, " "+mode) {
					t.Fatalf("bad shell command: %s", command)
				}
			}
		}
	}
	if len(hooks) != 2 || len(hooks["SessionStart"].([]any)) != 1 || len(hooks["PreToolUse"].([]any)) != 2 {
		t.Fatalf("missing comparison events/groups: %v", hooks)
	}
}

func TestShellComparisonWrappers(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "path with spaces ç")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(self)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "maestro-spike-hook.exe"), body, 0700); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string][]byte{"compare-shells.sh": compareBash, "compare-shells.ps1": comparePowerShell} {
		if err := os.WriteFile(filepath.Join(dir, name), body, 0700); err != nil {
			t.Fatal(err)
		}
	}
	for _, shell := range []string{"bash", "powershell.exe", "pwsh"} {
		t.Run(shell, func(t *testing.T) {
			launcher, err := exec.LookPath(shell)
			if err != nil {
				t.Skipf("comparison shell unavailable: %s", shell)
			}
			id, script := "bash", "compare-shells.sh"
			if shell != "bash" {
				id, script = "powershell", "compare-shells.ps1"
			}
			for _, mode := range []string{"session-start", "guard", "delegation"} {
				for _, deny := range []string{"0", "1"} {
					args := []string{filepath.Join(dir, script), mode}
					if shell != "bash" {
						args = append([]string{"-NoProfile", "-File"}, args...)
					}
					cmd := exec.Command(launcher, args...)
					cmd.Env = append(os.Environ(), "MAESTRO_SPIKE_WRAPPER_HELPER=1", "MAESTRO_SPIKE_WRAPPER_MODE="+mode, "MAESTRO_SPIKE_WRAPPER_DENY="+deny)
					input := `{"cwd":"path with spaces ç","tool_name":"Write"}`
					cmd.Stdin = strings.NewReader(input)
					var stdout, stderr bytes.Buffer
					cmd.Stdout, cmd.Stderr = &stdout, &stderr
					err := cmd.Run()
					if cmd.ProcessState == nil {
						t.Fatalf("start %s wrapper: %v", shell, err)
					}
					want := 0
					if deny == "1" {
						want = 2
					}
					if code := cmd.ProcessState.ExitCode(); code != want {
						t.Fatalf("%s deny=%s: code=%d err=%v stderr=%s", mode, deny, code, err, &stderr)
					}
					if stdout.String() != input {
						t.Fatalf("%s changed stdin/stdout: %q", shell, stdout.String())
					}
					if stderr.String() != "MAESTRO_SPIKE_HANDLER="+id+"/"+mode+"\nNATIVE_STDERR\n" && stderr.String() != "MAESTRO_SPIKE_HANDLER="+id+"/"+mode+"\r\nNATIVE_STDERR\n" {
						t.Fatalf("%s lost handler/native diagnostic: %q", shell, stderr.String())
					}
				}
			}
		})
	}
}
