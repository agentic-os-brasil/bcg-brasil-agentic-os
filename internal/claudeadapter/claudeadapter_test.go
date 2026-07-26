package claudeadapter

import (
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/lifecycle"
)

func TestGuardDeniesOnlyUnambiguousRootRemoval(t *testing.T) {
	denied := Guard(NativeInput{ToolName: "Bash", ToolInput: struct {
		Command string "json:\"command\""
	}{Command: "rm -rf /"}})
	if denied.HookSpecificOutput == nil || denied.HookSpecificOutput.PermissionDecision != "deny" {
		t.Fatalf("guard = %#v", denied)
	}
	allowed := Guard(NativeInput{ToolName: "Bash", ToolInput: struct {
		Command string "json:\"command\""
	}{Command: "rm -rf ./build"}})
	if allowed.HookSpecificOutput != nil {
		t.Fatalf("guard should not grant or deny normal cleanup: %#v", allowed)
	}
}

func TestReceiptHashesNativeIdentifiers(t *testing.T) {
	receipt := Receipt(lifecycle.PostActionObserve, NativeInput{SessionID: "secret-session", ToolUseID: "tool-1", ToolName: "Bash"})
	if receipt.IdempotencyKey == "" || receipt.IdempotencyKey == "secret-session" || receipt.ToolName != "Bash" {
		t.Fatalf("receipt = %#v", receipt)
	}
}
