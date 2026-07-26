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

// MaximumAdditionalContextBytes keeps native Session Start output small enough
// to stay within a predictable startup budget. It is deliberately lower than
// the typical native-hook payload ceilings and is an output limit, not a
// license to read more source material.
const MaximumAdditionalContextBytes = 8 << 10

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
	return BuildClaudeEvent(packet, "SessionStart")
}

// BuildClaudeEvent keeps the bounded shared packet identical across Claude's
// SessionStart and UserPromptSubmit surfaces while preserving each native event
// name in the adapter response.
func BuildClaudeEvent(packet sessionctx.Packet, eventName string) (ClaudeOutput, error) {
	if eventName != "SessionStart" && eventName != "UserPromptSubmit" {
		return ClaudeOutput{}, fmt.Errorf("unsupported Claude hook event %q", eventName)
	}
	context, err := contextFor("claude", packet)
	if err != nil {
		return ClaudeOutput{}, err
	}
	return ClaudeOutput{HookSpecificOutput: ClaudeHookSpecificOutput{HookEventName: eventName, AdditionalContext: context}}, nil
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
	// This serializer is invoked by an installed adapter (or its direct
	// conformance command). The manifest still owns capability state, so do not
	// claim native availability; merely avoid telling a real adapter session
	// that its wiring is absent.
	envelope.Message = "bounded adapter payload emitted; capability remains unavailable until qualified native-session conformance evidence is recorded"
	body, err := json.Marshal(envelope)
	if err != nil {
		return "", fmt.Errorf("encode session envelope: %w", err)
	}
	context := "Maestro bounded session context (pointers only; unavailable sources are explicit):\n" + string(body)
	if len(context) <= MaximumAdditionalContextBytes {
		return context, nil
	}

	// A hook must remain available even when a future packet gains an unusually
	// verbose warning. Do not fail the session or truncate JSON mid-document:
	// return a valid, explicit omission that directs the runtime to the normal
	// packet command instead.
	return "Maestro bounded session context omitted: packet exceeded the native hook output budget. Use `bcgos session packet` for the complete pointer-only packet.", nil
}
