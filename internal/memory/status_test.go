package memory

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStatusReportsEmptyAndCommittedWorkspace(t *testing.T) {
	engine := testEngine(t)
	empty, err := engine.Status("case-a")
	if err != nil || empty.State != "empty" || empty.ActivationLocked {
		t.Fatalf("empty status = %#v, err = %v", empty, err)
	}

	artifact := Artifact{
		SchemaVersion:     1,
		WorkspaceID:       "case-a",
		Layer:             "L1",
		Period:            "2026-07-20",
		GeneratedAt:       time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC),
		SourceFingerprint: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Sources:           []SourceRef{{ID: "capture", SHA256: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}},
		SynthesizerID:     "test-synth-v1",
		Content:           "daily",
	}
	if err := engine.activate("case-a", []Artifact{artifact}); err != nil {
		t.Fatal(err)
	}
	newer := artifact
	newer.Period = "2026-07-21"
	newer.SourceFingerprint = "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	if err := engine.activate("case-a", []Artifact{newer}); err != nil {
		t.Fatal(err)
	}
	status, err := engine.Status("case-a")
	if err != nil {
		t.Fatal(err)
	}
	if status.State != "ready" || status.TransactionID == "" || status.Layers["L1"] != "2026-07-21" {
		t.Fatalf("committed status = %#v", status)
	}
}

func TestAllInvalidCommitsAreCorruptNotMissing(t *testing.T) {
	engine := testEngine(t)
	commits := filepath.Join(engine.workspaceRoot("case-a"), "commits")
	if err := os.MkdirAll(commits, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(commits, "20260720T120000.000000000Z-corrupt.json"), []byte(`{"broken":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	status, err := engine.Status("case-a")
	if err != nil || status.State != "corrupt" {
		t.Fatalf("status = %#v, err = %v", status, err)
	}
	if _, err := engine.AssembleContext("case-a"); !errors.Is(err, ErrNoValidCommit) {
		t.Fatalf("context corruption error = %v", err)
	}
}

func TestStatusSeparatesAttestedContinuitySignalsFromLegacyCaptures(t *testing.T) {
	engine := testEngine(t)
	now := time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	if _, err := engine.Capture(Capture{WorkspaceID: "case-a", RecordedAt: now, Kind: "legacy", Text: "legacy", Sanitized: true}); err != nil {
		t.Fatal(err)
	}
	attestor := CaptureAttestor{Root: engine.Root}
	sealed, err := attestor.Seal(Capture{WorkspaceID: "case-a", RecordedAt: now, Kind: "skill_route", Text: "case-kickoff", Sanitized: true, ProducerID: "claude.context-injection", SanitizerID: SkillRouteSanitizerID, SourceDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Capture(sealed); err != nil {
		t.Fatal(err)
	}
	status, err := engine.Status("case-a")
	if err != nil || status.CaptureFiles != 2 || status.AttestedCaptureFiles != 1 {
		t.Fatalf("status = %#v, err = %v", status, err)
	}
}
