package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

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
