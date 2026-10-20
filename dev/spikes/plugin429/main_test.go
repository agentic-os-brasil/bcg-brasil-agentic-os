package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGuardMarkerAndInputFailures(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "maestro-spike-denied.txt")
	if err := os.WriteFile(marker, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(dir, "alias.txt")
	if err := os.Symlink(marker, alias); err != nil {
		t.Fatal(err)
	}
	dangling := filepath.Join(dir, "dangling.txt")
	if err := os.Symlink(filepath.Join(dir, "missing", markerName), dangling); err != nil {
		t.Fatal(err)
	}
	for _, tool := range []string{"Edit", "MultiEdit", "Write", "NotebookEdit"} {
		for _, path := range []string{marker, "./maestro-spike-denied.txt", "nested/../maestro-spike-denied.txt", alias, dangling} {
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
}

func TestGuardAllowsOtherFile(t *testing.T) {
	input := `{"hook_event_name":"PreToolUse","tool_name":"Write","cwd":"/workspace","tool_input":{"file_path":"allowed.txt"}}`
	if code := run([]string{"guard"}, strings.NewReader(input), &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
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
