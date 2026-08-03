package memory

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

const DeterministicL1SynthesizerID = "deterministic-l1-v1"

type DeterministicL1Synthesizer struct {
	MaxRunes   int
	MaxEntries int
}

func (synthesizer DeterministicL1Synthesizer) Synthesize(_ context.Context, request SynthesisRequest) (string, error) {
	if request.Cycle != "daily" || request.TargetLayer != "L1" || request.WorkspaceID == "" || request.Period == "" {
		return "", errors.New("deterministic L1 synthesizer accepts only a bounded daily L1 request")
	}
	if synthesizer.MaxRunes <= 0 || synthesizer.MaxEntries <= 0 {
		return "", errors.New("deterministic L1 synthesizer requires explicit positive bounds")
	}
	var captures []Capture
	for _, source := range request.Sources {
		scanner := bufio.NewScanner(bytes.NewReader(source.Content))
		scanner.Buffer(make([]byte, 4096), 1<<20)
		for scanner.Scan() {
			decoder := json.NewDecoder(bytes.NewReader(scanner.Bytes()))
			decoder.DisallowUnknownFields()
			var capture Capture
			if err := decoder.Decode(&capture); err != nil {
				return "", fmt.Errorf("decode sanitized L1 capture: %w", err)
			}
			if err := ensureJSONEOF(decoder); err != nil {
				return "", fmt.Errorf("decode sanitized L1 capture: %w", err)
			}
			if capture.WorkspaceID != request.WorkspaceID || !capture.Sanitized || capture.RecordedAt.IsZero() || capture.RecordedAt.UTC().Format("2006-01-02") != request.Period || strings.TrimSpace(capture.Kind) == "" || strings.TrimSpace(capture.Text) == "" {
				return "", errors.New("L1 synthesis source is not a valid sanitized workspace capture")
			}
			captures = append(captures, capture)
		}
		if err := scanner.Err(); err != nil {
			return "", err
		}
	}
	if len(captures) == 0 {
		return "", errors.New("daily L1 synthesis requires at least one sanitized capture")
	}
	sort.SliceStable(captures, func(left, right int) bool { return captures[left].RecordedAt.Before(captures[right].RecordedAt) })
	seen := map[string]bool{}
	lines := make([]string, 0, len(captures))
	for index := len(captures) - 1; index >= 0 && len(lines) < synthesizer.MaxEntries; index-- {
		capture := captures[index]
		kind, text := strings.Join(strings.Fields(capture.Kind), " "), strings.Join(strings.Fields(capture.Text), " ")
		key := kind + "\x00" + text
		if seen[key] {
			continue
		}
		seen[key] = true
		line := fmt.Sprintf("- %s | %s | %s", capture.RecordedAt.UTC().Format("15:04:05Z"), kind, text)
		lines = append(lines, line)
	}
	for left, right := 0, len(lines)-1; left < right; left, right = left+1, right-1 {
		lines[left], lines[right] = lines[right], lines[left]
	}
	header := "# L1 continuity · " + request.Period
	selected := make([]string, 0, len(lines))
	for index := len(lines) - 1; index >= 0; index-- {
		candidate := append([]string{lines[index]}, selected...)
		body := header + "\n\n" + strings.Join(candidate, "\n")
		if len([]rune(body)) > synthesizer.MaxRunes {
			continue
		}
		selected = candidate
	}
	if len(selected) == 0 {
		return "", errors.New("no complete sanitized L1 capture fits the configured budget")
	}
	return header + "\n\n" + strings.Join(selected, "\n"), nil
}
