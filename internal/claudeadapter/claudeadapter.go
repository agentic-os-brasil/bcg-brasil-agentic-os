// Package claudeadapter translates Claude native hook payloads into the
// runtime-neutral lifecycle boundary. It intentionally contains no product
// policy beyond a small local deny rule and no source-content persistence.
package claudeadapter

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/lifecycle"
)

type NativeInput struct {
	SessionID string `json:"session_id"`
	ToolUseID string `json:"tool_use_id"`
	ToolName  string `json:"tool_name"`
	ToolInput struct {
		Command string `json:"command"`
	} `json:"tool_input"`
}

type GuardOutput struct {
	HookSpecificOutput *GuardDecision `json:"hookSpecificOutput,omitempty"`
}

type GuardDecision struct {
	HookEventName            string `json:"hookEventName"`
	PermissionDecision       string `json:"permissionDecision"`
	PermissionDecisionReason string `json:"permissionDecisionReason"`
}

type FinalizationOutput struct {
	Continue bool `json:"continue"`
}

func Parse(input []byte) (NativeInput, error) {
	if len(input) == 0 {
		return NativeInput{}, errors.New("Claude hook input is required")
	}
	var parsed NativeInput
	if err := json.Unmarshal(input, &parsed); err != nil {
		return NativeInput{}, err
	}
	return parsed, nil
}

// Guard makes no allow decision: allowing implicitly would bypass Claude's own
// permission flow. It only denies an unambiguous destructive root deletion.
func Guard(input NativeInput) GuardOutput {
	if input.ToolName != "Bash" || !destructiveRootRemoval(input.ToolInput.Command) {
		return GuardOutput{}
	}
	return GuardOutput{HookSpecificOutput: &GuardDecision{
		HookEventName: "PreToolUse", PermissionDecision: "deny",
		PermissionDecisionReason: "Maestro denied an unambiguous recursive deletion of a filesystem root.",
	}}
}

func Receipt(event string, input NativeInput) lifecycle.Receipt {
	toolName := ""
	if event == lifecycle.PostActionObserve {
		toolName = input.ToolName
	}
	return lifecycle.Receipt{
		Runtime: "claude", Event: event, State: "observed", ToolName: toolName,
		IdempotencyKey: lifecycle.IdempotencyKey("claude", event, input.SessionID, input.ToolUseID, input.ToolName),
	}
}

func destructiveRootRemoval(command string) bool {
	fields := strings.Fields(command)
	if len(fields) < 3 || fields[0] != "rm" {
		return false
	}
	recursive, force := false, false
	for _, field := range fields[1:] {
		if strings.HasPrefix(field, "-") {
			recursive = recursive || strings.Contains(field, "r") || strings.Contains(field, "R")
			force = force || strings.Contains(field, "f")
		}
	}
	if !recursive || !force {
		return false
	}
	for _, field := range fields[1:] {
		if field == "/" || field == "~" || field == "$HOME" {
			return true
		}
	}
	return false
}
