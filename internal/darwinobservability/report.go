package darwinobservability

import (
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/activationpolicy"
)

type Window struct {
	ID    string    `json:"window_id"`
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

func (window Window) Validate() error {
	if !identifierPattern.MatchString(window.ID) || window.Start.IsZero() || window.End.IsZero() || !window.End.After(window.Start) {
		return errors.New("invalid report window")
	}
	return nil
}

type HealthScorecard struct {
	Records         int `json:"records"`
	Current         int `json:"current"`
	Aging           int `json:"aging"`
	Stale           int `json:"stale"`
	Missed          int `json:"missed"`
	Unavailable     int `json:"unavailable"`
	Recovered       int `json:"recovered"`
	RecoveryFailed  int `json:"recovery_failed"`
	RecoveryBlocked int `json:"recovery_blocked"`
}

type RouteScore struct {
	Route           activationpolicy.Route `json:"route"`
	Count           int                    `json:"count"`
	BasisPoints     int                    `json:"basis_points"`
	MeanDurationSec int                    `json:"mean_duration_seconds"`
	BudgetExhausted int                    `json:"budget_exhausted"`
	HumanOverrides  int                    `json:"human_overrides"`
	PACovered       int                    `json:"pa_covered"`
}

type SelectionScorecard struct {
	Records            int          `json:"records"`
	Completed          int          `json:"completed"`
	Failed             int          `json:"failed"`
	Blocked            int          `json:"blocked"`
	Routes             []RouteScore `json:"routes"`
	MissingPACoverage  int          `json:"missing_pa_coverage"`
	UnavailablePA      int          `json:"unavailable_pa_coverage"`
	CapabilityGapCount int          `json:"capability_gap_count"`
}

type IntegrityScorecard struct {
	InputRecords           int `json:"input_records"`
	AcceptedRecords        int `json:"accepted_records"`
	DuplicateRecords       int `json:"duplicate_records"`
	IndependenceViolations int `json:"independence_violations"`
}

type WeeklyReport struct {
	SchemaVersion       int                `json:"schema_version"`
	ReportKind          string             `json:"report_kind"`
	ReportVersion       string             `json:"report_version"`
	Window              Window             `json:"window"`
	InputSHA256         string             `json:"input_sha256"`
	Health              HealthScorecard    `json:"health"`
	Selection           SelectionScorecard `json:"selection"`
	Integrity           IntegrityScorecard `json:"integrity"`
	RecommendationCodes []string           `json:"recommendation_codes"`
	MayMutatePolicy     bool               `json:"may_mutate_policy"`
}

type AlternativeSummary struct {
	AlternativeID   AlternativeID  `json:"alternative_id"`
	Records         int            `json:"records"`
	MeanDurationSec int            `json:"mean_duration_seconds"`
	BudgetExhausted int            `json:"budget_exhausted"`
	PAUnavailable   int            `json:"pa_unavailable"`
	OutcomeCounts   []OutcomeCount `json:"outcome_counts"`
}

type OutcomeCount struct {
	Outcome Outcome `json:"outcome"`
	Count   int     `json:"count"`
}

type ProposalFunnel struct {
	Authored    int `json:"authored"`
	Accepted    int `json:"accepted"`
	Rejected    int `json:"rejected"`
	Deferred    int `json:"deferred"`
	Implemented int `json:"implemented"`
	RolledBack  int `json:"rolled_back"`
}

type EvaluationSummary struct {
	Improved     int `json:"improved"`
	Neutral      int `json:"neutral"`
	Regressed    int `json:"regressed"`
	Insufficient int `json:"insufficient"`
}

type IndependenceSummary struct {
	Evaluations    int `json:"evaluations"`
	SelfEvaluation int `json:"self_evaluation"`
	Violations     int `json:"violations"`
}

type MonthlyReport struct {
	SchemaVersion       int                  `json:"schema_version"`
	ReportKind          string               `json:"report_kind"`
	ReportVersion       string               `json:"report_version"`
	Windows             []Window             `json:"windows"`
	InputSHA256         string               `json:"input_sha256"`
	Alternatives        []AlternativeSummary `json:"alternatives"`
	ProposalFunnel      ProposalFunnel       `json:"proposal_funnel"`
	Evaluations         EvaluationSummary    `json:"evaluations"`
	Independence        IndependenceSummary  `json:"independence"`
	RecommendationCodes []string             `json:"recommendation_codes"`
	MayMutatePolicy     bool                 `json:"may_mutate_policy"`
}

func BuildWeekly(records []Record, window Window) (WeeklyReport, error) {
	if err := window.Validate(); err != nil {
		return WeeklyReport{}, err
	}
	if len(records) == 0 {
		return WeeklyReport{}, errors.New("weekly report requires evidence")
	}
	digest, err := InputDigest(records)
	if err != nil {
		return WeeklyReport{}, err
	}
	report := WeeklyReport{SchemaVersion: SchemaVersion, ReportKind: "weekly_operational", ReportVersion: "weekly-v1", Window: window, InputSHA256: digest, MayMutatePolicy: false}
	var selection []Record
	for _, record := range records {
		if record.WindowID != window.ID {
			return WeeklyReport{}, errors.New("weekly report cannot mix windows")
		}
		if record.RecordedAt.Before(window.Start) || record.RecordedAt.After(window.End) {
			return WeeklyReport{}, errors.New("evidence is outside report window")
		}
		switch record.Kind {
		case KindHealth:
			accumulateHealth(&report.Health, *record.Health)
		case KindSelection:
			selection = append(selection, record)
		case KindProposal, KindAcceptance, KindEvaluation, KindAlternative:
		default:
			return WeeklyReport{}, errors.New("unsupported evidence kind")
		}
	}
	report.Selection = aggregateSelection(selection)
	report.Integrity = IntegrityScorecard{InputRecords: len(records), AcceptedRecords: len(records)}
	report.RecommendationCodes = weeklyRecommendations(report)
	return report, nil
}

func BuildMonthly(records []Record, windows []Window) (MonthlyReport, error) {
	if len(windows) == 0 || len(records) == 0 {
		return MonthlyReport{}, errors.New("monthly report requires windows and evidence")
	}
	for _, window := range windows {
		if err := window.Validate(); err != nil {
			return MonthlyReport{}, err
		}
	}
	allowed := map[string]bool{}
	for _, window := range windows {
		allowed[window.ID] = true
	}
	digest, err := InputDigest(records)
	if err != nil {
		return MonthlyReport{}, err
	}
	report := MonthlyReport{SchemaVersion: SchemaVersion, ReportKind: "monthly_structural", ReportVersion: "monthly-v1", Windows: append([]Window(nil), windows...), InputSHA256: digest, MayMutatePolicy: false}
	alt := map[AlternativeID][]Record{}
	for _, record := range records {
		if !allowed[record.WindowID] {
			return MonthlyReport{}, errors.New("monthly evidence references an unknown window")
		}
		switch record.Kind {
		case KindAlternative:
			alt[record.Alternative.AlternativeID] = append(alt[record.Alternative.AlternativeID], record)
		case KindProposal:
			report.ProposalFunnel.Authored++
			addProposalStatus(&report.ProposalFunnel, record.Proposal.Status)
		case KindAcceptance:
			addDecision(&report.ProposalFunnel, record.Acceptance.Decision)
		case KindEvaluation:
			addEvaluation(&report.Evaluations, &report.Independence, record.Evaluation)
		}
	}
	for _, id := range []AlternativeID{AlternativeBaseline, AlternativeCandidateA, AlternativeCandidateB, AlternativeCandidateC} {
		if recordsFor := alt[id]; len(recordsFor) > 0 {
			report.Alternatives = append(report.Alternatives, summarizeAlternative(id, recordsFor))
		}
	}
	sort.Slice(report.Alternatives, func(i, j int) bool {
		return report.Alternatives[i].AlternativeID < report.Alternatives[j].AlternativeID
	})
	if report.Independence.Violations > 0 {
		report.RecommendationCodes = append(report.RecommendationCodes, "review_evaluator_independence")
	}
	if len(report.Alternatives) < 2 {
		report.RecommendationCodes = append(report.RecommendationCodes, "insufficient_alternatives")
	}
	if len(report.RecommendationCodes) == 0 {
		report.RecommendationCodes = []string{"hold_current_posture"}
	}
	sort.Strings(report.RecommendationCodes)
	return report, nil
}

func accumulateHealth(score *HealthScorecard, evidence HealthEvidence) {
	score.Records++
	switch evidence.Freshness {
	case FreshnessCurrent:
		score.Current++
	case FreshnessAging:
		score.Aging++
	case FreshnessStale:
		score.Stale++
	case FreshnessMissed:
		score.Missed++
	case FreshnessUnavailable:
		score.Unavailable++
	}
	switch evidence.Recovery {
	case RecoveryRecovered:
		score.Recovered++
	case RecoveryFailed:
		score.RecoveryFailed++
	case RecoveryBlocked:
		score.RecoveryBlocked++
	}
}

func aggregateSelection(records []Record) SelectionScorecard {
	score := SelectionScorecard{Records: len(records)}
	counts := map[activationpolicy.Route]*RouteScore{}
	for _, route := range []activationpolicy.Route{activationpolicy.D0Direct, activationpolicy.D1Targeted, activationpolicy.D2Governed, activationpolicy.Blocked} {
		counts[route] = &RouteScore{Route: route}
	}
	for _, record := range records {
		e := record.Selection
		route := counts[e.Route]
		route.Count++
		route.MeanDurationSec += e.DurationSeconds
		if e.BudgetExhausted {
			route.BudgetExhausted++
		}
		if e.HumanOverride {
			route.HumanOverrides++
		}
		if e.PACoverage == PACoverageCovered {
			route.PACovered++
		}
		if e.PACoverage == PACoverageMissing {
			score.MissingPACoverage++
		}
		if e.PACoverage == PACoverageUnavailable {
			score.UnavailablePA++
		}
		score.CapabilityGapCount += len(e.CapabilityGaps)
		switch e.Outcome {
		case OutcomeSucceeded:
			score.Completed++
		case OutcomeFailed:
			score.Failed++
		case OutcomeBlocked:
			score.Blocked++
		}
	}
	for _, route := range []activationpolicy.Route{activationpolicy.D0Direct, activationpolicy.D1Targeted, activationpolicy.D2Governed, activationpolicy.Blocked} {
		item := *counts[route]
		if item.Count > 0 {
			item.MeanDurationSec /= item.Count
		}
		if score.Records > 0 {
			item.BasisPoints = item.Count * 10000 / score.Records
		}
		score.Routes = append(score.Routes, item)
	}
	return score
}

func weeklyRecommendations(report WeeklyReport) []string {
	var codes []string
	if report.Selection.Records < 20 {
		codes = append(codes, "insufficient_sample")
	}
	if report.Health.Missed > 0 || report.Health.RecoveryFailed > 0 {
		codes = append(codes, "review_recovery")
	}
	if report.Selection.MissingPACoverage+report.Selection.UnavailablePA > 0 {
		codes = append(codes, "review_pa_coverage")
	}
	if report.Selection.CapabilityGapCount > 0 {
		codes = append(codes, "review_capability_gaps")
	}
	if len(codes) == 0 {
		codes = append(codes, "hold_current_posture")
	}
	sort.Strings(codes)
	return codes
}

func summarizeAlternative(id AlternativeID, records []Record) AlternativeSummary {
	result := AlternativeSummary{AlternativeID: id, Records: len(records)}
	counts := map[Outcome]int{}
	for _, record := range records {
		e := record.Alternative
		result.MeanDurationSec += e.DurationSeconds
		if e.BudgetExhausted {
			result.BudgetExhausted++
		}
		if e.PACoverage == PACoverageUnavailable {
			result.PAUnavailable++
		}
		counts[e.Outcome]++
	}
	if result.Records > 0 {
		result.MeanDurationSec /= result.Records
	}
	for _, outcome := range []Outcome{OutcomeSucceeded, OutcomeFailed, OutcomeBlocked, OutcomeUnavailable, OutcomeNoAction, OutcomePartial} {
		if counts[outcome] > 0 {
			result.OutcomeCounts = append(result.OutcomeCounts, OutcomeCount{Outcome: outcome, Count: counts[outcome]})
		}
	}
	return result
}

func addProposalStatus(funnel *ProposalFunnel, status ProposalStatus) {
	switch status {
	case ProposalAccepted:
		funnel.Accepted++
	case ProposalRejected:
		funnel.Rejected++
	case ProposalDeferred:
		funnel.Deferred++
	case ProposalImplemented:
		funnel.Implemented++
	case ProposalRolledBack:
		funnel.RolledBack++
	}
}
func addDecision(funnel *ProposalFunnel, decision Decision) {
	switch decision {
	case DecisionAccepted:
		funnel.Accepted++
	case DecisionRejected:
		funnel.Rejected++
	case DecisionDeferred:
		funnel.Deferred++
	}
}
func addEvaluation(summary *EvaluationSummary, independence *IndependenceSummary, evidence *EvaluationEvidence) {
	independence.Evaluations++
	if evidence.SelfEvaluation {
		independence.SelfEvaluation++
		independence.Violations++
	}
	switch evidence.Outcome {
	case EvaluationImproved:
		summary.Improved++
	case EvaluationNeutral:
		summary.Neutral++
	case EvaluationRegressed:
		summary.Regressed++
	case EvaluationInsufficient:
		summary.Insufficient++
	}
}

func (r WeeklyReport) Validate() error {
	if r.SchemaVersion != SchemaVersion || r.ReportKind != "weekly_operational" || r.ReportVersion != "weekly-v1" || !digestPattern.MatchString(r.InputSHA256) || r.MayMutatePolicy {
		return errors.New("invalid weekly report")
	}
	return r.Window.Validate()
}

func (r MonthlyReport) Validate() error {
	if r.SchemaVersion != SchemaVersion || r.ReportKind != "monthly_structural" || r.ReportVersion != "monthly-v1" || !digestPattern.MatchString(r.InputSHA256) || r.MayMutatePolicy || len(r.Windows) == 0 {
		return errors.New("invalid monthly report")
	}
	for _, window := range r.Windows {
		if err := window.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func (r WeeklyReport) String() string { return fmt.Sprintf("%s/%s", r.ReportKind, r.Window.ID) }
