package darwinobservability

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/activationpolicy"
)

func TestWeeklyAggregationIsOrderIndependentAndClosed(t *testing.T) {
	window := Window{ID: "week-1", Start: time.Date(2026, 7, 27, 0, 0, 0, 0, time.UTC), End: time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC)}
	one := selectionRecord(t, "ev-b", "week-1", activationpolicy.D1Targeted)
	two := selectionRecord(t, "ev-a", "week-1", activationpolicy.D0Direct)
	two.Selection.HumanOverride, two.Selection.OverrideKind = true, OverrideRoute
	health := Record{SchemaVersion: SchemaVersion, Kind: KindHealth, EvidenceID: "ev-c", WindowID: "week-1", RecordedAt: time.Date(2026, 7, 30, 10, 0, 0, 0, time.UTC), Health: &HealthEvidence{JobKind: JobDarwinHousekeeping, ScheduledAt: time.Date(2026, 7, 29, 10, 0, 0, 0, time.UTC), CapturedAt: time.Date(2026, 7, 30, 10, 0, 0, 0, time.UTC), Freshness: FreshnessMissed, Recovery: RecoveryRecovered, Outcome: OutcomeSucceeded}}
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
	windows := []Window{{ID: "week-1", Start: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), End: time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC)}, {ID: "week-2", Start: time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC), End: time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC)}}
	base := Record{SchemaVersion: SchemaVersion, Kind: KindAlternative, EvidenceID: "ev-base", WindowID: "week-1", RecordedAt: time.Date(2026, 7, 2, 0, 0, 0, 0, time.UTC), Alternative: &AlternativeEvidence{AlternativeID: AlternativeBaseline, PolicyVersion: activationpolicy.PolicyVersion, Posture: activationpolicy.Balanced, Route: activationpolicy.D0Direct, Outcome: OutcomeSucceeded, PACoverage: PACoverageNotRequired}}
	candidate := base
	candidate.EvidenceID = "ev-candidate"
	candidate.WindowID = "week-2"
	candidate.RecordedAt = time.Date(2026, 7, 9, 0, 0, 0, 0, time.UTC)
	candidate.Alternative = &AlternativeEvidence{AlternativeID: AlternativeCandidateA, PolicyVersion: "pae-v2-hypothesis", Posture: activationpolicy.Balanced, Route: activationpolicy.D1Targeted, Outcome: OutcomeSucceeded, PACoverage: PACoverageCovered}
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
}

func TestInputDigestRejectsDuplicateEvidence(t *testing.T) {
	a := selectionRecord(t, "ev-same", "week-1", activationpolicy.D0Direct)
	b := selectionRecord(t, "ev-same", "week-1", activationpolicy.D1Targeted)
	if _, err := InputDigest([]Record{a, b}); err == nil {
		t.Fatal("duplicate evidence accepted")
	}
	if !strings.HasPrefix(EvidenceID(KindSelection, "week-1", []byte("x")), "ev-") {
		t.Fatal("evidence ID is not deterministic digest form")
	}
}
