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

func TestBuildConnectsPluginToExternalStateAndRefusesOverwrite(t *testing.T) {
	base := t.TempDir()
	server := filepath.Join(base, "native server")
	if err := os.WriteFile(server, []byte("synthetic executable fixture"), 0700); err != nil {
		t.Fatal(err)
	}
	state := filepath.Join(base, "synthetic state with spaces")
	if err := os.Mkdir(state, 0700); err != nil {
		t.Fatal(err)
	}
	stateFile := filepath.Join(state, "do-not-change")
	if err := os.WriteFile(stateFile, []byte("unchanged"), 0600); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(base, "plugin with spaces")
	args := []string{"--destination", destination, "--server", server, "--state-root", state, "--workspace", "case-a", "--version", "0.0.2"}
	var stdout, stderr bytes.Buffer
	if code := run(args, &stdout, &stderr); code != 0 {
		t.Fatalf("exit=%d: %s", code, &stderr)
	}
	read := func(name string) []byte {
		t.Helper()
		b, err := os.ReadFile(filepath.Join(destination, name))
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	var config struct {
		Servers map[string]struct {
			Command string   `json:"command"`
			Args    []string `json:"args"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(read(".mcp.json"), &config); err != nil {
		t.Fatal(err)
	}
	entry, ok := config.Servers["maestro"]
	if !ok || strings.Join(entry.Args, "|") != "serve|--root|"+state+"|--workspace|case-a" {
		t.Fatalf("wrong launch: %+v", entry)
	}
	wantCommand := "${CLAUDE_PLUGIN_ROOT}/scripts/maestro-mcp-spike"
	if runtime.GOOS == "windows" {
		wantCommand += ".exe"
	}
	if entry.Command != wantCommand {
		t.Fatalf("command=%q", entry.Command)
	}
	if b := read(filepath.FromSlash(strings.TrimPrefix(wantCommand, "${CLAUDE_PLUGIN_ROOT}/"))); string(b) != "synthetic executable fixture" {
		t.Fatal("server was not copied")
	}
	for _, name := range []string{"operator", "resume"} {
		body := string(read(filepath.Join("skills", name, "SKILL.md")))
		if !strings.Contains(body, "revision=0.0.2") || !strings.Contains(body, "maestro_context") || strings.Contains(body, "{{VERSION}}") {
			t.Fatalf("invalid skill %s", name)
		}
	}
	if body := string(read("hooks/hooks.json")); !strings.Contains(body, "SessionStart") || !strings.Contains(body, "orient") || !strings.Contains(body, "0.0.2") {
		t.Fatal("orientation hook missing")
	}
	if _, err := os.Stat(filepath.Join(destination, "CLAUDE.md")); !os.IsNotExist(err) {
		t.Fatal("must not depend on CLAUDE.md")
	}
	if code := run(args, &stdout, &stderr); code == 0 {
		t.Fatal("overwriting existing plugin allowed")
	}
	if b, err := os.ReadFile(stateFile); err != nil || string(b) != "unchanged" {
		t.Fatal("external state changed")
	}
	entries, err := os.ReadDir(state)
	if err != nil || len(entries) != 1 {
		t.Fatal("builder wrote state")
	}
}

func TestBuildRejectsInvalidConfigurationBeforeCreatingDestination(t *testing.T) {
	base := t.TempDir()
	server := filepath.Join(base, "server")
	if err := os.WriteFile(server, []byte("fixture"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, root, workspace, version string }{
		{"relative-root", "relative", "case-a", "0.0.1"},
		{"unknown-workspace", base, "case-c", "0.0.1"},
		{"unsafe-version", base, "case-a", "../wrong"},
		{"overlapping-state", base, "case-a", "0.0.1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			destination := filepath.Join(base, tc.name)
			var out bytes.Buffer
			code := run([]string{"--destination", destination, "--server", server, "--state-root", tc.root, "--workspace", tc.workspace, "--version", tc.version}, &out, &out)
			if code == 0 {
				t.Fatal("invalid configuration accepted")
			}
			if _, err := os.Stat(destination); !os.IsNotExist(err) {
				t.Fatal("invalid build created destination")
			}
		})
	}
}
