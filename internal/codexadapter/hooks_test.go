package codexadapter

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/lifecycle"
)

func TestCodexNativePayloadAndOutputsAreAdapterOwned(t *testing.T) {
	input, err := ParseReader(strings.NewReader(`{"session_id":"session-a","tool_use_id":"tool-a","tool_name":"Bash","tool_input":{"command":"rm -rf /"}}`))
	if err != nil {
		t.Fatal(err)
	}
	output, err := Guard(input)
	if err != nil || output.HookSpecificOutput == nil || output.HookSpecificOutput.HookEventName != "PreToolUse" || output.HookSpecificOutput.PermissionDecision != "deny" {
		t.Fatalf("guard = %#v, %v", output, err)
	}
	receipt, err := Receipt(lifecycle.PostActionObserve, input)
	if err != nil || receipt.Runtime != "codex" || receipt.Provenance != lifecycle.AdapterCommand || receipt.ToolName != "Bash" {
		t.Fatalf("receipt = %#v, %v", receipt, err)
	}
	stop, err := Receipt(lifecycle.StopFinalize, NativeInput{SessionID: "session-a"})
	if err != nil || stop.Runtime != "codex" || stop.ToolName != "" {
		t.Fatalf("stop receipt = %#v, %v", stop, err)
	}
}

func TestCodexGuardLeavesOrdinaryShellPipelinesToNativePermissionFlow(t *testing.T) {
	output, err := Guard(NativeInput{ToolName: "Bash", ToolInput: struct {
		Command string `json:"command"`
	}{Command: "ls /Users/example/Developer/other-workspace 2>/dev/null | grep -i darwin"}})
	if err != nil || output.HookSpecificOutput != nil {
		t.Fatalf("ordinary pipeline was blocked: output=%#v err=%v", output, err)
	}
}

func TestCodexNativePayloadFailsClosed(t *testing.T) {
	if _, err := ParseReader(strings.NewReader(`{"session_id":`)); err == nil {
		t.Fatal("malformed Codex payload was accepted")
	}
	output := FailClosedDenial()
	if output.HookSpecificOutput == nil || output.HookSpecificOutput.PermissionDecision != "deny" {
		t.Fatalf("fail-closed output = %#v", output)
	}
}

func TestParseReaderPreservesBoundedIdentityPromptAndRawToolInput(t *testing.T) {
	input, err := ParseReader(strings.NewReader(`{"session_id":"session-a","prompt":"route this","tool_name":"mcp__github__create_pull_request","tool_input":{"repository":"org/repo","title":"private title"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if input.SessionID != "session-a" || input.Prompt != "route this" || !strings.Contains(string(input.ToolInputJSON()), `"private title"`) {
		t.Fatalf("parsed input = %#v raw=%s", input, input.ToolInputJSON())
	}
	spoofed, err := ParseReader(strings.NewReader(`{"actor_id":"attacker","session_id":"session-a","prompt":"route this","tool_input":{}}`))
	encoded, marshalErr := json.Marshal(spoofed)
	if err != nil || marshalErr != nil || strings.Contains(string(encoded), "actor_id") {
		t.Fatalf("caller actor was accepted as authority: %s, %v, %v", encoded, err, marshalErr)
	}
	if output := ExternalActionDenial("confirmation required"); output.HookSpecificOutput == nil || output.HookSpecificOutput.PermissionDecisionReason != "confirmation required" {
		t.Fatalf("external denial = %#v", output)
	}
}
