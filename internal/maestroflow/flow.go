// Package maestroflow models the canonical, metadata-only Maestro delivery
// state machine. Spokes never call one another: every transition is mediated
// by Maestro and only one spoke is active at a time.
package maestroflow

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	SchemaVersion    = 1
	DirectCaseReason = "simple_bounded_reversible_no_stakeholder_or_strategy"
	WalterSkipReason = "low_leverage_ordinary_reversible_no_external_artifact"
)

type EntryPath string

const (
	AccountFirst EntryPath = "account_first"
	CaseDirect   EntryPath = "case_direct"
)

type AccountSignal string

const (
	SignalClientStrategy       AccountSignal = "client_strategy"
	SignalRelationshipPosition AccountSignal = "relationship_positioning"
	SignalStakeholderPressure  AccountSignal = "stakeholder_pressure_test"
	SignalClientMaterial       AccountSignal = "client_material"
	SignalCrossCaseContext     AccountSignal = "cross_case_context"
	SignalPromotionCandidate   AccountSignal = "promotion_candidate"
	SignalExecutionOnly        AccountSignal = "execution_only"
)

type LeverageSignal string

const (
	SignalConsequentialDecision   LeverageSignal = "consequential_decision"
	SignalExecutiveRecommendation LeverageSignal = "executive_recommendation"
	SignalImportantTradeoff       LeverageSignal = "important_tradeoff"
	SignalExternalArtifact        LeverageSignal = "external_artifact"
	SignalReputationalRisk        LeverageSignal = "reputational_risk"
	SignalHardToReverse           LeverageSignal = "hard_to_reverse"
	SignalOrdinaryReversible      LeverageSignal = "ordinary_reversible"
)

type Phase string

const (
	PhaseStarted           Phase = "started"
	PhaseAccountBriefing   Phase = "account_briefing"
	PhaseCaseExecution     Phase = "case_execution"
	PhaseAccountValidation Phase = "account_validation"
	PhaseMaestroReturn     Phase = "maestro_return"
	PhaseWalterReview      Phase = "walter_review"
	PhaseRefinement        Phase = "refinement_requested"
	PhaseDelivered         Phase = "delivered"
)

type AccountVerdict string

const (
	AccountApproved AccountVerdict = "approved"
	AccountRefine   AccountVerdict = "refine"
)

type WalterVerdict string

const (
	WalterApproved WalterVerdict = "approved"
	WalterRefine   WalterVerdict = "refine"
	WalterHold     WalterVerdict = "hold"
)

const (
	RefinementClarity        = "clarity"
	RefinementNarrative      = "narrative"
	RefinementRecommendation = "recommendation"
	RefinementTradeoff       = "tradeoff"
	RefinementReadiness      = "readiness"
	RefinementGovernance     = "governance_blocker"
)

type Request struct {
	SchemaVersion               int             `json:"schema_version"`
	AttemptID                   string          `json:"attempt_id"`
	AttemptSHA256               string          `json:"attempt_sha256"`
	EntryPath                   EntryPath       `json:"entry_path"`
	AccountConsultationRequired bool            `json:"account_consultation_required"`
	AccountSignals              []AccountSignal `json:"account_signals,omitempty"`
	ReasonCode                  string          `json:"reason_code,omitempty"`
	WalterRequired              bool            `json:"walter_required"`
	WalterSkipReason            string          `json:"walter_skip_reason,omitempty"`
	WalterSkipEvidenceSHA256    string          `json:"walter_skip_evidence_sha256,omitempty"`
	StartedAt                   time.Time       `json:"started_at"`
}

type State struct {
	SchemaVersion                 int             `json:"schema_version"`
	AttemptID                     string          `json:"attempt_id"`
	AttemptSHA256                 string          `json:"attempt_sha256"`
	EntryPath                     EntryPath       `json:"entry_path"`
	Phase                         Phase           `json:"phase"`
	Cycle                         int             `json:"cycle"`
	PreAccountUsed                bool            `json:"pre_account_used"`
	AccountConsultationRequired   bool            `json:"account_consultation_required"`
	AccountSignals                []AccountSignal `json:"account_signals,omitempty"`
	PostAccountValidationRequired bool            `json:"post_account_validation_required"`
	PostAccountValidated          bool            `json:"post_account_validated"`
	WalterGate                    bool            `json:"walter_gate"`
	WalterRequired                bool            `json:"walter_required"`
	WalterSkipped                 bool            `json:"walter_skipped"`
	WalterSkipReason              string          `json:"walter_skip_reason,omitempty"`
	WalterSkipEvidenceSHA256      string          `json:"walter_skip_evidence_sha256,omitempty"`
	RefinementLoadBearing         bool            `json:"refinement_load_bearing"`
	RefinementActionable          bool            `json:"refinement_actionable"`
	RefinementKind                string          `json:"refinement_kind,omitempty"`
	AccountVerdict                AccountVerdict  `json:"account_verdict,omitempty"`
	WalterVerdict                 WalterVerdict   `json:"walter_verdict,omitempty"`
	AccountValidationCount        int             `json:"account_validation_count"`
	WalterReviewCount             int             `json:"walter_review_count"`
	BudgetExhausted               bool            `json:"budget_exhausted"`
}

type Receipt struct {
	SchemaVersion                 int             `json:"schema_version"`
	AttemptID                     string          `json:"attempt_id"`
	AttemptSHA256                 string          `json:"attempt_sha256"`
	Event                         string          `json:"event"`
	Phase                         Phase           `json:"phase"`
	Cycle                         int             `json:"cycle"`
	PreAccountUsed                bool            `json:"pre_account_used"`
	AccountConsultationRequired   bool            `json:"account_consultation_required"`
	AccountSignals                []AccountSignal `json:"account_signals,omitempty"`
	PostAccountValidationRequired bool            `json:"post_account_validation_required"`
	PostAccountValidated          bool            `json:"post_account_validated"`
	WalterGate                    bool            `json:"walter_gate"`
	WalterRequired                bool            `json:"walter_required"`
	WalterSkipped                 bool            `json:"walter_skipped"`
	WalterSkipReason              string          `json:"walter_skip_reason,omitempty"`
	WalterSkipEvidenceSHA256      string          `json:"walter_skip_evidence_sha256,omitempty"`
	RefinementLoadBearing         bool            `json:"refinement_load_bearing"`
	RefinementActionable          bool            `json:"refinement_actionable"`
	RefinementKind                string          `json:"refinement_kind,omitempty"`
	AccountVerdict                AccountVerdict  `json:"account_verdict,omitempty"`
	WalterVerdict                 WalterVerdict   `json:"walter_verdict,omitempty"`
	Invalidated                   bool            `json:"invalidated_after_mutation"`
	BudgetExhausted               bool            `json:"budget_exhausted"`
	At                            time.Time       `json:"at"`
}

func NewRequest(attemptID, material string, path EntryPath, reason string, walterRequired bool, walterSkipReason, walterSkipEvidence string, started time.Time) (Request, error) {
	signals := []AccountSignal{SignalExecutionOnly}
	if path == AccountFirst {
		signals = []AccountSignal{SignalClientStrategy}
	}
	return NewDecisionRequest(attemptID, material, path, path == AccountFirst, signals, reason, walterRequired, walterSkipReason, walterSkipEvidence, started)
}

func NewDecisionRequest(attemptID, material string, path EntryPath, accountConsultationRequired bool, signals []AccountSignal, reason string, walterRequired bool, walterSkipReason, walterSkipEvidence string, started time.Time) (Request, error) {
	if strings.TrimSpace(material) == "" {
		return Request{}, errors.New("flow material is required")
	}
	digest := sha256.Sum256([]byte(material))
	request := Request{SchemaVersion: SchemaVersion, AttemptID: attemptID, AttemptSHA256: hex.EncodeToString(digest[:]), EntryPath: path, AccountConsultationRequired: accountConsultationRequired, AccountSignals: append([]AccountSignal(nil), signals...), ReasonCode: reason, WalterRequired: walterRequired, WalterSkipReason: walterSkipReason, WalterSkipEvidenceSHA256: walterSkipEvidence, StartedAt: started.UTC()}
	if err := request.Validate(); err != nil {
		return Request{}, err
	}
	return request, nil
}

func (request Request) Validate() error {
	if request.SchemaVersion != SchemaVersion || strings.TrimSpace(request.AttemptID) == "" || len(request.AttemptSHA256) != 64 || request.StartedAt.IsZero() {
		return errors.New("invalid Maestro flow request metadata")
	}
	if request.EntryPath != AccountFirst && request.EntryPath != CaseDirect {
		return errors.New("invalid Maestro flow entry path")
	}
	if request.EntryPath == CaseDirect && request.ReasonCode != DirectCaseReason {
		return errors.New("direct Case entry requires the bounded reversible reason code")
	}
	if request.EntryPath == AccountFirst && request.ReasonCode != "" {
		return errors.New("account-first entry cannot carry a direct-case reason code")
	}
	if request.AccountConsultationRequired != (request.EntryPath == AccountFirst) {
		return errors.New("account consultation decision does not match the mediated path")
	}
	consultation, err := ResolveAccountConsultation(request.AccountSignals)
	if err != nil || consultation != request.AccountConsultationRequired {
		return errors.New("account consultation decision is not bound to strategic or stakeholder signals")
	}
	if request.WalterRequired {
		if request.WalterSkipReason != "" || request.WalterSkipEvidenceSHA256 != "" {
			return errors.New("Walter-required decision cannot carry a skip")
		}
	} else if request.WalterSkipReason != WalterSkipReason || len(request.WalterSkipEvidenceSHA256) != 64 {
		return errors.New("Walter skip requires the bounded low-materiality reason and evidence digest")
	}
	return nil
}

// ResolveAccountConsultation is a signal decision, never a task-size or
// complexity heuristic. Empty evidence is fail-safe and consults Account.
func ResolveAccountConsultation(signals []AccountSignal) (bool, error) {
	if len(signals) == 0 {
		return true, nil
	}
	known := map[AccountSignal]bool{SignalClientStrategy: true, SignalRelationshipPosition: true, SignalStakeholderPressure: true, SignalClientMaterial: true, SignalCrossCaseContext: true, SignalPromotionCandidate: true, SignalExecutionOnly: true}
	seen := map[AccountSignal]bool{}
	consult := false
	for _, signal := range signals {
		if !known[signal] || seen[signal] {
			return false, errors.New("unknown or duplicate account consultation signal")
		}
		seen[signal] = true
		if signal != SignalExecutionOnly {
			consult = true
		}
	}
	return consult, nil
}

// ResolveWalterRequirement uses high-leverage signals, not technical size.
// The resulting review is intentionally calm and load-bearing: the resolver
// decides whether to consult Walter; it does not prescribe a dramatic verdict.
func ResolveWalterRequirement(signals []LeverageSignal) (bool, error) {
	if len(signals) == 0 {
		return true, nil
	}
	known := map[LeverageSignal]bool{SignalConsequentialDecision: true, SignalExecutiveRecommendation: true, SignalImportantTradeoff: true, SignalExternalArtifact: true, SignalReputationalRisk: true, SignalHardToReverse: true, SignalOrdinaryReversible: true}
	seen := map[LeverageSignal]bool{}
	for _, signal := range signals {
		if !known[signal] || seen[signal] {
			return false, errors.New("unknown or duplicate Walter leverage signal")
		}
		seen[signal] = true
		if signal != SignalOrdinaryReversible {
			return true, nil
		}
	}
	return false, nil
}

func Start(request Request) (State, Receipt, error) {
	if err := request.Validate(); err != nil {
		return State{}, Receipt{}, err
	}
	phase := PhaseCaseExecution
	if request.EntryPath == AccountFirst {
		phase = PhaseAccountBriefing
	}
	preAccount := request.EntryPath == AccountFirst
	state := State{SchemaVersion: SchemaVersion, AttemptID: request.AttemptID, AttemptSHA256: request.AttemptSHA256, EntryPath: request.EntryPath, Phase: phase, PreAccountUsed: preAccount, AccountConsultationRequired: request.AccountConsultationRequired, AccountSignals: append([]AccountSignal(nil), request.AccountSignals...), PostAccountValidationRequired: preAccount, WalterRequired: request.WalterRequired, WalterSkipReason: request.WalterSkipReason, WalterSkipEvidenceSHA256: request.WalterSkipEvidenceSHA256}
	return state, state.receipt("started", false, time.Now().UTC()), nil
}

func (state State) Validate() error {
	if state.SchemaVersion != SchemaVersion || strings.TrimSpace(state.AttemptID) == "" || len(state.AttemptSHA256) != 64 || state.Cycle < 0 || state.AccountValidationCount < 0 || state.WalterReviewCount < 0 {
		return errors.New("invalid Maestro flow state metadata")
	}
	if state.EntryPath != AccountFirst && state.EntryPath != CaseDirect {
		return errors.New("invalid Maestro flow entry path")
	}
	if state.PostAccountValidated && state.AccountValidationCount == 0 {
		return errors.New("post-account validation flag has no validation attempt")
	}
	if state.PostAccountValidationRequired != state.PreAccountUsed {
		return errors.New("post-account validation requirement is asymmetric with framing")
	}
	if state.AccountConsultationRequired != state.PreAccountUsed {
		return errors.New("account consultation flag is asymmetric with the mediated path")
	}
	if !state.PostAccountValidationRequired && state.PostAccountValidated {
		return errors.New("direct Case path cannot carry account validation")
	}
	if state.WalterGate && state.WalterReviewCount == 0 {
		return errors.New("Walter gate has no review attempt")
	}
	if state.WalterRequired && state.WalterSkipped {
		return errors.New("Walter-required flow cannot be skipped")
	}
	if !state.WalterRequired && !state.WalterSkipped && state.WalterGate {
		return errors.New("Walter gate is inconsistent with a skipped flow")
	}
	if state.Phase == PhaseDelivered && ((state.PostAccountValidationRequired && !state.PostAccountValidated) || (state.WalterRequired && (!state.WalterGate || state.WalterVerdict != WalterApproved)) || (!state.WalterRequired && !state.WalterSkipped)) {
		return errors.New("material delivery requires the applicable account and Walter decision")
	}
	return nil
}

func (state State) CompleteAccountBriefing(now time.Time) (State, Receipt, error) {
	if state.Phase != PhaseAccountBriefing {
		return state, Receipt{}, errors.New("account briefing is not active")
	}
	state.Phase = PhaseCaseExecution
	return state, state.receipt("account_briefing_complete", false, now), nil
}

func (state State) CompleteCase(now time.Time) (State, Receipt, error) {
	if state.Phase != PhaseCaseExecution {
		return state, Receipt{}, errors.New("Case execution is not active")
	}
	state.PostAccountValidated = false
	state.WalterGate = false
	state.AccountVerdict = ""
	state.WalterVerdict = ""
	if state.PostAccountValidationRequired {
		state.Phase = PhaseAccountValidation
	} else {
		state.Phase = PhaseMaestroReturn
	}
	return state, state.receipt("case_complete", true, now), nil
}

func (state State) ValidateAccount(verdict AccountVerdict, now time.Time) (State, Receipt, error) {
	if state.Phase != PhaseAccountValidation || (verdict != AccountApproved && verdict != AccountRefine) {
		return state, Receipt{}, errors.New("invalid Client Account validation transition")
	}
	state.AccountValidationCount++
	state.AccountVerdict = verdict
	if verdict == AccountRefine {
		state.Phase = PhaseRefinement
		state.PostAccountValidated = false
		state.WalterGate = false
		receipt := state.receipt("account_refine", true, now)
		state.AccountVerdict = ""
		return state, receipt, nil
	}
	state.Phase = PhaseMaestroReturn
	state.PostAccountValidated = true
	return state, state.receipt("account_approved", false, now), nil
}

func (state State) OpenWalter(now time.Time) (State, Receipt, error) {
	if state.Phase != PhaseMaestroReturn || !state.WalterRequired || (state.PostAccountValidationRequired && !state.PostAccountValidated) {
		return state, Receipt{}, errors.New("Walter review requires the applicable post-Case gate")
	}
	state.Phase = PhaseWalterReview
	state.WalterGate = true
	state.WalterSkipped = false
	state.WalterReviewCount++
	return state, state.receipt("walter_opened", false, now), nil
}

func (state State) ApplyWalter(verdict WalterVerdict, now time.Time) (State, Receipt, error) {
	if state.Phase != PhaseWalterReview || !state.WalterRequired || !state.WalterGate || (verdict != WalterApproved && verdict != WalterRefine && verdict != WalterHold) {
		return state, Receipt{}, errors.New("invalid Walter review transition")
	}
	if verdict == WalterRefine && (!state.RefinementLoadBearing || !state.RefinementActionable || !validRefinementKind(state.RefinementKind) || state.RefinementKind == RefinementGovernance) {
		return state, Receipt{}, errors.New("Walter refinement requires a load-bearing actionable improvement")
	}
	if verdict == WalterHold && state.RefinementKind != RefinementGovernance {
		return state, Receipt{}, errors.New("Walter hold requires an exceptional governance blocker")
	}
	state.WalterVerdict = verdict
	if verdict == WalterHold {
		state.Phase = PhaseRefinement
		state.PostAccountValidated = false
		state.WalterGate = false
		return state, state.receipt("walter_hold", true, now), nil
	}
	if verdict == WalterRefine {
		state.Phase = PhaseRefinement
		state.PostAccountValidated = false
		state.WalterGate = false
		receipt := state.receipt("walter_refine", true, now)
		state.WalterVerdict = ""
		return state, receipt, nil
	}
	state.Phase = PhaseMaestroReturn
	return state, state.receipt("walter_approved", false, now), nil
}

func (state State) WithWalterRefinement(kind string) (State, error) {
	if !validRefinementKind(kind) || kind == RefinementGovernance {
		return state, errors.New("Walter refinement kind is not a constructive improvement")
	}
	state.RefinementLoadBearing = true
	state.RefinementActionable = true
	state.RefinementKind = kind
	return state, nil
}

func (state State) SkipWalter(now time.Time) (State, Receipt, error) {
	if state.Phase != PhaseMaestroReturn || state.WalterRequired || state.WalterSkipReason != WalterSkipReason || len(state.WalterSkipEvidenceSHA256) != 64 {
		return state, Receipt{}, errors.New("Walter skip is not an auditable low-materiality Maestro decision")
	}
	state.WalterSkipped = true
	return state, state.receipt("walter_skipped", false, now), nil
}

func (state State) EscalateMateriality(now time.Time) (State, Receipt, error) {
	if state.Phase != PhaseMaestroReturn || state.WalterRequired || !state.WalterSkipped {
		return state, Receipt{}, errors.New("materiality escalation requires a previously recorded Walter skip")
	}
	state.WalterRequired = true
	state.WalterSkipped = false
	state.WalterSkipReason = ""
	state.WalterSkipEvidenceSHA256 = ""
	return state, state.receipt("walter_skip_invalidated_materiality_escalated", true, now), nil
}

func (state State) Reenter(now time.Time) (State, Receipt, error) {
	if state.Phase != PhaseRefinement {
		return state, Receipt{}, errors.New("flow is not awaiting refinement re-entry")
	}
	state.Cycle++
	state.Phase = PhaseCaseExecution
	state.PostAccountValidated = false
	state.WalterGate = false
	state.WalterSkipped = false
	state.AccountVerdict = ""
	state.WalterVerdict = ""
	return state, state.receipt("reentered_case", true, now), nil
}

func (state State) Deliver(now time.Time) (State, Receipt, error) {
	if state.Phase != PhaseMaestroReturn || (state.PostAccountValidationRequired && !state.PostAccountValidated) || (state.WalterRequired && (!state.WalterGate || state.WalterVerdict != WalterApproved)) || (!state.WalterRequired && !state.WalterSkipped) {
		return state, Receipt{}, errors.New("delivery requires the applicable account and Walter decision")
	}
	state.Phase = PhaseDelivered
	return state, state.receipt("material_delivered", false, now), nil
}

func (state State) MarkBudgetExhausted(now time.Time) (State, Receipt, error) {
	state.BudgetExhausted = true
	return state, state.receipt("budget_exhausted", false, now), nil
}

func (state State) receipt(event string, invalidated bool, at time.Time) Receipt {
	return Receipt{SchemaVersion: SchemaVersion, AttemptID: state.AttemptID, AttemptSHA256: state.AttemptSHA256, Event: event, Phase: state.Phase, Cycle: state.Cycle, PreAccountUsed: state.PreAccountUsed, AccountConsultationRequired: state.AccountConsultationRequired, AccountSignals: append([]AccountSignal(nil), state.AccountSignals...), PostAccountValidationRequired: state.PostAccountValidationRequired, PostAccountValidated: state.PostAccountValidated, WalterGate: state.WalterGate, WalterRequired: state.WalterRequired, WalterSkipped: state.WalterSkipped, WalterSkipReason: state.WalterSkipReason, WalterSkipEvidenceSHA256: state.WalterSkipEvidenceSHA256, RefinementLoadBearing: state.RefinementLoadBearing, RefinementActionable: state.RefinementActionable, RefinementKind: state.RefinementKind, AccountVerdict: state.AccountVerdict, WalterVerdict: state.WalterVerdict, Invalidated: invalidated, BudgetExhausted: state.BudgetExhausted, At: at.UTC()}
}

func (receipt Receipt) Validate() error {
	if receipt.SchemaVersion != SchemaVersion || strings.TrimSpace(receipt.AttemptID) == "" || len(receipt.AttemptSHA256) != 64 || strings.TrimSpace(receipt.Event) == "" || receipt.At.IsZero() || receipt.Cycle < 0 {
		return errors.New("invalid Maestro flow receipt")
	}
	if receipt.PostAccountValidationRequired != receipt.PreAccountUsed {
		return errors.New("receipt account requirement is asymmetric with framing")
	}
	if receipt.AccountConsultationRequired != receipt.PreAccountUsed {
		return errors.New("receipt account consultation is asymmetric with the mediated path")
	}
	if receipt.Event == "walter_refine" && (!receipt.RefinementLoadBearing || !receipt.RefinementActionable || !validRefinementKind(receipt.RefinementKind) || receipt.RefinementKind == RefinementGovernance) {
		return errors.New("Walter refinement receipt is not actionable")
	}
	if receipt.Event == "walter_hold" && receipt.RefinementKind != RefinementGovernance {
		return errors.New("Walter hold receipt lacks a governance blocker")
	}
	if receipt.WalterRequired && receipt.WalterSkipped {
		return errors.New("Walter-required receipt cannot be skipped")
	}
	if receipt.Event == "material_delivered" && ((receipt.PostAccountValidationRequired && !receipt.PostAccountValidated) || (receipt.WalterRequired && (!receipt.WalterGate || receipt.WalterVerdict != WalterApproved)) || (!receipt.WalterRequired && !receipt.WalterSkipped)) {
		return errors.New("material delivery receipt is missing an applicable gate")
	}
	return nil
}

func (path EntryPath) String() string { return fmt.Sprint(string(path)) }

func validRefinementKind(kind string) bool {
	switch kind {
	case RefinementClarity, RefinementNarrative, RefinementRecommendation, RefinementTradeoff, RefinementReadiness, RefinementGovernance:
		return true
	default:
		return false
	}
}
