package memory

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestDeterministicL1SynthesizerDeduplicatesAndBounds(t *testing.T) {
	attestor := CaptureAttestor{Root: t.TempDir()}
	first := sealedCapture(t, attestor, "case-a", time.Date(2026, 8, 3, 9, 0, 0, 0, time.UTC), "meeting-close")
	second := first
	second.RecordedAt = second.RecordedAt.Add(time.Hour)
	second, _ = attestor.Seal(second)
	third := sealedCapture(t, attestor, "case-a", second.RecordedAt.Add(time.Hour), "owner confirmation required")
	var body strings.Builder
	for _, capture := range []Capture{first, second, third} {
		encoded, _ := json.Marshal(capture)
		body.Write(encoded)
		body.WriteByte('\n')
	}
	synthesizer := DeterministicL1Synthesizer{MaxRunes: 500, MaxEntries: 8, MaxInputBytes: 4096, MaxInputEntries: 16, Attestor: attestor}
	result, err := synthesizer.Synthesize(context.Background(), SynthesisRequest{Cycle: "daily", TargetLayer: "L1", WorkspaceID: "case-a", Period: "2026-08-03", Sources: []SourceDocument{{ID: "captures.jsonl", Content: []byte(body.String())}}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(result, "meeting-close") != 1 || !strings.Contains(result, "owner confirmation required") || len([]rune(result)) > 500 {
		t.Fatalf("unexpected bounded synthesis: %q", result)
	}
}

func TestDeterministicL1SynthesizerRejectsUnsanitizedOrWrongWorkspace(t *testing.T) {
	attestor := CaptureAttestor{Root: t.TempDir()}
	for _, capture := range []Capture{
		{SchemaVersion: 2, WorkspaceID: "case-a", RecordedAt: time.Date(2026, 8, 3, 9, 0, 0, 0, time.UTC), Kind: "skill_route", Text: "raw", Sanitized: true, ProducerID: "claude.context-injection", SanitizerID: SkillRouteSanitizerID, SourceDigest: strings.Repeat("a", 64), Attestation: strings.Repeat("0", 64)},
		sealedCapture(t, attestor, "case-b", time.Date(2026, 8, 3, 9, 0, 0, 0, time.UTC), "other workspace"),
	} {
		body, _ := json.Marshal(capture)
		_, err := (DeterministicL1Synthesizer{MaxRunes: 1000, MaxEntries: 8, MaxInputBytes: 4096, MaxInputEntries: 16, Attestor: attestor}).Synthesize(context.Background(), SynthesisRequest{Cycle: "daily", TargetLayer: "L1", WorkspaceID: "case-a", Period: "2026-08-03", Sources: []SourceDocument{{ID: "captures.jsonl", Content: append(body, '\n')}}})
		if err == nil {
			t.Fatalf("unsafe capture was synthesized: %#v", capture)
		}
	}
}

func TestDeterministicL1SynthesizerRejectsCrossPeriodCapture(t *testing.T) {
	attestor := CaptureAttestor{Root: t.TempDir()}
	capture := sealedCapture(t, attestor, "case-a", time.Date(2026, 8, 2, 23, 59, 0, 0, time.UTC), "wrong day")
	body, _ := json.Marshal(capture)
	_, err := (DeterministicL1Synthesizer{MaxRunes: 1000, MaxEntries: 8, MaxInputBytes: 4096, MaxInputEntries: 16, Attestor: attestor}).Synthesize(context.Background(), SynthesisRequest{Cycle: "daily", TargetLayer: "L1", WorkspaceID: "case-a", Period: "2026-08-03", Sources: []SourceDocument{{ID: "captures.jsonl", Content: append(body, '\n')}}})
	if err == nil {
		t.Fatal("cross-period capture was synthesized")
	}
}

func TestDeterministicL1SynthesizerRejectsInputBounds(t *testing.T) {
	attestor := CaptureAttestor{Root: t.TempDir()}
	capture := sealedCapture(t, attestor, "case-a", time.Date(2026, 8, 3, 9, 0, 0, 0, time.UTC), strings.Repeat("x", 256))
	body, _ := json.Marshal(capture)
	request := SynthesisRequest{Cycle: "daily", TargetLayer: "L1", WorkspaceID: "case-a", Period: "2026-08-03", Sources: []SourceDocument{{ID: "captures.jsonl", Content: append(append(body, '\n'), append(body, '\n')...)}}}
	if _, err := (DeterministicL1Synthesizer{MaxRunes: 1000, MaxEntries: 8, MaxInputBytes: len(body), MaxInputEntries: 16, Attestor: attestor}).Synthesize(context.Background(), request); err == nil {
		t.Fatal("oversized input bytes were accepted")
	}
	if _, err := (DeterministicL1Synthesizer{MaxRunes: 1000, MaxEntries: 8, MaxInputBytes: 4096, MaxInputEntries: 1, Attestor: attestor}).Synthesize(context.Background(), request); err == nil {
		t.Fatal("too many input entries were accepted")
	}
}

func sealedCapture(t *testing.T, attestor CaptureAttestor, workspace string, recordedAt time.Time, text string) Capture {
	t.Helper()
	capture, err := attestor.Seal(Capture{WorkspaceID: workspace, RecordedAt: recordedAt, Kind: "skill_route", Text: text, Sanitized: true, ProducerID: "claude.context-injection", SanitizerID: SkillRouteSanitizerID, SourceDigest: strings.Repeat("a", 64)})
	if err != nil {
		t.Fatal(err)
	}
	return capture
}
