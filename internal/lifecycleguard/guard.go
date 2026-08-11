// Package lifecycleguard contains the runtime-neutral bounded pre-action
// policy. Product adapters own payload parsing and native output shapes; this
// package owns only the deterministic protected-root decision.
package lifecycleguard

import (
	"errors"
	pathpkg "path"
	"strings"
)

// ProtectedRootRemoval evaluates the deliberately small command grammar used
// by both Claude and Codex adapters. It never executes or expands a command.
func ProtectedRootRemoval(command string) (bool, error) {
	segments, chained, err := splitAndThen(command)
	if err != nil {
		return false, err
	}
	if chained {
		for _, segment := range segments {
			destructive, segmentErr := protectedRootRemovalSimple(segment)
			if segmentErr != nil {
				return false, segmentErr
			}
			if destructive {
				return true, nil
			}
		}
		return false, nil
	}
	return protectedRootRemovalSimple(command)
}

func protectedRootRemovalSimple(command string) (bool, error) {
	fields, err := splitSimpleCommand(command)
	if err != nil {
		if !looksLikeRemovalCommand(command) {
			return false, nil
		}
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

// splitAndThen recognizes only a small, quote-aware `&&` sequence. It does
// not execute or expand shell syntax. Each segment is evaluated independently
// by the same protected-root grammar, so a harmless trailing `echo` no longer
// hides a safe file removal while any protected-root removal still denies the
// complete command.
func splitAndThen(command string) ([]string, bool, error) {
	var segments []string
	start := 0
	var quote byte
	for index := 0; index < len(command); index++ {
		character := command[index]
		if quote != 0 {
			if character == quote {
				quote = 0
			}
			continue
		}
		if character == '\'' || character == '"' {
			quote = character
			continue
		}
		if character != '&' {
			continue
		}
		if index+1 >= len(command) || command[index+1] != '&' {
			return nil, false, errors.New("command contains an unsupported shell operator")
		}
		segment := strings.TrimSpace(command[start:index])
		if segment == "" {
			return nil, false, errors.New("command contains an empty chained segment")
		}
		segments = append(segments, segment)
		if len(segments) >= 4 {
			return nil, false, errors.New("command exceeds the bounded chained-command limit")
		}
		index++
		start = index + 1
	}
	if quote != 0 {
		return nil, false, errors.New("command contains an unterminated quote")
	}
	if len(segments) == 0 {
		return nil, false, nil
	}
	last := strings.TrimSpace(command[start:])
	if last == "" {
		return nil, false, errors.New("command contains an empty chained segment")
	}
	segments = append(segments, last)
	return segments, true, nil
}

func looksLikeRemovalCommand(command string) bool {
	for _, field := range strings.Fields(command) {
		field = strings.Trim(field, "(){}[];|&<>")
		switch field {
		case "rm", "/bin/rm", "/usr/bin/rm":
			return true
		}
	}
	return false
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
