package activationpolicy

import "testing"

func TestDarwinReportObservesButCannotMutatePolicy(t *testing.T) {
	observations := make([]Observation, 0, 20)
	for index := 0; index < 20; index++ {
		route := D0Direct
		if index >= 14 && index < 18 {
			route = D1Targeted
		}
		if index >= 18 {
			route = D2Governed
		}
		observations = append(observations, Observation{
			SchemaVersion: 1, WindowID: "window-2026-07",
			PlanSHA256:    digest(string(rune(index + 1))),
			PolicyVersion: PolicyVersion, Posture: Balanced,
			Route: route, Outcome: CompletedOutcome, DurationSeconds: 120,
		})
	}
	report, err := EvaluateObservations(observations)
	if err != nil {
		t.Fatal(err)
	}
	if report.Episodes != 20 || report.RouteBasisPoints[D0Direct] != 7000 ||
		report.RouteBasisPoints[D1Targeted] != 2000 ||
		report.RouteBasisPoints[D2Governed] != 1000 {
		t.Fatalf("unexpected Darwin report: %#v", report)
	}
	if report.MayMutatePolicy || len(report.RecommendationCodes) != 1 ||
		report.RecommendationCodes[0] != "hold_current_posture" {
		t.Fatalf("Darwin exceeded proposal-only authority: %#v", report)
	}
}

func TestDarwinReportRejectsMixedPostures(t *testing.T) {
	_, err := EvaluateObservations([]Observation{
		{SchemaVersion: 1, WindowID: "window-a", PlanSHA256: digest("one"), PolicyVersion: PolicyVersion, Posture: Balanced, Route: D0Direct, Outcome: CompletedOutcome},
		{SchemaVersion: 1, WindowID: "window-a", PlanSHA256: digest("two"), PolicyVersion: PolicyVersion, Posture: Direct, Route: D0Direct, Outcome: CompletedOutcome},
	})
	if err == nil {
		t.Fatal("mixed-posture calibration window was accepted")
	}
}

func TestEmptyDarwinReportDoesNotInventAPosture(t *testing.T) {
	report, err := EvaluateObservations(nil)
	if err != nil {
		t.Fatal(err)
	}
	if report.Posture != "" || report.RecommendationCodes[0] != "insufficient_sample" {
		t.Fatalf("empty report invented posture: %#v", report)
	}
}
