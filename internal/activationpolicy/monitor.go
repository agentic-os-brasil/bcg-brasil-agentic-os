package activationpolicy

import (
	"errors"
	"sort"
)

type EpisodeOutcome string

const (
	CompletedOutcome EpisodeOutcome = "completed"
	FailedOutcome    EpisodeOutcome = "failed"
	BlockedOutcome   EpisodeOutcome = "blocked"
)

type Observation struct {
	SchemaVersion   int            `json:"schema_version"`
	WindowID        string         `json:"window_id"`
	PlanSHA256      string         `json:"plan_sha256"`
	PolicyVersion   string         `json:"policy_version"`
	Posture         Posture        `json:"posture"`
	Route           Route          `json:"route"`
	Outcome         EpisodeOutcome `json:"outcome"`
	DurationSeconds int            `json:"duration_seconds"`
	BudgetExhausted bool           `json:"budget_exhausted"`
	MissingReceipt  bool           `json:"missing_receipt"`
	HumanOverride   bool           `json:"human_override"`
}

type DarwinReport struct {
	SchemaVersion         int              `json:"schema_version"`
	WindowID              string           `json:"window_id,omitempty"`
	PolicyVersion         string           `json:"policy_version"`
	Posture               Posture          `json:"posture"`
	Episodes              int              `json:"episodes"`
	RouteCounts           map[Route]int    `json:"route_counts"`
	RouteBasisPoints      map[Route]int    `json:"route_basis_points"`
	MeanDurationSeconds   map[Route]int    `json:"mean_duration_seconds"`
	BudgetExhaustions     int              `json:"budget_exhaustions"`
	MissingReceipts       int              `json:"missing_receipts"`
	HumanOverrides        int              `json:"human_overrides"`
	HypothesisBasisPoints map[Route]string `json:"shadow_hypothesis_basis_points"`
	RecommendationCodes   []string         `json:"recommendation_codes"`
	MayMutatePolicy       bool             `json:"may_mutate_policy"`
	EvidenceAuthority     string           `json:"evidence_authority"`
}

func EvaluateObservations(observations []Observation) (DarwinReport, error) {
	report := DarwinReport{
		SchemaVersion: 1, PolicyVersion: PolicyVersion, Posture: Balanced,
		RouteCounts:         map[Route]int{D0Direct: 0, D1Targeted: 0, D2Governed: 0, Blocked: 0},
		RouteBasisPoints:    map[Route]int{D0Direct: 0, D1Targeted: 0, D2Governed: 0, Blocked: 0},
		MeanDurationSeconds: map[Route]int{D0Direct: 0, D1Targeted: 0, D2Governed: 0, Blocked: 0},
		HypothesisBasisPoints: map[Route]string{
			D0Direct: "7000", D1Targeted: "2000-2500", D2Governed: "500-1000",
		},
		MayMutatePolicy:   false,
		EvidenceAuthority: "caller_asserted_shadow",
	}
	if len(observations) == 0 {
		report.RecommendationCodes = []string{"insufficient_sample"}
		return report, nil
	}
	durationTotals := map[Route]int{}
	seenPlans := map[string]bool{}
	for _, observation := range observations {
		if observation.SchemaVersion != 1 || !validID(observation.WindowID) ||
			!validSHA256(observation.PlanSHA256) ||
			observation.PolicyVersion != PolicyVersion ||
			(observation.Posture != Direct && observation.Posture != Balanced && observation.Posture != Deliberative) ||
			(observation.Route != D0Direct && observation.Route != D1Targeted && observation.Route != D2Governed && observation.Route != Blocked) ||
			(observation.Outcome != CompletedOutcome && observation.Outcome != FailedOutcome && observation.Outcome != BlockedOutcome) ||
			observation.DurationSeconds < 0 || observation.DurationSeconds > 86400 {
			return DarwinReport{}, errors.New("Darwin observation violates the content-free routing contract")
		}
		if observation.WindowID != observations[0].WindowID {
			return DarwinReport{}, errors.New("Darwin report cannot mix calibration windows")
		}
		if seenPlans[observation.PlanSHA256] {
			return DarwinReport{}, errors.New("Darwin report rejects duplicate plan observations")
		}
		seenPlans[observation.PlanSHA256] = true
		if observation.Posture != observations[0].Posture {
			return DarwinReport{}, errors.New("Darwin report cannot mix activation postures")
		}
		report.RouteCounts[observation.Route]++
		durationTotals[observation.Route] += observation.DurationSeconds
		if observation.BudgetExhausted {
			report.BudgetExhaustions++
		}
		if observation.MissingReceipt {
			report.MissingReceipts++
		}
		if observation.HumanOverride {
			report.HumanOverrides++
		}
	}
	report.Posture = observations[0].Posture
	report.WindowID = observations[0].WindowID
	report.Episodes = len(observations)
	for route, count := range report.RouteCounts {
		report.RouteBasisPoints[route] = count * 10000 / report.Episodes
		if count > 0 {
			report.MeanDurationSeconds[route] = durationTotals[route] / count
		}
	}
	var codes []string
	if report.Episodes < 20 {
		codes = append(codes, "insufficient_sample")
	}
	if report.MissingReceipts > 0 {
		codes = append(codes, "review_receipt_coverage")
	}
	if report.BudgetExhaustions*10 > report.Episodes {
		codes = append(codes, "review_budget_pressure")
	}
	if report.MeanDurationSeconds[D1Targeted] > 1200 ||
		report.MeanDurationSeconds[D2Governed] > 2700 {
		codes = append(codes, "review_latency")
	}
	if report.HumanOverrides*5 > report.Episodes {
		codes = append(codes, "review_override_rate")
	}
	if report.Episodes >= 20 &&
		(report.RouteBasisPoints[D0Direct] < 5000 ||
			report.RouteBasisPoints[D2Governed] > 2000) {
		codes = append(codes, "review_mix_hypothesis")
	}
	if len(codes) == 0 {
		codes = append(codes, "hold_current_posture")
	}
	sort.Strings(codes)
	report.RecommendationCodes = codes
	return report, nil
}
