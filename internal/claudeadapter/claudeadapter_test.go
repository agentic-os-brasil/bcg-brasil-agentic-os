package claudeadapter

import (
	"strings"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/lifecycle"
)

func bashInput(command string) NativeInput {
	input := NativeInput{ToolName: "Bash"}
	input.ToolInput.Command = command
	return input
}

func TestGuardDeniesCanonicalDestructiveRootVariants(t *testing.T) {
	tests := []string{
		"rm -rf /",
		"/bin/rm -rf /",
		`rm -rf "/"`,
		"rm -rf /.",
		"rm --recursive --force -- /",
		"rm -Rf ~",
		`rm -rf "$HOME"`,
		"rm -rf $HOME/.",
		"rm -rf ${HOME}/.",
		"X=1 rm -rf /",
		"LC_ALL=C /bin/rm -rf $HOME/.",
	}
	for _, command := range tests {
		t.Run(command, func(t *testing.T) {
			output, err := Guard(bashInput(command))
			if err != nil {
				t.Fatal(err)
			}
			if output.HookSpecificOutput == nil || output.HookSpecificOutput.PermissionDecision != "deny" {
				t.Fatalf("guard = %#v", output)
			}
			if !strings.Contains(output.HookSpecificOutput.PermissionDecisionReason, "Nothing was changed") {
				t.Fatalf("denial omitted recovery assurance: %#v", output)
			}
		})
	}
}

func TestGuardLeavesNormalCommandsToClaudePermissionFlow(t *testing.T) {
	tests := []NativeInput{
		bashInput("rm -rf ./build"),
		bashInput("rm -f /tmp/result"),
		bashInput(`rm -rf '${HOME}'`),
		bashInput(`rm -rf "~"`),
		bashInput("go test ./..."),
		{ToolName: "Edit"},
	}
	for _, input := range tests {
		output, err := Guard(input)
		if err != nil {
			t.Fatal(err)
		}
		if output.HookSpecificOutput != nil {
			t.Fatalf("guard should not grant or deny normal action: %#v", output)
		}
	}
}

func TestGuardReportsEvaluationFailureForMalformedSimpleCommand(t *testing.T) {
	for _, command := range []string{
		`rm -rf "/`,
		`rm -rf "$(printf /)"`,
		`rm -rf ${UNSET:-/}`,
		`rm -rf /*`,
	} {
		if _, err := Guard(bashInput(command)); err == nil {
			t.Fatalf("Guard accepted command outside the bounded grammar: %q", command)
		}
	}
}

func TestParseBoundedRejectsMalformedAndOversizedNativeInput(t *testing.T) {
	if _, err := Parse([]byte(`{"tool_name":`)); err == nil {
		t.Fatal("Parse accepted malformed JSON")
	}
	oversized := []byte(`{"tool_name":"Bash","tool_input":{"command":"echo ` + strings.Repeat("x", MaximumNativeInputBytes) + `"}}`)
	if _, err := Parse(oversized); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("oversized parse error = %v", err)
	}
}

func TestFailClosedDenialUsesNativeClaudeShapeWithoutEchoingInput(t *testing.T) {
	output := FailClosedDenial()
	if output.HookSpecificOutput == nil ||
		output.HookSpecificOutput.HookEventName != "PreToolUse" ||
		output.HookSpecificOutput.PermissionDecision != "deny" ||
		!strings.Contains(output.HookSpecificOutput.PermissionDecisionReason, "could not evaluate") {
		t.Fatalf("output = %#v", output)
	}
}

func TestReceiptHashesValidatedNativeIdentifiers(t *testing.T) {
	input := NativeInput{SessionID: "session-123", ToolUseID: "toolu_123", ToolName: "Bash"}
	receipt, err := Receipt(lifecycle.PostActionObserve, input)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.SchemaVersion != 1 || receipt.IdempotencyKey == "" ||
		strings.Contains(receipt.IdempotencyKey, input.SessionID) ||
		receipt.ToolName != "Bash" || receipt.Provenance != lifecycle.AdapterCommand {
		t.Fatalf("receipt = %#v", receipt)
	}
	input.SessionID = "../escape"
	if _, err := Receipt(lifecycle.PostActionObserve, input); err == nil {
		t.Fatal("Receipt accepted a path-shaped native identifier")
	}
}
