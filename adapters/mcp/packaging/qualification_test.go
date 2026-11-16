package packaging

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
)

func TestQualificationScopeCannotSupplyCanonicalOrFaultPass(t *testing.T) {
	in := nativeFixture(t)
	o, _, err := prepare(in.Packet)
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(in.ScratchRoot, "scope")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(in.ChallengeDirectory, 0700); err != nil {
		t.Fatal(err)
	}
	p, err := prepareNativeProbe(o, root, in.ChallengeDirectory, 0, "missing")
	if err != nil {
		t.Fatal(err)
	}
	input := func(tool string, fields map[string]any) []byte {
		body, _ := json.Marshal(map[string]any{"hook_event_name": "PreToolUse", "session_id": p.session, "cwd": p.binding.WorkspacePath, "tool_name": tool, "tool_use_id": "scope-tool", "tool_input": fields})
		return body
	}
	for _, tc := range []struct {
		name, tool string
		fields     map[string]any
		allowed    bool
	}{
		{"control read", "Read", map[string]any{"file_path": filepath.Join(p.challenge, "allowed.txt")}, true},
		{"control write", "Write", map[string]any{"file_path": filepath.Join(p.challenge, "allowed-write.txt"), "content": "Synthetic native allowed write control.\n"}, true},
		{"public outside negative read", "Read", map[string]any{"file_path": p.outside}, true},
		{"fault edit", "Edit", map[string]any{"file_path": filepath.Join(p.challenge, "edit.txt"), "old_string": "original", "new_string": "changed"}, true},
		{"case dispatch", "Agent", map[string]any{"subagent_type": "maestro-product:case-agent"}, true},
		{"unclosed bare case alias", "Agent", map[string]any{"subagent_type": "case-agent"}, false},
		{"gamma dispatch", "Agent", map[string]any{"subagent_type": "maestro-product:gamma-guardian"}, true},
		{"unreviewed workspace read", "Read", map[string]any{"file_path": filepath.Join(p.binding.WorkspacePath, "README.md")}, false},
		{"unreviewed write", "Write", map[string]any{"file_path": filepath.Join(p.challenge, "allowed.txt"), "content": "unreviewed"}, false},
		{"unreviewed edit", "Edit", map[string]any{"file_path": filepath.Join(p.binding.WorkspacePath, "README.md"), "old_string": "x", "new_string": "y"}, false},
		{"arbitrary agent", "Agent", map[string]any{"subagent_type": "general-purpose"}, false},
		{"background child", "Agent", map[string]any{"subagent_type": "maestro-product:case-agent", "run_in_background": true}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			if err := QualificationScopeGuard(p.binding, p.challenge, p.outside, bytes.NewReader(input(tc.tool, tc.fields)), &out); err != nil {
				t.Fatal(err)
			}
			if tc.allowed {
				if string(bytes.TrimSpace(out.Bytes())) != "{}" {
					t.Fatal("scope instrumentation masked an exact reviewed control")
				}
			} else if !bytes.Contains(out.Bytes(), []byte(`"permissionDecision":"deny"`)) {
				t.Fatal("experiment allowed an unreviewed effect")
			}
			if bytes.Contains(out.Bytes(), []byte("maestro_native_canonical_hook")) {
				t.Fatal("scope guard manufactured canonical attribution")
			}
		})
	}
	bound, err := product.Open(p.binding)
	if err != nil {
		t.Fatal(err)
	}
	begin, _ := json.Marshal(map[string]any{"hook_event_name": "UserPromptSubmit", "session_id": p.session, "cwd": p.binding.WorkspacePath})
	var discard bytes.Buffer
	if err := bound.HandleClaudeHook("UserPromptSubmit", bytes.NewReader(begin), &discard); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := QualificationHook(p.binding, p.nonce, bytes.NewReader(input("Read", map[string]any{"file_path": p.outside})), &out); err != nil {
		t.Fatal(err)
	}
	var value struct {
		Message  string `json:"systemMessage"`
		Specific struct {
			Decision string `json:"permissionDecision"`
		} `json:"hookSpecificOutput"`
	}
	if json.Unmarshal(out.Bytes(), &value) != nil || value.Specific.Decision != "deny" || value.Message != qualificationHookMessage("PreToolUse", p.session, "scope-tool", "Read", p.nonce) {
		t.Fatal("canonical adapter's own denial/marker was lost")
	}
}

func TestQualificationHookMarkerHasFixedDomainSeparatedVector(t *testing.T) {
	// Independently calculated with Python hashlib over this canonical JSON array:
	// ["maestro.native.qualification-hook.v1","PostToolUse","session-a","tool-7","Read","0123456789abcdef0123456789abcdef"]
	const expected = "maestro_native_canonical_hook:v1:6a8404e02c4de1c66d22fc03e80824c5ac75d8779eda45e0dcc9a0a35db6224d"
	if got := qualificationHookMessage("PostToolUse", "session-a", "tool-7", "Read", "0123456789abcdef0123456789abcdef"); got != expected {
		t.Fatalf("canonical marker vector changed: %s", got)
	}
}

func TestQualificationHookCanonicalFailureHasNoMarker(t *testing.T) {
	in := nativeFixture(t)
	body, _ := json.Marshal(map[string]any{"hook_event_name": "PostToolUse", "session_id": "session-unreserved", "cwd": in.Packet.Binding.WorkspacePath, "tool_name": "Read", "tool_use_id": "tool-unreserved", "tool_input": map[string]any{"file_path": "allowed.txt"}, "agent_id": "unreserved", "agent_type": "maestro-product:case-agent"})
	var out bytes.Buffer
	if err := QualificationHook(in.Packet.Binding, "0123456789abcdef0123456789abcdef", bytes.NewReader(body), &out); err == nil || out.Len() != 0 {
		t.Fatal("canonical failure emitted a success marker")
	}
}
