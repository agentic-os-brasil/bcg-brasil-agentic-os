// Package claudeadapter translates Claude native hook payloads into the
// runtime-neutral lifecycle boundary. It contains no source-content
// persistence and does not attempt to implement a general shell parser.
package claudeadapter

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	pathpkg "path"
	"regexp"
	"strings"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/lifecycle"
)

const MaximumNativeInputBytes = 64 << 10

var nativeIdentifierPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,127}$`)

type NativeInput struct {
	ActorID   string `json:"actor_id"`
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

func Parse(input []byte) (NativeInput, error) {
	if len(input) == 0 {
		return NativeInput{}, errors.New("Claude hook input is required")
	}
	if len(input) > MaximumNativeInputBytes {
		return NativeInput{}, fmt.Errorf("Claude hook input exceeds %d-byte limit", MaximumNativeInputBytes)
	}
	var parsed NativeInput
	if err := json.Unmarshal(input, &parsed); err != nil {
		return NativeInput{}, fmt.Errorf("decode Claude hook input: %w", err)
	}
	return parsed, nil
}

func ParseReader(input io.Reader) (NativeInput, error) {
	body, err := io.ReadAll(io.LimitReader(input, MaximumNativeInputBytes+1))
	if err != nil {
		return NativeInput{}, fmt.Errorf("read Claude hook input: %w", err)
	}
	return Parse(body)
}

// Guard makes no allow decision: allowing implicitly would bypass Claude's own
// permission flow. It only denies an unequivocal protected-root deletion.
func Guard(input NativeInput) (GuardOutput, error) {
	if !nativeIdentifierPattern.MatchString(input.ToolName) {
		return GuardOutput{}, errors.New("Claude hook tool name is invalid")
	}
	if input.ToolName != "Bash" {
		return GuardOutput{}, nil
	}
	if strings.TrimSpace(input.ToolInput.Command) == "" {
		return GuardOutput{}, errors.New("Claude Bash command is required")
	}
	destructive, err := destructiveRootRemoval(input.ToolInput.Command)
	if err != nil {
		return GuardOutput{}, err
	}
	if !destructive {
		return GuardOutput{}, nil
	}
	return denial("Maestro denied an unambiguous recursive forced deletion of a protected filesystem root. Nothing was changed. Review the target and retry with a narrower path."), nil
}

func FailClosedDenial() GuardOutput {
	return denial("Maestro denied this action because the local safety guard could not evaluate the bounded Claude hook input. Nothing was changed. Review the command and retry.")
}

func ExternalActionDenial(reason string) GuardOutput {
	return denial(reason)
}

func Receipt(event string, input NativeInput) (lifecycle.Receipt, error) {
	if !nativeIdentifierPattern.MatchString(input.SessionID) {
		return lifecycle.Receipt{}, errors.New("Claude hook session ID is invalid")
	}
	parts := []string{"claude", event, input.SessionID}
	toolName := ""
	switch event {
	case lifecycle.PostActionObserve:
		if !nativeIdentifierPattern.MatchString(input.ToolUseID) {
			return lifecycle.Receipt{}, errors.New("Claude hook tool-use ID is invalid")
		}
		if !nativeIdentifierPattern.MatchString(input.ToolName) {
			return lifecycle.Receipt{}, errors.New("Claude hook tool name is invalid")
		}
		parts = append(parts, input.ToolUseID, input.ToolName)
		toolName = input.ToolName
	case lifecycle.StopFinalize:
		// Stop has no tool-use identity. The session and event form the
		// idempotent metadata-only key.
	default:
		return lifecycle.Receipt{}, fmt.Errorf("unsupported Claude receipt event %q", event)
	}
	return lifecycle.Receipt{
		SchemaVersion:  1,
		Runtime:        "claude",
		Event:          event,
		State:          "observed",
		Provenance:     lifecycle.AdapterCommand,
		ToolName:       toolName,
		IdempotencyKey: lifecycle.IdempotencyKey(parts...),
	}, nil
}

func denial(reason string) GuardOutput {
	return GuardOutput{HookSpecificOutput: &GuardDecision{
		HookEventName:            "PreToolUse",
		PermissionDecision:       "deny",
		PermissionDecisionReason: reason,
	}}
}

func destructiveRootRemoval(command string) (bool, error) {
	fields, err := splitSimpleCommand(command)
	if err != nil {
		return false, err
	}
	for len(fields) > 0 && isLeadingAssignment(fields[0].Value) {
		fields = fields[1:]
	}
	if len(fields) == 0 || !isRMExecutable(fields[0].Value) {
		return false, nil
	}
	recursive, force := false, false
	var targets []simpleWord
	optionsEnded := false
	for _, word := range fields[1:] {
		field := word.Value
		if !optionsEnded && field == "--" {
			optionsEnded = true
			continue
		}
		if !optionsEnded && strings.HasPrefix(field, "-") && field != "-" {
			switch field {
			case "--recursive":
				recursive = true
			case "--force":
				force = true
			default:
				if strings.HasPrefix(field, "--") {
					continue
				}
				flags := strings.TrimPrefix(field, "-")
				recursive = recursive || strings.ContainsAny(flags, "rR")
				force = force || strings.Contains(flags, "f")
			}
			continue
		}
		targets = append(targets, word)
	}
	if !recursive || !force {
		return false, nil
	}
	for _, target := range targets {
		if isProtectedRoot(target) {
			return true, nil
		}
	}
	return false, nil
}

func isLeadingAssignment(value string) bool {
	name, _, found := strings.Cut(value, "=")
	if !found || name == "" {
		return false
	}
	for index, character := range name {
		if !(character == '_' || character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || (index > 0 && character >= '0' && character <= '9')) {
			return false
		}
	}
	return true
}

func isRMExecutable(value string) bool {
	switch pathpkg.Clean(value) {
	case "rm", "/bin/rm", "/usr/bin/rm":
		return true
	default:
		return false
	}
}

func isProtectedRoot(word simpleWord) bool {
	cleaned := pathpkg.Clean(word.Value)
	switch cleaned {
	case "/":
		return true
	case "~":
		return word.TildeExpands
	case "$HOME", "${HOME}":
		return word.HomeExpands
	default:
		return false
	}
}

type simpleWord struct {
	Value        string
	HomeExpands  bool
	TildeExpands bool
}

// splitSimpleCommand recognizes only whitespace-separated words plus balanced
// single and double quotes. Shell operators, substitutions and escapes are
// deliberately rejected instead of being partially interpreted.
func splitSimpleCommand(command string) ([]simpleWord, error) {
	var (
		fields                 []simpleWord
		current                strings.Builder
		quote                  byte
		homeExpansionCandidate bool
		tildeExpands           bool
	)
	flush := func() {
		if current.Len() > 0 {
			value := current.String()
			fields = append(fields, simpleWord{
				Value:        value,
				HomeExpands:  homeExpansionCandidate && (strings.Contains(value, "$HOME") || strings.Contains(value, "${HOME}")),
				TildeExpands: tildeExpands,
			})
			current.Reset()
			homeExpansionCandidate = false
			tildeExpands = false
		}
	}
	for index := 0; index < len(command); index++ {
		character := command[index]
		if quote != 0 {
			if character == quote {
				quote = 0
				continue
			}
			if character == '$' && quote == '"' {
				length := supportedHomeExpansionLength(command[index:])
				if length == 0 {
					return nil, errors.New("unsupported parameter expansion is outside the bounded simple-command grammar")
				}
				homeExpansionCandidate = true
				current.WriteString(command[index : index+length])
				index += length - 1
				continue
			}
			current.WriteByte(character)
			continue
		}
		switch character {
		case '\'', '"':
			quote = character
		case ' ', '\t':
			flush()
		case '\r', '\n', '\\', ';', '|', '&', '<', '>', '`', '*', '?', '[', ']', '{', '}':
			return nil, errors.New("command is outside the bounded simple-command grammar")
		default:
			if current.Len() == 0 && character == '~' {
				tildeExpands = true
			}
			if character == '$' {
				length := supportedHomeExpansionLength(command[index:])
				if length == 0 {
					return nil, errors.New("unsupported parameter expansion is outside the bounded simple-command grammar")
				}
				homeExpansionCandidate = true
				current.WriteString(command[index : index+length])
				index += length - 1
				continue
			}
			current.WriteByte(character)
		}
	}
	if quote != 0 {
		return nil, errors.New("command contains an unterminated quote")
	}
	flush()
	return fields, nil
}

func supportedHomeExpansionLength(value string) int {
	if strings.HasPrefix(value, "${HOME}") {
		return len("${HOME}")
	}
	if !strings.HasPrefix(value, "$HOME") {
		return 0
	}
	if len(value) == len("$HOME") {
		return len("$HOME")
	}
	next := value[len("$HOME")]
	if (next >= 'a' && next <= 'z') || (next >= 'A' && next <= 'Z') ||
		(next >= '0' && next <= '9') || next == '_' {
		return 0
	}
	return len("$HOME")
}
