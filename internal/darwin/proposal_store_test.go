package darwin

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/maintenance"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

func testAssessmentArtifact(t *testing.T) AssessmentProposalArtifact {
	t.Helper()
	assessment, err := Plan(HealthPacket{SchemaVersion: SchemaVersion, WindowID: "window-proposal-store", Runtime: "runtime-neutral", Mode: DeepReview, Observations: []Observation{{Code: ObservationContractDrift, Severity: SeverityHigh, Count: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	occurrenceDigest := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	scheduledFor := time.Unix(10, 0).UTC()
	return AssessmentProposalArtifact{SchemaVersion: proposalArtifactSchemaVersion, RecordType: "assessment", AgentID: AgentID, JobID: "darwin-deep-weekly", OccurrenceDigest: occurrenceDigest, ArtifactID: assessmentArtifactID("darwin-deep-weekly", occurrenceDigest), WindowID: assessment.WindowID, ProposalDigest: proposalDigest(occurrenceDigest, assessment), Assessment: assessment, ScheduledFor: scheduledFor, RecordedAt: scheduledFor}
}

func TestProposalStoreIsIdempotentAndBindsExactAssessment(t *testing.T) {
	root := t.TempDir()
	store := ProposalStore{Root: root}
	artifact := testAssessmentArtifact(t)
	if err := store.Append(artifact); err != nil {
		t.Fatal(err)
	}
	if err := store.Append(artifact); err != nil {
		t.Fatalf("idempotent append failed: %v", err)
	}
	read, err := store.Read(artifact.ArtifactID)
	if err != nil || read.ProposalDigest != artifact.ProposalDigest || len(read.Assessment.Proposals) != 1 {
		t.Fatalf("read artifact=%#v err=%v", read, err)
	}

	tampered := artifact
	tampered.ScheduledFor = artifact.ScheduledFor.Add(time.Minute)
	tampered.RecordedAt = tampered.ScheduledFor
	if err := store.Append(tampered); !errors.Is(err, ErrProposalReplayConflict) {
		t.Fatalf("collision error=%v, want replay conflict", err)
	}

	path := filepath.Join(root, "proposals", artifact.ArtifactID+".json")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(body, []byte(`"proposal_digest"`)) {
		t.Fatal("artifact did not contain its bound digest")
	}
	body = bytes.Replace(body, []byte(`"priority":1`), []byte(`"priority":2`), 1)
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Read(artifact.ArtifactID); err == nil {
		t.Fatal("tampered artifact was accepted")
	}
}

func TestProposalStoreRejectsSymlinkedArtifact(t *testing.T) {
	root := t.TempDir()
	store := ProposalStore{Root: root}
	artifact := testAssessmentArtifact(t)
	if err := store.Append(artifact); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "proposals", artifact.ArtifactID+".json")
	backup := filepath.Join(root, "original.json")
	if err := os.Rename(path, backup); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(backup, path); err != nil {
		t.Skipf("symlink capability unavailable: %v", err)
	}
	if _, err := store.Read(artifact.ArtifactID); err == nil {
		t.Fatal("symlinked proposal artifact was accepted")
	}
}

func TestProposalStoreRejectsSymlinkedProposalDirectory(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "proposals")); err != nil {
		t.Skipf("symlink capability unavailable: %v", err)
	}
	if err := (ProposalStore{Root: root}).Append(testAssessmentArtifact(t)); err == nil {
		t.Fatal("proposal store traversed a symlinked ancestor directory")
	}
}

func TestProposalStoreRejectsSymlinkedAncestorBeforeCreatingStore(t *testing.T) {
	parent := t.TempDir()
	outside := t.TempDir()
	alias := filepath.Join(parent, "alias")
	if err := os.Symlink(outside, alias); err != nil {
		t.Skipf("symlink capability unavailable: %v", err)
	}
	root := filepath.Join(alias, "darwin")
	if err := (ProposalStore{Root: root}).Append(testAssessmentArtifact(t)); err == nil {
		t.Fatal("proposal store traversed a symlinked ancestor")
	}
	if _, err := os.Stat(filepath.Join(outside, "darwin")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("symlink target was modified, stat err=%v", err)
	}
}

func TestDeepReviewRetryAfterArtifactOnlyCrashIsSingleReceipt(t *testing.T) {
	root := t.TempDir()
	commandStore := maintenance.Store{Root: filepath.Join(root, "receipts")}
	proposalStore := ProposalStore{Root: filepath.Join(root, "proposals")}
	builds := 0
	handler := DeepReviewHandler{Build: HealthPacketBuilderFunc(func(context.Context, scheduler.Occurrence) (HealthPacket, error) {
		builds++
		code := ObservationContractDrift
		if builds > 1 {
			code = ObservationStateStale
		}
		return HealthPacket{SchemaVersion: SchemaVersion, WindowID: "window-retry", Runtime: "runtime-neutral", Mode: DeepReview, Observations: []Observation{{Code: code, Severity: SeverityHigh, Count: 1}}}, nil
	}), CommandStore: commandStore, ProposalStore: proposalStore, Now: func() time.Time { return time.Unix(20, 0).UTC() }}
	base := maintenance.Command{SchemaVersion: maintenance.CommandSchemaVersion, CommandID: "wake-first", JobID: "darwin-deep-weekly", WorkspaceID: "maestro-system", Trigger: maintenance.TriggerWeekly, ScheduledFor: time.Unix(10, 0).UTC(), RequestedAt: time.Unix(10, 0).UTC(), Deadline: time.Unix(30, 0).UTC()}
	first, err := handler.Execute(context.Background(), base)
	if err != nil || first.State != maintenance.ReceiptProposalEmitted {
		t.Fatalf("first proposal=%#v err=%v", first, err)
	}
	// Simulate a process crash after the artifact commit but before the outer
	// maintenance receipt publication. Remove the first terminal receipt while
	// leaving the durable proposal artifact in place.
	receipts, err := commandStore.Receipts(base.WorkspaceID, base.JobID)
	if err != nil || len(receipts) != 1 {
		t.Fatalf("first receipt=%#v err=%v", receipts, err)
	}
	if err := os.Remove(filepath.Join(root, "receipts", "workspaces", base.WorkspaceID, "receipts", base.JobID, receipts[0].CommandID+"--"+receipts[0].AttemptID+".json")); err != nil {
		t.Fatal(err)
	}
	retry := base
	retry.CommandID = "wake-retry"
	handler.Now = func() time.Time { return time.Unix(120, 0).UTC() }
	second, err := handler.Execute(context.Background(), retry)
	if err != nil || second.ProposalDigest != first.ProposalDigest || second.ProposalArtifactID != first.ProposalArtifactID || builds != 1 {
		t.Fatalf("retry proposal=%#v err=%v builds=%d", second, err, builds)
	}
	artifact, err := proposalStore.Read(first.ProposalArtifactID)
	if err != nil || len(artifact.Assessment.Proposals) != 1 || artifact.Assessment.Proposals[0].Finding != ObservationContractDrift {
		t.Fatalf("retry did not recover original artifact=%#v err=%v", artifact, err)
	}
	receipts, err = commandStore.Receipts(base.WorkspaceID, base.JobID)
	if err != nil || len(receipts) != 1 {
		t.Fatalf("retry produced duplicate terminal receipts: %#v err=%v", receipts, err)
	}
	entries, err := os.ReadDir(filepath.Join(root, "proposals", "proposals"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("retry produced duplicate proposal artifacts: entries=%d err=%v", len(entries), err)
	}
}

func TestDeepReviewNoProposalUsesReviewedNoChangeTerminalState(t *testing.T) {
	root := t.TempDir()
	commandStore := maintenance.Store{Root: filepath.Join(root, "receipts")}
	handler := DeepReviewHandler{
		Build: HealthPacketBuilderFunc(func(context.Context, scheduler.Occurrence) (HealthPacket, error) {
			return HealthPacket{SchemaVersion: SchemaVersion, WindowID: "window-no-change", Runtime: "runtime-neutral", Mode: DeepReview}, nil
		}),
		CommandStore: commandStore,
		Now:          func() time.Time { return time.Unix(20, 0).UTC() },
	}
	command := maintenance.Command{SchemaVersion: maintenance.CommandSchemaVersion, CommandID: "wake-no-change", JobID: "darwin-deep-weekly", WorkspaceID: "maestro-system", Trigger: maintenance.TriggerWeekly, ScheduledFor: time.Unix(10, 0).UTC(), RequestedAt: time.Unix(10, 0).UTC(), Deadline: time.Unix(30, 0).UTC()}
	result, err := handler.Execute(context.Background(), command)
	if err != nil || result.State != maintenance.ReceiptReviewedNoChange || result.ProposalCount != 0 || result.ProposalDigest != "" {
		t.Fatalf("no-change result=%#v err=%v", result, err)
	}
}
