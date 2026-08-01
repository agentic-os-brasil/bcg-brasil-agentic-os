package darwinobservability

import (
	"strings"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/darwin"
)

func TestSelfMaintenanceProjectsIntoWeeklyMetadataScorecard(t *testing.T) {
	receipt := darwin.SelfMaintenanceReceipt{
		SchemaVersion: darwin.SchemaVersion, AgentID: darwin.AgentID, SnapshotVersion: 3,
		SnapshotSHA256: strings.Repeat("a", 64), ObservationCount: 2, DuplicateCount: 1,
		ContradictionCount: 1, RecheckDue: 1, DecayCandidates: 1, OwnerConfirmedSignals: 1,
		ReevaluationProposals: 1, CanonicalMutations: 0, RecordedAt: time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC),
		ProposalEvidenceSHA256: []string{strings.Repeat("b", 64)},
	}
	record, err := FromDarwinSelfMaintenanceReceipt(receipt, "win-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", strings.Repeat("c", 64))
	if err != nil {
		t.Fatal(err)
	}
	window := Window{ID: record.WindowID, ScopeSHA256: record.ScopeSHA256, Start: receipt.RecordedAt.Add(-time.Hour), End: receipt.RecordedAt.Add(time.Hour)}
	report, err := BuildWeekly([]Record{record}, window)
	if err != nil {
		t.Fatal(err)
	}
	if report.Self.Records != 1 || report.Self.Contradictions != 1 || report.Self.CanonicalMutations != 0 {
		t.Fatalf("self scorecard = %#v", report.Self)
	}
}
