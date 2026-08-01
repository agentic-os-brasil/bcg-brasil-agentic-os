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
	return AssessmentProposalArtifact{SchemaVersion: proposalArtifactSchemaVersion, RecordType: "assessment", AgentID: AgentID, JobID: "darwin-deep-weekly", OccurrenceDigest: occurrenceDigest, WindowID: assessment.WindowID, ProposalDigest: proposalDigest(occurrenceDigest, assessment), Assessment: assessment, ScheduledFor: scheduledFor, RecordedAt: scheduledFor}
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
	read, err := store.Read(artifact.ProposalDigest)
	if err != nil || read.ProposalDigest != artifact.ProposalDigest || len(read.Assessment.Proposals) != 1 {
		t.Fatalf("read artifact=%#v err=%v", read, err)
	}

	tampered := artifact
	tampered.ScheduledFor = artifact.ScheduledFor.Add(time.Minute)
	tampered.RecordedAt = tampered.ScheduledFor
	if err := store.Append(tampered); !errors.Is(err, ErrProposalReplayConflict) {
		t.Fatalf("collision error=%v, want replay conflict", err)
	}

	path := filepath.Join(root, "proposals", artifact.ProposalDigest+".json")
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
	if _, err := store.Read(artifact.ProposalDigest); err == nil {
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
	path := filepath.Join(root, "proposals", artifact.ProposalDigest+".json")
	backup := filepath.Join(root, "original.json")
	if err := os.Rename(path, backup); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(backup, path); err != nil {
		t.Skipf("symlink capability unavailable: %v", err)
	}
	if _, err := store.Read(artifact.ProposalDigest); err == nil {
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

func TestDeepReviewRetryAfterArtifactOnlyCrashIsSingleReceipt(t *testing.T) {
	root := t.TempDir()
	commandStore := maintenance.Store{Root: filepath.Join(root, "receipts")}
	proposalStore := ProposalStore{Root: filepath.Join(root, "proposals")}
	builds := 0
	handler := DeepReviewHandler{Build: HealthPacketBuilderFunc(func(context.Context, scheduler.Occurrence) (HealthPacket, error) {
		builds++
		return HealthPacket{SchemaVersion: SchemaVersion, WindowID: "window-retry", Runtime: "runtime-neutral", Mode: DeepReview, Observations: []Observation{{Code: ObservationContractDrift, Severity: SeverityHigh, Count: 1}}}, nil
	}), CommandStore: commandStore, ProposalStore: proposalStore, Now: func() time.Time { return time.Unix(20, 0).UTC() }}
	base := maintenance.Command{SchemaVersion: maintenance.CommandSchemaVersion, CommandID: "wake-first", JobID: "darwin-deep-weekly", WorkspaceID: "maestro-system", Trigger: maintenance.TriggerWeekly, ScheduledFor: time.Unix(10, 0).UTC(), RequestedAt: time.Unix(10, 0).UTC(), Deadline: time.Unix(30, 0).UTC()}
	first, err := handler.Execute(context.Background(), base)
	if err != nil || first.State != maintenance.ReceiptProposalEmitted {
		t.Fatalf("first proposal=%#v err=%v", first, err)
	}
	// Simulate a process crash after the artifact commit but before the outer
	// maintenance receipt publication: the next command ID is a new attempt,
	// while the occurrence and assessment identity remain unchanged.
	retry := base
	retry.CommandID = "wake-retry"
	second, err := handler.Execute(context.Background(), retry)
	if err != nil || second.ProposalDigest != first.ProposalDigest || builds != 1 {
		t.Fatalf("retry proposal=%#v err=%v builds=%d", second, err, builds)
	}
	receipts, err := commandStore.Receipts(base.WorkspaceID, base.JobID)
	if err != nil || len(receipts) != 1 {
		t.Fatalf("retry produced duplicate terminal receipts: %#v err=%v", receipts, err)
	}
}
