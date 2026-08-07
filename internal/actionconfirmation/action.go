package actionconfirmation

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

type Action struct {
	Action      string
	Target      string
	InputDigest string
}

// Canonicalize returns nil for ordinary local actions. A protected request
// that resembles external mutation but cannot be represented by the bounded
// grammar returns an error so the caller can fail closed.
func Canonicalize(toolName string, raw json.RawMessage) (*Action, error) {
	actionName := ""
	if toolName != "Bash" {
		actionName = protectedToolAction(toolName)
		if actionName == "" {
			return nil, nil
		}
	}
	if toolName == "Bash" && (len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null"))) {
		return nil, nil
	}
	if err := rejectDuplicateKeys(raw); err != nil {
		return nil, errors.New("tool input is not canonical JSON")
	}
	var input map[string]any
	if len(raw) == 0 || json.Unmarshal(raw, &input) != nil {
		if toolName == "Bash" {
			return nil, nil
		}
		return nil, errors.New("tool input is not canonical JSON")
	}
	if toolName == "Bash" {
		command, ok := input["command"].(string)
		if !ok || strings.TrimSpace(command) == "" {
			// A valid hook envelope with incomplete shell metadata belongs to
			// the host runtime's normal permission flow, not Maestro's block.
			return nil, nil
		}
		return canonicalShellAction(command)
	}
	target := firstString(input, "target", "repository", "repo", "channel", "recipient", "to", "url")
	if target == "" {
		return nil, errors.New("protected external tool request has no canonical target")
	}
	canonical, err := json.Marshal(input)
	if err != nil {
		return nil, errors.New("protected external tool input is not canonicalizable")
	}
	return &Action{Action: actionName, Target: target, InputDigest: digest(string(canonical))}, nil
}

func rejectDuplicateKeys(raw json.RawMessage) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var visit func() error
	visit = func() error {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		delimiter, compound := token.(json.Delim)
		if !compound {
			return nil
		}
		switch delimiter {
		case '{':
			seen := map[string]bool{}
			for decoder.More() {
				keyToken, err := decoder.Token()
				if err != nil {
					return err
				}
				key, ok := keyToken.(string)
				if !ok || seen[key] {
					return errors.New("JSON object contains a duplicate key")
				}
				seen[key] = true
				if err := visit(); err != nil {
					return err
				}
			}
		case '[':
			for decoder.More() {
				if err := visit(); err != nil {
					return err
				}
			}
		default:
			return errors.New("invalid JSON delimiter")
		}
		_, err = decoder.Token()
		return err
	}
	if err := visit(); err != nil {
		return err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("tool input contains multiple JSON values")
		}
		return err
	}
	return nil
}

func canonicalShellAction(command string) (*Action, error) {
	fields, err := splitSimpleCommand(command)
	if err != nil {
		if looksProtectedShell(command) {
			return nil, errors.New("protected external command is outside the bounded grammar")
		}
		return nil, nil
	}
	if len(fields) < 2 {
		return nil, nil
	}
	executable := strings.TrimPrefix(strings.TrimPrefix(fields[0], "/usr/bin/"), "/opt/homebrew/bin/")
	switch executable {
	case "git":
		if fields[1] != "push" {
			return nil, nil
		}
		remote, ref, parseErr := gitPushTarget(fields[2:])
		if parseErr != nil {
			return nil, parseErr
		}
		return shellAction("git.push", remote+":"+ref, fields), nil
	case "gh":
		return canonicalGH(fields)
	case "curl":
		return canonicalCurl(fields)
	default:
		return nil, nil
	}
}

func gitPushTarget(arguments []string) (string, string, error) {
	var positional []string
	for _, argument := range arguments {
		if argument == "-u" || argument == "--set-upstream" || argument == "--force" || argument == "--force-with-lease" {
			continue
		}
		if strings.HasPrefix(argument, "-") {
			return "", "", errors.New("git push option is outside the bounded grammar")
		}
		positional = append(positional, argument)
	}
	if len(positional) != 2 || positional[0] == "" || positional[1] == "" {
		return "", "", errors.New("git push requires an explicit remote and refspec")
	}
	return positional[0], positional[1], nil
}

func canonicalGH(fields []string) (*Action, error) {
	if len(fields) < 3 {
		return nil, nil
	}
	var actionName string
	switch fields[1] + " " + fields[2] {
	case "pr create":
		actionName = "github.pull_request.create"
	case "pr merge":
		actionName = "github.pull_request.merge"
	case "release create":
		actionName = "github.release.create"
	case "release upload":
		actionName = "github.release.upload"
	case "issue create":
		actionName = "github.issue.create"
	case "issue comment":
		actionName = "github.issue.comment"
	default:
		return nil, nil
	}
	repository := flagValue(fields[3:], "--repo")
	if repository == "" {
		return nil, errors.New("protected gh command requires explicit --repo target")
	}
	target := repository
	if (actionName == "github.pull_request.merge" || actionName == "github.release.create" || actionName == "github.release.upload" || actionName == "github.issue.comment") && len(fields) > 3 && !strings.HasPrefix(fields[3], "-") {
		target += ":" + fields[3]
	}
	return shellAction(actionName, target, fields), nil
}

func canonicalCurl(fields []string) (*Action, error) {
	method := strings.ToUpper(flagValue(fields[1:], "-X", "--request"))
	if method != "POST" && method != "PUT" && method != "PATCH" && method != "DELETE" {
		return nil, nil
	}
	target := ""
	for _, field := range fields[1:] {
		if strings.HasPrefix(field, "https://") {
			target = field
		}
	}
	if target == "" {
		return nil, errors.New("mutating curl requires an explicit HTTPS target")
	}
	return shellAction("http."+strings.ToLower(method), target, fields), nil
}

func shellAction(action, target string, fields []string) *Action {
	return &Action{Action: action, Target: target, InputDigest: digest(strings.Join(fields, "\x00"))}
}

func protectedToolAction(toolName string) string {
	return map[string]string{
		"mcp__github__create_pull_request": "github.pull_request.create",
		"mcp__github__merge_pull_request":  "github.pull_request.merge",
		"mcp__outlook_email__send_email":   "email.send",
		"mcp__teams__send_message":         "teams.message.send",
		"mcp__slack__send_message":         "slack.message.send",
	}[toolName]
}

func firstString(values map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := values[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func flagValue(fields []string, names ...string) string {
	for index, field := range fields {
		for _, name := range names {
			if field == name && index+1 < len(fields) {
				return fields[index+1]
			}
			if strings.HasPrefix(field, name+"=") {
				return strings.TrimPrefix(field, name+"=")
			}
		}
	}
	return ""
}

func looksProtectedShell(command string) bool {
	lower := strings.ToLower(command)
	return strings.Contains(lower, "git push") || strings.Contains(lower, "gh pr create") || strings.Contains(lower, "gh pr merge") || strings.Contains(lower, "gh release create") || strings.Contains(lower, "gh release upload") || strings.Contains(lower, "gh issue create") || strings.Contains(lower, "gh issue comment") || strings.Contains(lower, "curl") && (strings.Contains(lower, " post") || strings.Contains(lower, " put") || strings.Contains(lower, " patch") || strings.Contains(lower, " delete"))
}

func splitSimpleCommand(command string) ([]string, error) {
	var fields []string
	var current strings.Builder
	var quote byte
	flush := func() {
		if current.Len() > 0 {
			fields = append(fields, current.String())
			current.Reset()
		}
	}
	for index := 0; index < len(command); index++ {
		character := command[index]
		if quote != 0 {
			if character == quote {
				quote = 0
				continue
			}
			if character == '\\' || character == '`' || character == '$' {
				return nil, errors.New("command expansion is outside the bounded grammar")
			}
			current.WriteByte(character)
			continue
		}
		switch character {
		case '\'', '"':
			quote = character
		case ' ', '\t':
			flush()
		case '\r', '\n', '\\', ';', '|', '&', '<', '>', '`', '$', '*', '?', '[', ']', '{', '}':
			return nil, errors.New("command is outside the bounded simple-command grammar")
		default:
			current.WriteByte(character)
		}
	}
	if quote != 0 {
		return nil, errors.New("command contains an unterminated quote")
	}
	flush()
	return fields, nil
}

func digest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func validateDigest(value string) error {
	if len(value) != sha256.Size*2 {
		return fmt.Errorf("invalid canonical input digest")
	}
	_, err := hex.DecodeString(value)
	return err
}
