package product_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/datahome"
)

type hookFixture struct{ binary, base, prod, owner, work, home string }

func newHookFixture(t *testing.T, binary string) hookFixture {
	t.Helper()
	base := physicalTempDir(t)
	f := hookFixture{binary: binary, base: base, prod: filepath.Join(base, "product"), owner: filepath.Join(base, "owner"), work: filepath.Join(base, "work")}
	for _, p := range []string{f.prod, f.owner} {
		if err := os.Mkdir(p, 0700); err != nil {
			t.Fatal(err)
		}
	}
	cmd := exec.Command(binary, append([]string{"init"}, f.flags()...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("init: %v %s", err, out)
	}
	h, err := datahome.Resolve(datahome.Options{ProductRoot: f.prod, UserHome: f.owner})
	if err != nil {
		t.Fatal(err)
	}
	f.home = h.Home
	return f
}
func (f hookFixture) flags() []string {
	return []string{"--product-root", f.prod, "--user-home", f.owner, "--workspace", f.work}
}
func (f hookFixture) input(event string) map[string]any {
	return map[string]any{"session_id": "session-a", "cwd": f.work, "hook_event_name": event, "prompt_id": "prompt-1"}
}
func (f hookFixture) raw(event string, body []byte) (string, string, int) {
	cmd := exec.Command(f.binary, append([]string{"hook", "--event", event}, f.flags()...)...)
	cmd.Stdin = bytes.NewReader(body)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	code := 0
	if err != nil {
		code = -1
		if e, ok := err.(*exec.ExitError); ok {
			code = e.ExitCode()
		}
	}
	return stdout.String(), stderr.String(), code
}
func (f hookFixture) call(event string, fields map[string]any) (string, string, int) {
	input := f.input(event)
	for k, v := range fields {
		input[k] = v
	}
	body, _ := json.Marshal(input)
	return f.raw(event, body)
}
func hookOK(t *testing.T, f hookFixture, event string, fields map[string]any) string {
	t.Helper()
	out, stderr, code := f.call(event, fields)
	if code != 0 {
		t.Fatalf("%s exit %d: %s", event, code, stderr)
	}
	return out
}
func toolFields(name, id string, input map[string]any) map[string]any {
	return map[string]any{"tool_name": name, "tool_use_id": id, "tool_input": input}
}
func hookDeny(t *testing.T, f hookFixture, fields map[string]any) {
	t.Helper()
	out := hookOK(t, f, "PreToolUse", fields)
	if !strings.Contains(out, `"permissionDecision":"deny"`) {
		t.Fatalf("expected denial, got %s", out)
	}
}
func hookAllow(t *testing.T, f hookFixture, fields map[string]any) {
	t.Helper()
	out := hookOK(t, f, "PreToolUse", fields)
	if strings.TrimSpace(out) != "{}" {
		t.Fatalf("expected no permission override, got %s", out)
	}
}
func dispatchFields(id, target string) map[string]any {
	return toolFields("Agent", id, map[string]any{"subagent_type": target, "prompt": "PRIVATE_CHILD_SENTINEL"})
}
func agentFields(id, target string) map[string]any {
	return map[string]any{"agent_id": id, "agent_type": target, "stop_hook_active": false}
}
func hookStart(t *testing.T, f hookFixture, id, target string) {
	t.Helper()
	hookAllow(t, f, dispatchFields("dispatch-"+id, target))
	hookOK(t, f, "SubagentStart", agentFields(id, target))
}
func treeSnapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		value := info.Mode().String()
		if info.Mode().IsRegular() {
			body, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			value += fmt.Sprintf(":%x", sha256.Sum256(body))
		}
		out[rel] = value
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// One real build supplies every public command oracle below. No native host is launched.
func TestCompiledClaudeHook(t *testing.T) {
	binary := filepath.Join(physicalTempDir(t), "maestro-mcp")
	if out, err := exec.Command("go", "build", "-o", binary, "./cmd/maestro-mcp").CombinedOutput(); err != nil {
		t.Fatalf("build: %v %s", err, out)
	}
	t.Run("root_removal_and_allowed_control", func(t *testing.T) {
		f := newHookFixture(t, binary)
		hookDeny(t, f, toolFields("Bash", "remove", map[string]any{"command": "rm -rf /"}))
		hookAllow(t, f, toolFields("Bash", "ordinary", map[string]any{"command": "printf hello"}))
	})
	t.Run("startup_read_only_and_current_context", func(t *testing.T) {
		f := newHookFixture(t, binary)
		before := treeSnapshot(t, f.base)
		out := hookOK(t, f, "SessionStart", map[string]any{"prompt_id": ""})
		if !strings.Contains(out, "maestro_context") || strings.Contains(out, "bcgos ") {
			t.Fatalf("invalid product context: %s", out)
		}
		if !reflect.DeepEqual(before, treeSnapshot(t, f.base)) {
			t.Fatal("startup changed fixture tree")
		}
	})
	t.Run("malformed_event_and_size_fail_closed", func(t *testing.T) {
		f := newHookFixture(t, binary)
		for event, want := range map[string]int{"SessionStart": 1, "UserPromptSubmit": 2, "PreToolUse": 2, "PostToolUse": 1, "SubagentStart": 1, "SubagentStop": 2, "Stop": 2} {
			out, stderr, code := f.raw(event, []byte("{PRIVATE_FAILURE_SENTINEL"))
			if code != want || out != "" || strings.Contains(stderr, "PRIVATE_FAILURE_SENTINEL") {
				t.Fatalf("%s failure contract %d/%d %q %q", event, code, want, out, stderr)
			}
		}
		for _, body := range [][]byte{[]byte("{PRIVATE_SENTINEL"), []byte(strings.Repeat("x", 65537)), []byte("null")} {
			out, stderr, code := f.raw("PreToolUse", body)
			if code != 2 || out != "" || strings.Contains(stderr, "PRIVATE_SENTINEL") {
				t.Fatalf("bad failure contract: %d %q %q", code, out, stderr)
			}
		}
		out, _, code := f.call("PreToolUse", map[string]any{"hook_event_name": "SessionStart"})
		if code != 2 || out != "" {
			t.Fatal("event mismatch admitted")
		}
		_, _, stopCode := f.call("Stop", nil)
		if stopCode != 2 {
			t.Fatal("missing Stop metadata admitted")
		}
		_, _, code = f.raw("SessionStart", []byte("{"))
		if code != 1 {
			t.Fatalf("startup error misclassified: %d", code)
		}
	})
	t.Run("paths_glob_regex_and_cross_case", func(t *testing.T) {
		f := newHookFixture(t, binary)
		outside := filepath.Join(f.base, "outside")
		os.Mkdir(outside, 0700)
		os.Symlink(outside, filepath.Join(f.work, "alias"))
		fields := toolFields("Read", "cross-cwd", map[string]any{"file_path": "safe.md"})
		fields["cwd"] = outside
		out, _, code := f.call("PreToolUse", fields)
		if code != 2 || out != "" {
			t.Fatal("foreign cwd admitted")
		}

		for _, input := range []map[string]any{{"pattern": "../outside/*"}, {"pattern": "alias/*"}, {"pattern": "**/*.md"}, {"pattern": "{safe,../outside}/*"}} {
			hookDeny(t, f, toolFields("Glob", "glob", input))
		}
		hookDeny(t, f, toolFields("Write", "outside", map[string]any{"file_path": filepath.Join(outside, "new.md")}))
		hookDeny(t, f, toolFields("Write", "secondary", map[string]any{"file_path": "safe.md", "path": outside}))
		os.Remove(filepath.Join(f.work, "alias"))
		nested := filepath.Join(f.work, "nested")
		os.Mkdir(nested, 0700)
		os.Symlink(outside, filepath.Join(nested, "escape"))
		os.Symlink(nested, filepath.Join(f.work, "internal-alias"))
		hookDeny(t, f, toolFields("Glob", "nested-alias", map[string]any{"pattern": "internal-alias/**/*.md"}))
		os.Remove(filepath.Join(nested, "escape"))
		os.Remove(filepath.Join(f.work, "internal-alias"))

		hookAllow(t, f, toolFields("Glob", "local", map[string]any{"pattern": "*.md"}))
		hookAllow(t, f, toolFields("Grep", "regex", map[string]any{"pattern": `../../[A-Z]+/.*`}))

		accounts := filepath.Join(f.work, "brain/accounts")
		os.MkdirAll(filepath.Join(accounts, "acme/cases/one"), 0700)
		os.MkdirAll(filepath.Join(accounts, "acme/cases/two"), 0700)
		os.WriteFile(filepath.Join(accounts, ".active"), []byte("acme/one"), 0600)
		hookAllow(t, f, toolFields("Write", "active", map[string]any{"file_path": "brain/accounts/acme/cases/one/new.md"}))
		hookDeny(t, f, toolFields("Write", "foreign", map[string]any{"file_path": "brain/accounts/acme/cases/two/new.md"}))
	})
	t.Run("namespace_and_predispatch", func(t *testing.T) {
		f := newHookFixture(t, binary)
		hookOK(t, f, "UserPromptSubmit", nil)
		for _, name := range []string{"maestro-product:unknown", "foreign:case-agent", "prefix:maestro-product:case-agent", "maestro-product:case-agent:extra"} {
			hookDeny(t, f, dispatchFields("bad", name))
		}
		hookDeny(t, f, dispatchFields("early-yoda", "maestro-product:yoda"))
		hookAllow(t, f, dispatchFields("case", "maestro-product:case-agent"))
		hookDeny(t, f, dispatchFields("parallel", "maestro-product:pa-expert"))
		_, _, code := f.call("SubagentStart", agentFields("wrong", "maestro-product:pa-expert"))
		if code != 1 {
			t.Fatal("mismatched start did not diagnose")
		}
		fields := toolFields("Read", "child-read", map[string]any{"file_path": "allowed.md"})
		fields["agent_id"] = "wrong"
		fields["agent_type"] = "maestro-product:pa-expert"
		hookDeny(t, f, fields)
		hookOK(t, f, "SubagentStart", agentFields("actual", "maestro-product:case-agent"))
		fields["agent_id"] = "actual"
		fields["agent_type"] = "maestro-product:case-agent"
		hookAllow(t, f, fields)
		fields["agent_type"] = "foreign:case-agent"
		hookDeny(t, f, fields)
		hookDeny(t, f, toolFields("Read", "missing-actor", map[string]any{"file_path": "allowed.md"}))
	})
	t.Run("observations_retries_and_body_privacy", func(t *testing.T) {
		f := newHookFixture(t, binary)
		hookOK(t, f, "UserPromptSubmit", map[string]any{"prompt": "PRIVATE_PROMPT_SENTINEL", "transcript_path": "/PRIVATE_TRANSCRIPT_SENTINEL"})
		hookStart(t, f, "case", "maestro-product:case-agent")
		before := treeSnapshot(t, f.home)
		hookOK(t, f, "SubagentStart", agentFields("case", "maestro-product:case-agent"))
		if !reflect.DeepEqual(before, treeSnapshot(t, f.home)) {
			t.Fatal("start retry mutated private state")
		}
		stop := agentFields("case", "maestro-product:case-agent")
		stop["last_assistant_message"] = "PRIVATE_RESULT_SENTINEL"
		hookOK(t, f, "SubagentStop", stop)
		before = treeSnapshot(t, f.home)
		hookOK(t, f, "SubagentStop", stop)
		if !reflect.DeepEqual(before, treeSnapshot(t, f.home)) {
			t.Fatal("stop retry mutated state")
		}
		hookStart(t, f, "gamma", "maestro-product:gamma-guardian")
		for _, tool := range []string{"Read", "Bash", "Agent", "mcp__anything"} {
			fields := toolFields(tool, "gamma-tool", map[string]any{"file_path": "safe.md", "command": "true", "subagent_type": "maestro-product:case-agent"})
			fields["agent_id"] = "gamma"
			fields["agent_type"] = "maestro-product:gamma-guardian"
			hookDeny(t, f, fields)
		}
		hookOK(t, f, "SubagentStop", agentFields("gamma", "maestro-product:gamma-guardian"))
		hookOK(t, f, "PostToolUse", toolFields("Agent", "dispatch-case", map[string]any{"subagent_type": "maestro-product:case-agent", "result": "PRIVATE_POST_SENTINEL"}))
		hookOK(t, f, "Stop", map[string]any{"stop_hook_active": false, "last_assistant_message": "PRIVATE_STOP_SENTINEL"})
		hookDeny(t, f, dispatchFields("dispatch-case", "maestro-product:case-agent"))
		hookOK(t, f, "UserPromptSubmit", map[string]any{"prompt_id": "prompt-2"})
		hookDeny(t, f, map[string]any{"prompt_id": "prompt-1", "tool_name": "Agent", "tool_use_id": "stale", "tool_input": map[string]any{"subagent_type": "maestro-product:case-agent"}})
		filepath.WalkDir(f.home, func(path string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() {
				body, _ := os.ReadFile(path)
				if strings.Contains(string(body), "PRIVATE_") {
					t.Errorf("private body persisted at %s", path)
				}
			}
			return err
		})
	})
	t.Run("optional_prompt_identity_never_resets", func(t *testing.T) {
		f := newHookFixture(t, binary)
		hookOK(t, f, "UserPromptSubmit", map[string]any{"prompt_id": ""})
		fields := dispatchFields("case", "case-agent")
		fields["prompt_id"] = ""
		hookAllow(t, f, fields)
		fields = agentFields("case", "case-agent")
		fields["prompt_id"] = ""
		hookOK(t, f, "SubagentStart", fields)
		out := hookOK(t, f, "UserPromptSubmit", map[string]any{"prompt_id": ""})
		if !strings.Contains(out, "turn_reset_unavailable") {
			t.Fatal("invented a turn identity")
		}
		hookOK(t, f, "SubagentStop", fields)
		hookOK(t, f, "Stop", map[string]any{"prompt_id": "", "stop_hook_active": false})
		out = hookOK(t, f, "UserPromptSubmit", map[string]any{"prompt_id": ""})
		if !strings.Contains(out, "turn_reset_unavailable") {
			t.Fatal("reset terminal flow without occurrence")
		}
	})
	t.Run("concurrent_reservations_choose_one", func(t *testing.T) {
		f := newHookFixture(t, binary)
		hookOK(t, f, "UserPromptSubmit", nil)
		results := make(chan string, 2)
		var wg sync.WaitGroup
		for _, id := range []string{"one", "two"} {
			wg.Add(1)
			go func(id string) {
				defer wg.Done()
				out, _, code := f.call("PreToolUse", dispatchFields(id, "case-agent"))
				results <- fmt.Sprintf("%d:%s", code, strings.TrimSpace(out))
			}(id)
		}
		wg.Wait()
		close(results)
		allowed := 0
		for out := range results {
			if out == "0:{}" {
				allowed++
			}
		}
		if allowed != 1 {
			t.Fatalf("concurrent reservations allowed %d", allowed)
		}
	})
	t.Run("private_alias_and_changed_binding", func(t *testing.T) {
		f := newHookFixture(t, binary)
		outside := filepath.Join(f.base, "outside")
		os.Mkdir(outside, 0700)
		os.Symlink(outside, filepath.Join(f.home, "state/native-agent-flow"))
		before := treeSnapshot(t, outside)
		_, _, code := f.call("UserPromptSubmit", nil)
		if code != 2 {
			t.Fatal("private alias admitted")
		}
		if !reflect.DeepEqual(before, treeSnapshot(t, outside)) {
			t.Fatal("private alias wrote outside")
		}
		o := product.Options{ProductRoot: f.prod, UserHome: f.owner, WorkspacePath: f.work}
		bound, err := product.Open(o)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(f.work, f.work+"-moved"); err != nil {
			t.Fatal(err)
		}
		payload, _ := json.Marshal(f.input("SessionStart"))
		var output bytes.Buffer
		if bound.HandleClaudeHook("SessionStart", bytes.NewReader(payload), &output) == nil || output.Len() != 0 {
			t.Fatal("stale bound instance admitted")
		}
		_, _, code = f.call("PreToolUse", toolFields("Read", "read", map[string]any{"file_path": "safe.md"}))
		if code != 2 {
			t.Fatal("stale command binding admitted")
		}
	})
	t.Run("failed_observation_grants_no_child_authority", func(t *testing.T) {
		f := newHookFixture(t, binary)
		hookOK(t, f, "UserPromptSubmit", nil)
		hookAllow(t, f, dispatchFields("case", "case-agent"))
		outside := filepath.Join(f.base, "receipt-outside")
		os.Mkdir(outside, 0700)
		os.Symlink(outside, filepath.Join(f.home, "state/runtime"))
		_, _, code := f.call("SubagentStart", agentFields("case", "case-agent"))
		if code != 1 {
			t.Fatal("receipt alias was not refused")
		}
		fields := toolFields("Read", "child-read", map[string]any{"file_path": "safe.md"})
		fields["agent_id"] = "case"
		fields["agent_type"] = "case-agent"
		hookDeny(t, f, fields)
		entries, _ := os.ReadDir(outside)
		if len(entries) != 0 {
			t.Fatal("receipt alias wrote outside")
		}
	})
	t.Run("truncated_state_never_reinitializes", func(t *testing.T) {
		f := newHookFixture(t, binary)
		hookOK(t, f, "UserPromptSubmit", nil)
		var state string
		err := filepath.WalkDir(filepath.Join(f.home, "state/native-agent-flow"), func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() && strings.HasSuffix(path, ".json") {
				state = path
			}
			return nil
		})
		if err != nil || state == "" {
			t.Fatalf("missing flow state: %v", err)
		}
		if err := os.WriteFile(state, nil, 0600); err != nil {
			t.Fatal(err)
		}
		hookDeny(t, f, toolFields("Read", "blocked", map[string]any{"file_path": "safe.md"}))
		_, _, code := f.call("UserPromptSubmit", map[string]any{"prompt_id": "prompt-2"})
		if code != 2 {
			t.Fatal("truncated state reinitialized")
		}
		body, _ := os.ReadFile(state)
		if len(body) != 0 {
			t.Fatal("corrupt state overwritten")
		}
	})
	t.Run("stop_recursion_has_no_completion_effect", func(t *testing.T) {
		f := newHookFixture(t, binary)
		hookOK(t, f, "UserPromptSubmit", nil)
		hookStart(t, f, "case", "case-agent")
		before := treeSnapshot(t, f.home)
		hookOK(t, f, "Stop", map[string]any{"stop_hook_active": true})
		if !reflect.DeepEqual(before, treeSnapshot(t, f.home)) {
			t.Fatal("recursive Stop synthesized a transition")
		}
		out := hookOK(t, f, "Stop", map[string]any{"stop_hook_active": false})
		if !strings.Contains(out, `"decision":"block"`) {
			t.Fatal("active route finalized")
		}
	})
}
