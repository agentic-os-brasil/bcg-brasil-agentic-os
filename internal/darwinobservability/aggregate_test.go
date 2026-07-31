package darwinobservability

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/activationpolicy"
)

func TestWeeklyAggregationIsOrderIndependentAndClosed(t *testing.T) {
	scope := strings.Repeat("f", 64)
	window := Window{ID: OpaqueWindowID("week-1"), ScopeSHA256: scope, Start: time.Date(2026, 7, 27, 0, 0, 0, 0, time.UTC), End: time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC)}
	one := selectionRecord(t, "ev-b", window.ID, activationpolicy.D1Targeted)
	two := selectionRecord(t, "ev-a", window.ID, activationpolicy.D0Direct)
	two.Selection.HumanOverride, two.Selection.OverrideKind = true, OverrideRoute
	two = bindRecord(t, two)
	health := bindRecord(t, Record{SchemaVersion: SchemaVersion, Kind: KindHealth, EvidenceID: "ev-c", WindowID: window.ID, ScopeSHA256: scope, Authority: AuthorityCallerAssertedShadow, RecordedAt: time.Date(2026, 7, 30, 10, 0, 0, 0, time.UTC), Health: &HealthEvidence{JobKind: JobDarwinHousekeeping, ScheduledAt: time.Date(2026, 7, 29, 10, 0, 0, 0, time.UTC), CapturedAt: time.Date(2026, 7, 30, 10, 0, 0, 0, time.UTC), Freshness: FreshnessMissed, Recovery: RecoveryRecovered, Outcome: OutcomeSucceeded}})
	first, err := BuildWeekly([]Record{one, two, health}, window)
	if err != nil {
		t.Fatalf("weekly failed: %v", err)
	}
	second, err := BuildWeekly([]Record{health, two, one}, window)
	if err != nil {
		t.Fatalf("weekly reorder failed: %v", err)
	}
	if first.InputSHA256 != second.InputSHA256 || !reflect.DeepEqual(first.Selection, second.Selection) || !reflect.DeepEqual(first.Health, second.Health) {
		t.Fatalf("aggregation was order-dependent")
	}
	if first.MayMutatePolicy || len(first.RecommendationCodes) == 0 {
		t.Fatalf("weekly report lacks fail-closed recommendation")
	}
}

func TestMonthlyComparesAlternativesWithoutThresholdDecision(t *testing.T) {
	scope := strings.Repeat("f", 64)
	cohort := strings.Repeat("c", 64)
	windows := []Window{{ID: OpaqueWindowID("week-1"), ScopeSHA256: scope, Start: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), End: time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC)}, {ID: OpaqueWindowID("week-2"), ScopeSHA256: scope, Start: time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC), End: time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC)}}
	base := bindRecord(t, Record{SchemaVersion: SchemaVersion, Kind: KindAlternative, EvidenceID: "ev-base", WindowID: windows[0].ID, ScopeSHA256: scope, Authority: AuthorityCallerAssertedShadow, RecordedAt: time.Date(2026, 7, 2, 0, 0, 0, 0, time.UTC), Alternative: &AlternativeEvidence{CohortSHA256: cohort, AlternativeID: AlternativeBaseline, PolicyVersion: activationpolicy.PolicyVersion, Posture: activationpolicy.Balanced, Route: activationpolicy.D0Direct, Outcome: OutcomeSucceeded, PACoverage: PACoverageNotRequired}})
	candidate := base
	candidate.EvidenceID = "ev-candidate"
	candidate.WindowID = windows[1].ID
	candidate.RecordedAt = time.Date(2026, 7, 9, 0, 0, 0, 0, time.UTC)
	candidate.Alternative = &AlternativeEvidence{CohortSHA256: cohort, AlternativeID: AlternativeCandidateA, PolicyVersion: "pae-v2-hypothesis", Posture: activationpolicy.Balanced, Route: activationpolicy.D1Targeted, Outcome: OutcomeSucceeded, PACoverage: PACoverageCovered}
	candidate = bindRecord(t, candidate)
	report, err := BuildMonthly([]Record{base, candidate}, windows)
	if err != nil {
		t.Fatalf("monthly failed: %v", err)
	}
	if err := report.Validate(); err != nil {
		t.Fatalf("monthly invalid: %v", err)
	}
	if report.MayMutatePolicy || len(report.Alternatives) != 2 {
		t.Fatalf("monthly report activated or omitted alternatives")
	}
	reordered, err := BuildMonthly([]Record{candidate, base}, []Window{windows[1], windows[0]})
	if err != nil {
		t.Fatalf("monthly reorder failed: %v", err)
	}
	if !reflect.DeepEqual(report, reordered) {
		t.Fatalf("monthly report changed after input reorder")
	}
}

func TestInputDigestRejectsDuplicateEvidence(t *testing.T) {
	a := selectionRecord(t, "ev-same", "week-1", activationpolicy.D0Direct)
	b := a
	if _, err := InputDigest([]Record{a, b}); err == nil {
		t.Fatal("duplicate evidence accepted")
	}
	b.EvidenceID = "ev-forged"
	if err := b.Validate(); err == nil {
		t.Fatal("same payload under a forged evidence ID accepted")
	}
	if !strings.HasPrefix(EvidenceID(KindSelection, "week-1", []byte("x")), "ev-") {
		t.Fatal("evidence ID is not deterministic digest form")
	}
}

func TestMonthlyRejectsOutOfWindowAndUnlinkedLifecycle(t *testing.T) {
	scope := strings.Repeat("f", 64)
	proposalSHA := strings.Repeat("a", 64)
	window := Window{ID: OpaqueWindowID("week-1"), ScopeSHA256: scope, Start: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), End: time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC)}
	acceptance := bindRecord(t, Record{
		SchemaVersion: SchemaVersion, Kind: KindAcceptance, EvidenceID: "ev-acceptance",
		WindowID: window.ID, ScopeSHA256: scope, Authority: AuthorityCallerAssertedShadow,
		RecordedAt: time.Date(2026, 7, 2, 0, 0, 0, 0, time.UTC),
		Acceptance: &AcceptanceEvidence{ProposalSHA256: proposalSHA, Decision: DecisionAccepted, ActorRole: "human_maintainer"},
	})
	if _, err := BuildMonthly([]Record{acceptance}, []Window{window}); err == nil {
		t.Fatal("unlinked acceptance was counted")
	}
	proposal := bindRecord(t, Record{
		SchemaVersion: SchemaVersion, Kind: KindProposal, EvidenceID: "ev-proposal",
		WindowID: window.ID, ScopeSHA256: scope, Authority: AuthorityCallerAssertedShadow,
		RecordedAt: window.End,
		Proposal:   &ProposalEvidence{ProposalSHA256: proposalSHA, ProposalKind: ProposalPolicyCalibration, Status: ProposalAccepted, AuthorRole: "darwin"},
	})
	if _, err := BuildMonthly([]Record{proposal, acceptance}, []Window{window}); err == nil {
		t.Fatal("end-exclusive out-of-window evidence was counted")
	}
	proposal.RecordedAt = time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)
	proposal.Proposal.Status = ProposalDraft
	proposal = bindRecord(t, proposal)
	if _, err := BuildMonthly([]Record{proposal, acceptance}, []Window{window}); err == nil {
		t.Fatal("draft proposal accepted as a decided lifecycle state")
	}
}

func TestWeeklyRejectsCrossScopeAndStructuralEvidence(t *testing.T) {
	scope := strings.Repeat("f", 64)
	window := Window{ID: OpaqueWindowID("week-1"), ScopeSHA256: scope, Start: time.Date(2026, 7, 27, 0, 0, 0, 0, time.UTC), End: time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC)}
	record := selectionRecord(t, "ev-selection", window.ID, activationpolicy.D0Direct)
	record.ScopeSHA256 = strings.Repeat("e", 64)
	record = bindRecord(t, record)
	if _, err := BuildWeekly([]Record{record}, window); err == nil {
		t.Fatal("cross-scope evidence accepted")
	}
}

func TestMonthlyCountsOnlyLinkedAcceptedLifecycle(t *testing.T) {
	scope := strings.Repeat("f", 64)
	proposalSHA := strings.Repeat("a", 64)
	windows := []Window{
		{ID: OpaqueWindowID("week-1"), ScopeSHA256: scope, Start: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), End: time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC)},
		{ID: OpaqueWindowID("week-2"), ScopeSHA256: scope, Start: time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC), End: time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC)},
	}
	records := []Record{
		bindRecord(t, Record{
			SchemaVersion: SchemaVersion, Kind: KindProposal, EvidenceID: "ev-proposal",
			WindowID: windows[0].ID, ScopeSHA256: scope, Authority: AuthorityCallerAssertedShadow,
			RecordedAt: time.Date(2026, 7, 2, 0, 0, 0, 0, time.UTC),
			Proposal:   &ProposalEvidence{ProposalSHA256: proposalSHA, ProposalKind: ProposalReliability, Status: ProposalImplemented, AuthorRole: "darwin"},
		}),
		bindRecord(t, Record{
			SchemaVersion: SchemaVersion, Kind: KindAcceptance, EvidenceID: "ev-acceptance",
			WindowID: windows[0].ID, ScopeSHA256: scope, Authority: AuthorityCallerAssertedShadow,
			RecordedAt: time.Date(2026, 7, 3, 0, 0, 0, 0, time.UTC),
			Acceptance: &AcceptanceEvidence{ProposalSHA256: proposalSHA, Decision: DecisionAccepted, ActorRole: "human_maintainer"},
		}),
		bindRecord(t, Record{
			SchemaVersion: SchemaVersion, Kind: KindEvaluation, EvidenceID: "ev-evaluation",
			WindowID: windows[1].ID, ScopeSHA256: scope, Authority: AuthorityCallerAssertedShadow,
			RecordedAt: time.Date(2026, 7, 9, 0, 0, 0, 0, time.UTC),
			Evaluation: &EvaluationEvidence{
				ProposalSHA256: proposalSHA, BaselineWindowID: windows[0].ID, PostChangeWindowID: windows[1].ID,
				ChangeSHA256: strings.Repeat("b", 64), EvaluatorRole: "independent_evaluator", Outcome: EvaluationImproved,
			},
		}),
	}
	report, err := BuildMonthly(records, windows)
	if err != nil {
		t.Fatalf("linked lifecycle rejected: %v", err)
	}
	if report.ProposalFunnel.Authored != 1 || report.ProposalFunnel.Accepted != 1 ||
		report.ProposalFunnel.Implemented != 1 || report.Independence.Evaluations != 1 ||
		report.Evaluations.Improved != 1 {
		t.Fatalf("linked lifecycle counted incorrectly: %+v", report)
	}
}

func TestReportValidationRejectsForgedNestedCounts(t *testing.T) {
	scope := strings.Repeat("f", 64)
	window := Window{ID: OpaqueWindowID("week-1"), ScopeSHA256: scope, Start: time.Date(2026, 7, 27, 0, 0, 0, 0, time.UTC), End: time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC)}
	record := selectionRecord(t, "ev-selection", window.ID, activationpolicy.D0Direct)
	report, err := BuildWeekly([]Record{record}, window)
	if err != nil {
		t.Fatal(err)
	}
	report.Selection.Routes[0].Count = 99
	if err := report.Validate(); err == nil {
		t.Fatal("forged nested route count accepted")
	}
}
