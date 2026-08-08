package codexadapter

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/lifecycle"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/lifecycleguard"
)

const MaximumNativeInputBytes = 64 << 10

var nativeIdentifierPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,127}$`)

// NativeInput is the bounded Codex command-hook payload. Codex-specific fields
// beyond this contract are deliberately ignored.
type NativeInput struct {
	SessionID string `json:"session_id"`
	Prompt    string `json:"prompt"`
	ToolUseID string `json:"tool_use_id"`
	ToolName  string `json:"tool_name"`
	ToolInput struct {
		Command string `json:"command"`
	} `json:"tool_input"`
	rawToolInput json.RawMessage
}

func (input *NativeInput) UnmarshalJSON(body []byte) error {
	type alias NativeInput
	var decoded alias
	if err := json.Unmarshal(body, &decoded); err != nil {
		return err
	}
	var raw struct {
		ToolInput json.RawMessage `json:"tool_input"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return err
	}
	*input = NativeInput(decoded)
	input.rawToolInput = append(json.RawMessage(nil), raw.ToolInput...)
	return nil
}

func (input NativeInput) ToolInputJSON() json.RawMessage {
	return append(json.RawMessage(nil), input.rawToolInput...)
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

func ParseReader(input io.Reader) (NativeInput, error) {
	body, err := io.ReadAll(io.LimitReader(input, MaximumNativeInputBytes+1))
	if err != nil {
		return NativeInput{}, fmt.Errorf("read Codex hook input: %w", err)
	}
	if len(body) == 0 {
		return NativeInput{}, errors.New("Codex hook input is required")
	}
	if len(body) > MaximumNativeInputBytes {
		return NativeInput{}, fmt.Errorf("Codex hook input exceeds %d-byte limit", MaximumNativeInputBytes)
	}
	var parsed NativeInput
	if err := json.Unmarshal(body, &parsed); err != nil {
		return NativeInput{}, fmt.Errorf("decode Codex hook input: %w", err)
	}
	return parsed, nil
}

func Guard(input NativeInput) (GuardOutput, error) {
	command := strings.TrimSpace(input.ToolInput.Command)
	if command == "" {
		// Incomplete metadata is handed back to Codex's own permission/runtime
		// flow. Maestro only makes a synchronous decision for a protected root.
		return GuardOutput{}, nil
	}
	destructive, err := lifecycleguard.ProtectedRootRemoval(command)
	if err != nil {
		return GuardOutput{}, err
	}
	if !destructive {
		return GuardOutput{}, nil
	}
	return denial("Maestro denied an unambiguous recursive forced deletion of a protected filesystem root. Nothing was changed. Review the target and retry with a narrower path."), nil
}

func FailClosedDenial() GuardOutput {
	return denial("Maestro could not verify this action safely. Nothing was changed. Review the action and try again.")
}

func ComplexRemovalDenial() GuardOutput {
	return denial("Maestro could not safely verify a removal inside a chained or complex shell command. Nothing was changed. Run each shell step separately so the removal target can be checked, then retry.")
}

func ExternalActionDenial(reason string) GuardOutput {
	return denial(reason)
}

func denial(reason string) GuardOutput {
	return GuardOutput{HookSpecificOutput: &GuardDecision{
		HookEventName: "PreToolUse", PermissionDecision: "deny", PermissionDecisionReason: reason,
	}}
}

func Receipt(event string, input NativeInput) (lifecycle.Receipt, error) {
	if !nativeIdentifierPattern.MatchString(input.SessionID) {
		return lifecycle.Receipt{}, errors.New("Codex hook session ID is invalid")
	}
	parts := []string{"codex", event, input.SessionID}
	toolName := ""
	switch event {
	case lifecycle.PostActionObserve:
		if !nativeIdentifierPattern.MatchString(input.ToolUseID) {
			return lifecycle.Receipt{}, errors.New("Codex hook tool-use ID is invalid")
		}
		if !nativeIdentifierPattern.MatchString(input.ToolName) {
			return lifecycle.Receipt{}, errors.New("Codex hook tool name is invalid")
		}
		parts = append(parts, input.ToolUseID, input.ToolName)
		toolName = input.ToolName
	case lifecycle.StopFinalize:
	default:
		return lifecycle.Receipt{}, fmt.Errorf("unsupported Codex receipt event %q", event)
	}
	return lifecycle.Receipt{
		SchemaVersion:  1,
		Runtime:        "codex",
		Event:          event,
		State:          "observed",
		Provenance:     lifecycle.AdapterCommand,
		ToolName:       toolName,
		IdempotencyKey: lifecycle.IdempotencyKey(parts...),
	}, nil
}
