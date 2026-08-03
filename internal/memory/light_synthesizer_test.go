package memory

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestDeterministicL1SynthesizerDeduplicatesAndBounds(t *testing.T) {
	first := Capture{WorkspaceID: "case-a", RecordedAt: time.Date(2026, 8, 3, 9, 0, 0, 0, time.UTC), Kind: "skill", Text: "meeting-close", Sanitized: true}
	second := first
	second.RecordedAt = second.RecordedAt.Add(time.Hour)
	third := Capture{WorkspaceID: "case-a", RecordedAt: second.RecordedAt.Add(time.Hour), Kind: "decision", Text: "owner confirmation required", Sanitized: true}
	var body strings.Builder
	for _, capture := range []Capture{first, second, third} {
		encoded, _ := json.Marshal(capture)
		body.Write(encoded)
		body.WriteByte('\n')
	}
	synthesizer := DeterministicL1Synthesizer{MaxRunes: 500, MaxEntries: 8}
	result, err := synthesizer.Synthesize(context.Background(), SynthesisRequest{Cycle: "daily", TargetLayer: "L1", WorkspaceID: "case-a", Period: "2026-08-03", Sources: []SourceDocument{{ID: "captures.jsonl", Content: []byte(body.String())}}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(result, "meeting-close") != 1 || !strings.Contains(result, "owner confirmation required") || len([]rune(result)) > 500 {
		t.Fatalf("unexpected bounded synthesis: %q", result)
	}
}

func TestDeterministicL1SynthesizerRejectsUnsanitizedOrWrongWorkspace(t *testing.T) {
	for _, capture := range []Capture{
		{WorkspaceID: "case-a", RecordedAt: time.Now(), Kind: "signal", Text: "raw", Sanitized: false},
		{WorkspaceID: "case-b", RecordedAt: time.Now(), Kind: "signal", Text: "other workspace", Sanitized: true},
	} {
		body, _ := json.Marshal(capture)
		_, err := (DeterministicL1Synthesizer{MaxRunes: 1000, MaxEntries: 8}).Synthesize(context.Background(), SynthesisRequest{Cycle: "daily", TargetLayer: "L1", WorkspaceID: "case-a", Period: "2026-08-03", Sources: []SourceDocument{{ID: "captures.jsonl", Content: append(body, '\n')}}})
		if err == nil {
			t.Fatalf("unsafe capture was synthesized: %#v", capture)
		}
	}
}

func TestDeterministicL1SynthesizerRejectsCrossPeriodCapture(t *testing.T) {
	capture := Capture{WorkspaceID: "case-a", RecordedAt: time.Date(2026, 8, 2, 23, 59, 0, 0, time.UTC), Kind: "signal", Text: "wrong day", Sanitized: true}
	body, _ := json.Marshal(capture)
	_, err := (DeterministicL1Synthesizer{MaxRunes: 1000, MaxEntries: 8}).Synthesize(context.Background(), SynthesisRequest{Cycle: "daily", TargetLayer: "L1", WorkspaceID: "case-a", Period: "2026-08-03", Sources: []SourceDocument{{ID: "captures.jsonl", Content: append(body, '\n')}}})
	if err == nil {
		t.Fatal("cross-period capture was synthesized")
	}
}
