package memory

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
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

func TestBootstrapMaterializesAnEmptyWorkspaceMemoryRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "memory")
	if err := Bootstrap(root, "case-a"); err != nil {
		t.Fatal(err)
	}
	for _, relative := range []string{
		"l1/captures",
		"l1/attested-captures",
		"commits",
		"versions",
		".transactions",
		".locks",
	} {
		info, err := os.Stat(filepath.Join(root, "workspaces", "case-a", filepath.FromSlash(relative)))
		if err != nil || !info.IsDir() {
			t.Fatalf("memory bootstrap directory %s: info=%v err=%v", relative, info, err)
		}
	}
	report, err := (&Engine{Root: root}).Status("case-a")
	if err != nil || report.State != "empty" || report.CaptureFiles != 0 || report.ActivationLocked {
		t.Fatalf("bootstrapped memory status=%#v err=%v", report, err)
	}
}

func TestBootstrapRejectsSymlinkedAncestorWithoutWritingOutsideRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires privileges on some Windows runners")
	}
	root := t.TempDir()
	external := filepath.Join(root, "external")
	if err := os.MkdirAll(external, 0o700); err != nil {
		t.Fatal(err)
	}
	linked := filepath.Join(root, "linked")
	if err := os.Symlink(external, linked); err != nil {
		t.Fatal(err)
	}
	if err := Bootstrap(filepath.Join(linked, "memory"), "case-a"); err == nil {
		t.Fatal("memory bootstrap followed a symlinked ancestor")
	}
	entries, err := os.ReadDir(external)
	if err != nil || len(entries) != 0 {
		t.Fatalf("external target was modified: entries=%v err=%v", entries, err)
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
	sealed, err := attestor.Seal(Capture{WorkspaceID: "case-a", RecordedAt: now, Kind: "skill_route", Text: "bcg-case-kickoff", Sanitized: true, ProducerID: "claude.context-injection", SanitizerID: SkillRouteSanitizerID, SourceDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"})
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

func TestContinuityStatusVerifiesAndBoundsAttestedCaptures(t *testing.T) {
	engine := testEngine(t)
	now := time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	attestor := CaptureAttestor{Root: engine.Root}
	sealed, err := attestor.Seal(Capture{WorkspaceID: "case-a", RecordedAt: now, Kind: "skill_route", Text: "bcg-case-kickoff", Sanitized: true, ProducerID: "claude.context-injection", SanitizerID: SkillRouteSanitizerID, SourceDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Capture(sealed); err != nil {
		t.Fatal(err)
	}
	status, err := engine.ContinuityStatus("case-a")
	if err != nil || status.AttestedCaptureFiles != 1 || status.State != "empty" {
		t.Fatalf("continuity status=%#v err=%v", status, err)
	}
	path := filepath.Join(engine.workspaceRoot("case-a"), "l1", "attested-captures", "2026-08-06.jsonl")
	if err := os.WriteFile(path, []byte(`{"schema_version":2`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.ContinuityStatus("case-a"); err == nil {
		t.Fatal("truncated attested capture was accepted as continuity evidence")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	for index := 0; index <= MaximumContinuityCaptureFiles; index++ {
		candidate := sealed
		candidate.RecordedAt = now.AddDate(0, 0, index+1)
		candidate, err = attestor.Seal(candidate)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := engine.Capture(candidate); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := engine.ContinuityStatus("case-a"); err == nil {
		t.Fatal("unbounded attested capture history was accepted")
	}
	if err := os.RemoveAll(filepath.Join(engine.workspaceRoot("case-a"), "l1", "attested-captures")); err != nil {
		t.Fatal(err)
	}
	commits := filepath.Join(engine.workspaceRoot("case-a"), "commits")
	if err := os.MkdirAll(commits, 0o700); err != nil {
		t.Fatal(err)
	}
	for index := 0; index <= MaximumContinuityCommitEntries; index++ {
		if err := os.WriteFile(filepath.Join(commits, fmt.Sprintf("20260805T120000.%09dZ-overflow.json", index)), []byte(`{}`), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := engine.ContinuityStatus("case-a"); err == nil {
		t.Fatal("unbounded memory commit history was accepted")
	}
}
