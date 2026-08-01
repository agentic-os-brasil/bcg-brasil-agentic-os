// Package darwinobservability owns the local, metadata-only evidence contract
// used to evaluate Darwin's operational health and selection calibration.
// It deliberately has no network, federation, prompt, output or client-data
// dependency.
package darwinobservability

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/activationpolicy"
)

const (
	SchemaVersion   = 1
	MaxInputRecords = 10000
	MaxWindows      = 64
	MaxCalls        = 6
	MaxTokenUnits   = 24000
)

type EvidenceAuthority string

const (
	// AuthorityCallerAssertedShadow is deliberately the only authority in this
	// slice. Native provenance must be introduced by a separate qualified
	// adapter and schema version.
	AuthorityCallerAssertedShadow EvidenceAuthority = "caller_asserted_shadow"
)

type Kind string

const (
	KindHealth      Kind = "health"
	KindSelection   Kind = "selection"
	KindProposal    Kind = "proposal"
	KindAcceptance  Kind = "acceptance"
	KindEvaluation  Kind = "evaluation"
	KindAlternative Kind = "alternative"
	KindFlow        Kind = "maestro_flow"
)

type JobKind string

const (
	JobDarwinHousekeeping JobKind = "darwin_housekeeping"
	JobFederationWeekly   JobKind = "federation_weekly"
	JobActivationMonitor  JobKind = "activation_monitor"
)

type Freshness string

const (
	FreshnessCurrent     Freshness = "current"
	FreshnessAging       Freshness = "aging"
	FreshnessStale       Freshness = "stale"
	FreshnessMissed      Freshness = "missed"
	FreshnessUnavailable Freshness = "unavailable"
)

type Recovery string

const (
	RecoveryNotNeeded Recovery = "not_needed"
	RecoveryRecovered Recovery = "recovered"
	RecoveryFailed    Recovery = "failed"
	RecoveryBlocked   Recovery = "blocked"
)

type Outcome string

const (
	OutcomeSucceeded   Outcome = "succeeded"
	OutcomeFailed      Outcome = "failed"
	OutcomeBlocked     Outcome = "blocked"
	OutcomeUnavailable Outcome = "unavailable"
	OutcomeNoAction    Outcome = "no_action"
	OutcomePartial     Outcome = "partial"
)

type OverrideKind string

const (
	OverrideNone     OverrideKind = "none"
	OverrideRoute    OverrideKind = "route_change"
	OverrideBudget   OverrideKind = "budget_extension"
	OverrideResume   OverrideKind = "manual_resume"
	OverrideRecovery OverrideKind = "release_recovery"
)

type PACoverage string

const (
	PACoverageNotRequired PACoverage = "not_required"
	PACoverageCovered     PACoverage = "covered"
	PACoverageMissing     PACoverage = "missing"
	PACoverageUnavailable PACoverage = "unavailable"
)

type CapabilityGap string

const (
	GapNativeRuntime     CapabilityGap = "native_runtime"
	GapReceiptCoverage   CapabilityGap = "receipt_coverage"
	GapPAExpertCoverage  CapabilityGap = "pa_expert_coverage"
	GapRecovery          CapabilityGap = "recovery"
	GapBudget            CapabilityGap = "budget"
	GapLatency           CapabilityGap = "latency"
	GapHumanConfirmation CapabilityGap = "human_confirmation"
)

type ProposalKind string

const (
	ProposalPolicyCalibration ProposalKind = "policy_calibration"
	ProposalCapability        ProposalKind = "capability_improvement"
	ProposalReliability       ProposalKind = "reliability_fix"
	ProposalSkillPattern      ProposalKind = "skill_pattern"
)

type ProposalStatus string

const (
	ProposalDraft       ProposalStatus = "draft"
	ProposalAccepted    ProposalStatus = "accepted"
	ProposalRejected    ProposalStatus = "rejected"
	ProposalDeferred    ProposalStatus = "deferred"
	ProposalImplemented ProposalStatus = "implemented"
	ProposalRolledBack  ProposalStatus = "rolled_back"
)

type Decision string

const (
	DecisionAccepted Decision = "accepted"
	DecisionRejected Decision = "rejected"
	DecisionDeferred Decision = "deferred"
)

type EvaluationOutcome string

const (
	EvaluationImproved     EvaluationOutcome = "improved"
	EvaluationNeutral      EvaluationOutcome = "neutral"
	EvaluationRegressed    EvaluationOutcome = "regressed"
	EvaluationInsufficient EvaluationOutcome = "insufficient"
)

type AlternativeID string

const (
	AlternativeBaseline   AlternativeID = "baseline"
	AlternativeCandidateA AlternativeID = "candidate_a"
	AlternativeCandidateB AlternativeID = "candidate_b"
	AlternativeCandidateC AlternativeID = "candidate_c"
)

// Record is a closed union. Exactly one payload must be populated according
// to Kind; incompatible payloads are rejected by Validate.
type Record struct {
	SchemaVersion int                  `json:"schema_version"`
	Kind          Kind                 `json:"kind"`
	EvidenceID    string               `json:"evidence_id"`
	WindowID      string               `json:"window_id"`
	ScopeSHA256   string               `json:"scope_sha256"`
	Authority     EvidenceAuthority    `json:"evidence_authority"`
	RecordedAt    time.Time            `json:"recorded_at"`
	Health        *HealthEvidence      `json:"health,omitempty"`
	Selection     *SelectionEvidence   `json:"selection,omitempty"`
	Proposal      *ProposalEvidence    `json:"proposal,omitempty"`
	Acceptance    *AcceptanceEvidence  `json:"acceptance,omitempty"`
	Evaluation    *EvaluationEvidence  `json:"evaluation,omitempty"`
	Alternative   *AlternativeEvidence `json:"alternative,omitempty"`
	Flow          *FlowEvidence        `json:"flow,omitempty"`
}

type HealthEvidence struct {
	JobKind     JobKind   `json:"job_kind"`
	ScheduledAt time.Time `json:"scheduled_at"`
	CapturedAt  time.Time `json:"captured_at"`
	Freshness   Freshness `json:"freshness"`
	Recovery    Recovery  `json:"recovery"`
	Outcome     Outcome   `json:"outcome"`
}

type SelectionEvidence struct {
	PlanSHA256      string                   `json:"plan_sha256"`
	PolicyVersion   string                   `json:"policy_version"`
	Posture         activationpolicy.Posture `json:"posture"`
	Route           activationpolicy.Route   `json:"route"`
	Outcome         Outcome                  `json:"outcome"`
	DurationSeconds int                      `json:"duration_seconds"`
	MaxCalls        int                      `json:"max_calls"`
	CallsUsed       int                      `json:"calls_used"`
	MaxTokenUnits   int                      `json:"max_token_units"`
	TokenUnitsUsed  int                      `json:"token_units_used"`
	BudgetExhausted bool                     `json:"budget_exhausted"`
	HumanOverride   bool                     `json:"human_override"`
	OverrideKind    OverrideKind             `json:"override_kind"`
	PACoverage      PACoverage               `json:"pa_coverage"`
	PAExpertCount   int                      `json:"pa_expert_count"`
	CapabilityGaps  []CapabilityGap          `json:"capability_gaps"`
}

type ProposalEvidence struct {
	ProposalSHA256 string         `json:"proposal_sha256"`
	ProposalKind   ProposalKind   `json:"proposal_kind"`
	Status         ProposalStatus `json:"status"`
	AuthorRole     string         `json:"author_role"`
}

type AcceptanceEvidence struct {
	ProposalSHA256 string   `json:"proposal_sha256"`
	Decision       Decision `json:"decision"`
	ActorRole      string   `json:"actor_role"`
}

type EvaluationEvidence struct {
	ProposalSHA256     string            `json:"proposal_sha256"`
	BaselineWindowID   string            `json:"baseline_window_id"`
	PostChangeWindowID string            `json:"post_change_window_id"`
	ChangeSHA256       string            `json:"change_sha256"`
	EvaluatorRole      string            `json:"evaluator_role"`
	Outcome            EvaluationOutcome `json:"outcome"`
	SelfEvaluation     bool              `json:"self_evaluation"`
}

type AlternativeEvidence struct {
	CohortSHA256       string                   `json:"cohort_sha256"`
	AlternativeID      AlternativeID            `json:"alternative_id"`
	PolicyVersion      string                   `json:"policy_version"`
	Posture            activationpolicy.Posture `json:"posture"`
	Route              activationpolicy.Route   `json:"route"`
	Outcome            Outcome                  `json:"outcome"`
	DurationSeconds    int                      `json:"duration_seconds"`
	BudgetExhausted    bool                     `json:"budget_exhausted"`
	PACoverage         PACoverage               `json:"pa_coverage"`
	CapabilityGapCount int                      `json:"capability_gap_count"`
}

// FlowEvidence is the metadata-only Darwin view of one canonical Maestro
// attempt. It contains decisions, digests, counters and violations only.
type FlowEvidence struct {
	AttemptID                         string   `json:"attempt_id"`
	AttemptSHA256                     string   `json:"attempt_sha256"`
	EntryPath                         string   `json:"entry_path"`
	PreAccountUsed                    bool     `json:"pre_account_used"`
	AccountConsultationRequired       bool     `json:"account_consultation_required"`
	AccountSignals                    []string `json:"account_signals,omitempty"`
	PostAccountValidationRequired     bool     `json:"post_account_validation_required"`
	PostAccountValidated              bool     `json:"post_account_validated"`
	WalterRequired                    bool     `json:"walter_required"`
	WalterGate                        bool     `json:"walter_gate"`
	WalterSkipped                     bool     `json:"walter_skipped"`
	WalterSkipReason                  string   `json:"walter_skip_reason,omitempty"`
	WalterSkipEvidenceSHA256          string   `json:"walter_skip_evidence_sha256,omitempty"`
	RefinementLoadBearing             bool     `json:"refinement_load_bearing"`
	RefinementActionable              bool     `json:"refinement_actionable"`
	RefinementKind                    string   `json:"refinement_kind,omitempty"`
	AccountVerdict                    string   `json:"account_verdict,omitempty"`
	WalterVerdict                     string   `json:"walter_verdict,omitempty"`
	AccountValidationCount            int      `json:"account_validation_count"`
	WalterReviewCount                 int      `json:"walter_review_count"`
	Cycles                            int      `json:"cycles"`
	BudgetExhausted                   bool     `json:"budget_exhausted"`
	MaterialityEscalations            int      `json:"materiality_escalations"`
	InvalidationsAfterMutation        int      `json:"invalidations_after_mutation"`
	ReturnToWalterWithoutAccountCheck int      `json:"return_to_walter_without_account_validation"`
	MaterialFinishWithoutWalter       int      `json:"material_finish_without_walter"`
	WalterUsefulRefinements           int      `json:"walter_useful_refinements"`
	WalterNitpickBlocks               int      `json:"walter_nitpick_blocks"`
	OneActiveSpokeViolations          int      `json:"one_active_spoke_violations"`
	NestingViolations                 int      `json:"nesting_violations"`
	DirectAgentCalls                  int      `json:"direct_agent_calls"`
	MaterialDelivered                 bool     `json:"material_delivered"`
}

var (
	identifierPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._:-]{0,63}$`)
	windowPattern     = regexp.MustCompile(`^win-[a-f0-9]{32}$`)
	digestPattern     = regexp.MustCompile(`^[a-f0-9]{64}$`)
	policyPattern     = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)
)

func (record Record) Validate() error {
	if record.SchemaVersion != SchemaVersion || !identifierPattern.MatchString(record.EvidenceID) ||
		!windowPattern.MatchString(record.WindowID) || !digestPattern.MatchString(record.ScopeSHA256) ||
		record.Authority != AuthorityCallerAssertedShadow || record.RecordedAt.IsZero() {
		return errors.New("invalid Darwin evidence header")
	}
	count := 0
	if record.Health != nil {
		count++
	}
	if record.Selection != nil {
		count++
	}
	if record.Proposal != nil {
		count++
	}
	if record.Acceptance != nil {
		count++
	}
	if record.Evaluation != nil {
		count++
	}
	if record.Alternative != nil {
		count++
	}
	if record.Flow != nil {
		count++
	}
	if count != 1 {
		return errors.New("Darwin evidence must contain exactly one payload")
	}
	var err error
	switch record.Kind {
	case KindHealth:
		if record.Health == nil {
			return errors.New("health evidence payload is missing")
		}
		err = record.Health.validate()
		if err == nil && !record.RecordedAt.Equal(record.Health.CapturedAt) {
			err = errors.New("health evidence time does not match capture time")
		}
	case KindSelection:
		if record.Selection == nil {
			return errors.New("selection evidence payload is missing")
		}
		err = record.Selection.validate()
	case KindProposal:
		if record.Proposal == nil {
			return errors.New("proposal evidence payload is missing")
		}
		err = record.Proposal.validate()
	case KindAcceptance:
		if record.Acceptance == nil {
			return errors.New("acceptance evidence payload is missing")
		}
		err = record.Acceptance.validate()
	case KindEvaluation:
		if record.Evaluation == nil {
			return errors.New("evaluation evidence payload is missing")
		}
		err = record.Evaluation.validate()
	case KindAlternative:
		if record.Alternative == nil {
			return errors.New("alternative evidence payload is missing")
		}
		err = record.Alternative.validate()
	case KindFlow:
		if record.Flow == nil {
			return errors.New("Maestro flow evidence payload is missing")
		}
		err = record.Flow.validate()
	default:
		return errors.New("unknown Darwin evidence kind")
	}
	if err != nil {
		return err
	}
	if record.EvidenceID != canonicalEvidenceID(record) {
		return errors.New("Darwin evidence ID does not bind the complete record")
	}
	return nil
}

func (e HealthEvidence) validate() error {
	if !validJobKind[e.JobKind] || e.ScheduledAt.IsZero() || e.CapturedAt.IsZero() || e.CapturedAt.Before(e.ScheduledAt) ||
		!validFreshness[e.Freshness] || !validRecovery[e.Recovery] || !validOutcome[e.Outcome] {
		return errors.New("invalid health evidence")
	}
	if e.Freshness == FreshnessMissed && e.Recovery == RecoveryNotNeeded {
		return errors.New("missed health evidence requires a recovery result")
	}
	switch e.Recovery {
	case RecoveryNotNeeded:
		if e.Freshness == FreshnessMissed || e.Outcome == OutcomeFailed || e.Outcome == OutcomeBlocked ||
			e.Outcome == OutcomeUnavailable || e.Outcome == OutcomePartial {
			return errors.New("health evidence incorrectly marks recovery as unnecessary")
		}
	case RecoveryRecovered:
		if e.Freshness != FreshnessMissed || (e.Outcome != OutcomeSucceeded && e.Outcome != OutcomeNoAction) {
			return errors.New("recovered health evidence requires a successful missed occurrence")
		}
	case RecoveryFailed:
		if e.Outcome != OutcomeFailed && e.Outcome != OutcomePartial {
			return errors.New("failed recovery is incompatible with health outcome")
		}
	case RecoveryBlocked:
		if e.Outcome != OutcomeBlocked && e.Outcome != OutcomeUnavailable {
			return errors.New("blocked recovery is incompatible with health outcome")
		}
	}
	return nil
}

func (e SelectionEvidence) validate() error {
	if !digestPattern.MatchString(e.PlanSHA256) || !policyPattern.MatchString(e.PolicyVersion) ||
		!validPosture[e.Posture] || !validRoute[e.Route] || !validOutcome[e.Outcome] ||
		e.DurationSeconds < 0 || e.DurationSeconds > 86400 || e.MaxCalls < 0 || e.MaxCalls > MaxCalls ||
		e.CallsUsed < 0 || e.CallsUsed > e.MaxCalls || e.MaxTokenUnits < 0 || e.MaxTokenUnits > MaxTokenUnits ||
		e.TokenUnitsUsed < 0 || e.TokenUnitsUsed > e.MaxTokenUnits || e.PAExpertCount < 0 || e.PAExpertCount > 2 ||
		!validOverride[e.OverrideKind] || !validCoverage[e.PACoverage] || len(e.CapabilityGaps) > 8 {
		return errors.New("invalid selection evidence")
	}
	if !e.HumanOverride && e.OverrideKind != OverrideNone {
		return errors.New("override kind requires a human override")
	}
	if e.HumanOverride && e.OverrideKind == OverrideNone {
		return errors.New("human override requires an override kind")
	}
	seen := map[CapabilityGap]bool{}
	for _, gap := range e.CapabilityGaps {
		if !validGap[gap] || seen[gap] {
			return errors.New("invalid or duplicate capability gap")
		}
		seen[gap] = true
	}
	if (e.PACoverage == PACoverageNotRequired || e.PACoverage == PACoverageMissing || e.PACoverage == PACoverageUnavailable) &&
		e.PAExpertCount != 0 {
		return errors.New("PA expert count is incompatible with coverage")
	}
	if e.PACoverage == PACoverageCovered && e.PAExpertCount == 0 {
		return errors.New("covered PA evidence requires an expert")
	}
	if e.Route == activationpolicy.D0Direct && (e.PACoverage != PACoverageNotRequired || e.PAExpertCount != 0) {
		return errors.New("direct route cannot claim PA expert coverage")
	}
	if e.Route == activationpolicy.D1Targeted && e.PAExpertCount > 1 {
		return errors.New("targeted route cannot claim more than one PA expert")
	}
	expectedCalls, expectedTokens := routeBudget(e.Route)
	if e.MaxCalls != expectedCalls || e.MaxTokenUnits != expectedTokens {
		return errors.New("selection budget does not match the route policy")
	}
	return nil
}

func (e ProposalEvidence) validate() error {
	if !digestPattern.MatchString(e.ProposalSHA256) || !validProposalKind[e.ProposalKind] || !validProposalStatus[e.Status] || e.AuthorRole != "darwin" {
		return errors.New("invalid proposal evidence")
	}
	return nil
}

func (e AcceptanceEvidence) validate() error {
	if !digestPattern.MatchString(e.ProposalSHA256) || !validDecision[e.Decision] || e.ActorRole != "human_maintainer" {
		return errors.New("invalid acceptance evidence")
	}
	return nil
}

func (e EvaluationEvidence) validate() error {
	if !digestPattern.MatchString(e.ProposalSHA256) || !windowPattern.MatchString(e.BaselineWindowID) ||
		!windowPattern.MatchString(e.PostChangeWindowID) || !digestPattern.MatchString(e.ChangeSHA256) ||
		e.EvaluatorRole != "independent_evaluator" || e.SelfEvaluation || !validEvaluation[e.Outcome] || e.BaselineWindowID == e.PostChangeWindowID {
		return errors.New("invalid or non-independent evaluation evidence")
	}
	return nil
}

func (e AlternativeEvidence) validate() error {
	if !digestPattern.MatchString(e.CohortSHA256) || !validAlternative[e.AlternativeID] ||
		!policyPattern.MatchString(e.PolicyVersion) || !validPosture[e.Posture] ||
		!validRoute[e.Route] || !validOutcome[e.Outcome] || e.DurationSeconds < 0 || e.DurationSeconds > 86400 ||
		!validCoverage[e.PACoverage] || e.CapabilityGapCount < 0 || e.CapabilityGapCount > 8 {
		return errors.New("invalid alternative evidence")
	}
	return nil
}

func (e FlowEvidence) validate() error {
	if !identifierPattern.MatchString(e.AttemptID) || !digestPattern.MatchString(e.AttemptSHA256) ||
		(e.EntryPath != "account_first" && e.EntryPath != "case_direct") ||
		e.PostAccountValidationRequired != e.PreAccountUsed || e.AccountValidationCount < 0 ||
		e.WalterReviewCount < 0 || e.Cycles < 0 || e.MaterialityEscalations < 0 ||
		e.InvalidationsAfterMutation < 0 || e.ReturnToWalterWithoutAccountCheck < 0 ||
		e.MaterialFinishWithoutWalter < 0 || e.OneActiveSpokeViolations < 0 ||
		e.NestingViolations < 0 || e.DirectAgentCalls < 0 || e.WalterUsefulRefinements < 0 || e.WalterNitpickBlocks < 0 {
		return errors.New("invalid Maestro flow evidence")
	}
	if e.AccountConsultationRequired != e.PreAccountUsed {
		return errors.New("flow evidence account consultation is asymmetric with entry")
	}
	for _, signal := range e.AccountSignals {
		switch signal {
		case "client_strategy", "relationship_positioning", "stakeholder_pressure_test", "client_material", "cross_case_context", "promotion_candidate", "execution_only":
		default:
			return errors.New("invalid account consultation signal")
		}
	}
	if e.PostAccountValidated && (!e.PostAccountValidationRequired || e.AccountValidationCount == 0) {
		return errors.New("flow evidence has an account validation without framing")
	}
	if e.WalterSkipped {
		if e.WalterRequired || e.WalterGate || e.WalterSkipReason != "low_leverage_ordinary_reversible_no_external_artifact" || !digestPattern.MatchString(e.WalterSkipEvidenceSHA256) {
			return errors.New("flow evidence contains an invalid Walter skip")
		}
	}
	if e.WalterRequired && e.WalterSkipped {
		return errors.New("Walter-required flow cannot be skipped")
	}
	if e.MaterialDelivered && ((e.PostAccountValidationRequired && !e.PostAccountValidated) || (e.WalterRequired && !e.WalterGate) || (!e.WalterRequired && !e.WalterSkipped)) {
		return errors.New("material delivery is missing an applicable gate")
	}
	if e.MaterialFinishWithoutWalter > 0 && (e.WalterRequired || !e.WalterSkipped) {
		return errors.New("material finish without Walter is inconsistent")
	}
	return nil
}

var (
	validJobKind        = map[JobKind]bool{JobDarwinHousekeeping: true, JobFederationWeekly: true, JobActivationMonitor: true}
	validFreshness      = map[Freshness]bool{FreshnessCurrent: true, FreshnessAging: true, FreshnessStale: true, FreshnessMissed: true, FreshnessUnavailable: true}
	validRecovery       = map[Recovery]bool{RecoveryNotNeeded: true, RecoveryRecovered: true, RecoveryFailed: true, RecoveryBlocked: true}
	validOutcome        = map[Outcome]bool{OutcomeSucceeded: true, OutcomeFailed: true, OutcomeBlocked: true, OutcomeUnavailable: true, OutcomeNoAction: true, OutcomePartial: true}
	validPosture        = map[activationpolicy.Posture]bool{activationpolicy.Direct: true, activationpolicy.Balanced: true, activationpolicy.Deliberative: true}
	validRoute          = map[activationpolicy.Route]bool{activationpolicy.D0Direct: true, activationpolicy.D1Targeted: true, activationpolicy.D2Governed: true, activationpolicy.Blocked: true}
	validOverride       = map[OverrideKind]bool{OverrideNone: true, OverrideRoute: true, OverrideBudget: true, OverrideResume: true, OverrideRecovery: true}
	validCoverage       = map[PACoverage]bool{PACoverageNotRequired: true, PACoverageCovered: true, PACoverageMissing: true, PACoverageUnavailable: true}
	validGap            = map[CapabilityGap]bool{GapNativeRuntime: true, GapReceiptCoverage: true, GapPAExpertCoverage: true, GapRecovery: true, GapBudget: true, GapLatency: true, GapHumanConfirmation: true}
	validProposalKind   = map[ProposalKind]bool{ProposalPolicyCalibration: true, ProposalCapability: true, ProposalReliability: true, ProposalSkillPattern: true}
	validProposalStatus = map[ProposalStatus]bool{ProposalDraft: true, ProposalAccepted: true, ProposalRejected: true, ProposalDeferred: true, ProposalImplemented: true, ProposalRolledBack: true}
	validDecision       = map[Decision]bool{DecisionAccepted: true, DecisionRejected: true, DecisionDeferred: true}
	validEvaluation     = map[EvaluationOutcome]bool{EvaluationImproved: true, EvaluationNeutral: true, EvaluationRegressed: true, EvaluationInsufficient: true}
	validAlternative    = map[AlternativeID]bool{AlternativeBaseline: true, AlternativeCandidateA: true, AlternativeCandidateB: true, AlternativeCandidateC: true}
)

// DecodeStrict rejects unknown fields, duplicate keys and trailing JSON.
func DecodeStrict(body []byte, target any) error {
	if err := rejectDuplicateKeys(body); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}

func rejectDuplicateKeys(body []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	var walk func(json.Token) error
	walk = func(token json.Token) error {
		delim, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		if delim == '{' {
			seen := map[string]bool{}
			for decoder.More() {
				keyToken, err := decoder.Token()
				if err != nil {
					return err
				}
				key, ok := keyToken.(string)
				if !ok {
					return errors.New("invalid JSON object key")
				}
				if seen[key] {
					return fmt.Errorf("duplicate JSON object key %q", key)
				}
				seen[key] = true
				value, err := decoder.Token()
				if err != nil {
					return err
				}
				if err := walk(value); err != nil {
					return err
				}
			}
			_, err := decoder.Token()
			return err
		}
		if delim == '[' {
			for decoder.More() {
				value, err := decoder.Token()
				if err != nil {
					return err
				}
				if err := walk(value); err != nil {
					return err
				}
			}
			_, err := decoder.Token()
			return err
		}
		return nil
	}
	first, err := decoder.Token()
	if err != nil {
		return err
	}
	if err := walk(first); err != nil {
		return err
	}
	return nil
}

func SHA256Hex(body []byte) string {
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:])
}

// OpaqueWindowID converts a local window label into the only representation
// permitted to cross the observability boundary.
func OpaqueWindowID(localID string) string {
	if strings.TrimSpace(localID) == "" {
		return ""
	}
	digest := SHA256Hex([]byte(localID))
	return "win-" + digest[:32]
}

func routeBudget(route activationpolicy.Route) (int, int) {
	switch route {
	case activationpolicy.D0Direct:
		return 1, 4000
	case activationpolicy.D1Targeted:
		return 3, 10000
	case activationpolicy.D2Governed:
		return 6, 24000
	case activationpolicy.Blocked:
		return 0, 0
	default:
		return -1, -1
	}
}

func EvidenceID(kind Kind, windowID string, value []byte) string {
	digest := SHA256Hex(append([]byte(string(kind)+"\x00"+windowID+"\x00"), value...))
	return "ev-" + digest[:32]
}

func canonicalEvidenceID(record Record) string {
	copy := record
	copy.EvidenceID = ""
	body, _ := json.Marshal(copy)
	return EvidenceID(record.Kind, record.WindowID, body)
}

// BindEvidenceID finalizes a caller-constructed closed record and rejects it
// unless the complete content satisfies the contract.
func BindEvidenceID(record Record) (Record, error) {
	record.EvidenceID = canonicalEvidenceID(record)
	if err := record.Validate(); err != nil {
		return Record{}, err
	}
	return record, nil
}

func InputDigest(records []Record) (string, error) {
	if len(records) == 0 || len(records) > MaxInputRecords {
		return "", errors.New("evidence input is empty or exceeds the bounded limit")
	}
	canonical := make([][]byte, len(records))
	seen := map[string]bool{}
	for i, record := range records {
		if err := record.Validate(); err != nil {
			return "", err
		}
		if seen[record.EvidenceID] {
			return "", errors.New("duplicate evidence ID")
		}
		seen[record.EvidenceID] = true
		body, err := json.Marshal(record)
		if err != nil {
			return "", err
		}
		canonical[i] = body
	}
	sort.Slice(canonical, func(i, j int) bool { return string(canonical[i]) < string(canonical[j]) })
	return SHA256Hex(bytes.Join(canonical, []byte{'\n'})), nil
}

func normalizeString(value string) string { return strings.ToLower(strings.TrimSpace(value)) }
