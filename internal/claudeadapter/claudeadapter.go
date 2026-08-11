// Package claudeadapter translates Claude native hook payloads into the
// runtime-neutral lifecycle boundary. It contains no source-content
// persistence and does not attempt to implement a general shell parser.
package claudeadapter

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

type NativeInput struct {
	SessionID      string `json:"session_id"`
	CWD            string `json:"cwd"`
	Prompt         string `json:"prompt"`
	ToolUseID      string `json:"tool_use_id"`
	ToolName       string `json:"tool_name"`
	AgentID        string `json:"agent_id"`
	AgentType      string `json:"agent_type"`
	StopHookActive bool   `json:"stop_hook_active"`
	ToolInput      struct {
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

type StopOutput struct {
	Decision string `json:"decision,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

func BlockStop(reason string) StopOutput {
	return StopOutput{Decision: "block", Reason: reason}
}

type SubagentStartOutput struct {
	HookSpecificOutput struct {
		HookEventName     string `json:"hookEventName"`
		AdditionalContext string `json:"additionalContext"`
	} `json:"hookSpecificOutput"`
}

func ManagedSubagentStartContext(agentType string) SubagentStartOutput {
	var output SubagentStartOutput
	output.HookSpecificOutput.HookEventName = "SubagentStart"
	output.HookSpecificOutput.AdditionalContext = "You are running as the managed Maestro specialist " + agentType + ". Stay inside the exact delegated packet, never delegate, and return only to Maestro. Native qualification is beta telemetry; the deterministic scope and tool guard is authoritative."
	return output
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
// permission flow. It only denies an unequivocal protected-root deletion on
// POSIX or Windows. Anything else — normal commands, chained commands, unknown
// shells — is passed through to Claude's own permission flow.
func Guard(input NativeInput) (GuardOutput, error) {
	command := strings.TrimSpace(input.ToolInput.Command)
	if command == "" {
		// Incomplete metadata is not a Maestro safety decision. Let Claude's
		// own permission/runtime layer handle it.
		return GuardOutput{}, nil
	}
	// Windows destructive commands (cmd.exe and PowerShell) live outside the
	// POSIX simple-command grammar because backslash is a path separator and
	// PowerShell parameters use different tokenization. Evaluate them first
	// with a dedicated matcher so the guard is not one-platform-blind.
	if destructive, matched := destructiveWindowsRemoval(command); matched {
		if destructive {
			return denial("Maestro denied an unambiguous recursive forced deletion of a protected filesystem root. Nothing was changed. Review the target and retry with a narrower path."), nil
		}
		// The command was recognised as a Windows removal verb but is not against
		// a protected root. Do not fall through to the POSIX simple-command
		// grammar: backslash-separated Windows paths are outside that grammar
		// and would spuriously fail-close. Pass through to Claude's own
		// permission flow.
		return GuardOutput{}, nil
	}
	destructive, err := lifecycleguard.ProtectedRootRemoval(command)
	if err != nil {
		// The guard owns one narrow invariant: protected-root removal. Shell
		// operators in an otherwise ordinary command belong to Claude's own
		// permission flow and must not turn the entire session into a prison.
		// Keep fail-closed behavior for commands that could still be removals.
		if !looksLikeRemovalCommand(command) {
			return GuardOutput{}, nil
		}
		return GuardOutput{}, err
	}
	if !destructive {
		return GuardOutput{}, nil
	}
	return denial("Maestro denied an unambiguous recursive forced deletion of a protected filesystem root. Nothing was changed. Review the target and retry with a narrower path."), nil
}

func looksLikeRemovalCommand(command string) bool {
	for _, field := range strings.Fields(command) {
		field = strings.Trim(field, "(){}[];|&<>")
		switch strings.ToLower(field) {
		case "rm", "/bin/rm", "/usr/bin/rm",
			"rd", "rmdir", "del", "erase", "format",
			"remove-item", "ri":
			return true
		}
	}
	return false
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
	case lifecycle.SubagentStart, lifecycle.SubagentStop:
		if !nativeIdentifierPattern.MatchString(input.AgentID) || !nativeIdentifierPattern.MatchString(input.AgentType) {
			return lifecycle.Receipt{}, errors.New("Claude subagent identity is invalid")
		}
		parts = append(parts, input.AgentID, input.AgentType)
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
		AgentType:      input.AgentType,
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

// destructiveWindowsRemoval returns (destructive, matched).
//
//   - matched=false means "this does not look like a Windows destructive verb";
//     the caller should continue with the POSIX check.
//   - matched=true, destructive=false means "recognised as a Windows removal
//     command, but not against a protected root"; pass through.
//   - matched=true, destructive=true means "unambiguous protected-root removal";
//     the caller must deny.
//
// The matcher is deliberately conservative: it recognises the standard cmd.exe
// verbs (rd, rmdir, del, erase, format), the PowerShell Remove-Item cmdlet, and
// their common aliases. It denies only when both recursive-force semantics and
// a protected-root target are unambiguous. It does not attempt to interpret
// PowerShell operators, subexpressions, or splatting.
func destructiveWindowsRemoval(command string) (bool, bool) {
	tokens := splitWindowsFields(command)
	if len(tokens) == 0 {
		return false, false
	}
	head := strings.ToLower(strings.Trim(tokens[0], "&"))
	// cmd.exe: rd /s /q C:\  |  rmdir /s /q %USERPROFILE%
	// cmd.exe: del /f /s /q C:\  |  erase /f /s /q C:\
	// cmd.exe: format C: /y
	// PowerShell: Remove-Item -Recurse -Force C:\  |  ri -Recurse -Force $env:USERPROFILE
	switch head {
	case "rd", "rmdir":
		return windowsCmdRmdirDestructive(tokens[1:]), true
	case "del", "erase":
		return windowsCmdDelDestructive(tokens[1:]), true
	case "format":
		return windowsFormatDestructive(tokens[1:]), true
	case "remove-item", "ri":
		return windowsRemoveItemDestructive(tokens[1:]), true
	}
	return false, false
}

// splitWindowsFields tokenises a Windows-style command line into fields.
// It handles double quotes as a single lexical unit and passes single quotes,
// backticks, and backslashes through verbatim so paths like C:\Users survive.
// It does not attempt to expand env vars — the token strings are matched
// literally against the protected-root set below.
func splitWindowsFields(command string) []string {
	var (
		fields  []string
		current strings.Builder
		inQuote bool
	)
	flush := func() {
		if current.Len() > 0 {
			fields = append(fields, current.String())
			current.Reset()
		}
	}
	for index := 0; index < len(command); index++ {
		character := command[index]
		if inQuote {
			if character == '"' {
				inQuote = false
				continue
			}
			current.WriteByte(character)
			continue
		}
		switch character {
		case '"':
			inQuote = true
		case ' ', '\t', '\r', '\n':
			flush()
		default:
			current.WriteByte(character)
		}
	}
	flush()
	return fields
}

func windowsCmdRmdirDestructive(args []string) bool {
	recursive, quiet := false, false
	var targets []string
	for _, arg := range args {
		switch strings.ToLower(arg) {
		case "/s":
			recursive = true
		case "/q":
			quiet = true
		default:
			if strings.HasPrefix(arg, "/") {
				continue
			}
			targets = append(targets, arg)
		}
	}
	if !recursive || !quiet {
		return false
	}
	return anyProtectedWindowsRoot(targets)
}

func windowsCmdDelDestructive(args []string) bool {
	force, recursive := false, false
	var targets []string
	for _, arg := range args {
		lowered := strings.ToLower(arg)
		switch lowered {
		case "/f":
			force = true
		case "/s":
			recursive = true
		case "/q", "/a", "/p":
			continue
		default:
			if strings.HasPrefix(arg, "/") {
				continue
			}
			targets = append(targets, arg)
		}
	}
	if !force || !recursive {
		return false
	}
	return anyProtectedWindowsRoot(targets)
}

func windowsFormatDestructive(args []string) bool {
	// `format C:` and `format C: /y` unambiguously erase a drive.
	// A single positional drive-letter argument is enough.
	for _, arg := range args {
		if strings.HasPrefix(arg, "/") {
			continue
		}
		if isProtectedWindowsRoot(arg) {
			return true
		}
	}
	return false
}

func windowsRemoveItemDestructive(args []string) bool {
	recursive, force := false, false
	var targets []string
	for _, arg := range args {
		trimmed := strings.TrimPrefix(strings.TrimPrefix(arg, "--"), "-")
		lowered := strings.ToLower(trimmed)
		switch {
		case matchesPowerShellFlag(lowered, "recurse"):
			recursive = true
		case matchesPowerShellFlag(lowered, "force"):
			force = true
		case strings.HasPrefix(arg, "-"):
			// Any other switch (e.g. -Path, -LiteralPath, -Confirm) is skipped.
			// Its value, if any, becomes the next token and may still be a
			// protected root — we let the target loop below catch it.
			continue
		default:
			targets = append(targets, arg)
		}
	}
	if !recursive || !force {
		return false
	}
	return anyProtectedWindowsRoot(targets)
}

// matchesPowerShellFlag returns true when candidate is a valid PowerShell
// prefix of expected (case-insensitive), matching PowerShell's own parameter
// resolution. "recu" is enough for "Recurse".
func matchesPowerShellFlag(candidate, expected string) bool {
	if candidate == "" || len(candidate) > len(expected) {
		return false
	}
	return strings.HasPrefix(expected, candidate)
}

func anyProtectedWindowsRoot(targets []string) bool {
	for _, target := range targets {
		if isProtectedWindowsRoot(target) {
			return true
		}
	}
	return false
}

// isProtectedWindowsRoot recognises the set of paths whose recursive-forced
// removal never has a legitimate purpose from an agent. It is deliberately a
// short list; expanding it further risks false positives on paths users may
// operate on (e.g. C:\Users\me\Projects should NOT be here).
func isProtectedWindowsRoot(raw string) bool {
	// Strip surrounding whitespace and enclosing quotes cmd may leave behind.
	cleaned := strings.TrimSpace(raw)
	cleaned = strings.Trim(cleaned, `"'`)
	if cleaned == "" {
		return false
	}
	// Normalise separators and case for matching.
	lowered := strings.ToLower(cleaned)
	lowered = strings.ReplaceAll(lowered, "/", `\`)
	// Trailing separators (`C:\`, `C:\Windows\`) collapse to `C:` / `c:\windows`.
	for strings.HasSuffix(lowered, `\`) && lowered != `\` {
		lowered = strings.TrimSuffix(lowered, `\`)
	}
	// Drive root: C:  C:\  D:  D:\ ... Z:\ — a single letter followed by a colon.
	if len(lowered) >= 2 && lowered[1] == ':' && lowered[0] >= 'a' && lowered[0] <= 'z' &&
		(len(lowered) == 2 || lowered == string(lowered[0])+":") {
		return true
	}
	// POSIX-ish roots that PowerShell also accepts.
	switch lowered {
	case `\`, "/", "~":
		return true
	}
	// Environment expansions that resolve to user or system roots.
	switch strings.ToLower(cleaned) {
	case "%userprofile%", "%localappdata%", "%appdata%", "%systemroot%", "%windir%",
		"%homedrive%%homepath%", "%homedrive%\\%homepath%",
		"$env:userprofile", "$env:localappdata", "$env:appdata",
		"$env:systemroot", "$env:windir", "$env:home",
		"$home":
		return true
	}
	// Well-known system directories.
	switch lowered {
	case `c:\windows`, `c:\windows\system32`, `c:\program files`,
		`c:\program files (x86)`, `c:\programdata`, `c:\users`:
		return true
	}
	return false
}
