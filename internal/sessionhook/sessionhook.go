// Package sessionhook translates the bounded Session Start envelope into each
// runtime's native command-hook output. The envelope is shared; serialization
// remains adapter-specific.
package sessionhook

import (
	"encoding/json"
	"fmt"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/sessionctx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/sessionstart"
)

type ClaudeOutput struct {
	HookSpecificOutput ClaudeHookSpecificOutput `json:"hookSpecificOutput"`
}

type CodexOutput struct {
	HookSpecificOutput CodexHookSpecificOutput `json:"hookSpecificOutput"`
}

// ClaudeHookSpecificOutput follows the current Claude project-hook contract
// already exercised by this repository's development hook.
type ClaudeHookSpecificOutput struct {
	HookEventName     string `json:"hookEventName"`
	AdditionalContext string `json:"additionalContext"`
}

// CodexHookSpecificOutput follows the documented Codex SessionStart command
// hook output shape. It intentionally remains a distinct type because native
// contracts evolve independently even when their current JSON is identical.
type CodexHookSpecificOutput struct {
	HookEventName     string `json:"hookEventName"`
	AdditionalContext string `json:"additionalContext"`
}

func BuildClaude(packet sessionctx.Packet) (ClaudeOutput, error) {
	context, err := contextFor("claude", packet)
	if err != nil {
		return ClaudeOutput{}, err
	}
	return ClaudeOutput{HookSpecificOutput: ClaudeHookSpecificOutput{HookEventName: "SessionStart", AdditionalContext: context}}, nil
}

func BuildCodex(packet sessionctx.Packet) (CodexOutput, error) {
	context, err := contextFor("codex", packet)
	if err != nil {
		return CodexOutput{}, err
	}
	return CodexOutput{HookSpecificOutput: CodexHookSpecificOutput{HookEventName: "SessionStart", AdditionalContext: context}}, nil
}

func contextFor(runtime string, packet sessionctx.Packet) (string, error) {
	envelope, err := sessionstart.Build(runtime, packet)
	if err != nil {
		return "", err
	}
	body, err := json.Marshal(envelope)
	if err != nil {
		return "", fmt.Errorf("encode session envelope: %w", err)
	}
	return "Maestro bounded session context (pointers only; unavailable sources are explicit):\n" + string(body), nil
}
