package darwinobservability

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/activationpolicy"
)

type Window struct {
	ID          string    `json:"window_id"`
	ScopeSHA256 string    `json:"scope_sha256"`
	Start       time.Time `json:"start"`
	End         time.Time `json:"end"`
}

func (window Window) Validate() error {
	if !windowPattern.MatchString(window.ID) || !digestPattern.MatchString(window.ScopeSHA256) ||
		window.Start.IsZero() || window.End.IsZero() || !window.End.After(window.Start) {
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

type FlowReasonCount struct {
	Reason string `json:"reason"`
	Count  int    `json:"count"`
}

type FlowScorecard struct {
	Records                     int               `json:"records"`
	AccountFirst                int               `json:"account_first"`
	DirectCase                  int               `json:"direct_case"`
	StrategicSignalRecords      int               `json:"strategic_signal_records"`
	ExecutionOnlyRecords        int               `json:"execution_only_records"`
	AccountSelectionMismatches  int               `json:"account_selection_mismatches"`
	AccountUnderRouting         int               `json:"account_under_routing"`
	AccountOverRouting          int               `json:"account_over_routing"`
	WalterRequired              int               `json:"walter_required"`
	WalterSkipped               int               `json:"walter_skipped"`
	BudgetExhausted             int               `json:"budget_exhausted"`
	InvalidationsAfterMutation  int               `json:"invalidations_after_mutation"`
	MaterialityEscalations      int               `json:"materiality_escalations"`
	MaterialFinishWithoutWalter int               `json:"material_finish_without_walter"`
	WalterUsefulRefinements     int               `json:"walter_useful_refinements"`
	WalterNitpickBlocks         int               `json:"walter_nitpick_blocks"`
	OneActiveSpokeViolations    int               `json:"one_active_spoke_violations"`
	NestingViolations           int               `json:"nesting_violations"`
	DirectAgentCalls            int               `json:"direct_agent_calls"`
	WalterSkipReasons           []FlowReasonCount `json:"walter_skip_reasons"`
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
	EvidenceAuthority   EvidenceAuthority  `json:"evidence_authority"`
	Health              HealthScorecard    `json:"health"`
	Selection           SelectionScorecard `json:"selection"`
	Flow                FlowScorecard      `json:"maestro_flow"`
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
	EvidenceAuthority   EvidenceAuthority    `json:"evidence_authority"`
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
	if len(records) == 0 || len(records) > MaxInputRecords {
		return WeeklyReport{}, errors.New("weekly report requires evidence")
	}
	digest, err := reportInputDigest(records, []Window{window})
	if err != nil {
		return WeeklyReport{}, err
	}
	report := WeeklyReport{
		SchemaVersion: SchemaVersion, ReportKind: "weekly_operational", ReportVersion: "weekly-v1",
		Window: window, InputSHA256: digest, EvidenceAuthority: AuthorityCallerAssertedShadow, MayMutatePolicy: false,
	}
	var selection []Record
	var flow []Record
	for _, record := range records {
		if record.WindowID != window.ID || record.ScopeSHA256 != window.ScopeSHA256 ||
			record.Authority != AuthorityCallerAssertedShadow {
			return WeeklyReport{}, errors.New("weekly report cannot mix windows, scopes or authorities")
		}
		if record.RecordedAt.Before(window.Start) || !record.RecordedAt.Before(window.End) {
			return WeeklyReport{}, errors.New("evidence is outside report window")
		}
		switch record.Kind {
		case KindHealth:
			accumulateHealth(&report.Health, *record.Health)
		case KindSelection:
			selection = append(selection, record)
		case KindFlow:
			flow = append(flow, record)
		case KindProposal, KindAcceptance, KindEvaluation, KindAlternative:
			return WeeklyReport{}, errors.New("weekly report accepts only health and selection evidence")
		default:
			return WeeklyReport{}, errors.New("unsupported evidence kind")
		}
	}
	report.Selection = aggregateSelection(selection)
	report.Flow = aggregateFlow(flow)
	report.Integrity = IntegrityScorecard{InputRecords: len(records), AcceptedRecords: len(records)}
	report.RecommendationCodes = weeklyRecommendations(report)
	return report, report.Validate()
}

func BuildMonthly(records []Record, windows []Window) (MonthlyReport, error) {
	if len(windows) == 0 || len(windows) > MaxWindows || len(records) == 0 || len(records) > MaxInputRecords {
		return MonthlyReport{}, errors.New("monthly report requires windows and evidence")
	}
	normalizedWindows, windowByID, err := normalizeWindows(windows)
	if err != nil {
		return MonthlyReport{}, err
	}
	digest, err := reportInputDigest(records, normalizedWindows)
	if err != nil {
		return MonthlyReport{}, err
	}
	report := MonthlyReport{
		SchemaVersion: SchemaVersion, ReportKind: "monthly_structural", ReportVersion: "monthly-v1",
		Windows: normalizedWindows, InputSHA256: digest, EvidenceAuthority: AuthorityCallerAssertedShadow, MayMutatePolicy: false,
		RecommendationCodes: []string{"review_evidence_authority"},
	}
	alt := map[AlternativeID][]Record{}
	alternativeCohorts := map[string]map[AlternativeID]bool{}
	proposals := map[string]*ProposalEvidence{}
	acceptances := map[string]*AcceptanceEvidence{}
	evaluations := map[string]*EvaluationEvidence{}
	for _, record := range records {
		window, ok := windowByID[record.WindowID]
		if !ok || record.ScopeSHA256 != normalizedWindows[0].ScopeSHA256 ||
			record.Authority != AuthorityCallerAssertedShadow {
			return MonthlyReport{}, errors.New("monthly evidence references an unknown window, scope or authority")
		}
		if record.RecordedAt.Before(window.Start) || !record.RecordedAt.Before(window.End) {
			return MonthlyReport{}, errors.New("monthly evidence is outside its referenced window")
		}
		switch record.Kind {
		case KindAlternative:
			cohort := record.Alternative.CohortSHA256
			if alternativeCohorts[cohort] == nil {
				alternativeCohorts[cohort] = map[AlternativeID]bool{}
			}
			if alternativeCohorts[cohort][record.Alternative.AlternativeID] {
				return MonthlyReport{}, errors.New("duplicate alternative in comparison cohort")
			}
			alternativeCohorts[cohort][record.Alternative.AlternativeID] = true
			alt[record.Alternative.AlternativeID] = append(alt[record.Alternative.AlternativeID], record)
		case KindProposal:
			if proposals[record.Proposal.ProposalSHA256] != nil {
				return MonthlyReport{}, errors.New("duplicate proposal evidence")
			}
			proposals[record.Proposal.ProposalSHA256] = record.Proposal
			report.ProposalFunnel.Authored++
			addProposalStatus(&report.ProposalFunnel, record.Proposal.Status)
		case KindAcceptance:
			if acceptances[record.Acceptance.ProposalSHA256] != nil {
				return MonthlyReport{}, errors.New("duplicate acceptance evidence")
			}
			acceptances[record.Acceptance.ProposalSHA256] = record.Acceptance
		case KindEvaluation:
			if record.WindowID != record.Evaluation.PostChangeWindowID {
				return MonthlyReport{}, errors.New("evaluation record must belong to its post-change window")
			}
			if evaluations[record.Evaluation.ProposalSHA256] != nil {
				return MonthlyReport{}, errors.New("duplicate evaluation evidence")
			}
			evaluations[record.Evaluation.ProposalSHA256] = record.Evaluation
		case KindHealth, KindSelection:
			return MonthlyReport{}, errors.New("monthly report accepts only structural evidence")
		default:
			return MonthlyReport{}, errors.New("unsupported monthly evidence kind")
		}
	}
	if err := validateAlternativeCohorts(alternativeCohorts); err != nil {
		return MonthlyReport{}, err
	}
	if err := aggregateProposalLifecycle(&report, proposals, acceptances, evaluations, windowByID); err != nil {
		return MonthlyReport{}, err
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
	sort.Strings(report.RecommendationCodes)
	return report, report.Validate()
}

func normalizeWindows(windows []Window) ([]Window, map[string]Window, error) {
	normalized := append([]Window(nil), windows...)
	sort.Slice(normalized, func(i, j int) bool {
		if normalized[i].Start.Equal(normalized[j].Start) {
			return normalized[i].ID < normalized[j].ID
		}
		return normalized[i].Start.Before(normalized[j].Start)
	})
	byID := make(map[string]Window, len(normalized))
	var scope string
	for i, window := range normalized {
		if err := window.Validate(); err != nil {
			return nil, nil, err
		}
		if _, exists := byID[window.ID]; exists {
			return nil, nil, errors.New("duplicate report window")
		}
		if i == 0 {
			scope = window.ScopeSHA256
		} else {
			if window.ScopeSHA256 != scope {
				return nil, nil, errors.New("monthly report cannot mix scopes")
			}
			if window.Start.Before(normalized[i-1].End) {
				return nil, nil, errors.New("monthly report windows cannot overlap")
			}
		}
		byID[window.ID] = window
	}
	return normalized, byID, nil
}

func reportInputDigest(records []Record, windows []Window) (string, error) {
	recordDigest, err := InputDigest(records)
	if err != nil {
		return "", err
	}
	windowBody, err := json.Marshal(windows)
	if err != nil {
		return "", err
	}
	return SHA256Hex(bytes.Join([][]byte{[]byte(recordDigest), windowBody}, []byte{'\n'})), nil
}

func validateAlternativeCohorts(cohorts map[string]map[AlternativeID]bool) error {
	if len(cohorts) == 0 {
		return nil
	}
	var expected map[AlternativeID]bool
	for _, alternatives := range cohorts {
		if !alternatives[AlternativeBaseline] || len(alternatives) < 2 {
			return errors.New("alternative cohort requires baseline and candidate")
		}
		if expected == nil {
			expected = alternatives
			continue
		}
		if len(alternatives) != len(expected) {
			return errors.New("alternative cohorts must contain the same candidates")
		}
		for id := range expected {
			if !alternatives[id] {
				return errors.New("alternative cohorts must contain the same candidates")
			}
		}
	}
	return nil
}

func aggregateProposalLifecycle(
	report *MonthlyReport,
	proposals map[string]*ProposalEvidence,
	acceptances map[string]*AcceptanceEvidence,
	evaluations map[string]*EvaluationEvidence,
	windows map[string]Window,
) error {
	for proposalSHA, acceptance := range acceptances {
		proposal := proposals[proposalSHA]
		if proposal == nil {
			return errors.New("acceptance references an unknown proposal")
		}
		if !proposalDecisionConsistent(proposal.Status, acceptance.Decision) {
			return errors.New("proposal status and human decision are inconsistent")
		}
		addDecision(&report.ProposalFunnel, acceptance.Decision)
	}
	for proposalSHA, proposal := range proposals {
		acceptance := acceptances[proposalSHA]
		switch proposal.Status {
		case ProposalAccepted, ProposalRejected, ProposalDeferred, ProposalImplemented, ProposalRolledBack:
			if acceptance == nil {
				return errors.New("proposal lifecycle state requires human acceptance evidence")
			}
		}
		if (proposal.Status == ProposalImplemented || proposal.Status == ProposalRolledBack) &&
			acceptance.Decision != DecisionAccepted {
			return errors.New("implementation lifecycle requires an accepted proposal")
		}
	}
	for proposalSHA, evaluation := range evaluations {
		proposal := proposals[proposalSHA]
		acceptance := acceptances[proposalSHA]
		if proposal == nil || acceptance == nil || acceptance.Decision != DecisionAccepted ||
			(proposal.Status != ProposalImplemented && proposal.Status != ProposalRolledBack) {
			return errors.New("evaluation requires an accepted implemented proposal")
		}
		baseline, baselineOK := windows[evaluation.BaselineWindowID]
		postChange, postChangeOK := windows[evaluation.PostChangeWindowID]
		if !baselineOK || !postChangeOK || baseline.Start.After(postChange.Start) ||
			baseline.ID == postChange.ID {
			return errors.New("evaluation references invalid baseline or post-change windows")
		}
		addEvaluation(&report.Evaluations, &report.Independence, evaluation)
	}
	return nil
}

func proposalDecisionConsistent(status ProposalStatus, decision Decision) bool {
	switch status {
	case ProposalAccepted, ProposalImplemented, ProposalRolledBack:
		return decision == DecisionAccepted
	case ProposalRejected:
		return decision == DecisionRejected
	case ProposalDeferred:
		return decision == DecisionDeferred
	case ProposalDraft:
		return false
	default:
		return false
	}
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

func aggregateFlow(records []Record) FlowScorecard {
	score := FlowScorecard{Records: len(records), WalterSkipReasons: []FlowReasonCount{}}
	reasons := map[string]int{}
	for _, record := range records {
		e := record.Flow
		if e.PreAccountUsed {
			score.AccountFirst++
		} else {
			score.DirectCase++
		}
		expectedAccount := len(e.AccountSignals) == 0
		for _, signal := range e.AccountSignals {
			if signal == "execution_only" {
				score.ExecutionOnlyRecords++
			} else {
				score.StrategicSignalRecords++
				expectedAccount = true
			}
		}
		if expectedAccount != e.PreAccountUsed {
			score.AccountSelectionMismatches++
			if expectedAccount {
				score.AccountUnderRouting++
			} else {
				score.AccountOverRouting++
			}
		}
		if e.WalterRequired {
			score.WalterRequired++
		}
		if e.WalterSkipped {
			score.WalterSkipped++
			reasons[e.WalterSkipReason]++
		}
		if e.BudgetExhausted {
			score.BudgetExhausted++
		}
		score.InvalidationsAfterMutation += e.InvalidationsAfterMutation
		score.MaterialityEscalations += e.MaterialityEscalations
		score.MaterialFinishWithoutWalter += e.MaterialFinishWithoutWalter
		score.WalterUsefulRefinements += e.WalterUsefulRefinements
		score.WalterNitpickBlocks += e.WalterNitpickBlocks
		score.OneActiveSpokeViolations += e.OneActiveSpokeViolations
		score.NestingViolations += e.NestingViolations
		score.DirectAgentCalls += e.DirectAgentCalls
	}
	for reason, count := range reasons {
		score.WalterSkipReasons = append(score.WalterSkipReasons, FlowReasonCount{Reason: reason, Count: count})
	}
	sort.Slice(score.WalterSkipReasons, func(i, j int) bool { return score.WalterSkipReasons[i].Reason < score.WalterSkipReasons[j].Reason })
	return score
}

func weeklyRecommendations(report WeeklyReport) []string {
	codes := []string{"review_evidence_authority"}
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
	if r.SchemaVersion != SchemaVersion || r.ReportKind != "weekly_operational" || r.ReportVersion != "weekly-v1" ||
		!digestPattern.MatchString(r.InputSHA256) || r.EvidenceAuthority != AuthorityCallerAssertedShadow || r.MayMutatePolicy {
		return errors.New("invalid weekly report")
	}
	if err := r.Window.Validate(); err != nil {
		return err
	}
	if err := validateRecommendations(r.RecommendationCodes); err != nil {
		return err
	}
	if err := validateHealthScorecard(r.Health); err != nil {
		return err
	}
	if err := validateSelectionScorecard(r.Selection); err != nil {
		return err
	}
	if err := validateFlowScorecard(r.Flow); err != nil {
		return err
	}
	if r.Integrity.InputRecords < 1 || r.Integrity.InputRecords > MaxInputRecords ||
		r.Integrity.AcceptedRecords != r.Integrity.InputRecords || r.Integrity.DuplicateRecords != 0 ||
		r.Integrity.IndependenceViolations != 0 ||
		r.Health.Records+r.Selection.Records+r.Flow.Records != r.Integrity.AcceptedRecords {
		return errors.New("invalid weekly integrity scorecard")
	}
	return nil
}

func (r MonthlyReport) Validate() error {
	if r.SchemaVersion != SchemaVersion || r.ReportKind != "monthly_structural" || r.ReportVersion != "monthly-v1" ||
		!digestPattern.MatchString(r.InputSHA256) || r.EvidenceAuthority != AuthorityCallerAssertedShadow ||
		r.MayMutatePolicy || len(r.Windows) == 0 || len(r.Windows) > MaxWindows {
		return errors.New("invalid monthly report")
	}
	normalized, _, err := normalizeWindows(r.Windows)
	if err != nil {
		return err
	}
	if !windowsEqual(r.Windows, normalized) {
		return errors.New("monthly report windows are not canonical")
	}
	if err := validateRecommendations(r.RecommendationCodes); err != nil {
		return err
	}
	seenAlternatives := map[AlternativeID]bool{}
	for i, alternative := range r.Alternatives {
		if !validAlternative[alternative.AlternativeID] || alternative.Records < 1 ||
			alternative.MeanDurationSec < 0 || alternative.BudgetExhausted < 0 ||
			alternative.BudgetExhausted > alternative.Records || alternative.PAUnavailable < 0 ||
			alternative.PAUnavailable > alternative.Records || seenAlternatives[alternative.AlternativeID] ||
			(i > 0 && r.Alternatives[i-1].AlternativeID >= alternative.AlternativeID) {
			return errors.New("invalid monthly alternative score")
		}
		seenAlternatives[alternative.AlternativeID] = true
		totalOutcomes := 0
		seenOutcomes := map[Outcome]bool{}
		for _, outcome := range alternative.OutcomeCounts {
			if !validOutcome[outcome.Outcome] || outcome.Count < 1 || seenOutcomes[outcome.Outcome] {
				return errors.New("invalid monthly alternative outcome")
			}
			seenOutcomes[outcome.Outcome] = true
			totalOutcomes += outcome.Count
		}
		if totalOutcomes != alternative.Records {
			return errors.New("monthly alternative outcome count mismatch")
		}
	}
	if !nonnegativeFunnel(r.ProposalFunnel) || !nonnegativeEvaluations(r.Evaluations) ||
		r.Independence.Evaluations < 0 || r.Independence.SelfEvaluation != 0 || r.Independence.Violations != 0 ||
		r.Independence.Evaluations != r.Evaluations.Improved+r.Evaluations.Neutral+r.Evaluations.Regressed+r.Evaluations.Insufficient {
		return errors.New("invalid monthly lifecycle scorecard")
	}
	return nil
}

var validRecommendation = map[string]bool{
	"insufficient_sample": true, "review_recovery": true, "review_pa_coverage": true,
	"review_capability_gaps": true, "review_evaluator_independence": true,
	"review_evidence_authority": true, "insufficient_alternatives": true, "hold_current_posture": true,
}

func validateRecommendations(codes []string) error {
	if len(codes) == 0 || len(codes) > len(validRecommendation) {
		return errors.New("invalid recommendation codes")
	}
	seen := map[string]bool{}
	for i, code := range codes {
		if !validRecommendation[code] || seen[code] || (i > 0 && codes[i-1] >= code) {
			return errors.New("recommendation codes must be closed, unique and sorted")
		}
		seen[code] = true
	}
	return nil
}

func validateHealthScorecard(score HealthScorecard) error {
	values := []int{score.Records, score.Current, score.Aging, score.Stale, score.Missed, score.Unavailable,
		score.Recovered, score.RecoveryFailed, score.RecoveryBlocked}
	for _, value := range values {
		if value < 0 {
			return errors.New("negative health score")
		}
	}
	if score.Current+score.Aging+score.Stale+score.Missed+score.Unavailable != score.Records ||
		score.Recovered+score.RecoveryFailed+score.RecoveryBlocked > score.Records {
		return errors.New("health score totals are inconsistent")
	}
	return nil
}

func validateSelectionScorecard(score SelectionScorecard) error {
	values := []int{score.Records, score.Completed, score.Failed, score.Blocked, score.MissingPACoverage,
		score.UnavailablePA, score.CapabilityGapCount}
	for _, value := range values {
		if value < 0 {
			return errors.New("negative selection score")
		}
	}
	if len(score.Routes) != 4 || score.Completed+score.Failed+score.Blocked > score.Records ||
		score.MissingPACoverage > score.Records || score.UnavailablePA > score.Records {
		return errors.New("selection score totals are inconsistent")
	}
	expectedRoutes := []activationpolicy.Route{activationpolicy.D0Direct, activationpolicy.D1Targeted, activationpolicy.D2Governed, activationpolicy.Blocked}
	totalRoutes := 0
	for i, route := range score.Routes {
		if route.Route != expectedRoutes[i] || route.Count < 0 || route.MeanDurationSec < 0 ||
			route.BudgetExhausted < 0 || route.BudgetExhausted > route.Count ||
			route.HumanOverrides < 0 || route.HumanOverrides > route.Count ||
			route.PACovered < 0 || route.PACovered > route.Count {
			return errors.New("invalid route score")
		}
		expectedBasisPoints := 0
		if score.Records > 0 {
			expectedBasisPoints = route.Count * 10000 / score.Records
		}
		if route.BasisPoints != expectedBasisPoints {
			return errors.New("route basis points are inconsistent")
		}
		totalRoutes += route.Count
	}
	if totalRoutes != score.Records {
		return errors.New("route counts do not match selection records")
	}
	return nil
}

func validateFlowScorecard(score FlowScorecard) error {
	values := []int{score.Records, score.AccountFirst, score.DirectCase, score.StrategicSignalRecords, score.ExecutionOnlyRecords, score.AccountSelectionMismatches, score.AccountUnderRouting, score.AccountOverRouting, score.WalterRequired, score.WalterSkipped, score.BudgetExhausted, score.InvalidationsAfterMutation, score.MaterialityEscalations, score.MaterialFinishWithoutWalter, score.WalterUsefulRefinements, score.WalterNitpickBlocks, score.OneActiveSpokeViolations, score.NestingViolations, score.DirectAgentCalls}
	for _, value := range values {
		if value < 0 {
			return errors.New("negative flow score")
		}
	}
	if score.AccountFirst+score.DirectCase != score.Records || score.WalterSkipped > score.Records || score.WalterRequired+score.WalterSkipped > score.Records {
		return errors.New("flow score totals are inconsistent")
	}
	seen := map[string]bool{}
	for _, reason := range score.WalterSkipReasons {
		if reason.Reason != "low_leverage_ordinary_reversible_no_external_artifact" || reason.Count < 1 || seen[reason.Reason] {
			return errors.New("invalid Walter skip reason score")
		}
		seen[reason.Reason] = true
	}
	return nil
}

func windowsEqual(left, right []Window) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func nonnegativeFunnel(funnel ProposalFunnel) bool {
	return funnel.Authored >= 0 && funnel.Accepted >= 0 && funnel.Rejected >= 0 && funnel.Deferred >= 0 &&
		funnel.Implemented >= 0 && funnel.RolledBack >= 0 &&
		funnel.Accepted+funnel.Rejected+funnel.Deferred <= funnel.Authored &&
		funnel.Implemented+funnel.RolledBack <= funnel.Accepted
}

func nonnegativeEvaluations(summary EvaluationSummary) bool {
	return summary.Improved >= 0 && summary.Neutral >= 0 && summary.Regressed >= 0 && summary.Insufficient >= 0
}

func (r WeeklyReport) String() string { return fmt.Sprintf("%s/%s", r.ReportKind, r.Window.ID) }
