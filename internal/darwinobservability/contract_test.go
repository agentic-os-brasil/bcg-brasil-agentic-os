package darwinobservability

import (
	"strings"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/activationpolicy"
)

func selectionRecord(t *testing.T, id, window string, route activationpolicy.Route) Record {
	t.Helper()
	record := Record{
		SchemaVersion: SchemaVersion, Kind: KindSelection, EvidenceID: id, WindowID: window,
		RecordedAt: time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC),
		Selection: &SelectionEvidence{
			PlanSHA256: strings.Repeat("a", 64), PolicyVersion: activationpolicy.PolicyVersion,
			Posture: activationpolicy.Balanced, Route: route, Outcome: OutcomeSucceeded,
			DurationSeconds: 30, MaxCalls: 3, CallsUsed: 1, MaxTokenUnits: 10000,
			TokenUnitsUsed: 1000, OverrideKind: OverrideNone, PACoverage: PACoverageCovered,
			PAExpertCount: 1, CapabilityGaps: []CapabilityGap{},
		},
	}
	if err := record.Validate(); err != nil {
		t.Fatalf("valid selection rejected: %v", err)
	}
	return record
}

func TestRecordRejectsCrossPayloadAndForbiddenLifecycle(t *testing.T) {
	record := selectionRecord(t, "ev-one", "week-1", activationpolicy.D0Direct)
	record.Health = &HealthEvidence{}
	if err := record.Validate(); err == nil {
		t.Fatal("cross-payload evidence accepted")
	}

	proposal := Record{SchemaVersion: SchemaVersion, Kind: KindProposal, EvidenceID: "ev-proposal", WindowID: "week-1", RecordedAt: time.Now().UTC(), Proposal: &ProposalEvidence{ProposalSHA256: strings.Repeat("b", 64), ProposalKind: ProposalPolicyCalibration, Status: ProposalDraft, AuthorRole: "human_maintainer"}}
	if err := proposal.Validate(); err == nil {
		t.Fatal("non-Darwin author accepted")
	}
}

func TestDecodeStrictRejectsUnknownDuplicateAndTrailing(t *testing.T) {
	valid := `{"schema_version":1,"kind":"proposal","evidence_id":"ev-1","window_id":"week-1","recorded_at":"2026-07-30T12:00:00Z","proposal":{"proposal_sha256":"` + strings.Repeat("a", 64) + `","proposal_kind":"policy_calibration","status":"draft","author_role":"darwin"}}`
	var record Record
	if err := DecodeStrict([]byte(valid), &record); err != nil {
		t.Fatalf("valid JSON rejected: %v", err)
	}
	if err := DecodeStrict([]byte(strings.Replace(valid, `"kind":"proposal"`, `"kind":"proposal","extra":true`, 1)), &record); err == nil {
		t.Fatal("unknown field accepted")
	}
	if err := DecodeStrict([]byte(strings.Replace(valid, `"kind":"proposal"`, `"kind":"proposal","kind":"proposal"`, 1)), &record); err == nil {
		t.Fatal("duplicate field accepted")
	}
	if err := DecodeStrict([]byte(valid+" {}"), &record); err == nil {
		t.Fatal("trailing JSON accepted")
	}
}

func TestEvaluationRequiresIndependentActor(t *testing.T) {
	record := Record{SchemaVersion: SchemaVersion, Kind: KindEvaluation, EvidenceID: "ev-eval", WindowID: "week-2", RecordedAt: time.Now().UTC(), Evaluation: &EvaluationEvidence{
		ProposalSHA256: strings.Repeat("a", 64), BaselineWindowID: "week-1", PostChangeWindowID: "week-2", ChangeSHA256: strings.Repeat("b", 64), EvaluatorRole: "darwin", Outcome: EvaluationImproved,
	}}
	if err := record.Validate(); err == nil {
		t.Fatal("Darwin evaluation accepted")
	}
}
