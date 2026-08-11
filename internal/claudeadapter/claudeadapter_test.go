package claudeadapter

import (
	"encoding/json"
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

func TestGuardDeniesCanonicalDestructiveWindowsVariants(t *testing.T) {
	tests := []string{
		`rd /s /q C:\`,
		`rmdir /s /q C:\`,
		`RD /S /Q C:\`,
		`rd /s /q "C:\"`,
		`rd /s /q %USERPROFILE%`,
		`rmdir /s /q %LOCALAPPDATA%`,
		`del /f /s /q C:\`,
		`erase /f /s /q C:\`,
		`del /f /s /q %USERPROFILE%`,
		`format C: /y`,
		`format D:`,
		`Remove-Item -Recurse -Force C:\`,
		`Remove-Item -Recurse -Force "C:\"`,
		`Remove-Item -Recu -For C:\`,
		`ri -Recurse -Force $env:USERPROFILE`,
		`Remove-Item -Recurse -Force $HOME`,
		`Remove-Item -Recurse -Force C:\Windows\System32`,
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

func TestGuardLeavesNormalWindowsRemovalsToClaudePermissionFlow(t *testing.T) {
	tests := []NativeInput{
		bashInput(`rd .\node_modules`),
		bashInput(`rd /s /q .\build`),
		bashInput(`rmdir .\empty-dir`),
		bashInput(`del temp.txt`),
		bashInput(`del /f /s /q .\dist`),
		bashInput(`erase /q report.log`),
		bashInput(`Remove-Item .\notes.md`),
		bashInput(`Remove-Item -Recurse -Force .\build`),
		bashInput(`Remove-Item -Recurse -Force $PWD\dist`),
		// Missing the destructive flag combination — must not deny.
		bashInput(`rd C:\`),
		bashInput(`del C:\`),
		bashInput(`Remove-Item C:\`),
		bashInput(`Remove-Item -Force C:\`),
		bashInput(`Remove-Item -Recurse C:\`),
	}
	for _, input := range tests {
		t.Run(input.ToolInput.Command, func(t *testing.T) {
			output, err := Guard(input)
			if err != nil {
				t.Fatal(err)
			}
			if output.HookSpecificOutput != nil {
				t.Fatalf("guard should not grant or deny normal Windows removal: %#v", output)
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
		bashInput("ls /Users/example/Developer/other-workspace 2>/dev/null | grep -i darwin"),
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

func TestGuardLeavesIncompleteMetadataToClaudePermissionFlow(t *testing.T) {
	for _, input := range []NativeInput{{}, {ToolName: "Bash"}} {
		output, err := Guard(input)
		if err != nil || output.HookSpecificOutput != nil {
			t.Fatalf("incomplete metadata was blocked: output=%#v err=%v", output, err)
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
		!strings.Contains(output.HookSpecificOutput.PermissionDecisionReason, "could not verify") {
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

func TestSubagentReceiptValidatesAndPersistsOnlyBoundedIdentityMetadata(t *testing.T) {
	input := NativeInput{SessionID: "session-123", AgentID: "agent-123", AgentType: "case-agent"}
	receipt, err := Receipt(lifecycle.SubagentStart, input)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Event != lifecycle.SubagentStart || receipt.AgentType != "case-agent" || strings.Contains(receipt.IdempotencyKey, input.AgentID) {
		t.Fatalf("receipt=%#v", receipt)
	}
	input.AgentID = "../escape"
	if _, err := Receipt(lifecycle.SubagentStart, input); err == nil {
		t.Fatal("path-shaped subagent identity was accepted")
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
