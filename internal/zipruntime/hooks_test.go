package zipruntime

import (
	"bytes"
	"encoding/json"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/lifecycle"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCodexLifecycleReceiptAndMigration(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "data/profile"), 0700)
	os.WriteFile(filepath.Join(root, "data/profile/identity.md"), []byte("private body"), 0600)
	var start bytes.Buffer
	if err := Run([]string{"codex-hook", "SessionStart", "--root", root}, strings.NewReader("{\"session_id\":\"fixture\"}"), &start); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "brain/owner/identity.md")); err != nil {
		t.Fatal("session start did not migrate", err)
	}
	for _, event := range []string{"PostToolUse", "Stop"} {
		var out bytes.Buffer
		if err := Run([]string{"codex-hook", event, "--root", root}, strings.NewReader("{\"session_id\":\"fixture\",\"tool_name\":\"Write\",\"tool_use_id\":\"tool1\",\"prompt\":\"private body\"}"), &out); err != nil {
			t.Fatal(err)
		}
	}
	summary, err := lifecycle.DiagnoseRuntime(filepath.Join(root, "brain/.maestro"), lifecycle.IdempotencyKey(root), "codex")
	if err != nil || summary.Observed != 2 {
		t.Fatalf("missing adapter evidence: %+v %v", summary, err)
	}
	if strings.Contains(start.String(), "private body") {
		t.Fatal("private content leaked")
	}
}

func TestCodexGuardCasePaths(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "brain/accounts/a/cases/one"), 0700)
	os.MkdirAll(filepath.Join(root, "brain/accounts/b/cases/two"), 0700)
	os.WriteFile(filepath.Join(root, "brain/accounts/.active"), []byte("a/one"), 0600)
	for _, tc := range []struct {
		name, tool string
		input      map[string]any
		deny       bool
	}{
		{"same", "Write", map[string]any{"file_path": "brain/accounts/a/cases/one/ok.md"}, false},
		{"different", "Edit", map[string]any{"file_path": "brain/accounts/b/cases/two/no.md"}, true},
		{"traversal", "Write", map[string]any{"file_path": "brain/accounts/a/cases/one/../../../b/cases/two/no.md"}, true},
		{"patch", "apply_patch", map[string]any{"command": "*** Begin Patch\n*** Add File: brain/accounts/b/cases/two/no.md\n+x\n*** End Patch"}, true},
		{"patchmove", "apply_patch", map[string]any{"command": "*** Begin Patch\n*** Update File: brain/accounts/a/cases/one/a.md\n*** Move to: brain/accounts/b/cases/two/a.md\n@@\n-x\n+y\n*** End Patch"}, true},
		{"missing", "Write", map[string]any{}, true},
		{"agent", "Agent", map[string]any{"subagent_type": "case-agent"}, false},
		{"spawn", "spawn_agent", map[string]any{"agent_type": "case-agent"}, false},
		{"read", "Read", map[string]any{"file_path": "brain/accounts/b/cases/two/no.md"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, _ := json.Marshal(map[string]any{"session_id": "test-session", "tool_name": tc.tool, "tool_input": tc.input})
			var out bytes.Buffer
			if err := Run([]string{"codex-hook", "PreToolUse", "--root", root}, bytes.NewReader(b), &out); err != nil {
				t.Fatal(err)
			}
			var got struct {
				Hook map[string]any `json:"hookSpecificOutput"`
			}
			if err := json.Unmarshal(out.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if (got.Hook["permissionDecision"] == "deny") != tc.deny {
				t.Fatalf("denial=%v output=%s", tc.deny, out.String())
			}
		})
	}
	if err := os.Symlink(filepath.Join(root, "brain/accounts/b/cases/two"), filepath.Join(root, "alias")); err == nil {
		b := `{"tool_name":"Write","tool_input":{"file_path":"alias/leak.md"}}`
		var out bytes.Buffer
		Run([]string{"codex-hook", "PreToolUse", "--root", root}, strings.NewReader(b), &out)
		if !strings.Contains(out.String(), `"permissionDecision":"deny"`) {
			t.Fatal(out.String())
		}
	}
}

func TestCodexMalformedFailClosedAndNoPayloadLeak(t *testing.T) {
	for _, body := range []string{"{", strings.Repeat("x", 65537), `{"tool_name":"apply_patch","tool_input":{"command":"secret-malformed"}}`} {
		var out bytes.Buffer
		if err := Run([]string{"codex-hook", "PreToolUse", "--root", t.TempDir()}, strings.NewReader(body), &out); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out.String(), `"permissionDecision":"deny"`) || strings.Contains(out.String(), "secret-malformed") {
			t.Fatal(out.String())
		}
	}
}

func TestCodexRelativeTargetUsesSessionCWD(t *testing.T) {
	root := t.TempDir()
	cwd := filepath.Join(root, "brain/accounts/b/cases/two")
	os.MkdirAll(cwd, 0700)
	os.WriteFile(filepath.Join(root, "brain/accounts/.active"), []byte("a/one"), 0600)
	b, _ := json.Marshal(map[string]any{"cwd": cwd, "tool_name": "Write", "tool_input": map[string]any{"file_path": "secret.md"}})
	var out bytes.Buffer
	if err := Run([]string{"codex-hook", "PreToolUse", "--root", root}, bytes.NewReader(b), &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "deny") {
		t.Fatal("relative cwd escaped case guard", out.String())
	}
}
