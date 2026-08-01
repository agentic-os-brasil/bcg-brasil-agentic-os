package agentdispatch

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/agentcatalog"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/agentorchestration"
)

// WalterReviewTrigger is the closed set of material situations that enter
// the Maestro-to-Walter gate. Factual lookups and mechanical operations have
// no trigger and remain on the normal completion path.
type WalterReviewTrigger string

const (
	ReviewMaterialRecommendation WalterReviewTrigger = "material_recommendation"
	ReviewConsequentialTradeoff  WalterReviewTrigger = "consequential_tradeoff"
	ReviewExternalArtifact       WalterReviewTrigger = "external_artifact"
)

const WalterReviewPosture = "calm_precise_load_bearing"

// ReviewState is intentionally a small metadata projection. It is safe for a
// status surface and contains no prompt, rationale, client body or objection
// text.
type ReviewState string

const (
	ReviewDispatched   ReviewState = "dispatched"
	ReviewApproved     ReviewState = "approved"
	ReviewRefineReturn ReviewState = "refine-and-return"
	ReviewMissingMark  ReviewState = "missing-the-mark"
	ReviewUnavailable  ReviewState = "unavailable"
	ReviewSkipped      ReviewState = "skipped"
	ReviewHold         ReviewState = "hold"
)

// ReviewChainMode records how the accountable Case output reached Walter.
// All steps are Maestro-issued roots. A direct Case exception is intentionally
// explicit and narrower than the default Account -> Case -> Account flow.
type ReviewChainMode string

const (
	ReviewChainAccountCaseAccount ReviewChainMode = "account_case_account_validation"
	ReviewChainDirectCase         ReviewChainMode = "direct_case"
	DirectCaseReasonSimpleBounded ReviewChainMode = "simple_bounded_reversible"
)

type ReviewChainStep struct {
	Sequence       int    `json:"sequence"`
	AgentID        string `json:"agent_id"`
	Role           string `json:"role"`
	PacketID       string `json:"packet_id"`
	PacketSHA256   string `json:"packet_sha256"`
	IssuerAgentID  string `json:"issuer_agent_id"`
	ParentPacketID string `json:"parent_packet_id,omitempty"`
}

type DirectCaseException struct {
	ReasonCode           string   `json:"reason_code"`
	EvidenceRefs         []string `json:"evidence_refs"`
	Reversible           bool     `json:"reversible"`
	StakeholderImpact    bool     `json:"stakeholder_impact"`
	ClientImpact         bool     `json:"client_impact"`
	StrategyImplication  bool     `json:"strategy_implication"`
	PromotionImplication bool     `json:"promotion_implication"`
}

// ReviewChain is the bounded provenance Walter must see before reviewing a
// material Case output. Packet IDs and digests are evidence of the mediated
// sequence; no agent-to-agent parent is accepted.
type ReviewChain struct {
	Mode                            ReviewChainMode      `json:"mode"`
	AccountConsultationRequired     bool                 `json:"account_consultation_required"`
	PostAccountValidationRequired   bool                 `json:"post_account_validation_required"`
	AccountConsultationReasonCode   string               `json:"account_consultation_reason_code,omitempty"`
	AccountConsultationEvidenceRefs []string             `json:"account_consultation_evidence_refs,omitempty"`
	Steps                           []ReviewChainStep    `json:"steps"`
	ValidatedPacketID               string               `json:"validated_packet_id,omitempty"`
	ValidatedPacketSHA256           string               `json:"validated_packet_sha256,omitempty"`
	DirectCaseException             *DirectCaseException `json:"direct_case_exception,omitempty"`
}

// ReviewPacket is the sealed, bounded input Walter receives. Content remains
// in the ephemeral dispatch body; durable receipts retain only its digests.
type ReviewPacket struct {
	SourcePacketID       string              `json:"source_packet_id"`
	SourcePacketSHA256   string              `json:"source_packet_sha256"`
	SourceScopeKind      string              `json:"source_scope_kind"`
	SourceScopeID        string              `json:"source_scope_id"`
	Trigger              WalterReviewTrigger `json:"trigger"`
	Audience             string              `json:"audience"`
	Recommendation       string              `json:"recommendation"`
	DefinitionOfDone     string              `json:"definition_of_done"`
	ArtifactRefs         []string            `json:"artifact_refs,omitempty"`
	EvidenceRefs         []string            `json:"evidence_refs,omitempty"`
	Uncertainties        []string            `json:"uncertainties,omitempty"`
	ExecutionWorkspaceID string              `json:"execution_workspace_id,omitempty"`
	ExecutionItemID      string              `json:"execution_item_id,omitempty"`
	Posture              string              `json:"posture"`
	Chain                ReviewChain         `json:"review_chain"`
}

// WalterReviewRequest is assembled by Maestro after the producing branch has
// closed. It deliberately accepts pointers and bounded text, never a
// transcript or an unbounded context dump.
type WalterReviewRequest struct {
	Trigger              WalterReviewTrigger
	ReviewObjective      string
	Audience             string
	Recommendation       string
	DefinitionOfDone     string
	ArtifactRefs         []string
	EvidenceRefs         []string
	Uncertainties        []string
	ExecutionWorkspaceID string
	ExecutionItemID      string
	Chain                ReviewChain
	TTL                  time.Duration
}

// AccountConsultationSignals are the Maestro-owned inputs for the client
// strategic lens. Technical size and implementation depth are deliberately
// absent: this decision is about client, stakeholder and account context.
type AccountConsultationSignals struct {
	StrategicClientImportance bool
	RelationshipOrPositioning bool
	StakeholderPressureTest   bool
	ClientNarrativeOrDecision bool
	CrossCaseAccountContext   bool
	PromotionCandidate        bool
	SignalsSufficient         bool
}

type AccountConsultationDecision struct {
	Required     bool
	ReasonCode   string
	EvidenceRefs []string
	ResolvedBy   string
}

// ResolveAccountConsultation is the fail-safe Maestro planner decision. A
// missing strategic/stakeholder signal set consults Client Account; a purely
// execution-only task may take the direct Case path when that absence is
// explicitly evidenced.
func ResolveAccountConsultation(signals AccountConsultationSignals, evidenceRefs []string) AccountConsultationDecision {
	decision := AccountConsultationDecision{ResolvedBy: "maestro", EvidenceRefs: append([]string(nil), evidenceRefs...)}
	if !signals.SignalsSufficient {
		decision.Required = true
		decision.ReasonCode = "insufficient_client_lens_signals"
		return decision
	}
	if signals.StrategicClientImportance || signals.RelationshipOrPositioning ||
		signals.StakeholderPressureTest || signals.ClientNarrativeOrDecision ||
		signals.CrossCaseAccountContext || signals.PromotionCandidate {
		decision.Required = true
		decision.ReasonCode = "client_strategic_lens_required"
		return decision
	}
	decision.ReasonCode = "execution_only_no_client_lens"
	return decision
}

// WalterLeverageSignals are independent of Account consultation. Walter is
// selected for high-leverage decisions and risks, not for technical size.
type WalterLeverageSignals struct {
	ConsequentialDecision   bool
	ExecutiveRecommendation bool
	ImportantTradeoff       bool
	ExternalArtifact        bool
	ReputationalRisk        bool
	HardToReverse           bool
	MaterialityUnclear      bool
	LowLeverage             bool
}

func ResolveWalterRequired(signals WalterLeverageSignals) bool {
	return !signals.LowLeverage || signals.MaterialityUnclear ||
		signals.ConsequentialDecision || signals.ExecutiveRecommendation ||
		signals.ImportantTradeoff || signals.ExternalArtifact ||
		signals.ReputationalRisk || signals.HardToReverse
}

// WalterVerdict is the conversational contract. It is not the execution
// ledger's binary authenticated approval decision.
type WalterVerdict string

const (
	WalterApproved        WalterVerdict = "approved"
	WalterRefineAndReturn WalterVerdict = "refine-and-return"
	WalterMissingTheMark  WalterVerdict = "missing-the-mark"
	WalterHold            WalterVerdict = "hold"
)

type WalterObjection struct {
	Code          string `json:"code"`
	Fix           string `json:"fix"`
	ExitCondition string `json:"exit_condition"`
	Blocking      bool   `json:"blocking"`
}

type WalterReviewBody struct {
	Verdict      WalterVerdict     `json:"verdict"`
	Objections   []WalterObjection `json:"objections,omitempty"`
	EvidenceRefs []string          `json:"evidence_refs,omitempty"`
	Uncertainty  string            `json:"uncertainty,omitempty"`
}

type ReviewSummary struct {
	Trigger                       WalterReviewTrigger `json:"trigger"`
	State                         ReviewState         `json:"state"`
	SourcePacketID                string              `json:"source_packet_id"`
	SourcePacketSHA256            string              `json:"source_packet_sha256"`
	ChainMode                     ReviewChainMode     `json:"chain_mode"`
	ChainSHA256                   string              `json:"chain_sha256"`
	AccountConsultationRequired   bool                `json:"account_consultation_required"`
	PostAccountValidationRequired bool                `json:"post_account_validation_required"`
	WalterRequired                bool                `json:"walter_required"`
	AccountConsultationReasonCode string              `json:"account_consultation_reason_code,omitempty"`
	SkipReasonCode                string              `json:"skip_reason_code,omitempty"`
	SkipEvidenceSHA256            []string            `json:"skip_evidence_sha256,omitempty"`
	ValidatedPacketID             string              `json:"validated_packet_id,omitempty"`
	ValidatedPacketSHA256         string              `json:"validated_packet_sha256,omitempty"`
	DirectCaseReasonCode          string              `json:"direct_case_reason_code,omitempty"`
	ExecutionWorkspaceID          string              `json:"execution_workspace_id,omitempty"`
	ExecutionItemID               string              `json:"execution_item_id,omitempty"`
	ObjectionCount                int                 `json:"objection_count,omitempty"`
}

// WalterSkipDecision is the Maestro-owned, auditable low-materiality gate.
// Case agents cannot create or mutate this decision.
type WalterSkipDecision struct {
	ReasonCode     string   `json:"reason_code"`
	EvidenceRefs   []string `json:"evidence_refs"`
	LowMateriality bool     `json:"low_materiality"`
	ResolvedBy     string   `json:"resolved_by"`
}

const (
	maxReviewFieldBytes = 1000
	maxWalterObjections = 3
)

func (trigger WalterReviewTrigger) valid() bool {
	switch trigger {
	case ReviewMaterialRecommendation, ReviewConsequentialTradeoff, ReviewExternalArtifact:
		return true
	default:
		return false
	}
}

func RequiresWalterReview(trigger WalterReviewTrigger) bool {
	return trigger.valid()
}

func validateReviewPacket(review *ReviewPacket, packetID, objective string) error {
	if review == nil {
		return nil
	}
	if !validPacketID(review.SourcePacketID) || (packetID != "" && review.SourcePacketID == packetID) ||
		!validSHA256(review.SourcePacketSHA256) || !validReviewScope(review.SourceScopeKind, review.SourceScopeID) ||
		!review.Trigger.valid() || review.Posture != WalterReviewPosture || validateReviewChain(review.Chain) != nil {
		return errors.New("Walter review packet identity or trigger is invalid")
	}
	for label, value := range map[string]string{
		"objective": objective, "audience": review.Audience,
		"recommendation": review.Recommendation, "definition of done": review.DefinitionOfDone,
	} {
		if strings.TrimSpace(value) == "" || len([]byte(strings.TrimSpace(value))) > maxReviewFieldBytes {
			return errors.New("Walter review packet " + label + " is empty or oversized")
		}
	}
	if len(review.ArtifactRefs)+len(review.EvidenceRefs) > maxPointers || len(review.Uncertainties) > maxConstraints {
		return errors.New("Walter review packet exceeds its bounded item budget")
	}
	seen := make(map[string]bool, len(review.ArtifactRefs)+len(review.EvidenceRefs))
	for _, ref := range append(append([]string(nil), review.ArtifactRefs...), review.EvidenceRefs...) {
		normalized, valid := agentorchestration.NormalizeResource(ref)
		if !valid || normalized != ref || !reviewResourceWithinSource(ref, review.SourceScopeKind, review.SourceScopeID) || seen[ref] {
			return errors.New("Walter review packet contains an invalid, duplicate or cross-scope reference")
		}
		seen[ref] = true
	}
	for _, uncertainty := range review.Uncertainties {
		if strings.TrimSpace(uncertainty) == "" || len([]byte(strings.TrimSpace(uncertainty))) > maxConstraintBytes {
			return errors.New("Walter review packet uncertainty is empty or oversized")
		}
	}
	if review.ExecutionWorkspaceID != "" && !agentcatalog.ValidAgentID(review.ExecutionWorkspaceID) {
		return errors.New("Walter review packet execution workspace is invalid")
	}
	if review.ExecutionItemID != "" && !agentcatalog.ValidAgentID(review.ExecutionItemID) {
		return errors.New("Walter review packet execution item is invalid")
	}
	return nil
}

func validateReviewRequest(request WalterReviewRequest) error {
	if !request.Trigger.valid() || request.TTL <= 0 || request.TTL > maxPacketTTL {
		return errors.New("Walter review request has an invalid trigger or TTL")
	}
	if strings.TrimSpace(request.ReviewObjective) == "" || len([]byte(strings.TrimSpace(request.ReviewObjective))) > maxObjectiveBytes {
		return errors.New("Walter review objective is empty or oversized")
	}
	if len(request.ArtifactRefs)+len(request.EvidenceRefs) > maxPointers || len(request.Uncertainties) > maxConstraints {
		return errors.New("Walter review request exceeds its bounded item budget")
	}
	if validateReviewChain(request.Chain) != nil {
		return errors.New("Walter review request has an invalid Account/Case provenance chain")
	}
	if request.ExecutionWorkspaceID != "" && !agentcatalog.ValidAgentID(request.ExecutionWorkspaceID) {
		return errors.New("Walter review request execution workspace is invalid")
	}
	if request.ExecutionItemID != "" && !agentcatalog.ValidAgentID(request.ExecutionItemID) {
		return errors.New("Walter review request execution item is invalid")
	}
	return nil
}

func validateWalterSkipDecision(skip WalterSkipDecision, scopeKind, scopeID string) error {
	if skip.ResolvedBy != "maestro" || !skip.LowMateriality ||
		(skip.ReasonCode != "low_materiality" && skip.ReasonCode != "mechanical_nonmaterial") ||
		len(skip.EvidenceRefs) == 0 || len(skip.EvidenceRefs) > maxPointers {
		return errors.New("Walter skip requires a Maestro low-materiality decision and evidence")
	}
	seen := make(map[string]bool, len(skip.EvidenceRefs))
	for _, ref := range skip.EvidenceRefs {
		normalized, valid := agentorchestration.NormalizeResource(ref)
		if !valid || normalized != ref || !reviewResourceWithinSource(ref, scopeKind, scopeID) || seen[ref] {
			return errors.New("Walter skip evidence is invalid or outside the active scope")
		}
		seen[ref] = true
	}
	return nil
}

func validateReviewChain(chain ReviewChain) error {
	if chain.Mode != ReviewChainAccountCaseAccount && chain.Mode != ReviewChainDirectCase {
		return errors.New("Walter review chain mode is invalid")
	}
	if chain.AccountConsultationRequired != chain.PostAccountValidationRequired {
		return errors.New("Walter review chain violates the account-consultation/post-validation invariant")
	}
	if len(chain.Steps) == 0 || len(chain.Steps) > 3 {
		return errors.New("Walter review chain has an invalid step count")
	}
	seen := make(map[string]bool, len(chain.Steps))
	for index, step := range chain.Steps {
		if step.Sequence != index+1 || !agentcatalog.ValidAgentID(step.AgentID) ||
			!validPacketID(step.PacketID) || !validSHA256(step.PacketSHA256) ||
			step.IssuerAgentID != "maestro" || step.ParentPacketID != "" || seen[step.PacketID] {
			return errors.New("Walter review chain contains an unauthenticated, nested or duplicate step")
		}
		if step.Role == "capability_specialist" || strings.HasPrefix(step.AgentID, "capability-") || strings.HasPrefix(step.AgentID, "capability_") {
			return errors.New("Capability Specialist is not a Walter review participant")
		}
		seen[step.PacketID] = true
	}
	switch chain.Mode {
	case ReviewChainAccountCaseAccount:
		if len(chain.Steps) != 3 || chain.Steps[0].Role != "client_account_agent" ||
			chain.Steps[1].Role != "case_agent" || chain.Steps[2].Role != "client_account_agent" ||
			!chain.AccountConsultationRequired || !chain.PostAccountValidationRequired ||
			chain.Steps[0].AgentID != chain.Steps[2].AgentID ||
			chain.ValidatedPacketID != chain.Steps[1].PacketID ||
			chain.ValidatedPacketSHA256 != chain.Steps[1].PacketSHA256 ||
			!validPacketID(chain.ValidatedPacketID) || !validSHA256(chain.ValidatedPacketSHA256) ||
			chain.DirectCaseException != nil {
			return errors.New("Walter review chain must prove Account/Case/Account validation")
		}
		if !validAccountConsultationReason(chain.AccountConsultationReasonCode) || len(chain.AccountConsultationEvidenceRefs) == 0 {
			return errors.New("Walter review chain must prove why the client strategic lens was required")
		}
		if err := validateScopedEvidence(chain.AccountConsultationEvidenceRefs); err != nil {
			return fmt.Errorf("Walter Account consultation evidence is invalid: %w", err)
		}
	case ReviewChainDirectCase:
		if len(chain.Steps) != 1 || chain.Steps[0].Role != "case_agent" ||
			chain.AccountConsultationRequired || chain.PostAccountValidationRequired ||
			chain.ValidatedPacketID != "" || chain.ValidatedPacketSHA256 != "" || chain.DirectCaseException == nil {
			return errors.New("Walter direct Case review requires a bounded direct-case path")
		}
		if err := validateDirectCaseException(*chain.DirectCaseException); err != nil {
			return err
		}
	}
	return nil
}

func validateDirectCaseException(exception DirectCaseException) error {
	if (exception.ReasonCode != string(DirectCaseReasonSimpleBounded) && exception.ReasonCode != "execution_only_no_client_lens") ||
		(exception.ReasonCode == string(DirectCaseReasonSimpleBounded) && !exception.Reversible) ||
		exception.StakeholderImpact || exception.ClientImpact ||
		exception.StrategyImplication || exception.PromotionImplication || len(exception.EvidenceRefs) == 0 ||
		len(exception.EvidenceRefs) > maxPointers {
		return errors.New("Walter direct Case exception is not a simple bounded reversible exception")
	}
	seen := make(map[string]bool, len(exception.EvidenceRefs))
	for _, ref := range exception.EvidenceRefs {
		normalized, valid := agentorchestration.NormalizeResource(ref)
		if !valid || normalized != ref || seen[ref] {
			return errors.New("Walter direct Case exception evidence is invalid")
		}
		seen[ref] = true
	}
	return nil
}

func validAccountConsultationReason(reason string) bool {
	switch reason {
	case "client_strategic_lens_required", "insufficient_client_lens_signals", "client_relationship_positioning", "stakeholder_pressure_test", "client_narrative_or_decision", "cross_case_account_context", "promotion_candidate":
		return true
	default:
		return false
	}
}

func validateScopedEvidence(refs []string) error {
	if len(refs) == 0 || len(refs) > maxPointers {
		return errors.New("evidence count is outside the bounded range")
	}
	seen := make(map[string]bool, len(refs))
	for _, ref := range refs {
		normalized, valid := agentorchestration.NormalizeResource(ref)
		if !valid || normalized != ref || seen[ref] {
			return errors.New("evidence references must be normalized and unique")
		}
		seen[ref] = true
	}
	return nil
}

func validateReviewChainForSource(chain ReviewChain, source WorkPacket, sourceRole string) error {
	if err := validateReviewChain(chain); err != nil {
		return err
	}
	if sourceRole == "capability_specialist" || strings.HasPrefix(source.TargetAgentID, "capability-") || strings.HasPrefix(source.TargetAgentID, "capability_") {
		return errors.New("Walter cannot review Capability Specialist output")
	}
	caseStep := chain.Steps[0]
	if chain.Mode == ReviewChainAccountCaseAccount {
		caseStep = chain.Steps[1]
	}
	if caseStep.PacketID != source.PacketID || caseStep.PacketSHA256 != digestBody(source) ||
		caseStep.AgentID != source.TargetAgentID || (sourceRole != "case_agent" && sourceRole != "workspace_agent") {
		return errors.New("Walter review chain does not bind the current Case output")
	}
	if chain.Mode == ReviewChainDirectCase {
		for _, ref := range chain.DirectCaseException.EvidenceRefs {
			if !reviewResourceWithinSource(ref, source.ScopeKind, source.ScopeID) {
				return errors.New("Walter direct Case exception evidence is outside the Case scope")
			}
		}
	}
	return nil
}

func reviewChainDigest(chain ReviewChain) (string, error) {
	body, err := json.Marshal(chain)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:]), nil
}

func validateWalterReviewBody(body WalterReviewBody, review ReviewPacket) error {
	if body.Verdict != WalterApproved && body.Verdict != WalterRefineAndReturn && body.Verdict != WalterMissingTheMark && body.Verdict != WalterHold {
		return errors.New("Walter review verdict is invalid")
	}
	if len(body.Objections) > maxWalterObjections ||
		((body.Verdict == WalterRefineAndReturn || body.Verdict == WalterMissingTheMark || body.Verdict == WalterHold) && len(body.Objections) == 0) {
		return errors.New("Walter review objection count does not match the verdict")
	}
	seen := make(map[string]bool, len(body.Objections))
	for _, objection := range body.Objections {
		if !safeReviewCode(objection.Code) || seen[objection.Code] ||
			((objection.Code == "cosmetic" || objection.Code == "nitpick" || objection.Code == "style" || objection.Code == "wording" || objection.Code == "preference") && objection.Blocking) ||
			strings.TrimSpace(objection.Fix) == "" || strings.TrimSpace(objection.ExitCondition) == "" ||
			((body.Verdict == WalterRefineAndReturn || body.Verdict == WalterMissingTheMark || body.Verdict == WalterHold) && !objection.Blocking) ||
			(body.Verdict == WalterApproved && objection.Blocking) ||
			len([]byte(strings.TrimSpace(objection.Fix))) > maxConstraintBytes ||
			len([]byte(strings.TrimSpace(objection.ExitCondition))) > maxConstraintBytes {
			return errors.New("Walter objection requires a unique code, concrete fix and exit condition")
		}
		seen[objection.Code] = true
	}
	if len(body.EvidenceRefs) > maxPointers {
		return errors.New("Walter review evidence exceeds its bounded item budget")
	}
	for _, ref := range body.EvidenceRefs {
		normalized, valid := agentorchestration.NormalizeResource(ref)
		if !valid || normalized != ref || !reviewResourceWithinSource(ref, review.SourceScopeKind, review.SourceScopeID) {
			return errors.New("Walter review evidence is outside the source scope")
		}
	}
	if len([]byte(strings.TrimSpace(body.Uncertainty))) > maxConstraintBytes {
		return errors.New("Walter review uncertainty is oversized")
	}
	return nil
}

func normalizeWalterReviewBody(body WalterReviewBody) WalterReviewBody {
	body.Uncertainty = strings.TrimSpace(body.Uncertainty)
	body.EvidenceRefs = append([]string(nil), body.EvidenceRefs...)
	sort.Strings(body.EvidenceRefs)
	return body
}

func cloneReviewPacket(review *ReviewPacket) *ReviewPacket {
	if review == nil {
		return nil
	}
	copy := *review
	copy.ArtifactRefs = append([]string(nil), review.ArtifactRefs...)
	copy.EvidenceRefs = append([]string(nil), review.EvidenceRefs...)
	copy.Uncertainties = append([]string(nil), review.Uncertainties...)
	copy.Chain.Steps = append([]ReviewChainStep(nil), review.Chain.Steps...)
	copy.Chain.AccountConsultationEvidenceRefs = append([]string(nil), review.Chain.AccountConsultationEvidenceRefs...)
	if review.Chain.DirectCaseException != nil {
		exception := *review.Chain.DirectCaseException
		exception.EvidenceRefs = append([]string(nil), exception.EvidenceRefs...)
		copy.Chain.DirectCaseException = &exception
	}
	return &copy
}

func cloneWalterSkipDecision(skip *WalterSkipDecision) *WalterSkipDecision {
	if skip == nil {
		return nil
	}
	copy := *skip
	copy.EvidenceRefs = append([]string(nil), skip.EvidenceRefs...)
	return &copy
}

func reviewSummary(review *ReviewPacket, state ReviewState) *ReviewSummary {
	if review == nil {
		return nil
	}
	chainDigest, _ := reviewChainDigest(review.Chain)
	summary := &ReviewSummary{
		Trigger: review.Trigger, State: state,
		SourcePacketID: review.SourcePacketID, SourcePacketSHA256: review.SourcePacketSHA256,
		ChainMode: review.Chain.Mode, ChainSHA256: chainDigest,
		ValidatedPacketID: review.Chain.ValidatedPacketID, ValidatedPacketSHA256: review.Chain.ValidatedPacketSHA256,
		AccountConsultationRequired: review.Chain.AccountConsultationRequired, PostAccountValidationRequired: review.Chain.PostAccountValidationRequired,
		WalterRequired:                true,
		AccountConsultationReasonCode: review.Chain.AccountConsultationReasonCode,
		ExecutionWorkspaceID:          review.ExecutionWorkspaceID, ExecutionItemID: review.ExecutionItemID,
	}
	if review.Chain.DirectCaseException != nil {
		summary.DirectCaseReasonCode = review.Chain.DirectCaseException.ReasonCode
	}
	return summary
}

func reviewSummaryForSkip(skip *WalterSkipDecision) *ReviewSummary {
	if skip == nil {
		return nil
	}
	return &ReviewSummary{
		State: ReviewSkipped, WalterRequired: false, SkipReasonCode: skip.ReasonCode,
		SkipEvidenceSHA256: reviewEvidenceDigests(skip.EvidenceRefs),
	}
}

func reviewEvidenceDigests(refs []string) []string {
	digests := make([]string, 0, len(refs))
	for _, ref := range refs {
		digest := sha256.Sum256([]byte(ref))
		digests = append(digests, hex.EncodeToString(digest[:]))
	}
	return digests
}

func reviewResourceWithinSource(resource, scopeKind, scopeID string) bool {
	return agentorchestration.ResourceWithinScope(resource, scopeKind, scopeID)
}

func validReviewScope(kind, scope string) bool {
	if scope == "" || !agentcatalog.ValidAgentID(scope) {
		return false
	}
	switch kind {
	case "workspace", "case", "account", "practice", "health", "errand":
		return true
	default:
		return false
	}
}

func safeReviewCode(value string) bool {
	if value == "" || len(value) > 48 {
		return false
	}
	for _, character := range value {
		if (character < 'a' || character > 'z') && (character < '0' || character > '9') && character != '_' && character != '-' {
			return false
		}
	}
	return true
}

func validateReviewChainForReceipt(summary ReviewSummary) error {
	if !summary.WalterRequired || !summary.Trigger.valid() || summary.AccountConsultationRequired != summary.PostAccountValidationRequired {
		return fmt.Errorf("Walter receipt has an invalid independent requirement decision")
	}
	if summary.ChainMode != ReviewChainAccountCaseAccount && summary.ChainMode != ReviewChainDirectCase {
		return fmt.Errorf("Walter receipt has an invalid provenance chain mode")
	}
	if !validSHA256(summary.ChainSHA256) || !validPacketID(summary.SourcePacketID) || !validSHA256(summary.SourcePacketSHA256) {
		return fmt.Errorf("Walter receipt is missing provenance digests")
	}
	if summary.ChainMode == ReviewChainAccountCaseAccount &&
		(!validPacketID(summary.ValidatedPacketID) || !validSHA256(summary.ValidatedPacketSHA256) || !validAccountConsultationReason(summary.AccountConsultationReasonCode)) {
		return fmt.Errorf("Walter receipt is missing Account validation binding")
	}
	if summary.ChainMode == ReviewChainDirectCase {
		if summary.AccountConsultationRequired || summary.PostAccountValidationRequired || summary.ValidatedPacketID != "" || summary.ValidatedPacketSHA256 != "" {
			return fmt.Errorf("Walter direct Case receipt contains an invalid Account validation binding")
		}
		if !safeReviewCode(summary.DirectCaseReasonCode) {
			return fmt.Errorf("Walter receipt is missing the direct Case reason code")
		}
	}
	return nil
}
