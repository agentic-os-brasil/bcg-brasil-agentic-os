package agentdispatch

import (
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/agentcatalog"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/agentorchestration"
)

// YodaReviewTrigger is the closed set of material situations that enter
// the Maestro-to-Yoda gate. Factual lookups and mechanical operations have
// no trigger and remain on the normal completion path.
type YodaReviewTrigger string

const (
	ReviewMaterialRecommendation YodaReviewTrigger = "material_recommendation"
	ReviewConsequentialTradeoff  YodaReviewTrigger = "consequential_tradeoff"
	ReviewExternalArtifact       YodaReviewTrigger = "external_artifact"
)

// ReviewState is intentionally a small metadata projection. It is safe for a
// status surface and contains no prompt, rationale, client body or objection
// text.
type ReviewState string

const (
	ReviewDispatched   ReviewState = "dispatched"
	ReviewApproved     ReviewState = "approved"
	ReviewRefineReturn ReviewState = "refine-and-return"
	ReviewMissingMark  ReviewState = "missing-the-mark"
	ReviewHold         ReviewState = "hold"
	ReviewUnavailable  ReviewState = "unavailable"
)

// ReviewPacket is the sealed, bounded input Yoda receives. Content remains
// in the ephemeral dispatch body; durable receipts retain only its digests.
type ReviewPacket struct {
	SourcePacketID     string            `json:"source_packet_id"`
	SourcePacketSHA256 string            `json:"source_packet_sha256"`
	SourceScopeKind    string            `json:"source_scope_kind"`
	SourceScopeID      string            `json:"source_scope_id"`
	Trigger            YodaReviewTrigger `json:"trigger"`
	Audience           string            `json:"audience"`
	Recommendation     string            `json:"recommendation"`
	DefinitionOfDone   string            `json:"definition_of_done"`
	ArtifactRefs       []string          `json:"artifact_refs,omitempty"`
	EvidenceRefs       []string          `json:"evidence_refs,omitempty"`
	Uncertainties      []string          `json:"uncertainties,omitempty"`
}

// YodaReviewRequest is assembled by Maestro after the producing branch has
// closed. It deliberately accepts pointers and bounded text, never a
// transcript or an unbounded context dump.
type YodaReviewRequest struct {
	Trigger          YodaReviewTrigger
	ReviewObjective  string
	Audience         string
	Recommendation   string
	DefinitionOfDone string
	ArtifactRefs     []string
	EvidenceRefs     []string
	Uncertainties    []string
	TTL              time.Duration
}

// YodaVerdict is the conversational contract. It is not the execution
// ledger's binary authenticated approval decision.
type YodaVerdict string

const (
	YodaApproved        YodaVerdict = "approved"
	YodaRefineAndReturn YodaVerdict = "refine-and-return"
	YodaMissingTheMark  YodaVerdict = "missing-the-mark"
	YodaHold            YodaVerdict = "hold"
)

type YodaObjection struct {
	Code               string `json:"code"`
	Fix                string `json:"fix"`
	ExitCondition      string `json:"exit_condition"`
	ProposedRefinement string `json:"proposed_refinement,omitempty"`
	Blocking           bool   `json:"blocking"`
}

type YodaReviewBody struct {
	Verdict         YodaVerdict     `json:"verdict"`
	Objections      []YodaObjection `json:"objections,omitempty"`
	EvidenceRefs    []string        `json:"evidence_refs,omitempty"`
	Uncertainty     string          `json:"uncertainty,omitempty"`
	PreservesIntent bool            `json:"preserves_intent"`
}

type ReviewSummary struct {
	Trigger            YodaReviewTrigger `json:"trigger"`
	State              ReviewState       `json:"state"`
	SourcePacketID     string            `json:"source_packet_id"`
	SourcePacketSHA256 string            `json:"source_packet_sha256"`
	ObjectionCount     int               `json:"objection_count,omitempty"`
	LeverageDecision   string            `json:"leverage_decision"`
	Posture            string            `json:"posture"`
}

const (
	maxReviewFieldBytes = 1000
	maxYodaObjections   = 3
)

func (trigger YodaReviewTrigger) valid() bool {
	switch trigger {
	case ReviewMaterialRecommendation, ReviewConsequentialTradeoff, ReviewExternalArtifact:
		return true
	default:
		return false
	}
}

func RequiresYodaReview(trigger YodaReviewTrigger) bool {
	return trigger.valid()
}

func validateReviewPacket(review *ReviewPacket, packetID, objective string) error {
	if review == nil {
		return nil
	}
	if !validPacketID(review.SourcePacketID) || (packetID != "" && review.SourcePacketID == packetID) ||
		!validSHA256(review.SourcePacketSHA256) || !validReviewScope(review.SourceScopeKind, review.SourceScopeID) ||
		!review.Trigger.valid() {
		return errors.New("Yoda review packet identity or trigger is invalid")
	}
	for label, value := range map[string]string{
		"objective": objective, "audience": review.Audience,
		"recommendation": review.Recommendation, "definition of done": review.DefinitionOfDone,
	} {
		if strings.TrimSpace(value) == "" || len([]byte(strings.TrimSpace(value))) > maxReviewFieldBytes {
			return errors.New("Yoda review packet " + label + " is empty or oversized")
		}
	}
	if len(review.ArtifactRefs)+len(review.EvidenceRefs) > maxPointers || len(review.Uncertainties) > maxConstraints {
		return errors.New("Yoda review packet exceeds its bounded item budget")
	}
	seen := make(map[string]bool, len(review.ArtifactRefs)+len(review.EvidenceRefs))
	for _, ref := range append(append([]string(nil), review.ArtifactRefs...), review.EvidenceRefs...) {
		normalized, valid := agentorchestration.NormalizeResource(ref)
		if !valid || normalized != ref || !reviewResourceWithinSource(ref, review.SourceScopeKind, review.SourceScopeID) || seen[ref] {
			return errors.New("Yoda review packet contains an invalid, duplicate or cross-scope reference")
		}
		seen[ref] = true
	}
	for _, uncertainty := range review.Uncertainties {
		if strings.TrimSpace(uncertainty) == "" || len([]byte(strings.TrimSpace(uncertainty))) > maxConstraintBytes {
			return errors.New("Yoda review packet uncertainty is empty or oversized")
		}
	}
	return nil
}

func validateReviewRequest(request YodaReviewRequest) error {
	if !request.Trigger.valid() || request.TTL <= 0 || request.TTL > maxPacketTTL {
		return errors.New("Yoda review request has an invalid trigger or TTL")
	}
	if strings.TrimSpace(request.ReviewObjective) == "" || len([]byte(strings.TrimSpace(request.ReviewObjective))) > maxObjectiveBytes {
		return errors.New("Yoda review objective is empty or oversized")
	}
	if len(request.ArtifactRefs)+len(request.EvidenceRefs) > maxPointers || len(request.Uncertainties) > maxConstraints {
		return errors.New("Yoda review request exceeds its bounded item budget")
	}
	return nil
}

func validateYodaReviewBody(body YodaReviewBody, review ReviewPacket) error {
	if body.Verdict != YodaApproved && body.Verdict != YodaRefineAndReturn && body.Verdict != YodaMissingTheMark && body.Verdict != YodaHold {
		return errors.New("Yoda review verdict is invalid")
	}
	if !body.PreservesIntent {
		return errors.New("Yoda review must preserve the user's defensible intent")
	}
	if len(body.Objections) > maxYodaObjections ||
		((body.Verdict == YodaMissingTheMark || body.Verdict == YodaHold || body.Verdict == YodaRefineAndReturn) && len(body.Objections) == 0) {
		return errors.New("Yoda review objection count does not match the verdict")
	}
	seen := make(map[string]bool, len(body.Objections))
	for _, objection := range body.Objections {
		if !safeReviewCode(objection.Code) || seen[objection.Code] ||
			strings.TrimSpace(objection.Fix) == "" || strings.TrimSpace(objection.ExitCondition) == "" ||
			len([]byte(strings.TrimSpace(objection.Fix))) > maxConstraintBytes ||
			len([]byte(strings.TrimSpace(objection.ExitCondition))) > maxConstraintBytes {
			return errors.New("Yoda objection requires a unique code, concrete fix and exit condition")
		}
		if objection.ProposedRefinement != "" && len([]byte(strings.TrimSpace(objection.ProposedRefinement))) > maxConstraintBytes {
			return errors.New("Yoda proposed refinement is oversized")
		}
		if body.Verdict == YodaApproved && objection.Blocking {
			return errors.New("approved Yoda review cannot contain a blocking objection")
		}
		if body.Verdict == YodaHold && !objection.Blocking {
			return errors.New("Yoda hold requires a material blocking objection")
		}
		seen[objection.Code] = true
	}
	if len(body.EvidenceRefs) > maxPointers {
		return errors.New("Yoda review evidence exceeds its bounded item budget")
	}
	for _, ref := range body.EvidenceRefs {
		normalized, valid := agentorchestration.NormalizeResource(ref)
		if !valid || normalized != ref || !reviewResourceWithinSource(ref, review.SourceScopeKind, review.SourceScopeID) {
			return errors.New("Yoda review evidence is outside the source scope")
		}
	}
	if len([]byte(strings.TrimSpace(body.Uncertainty))) > maxConstraintBytes {
		return errors.New("Yoda review uncertainty is oversized")
	}
	return nil
}

func normalizeYodaReviewBody(body YodaReviewBody) YodaReviewBody {
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
	return &copy
}

func reviewSummary(review *ReviewPacket, state ReviewState) *ReviewSummary {
	if review == nil {
		return nil
	}
	return &ReviewSummary{
		Trigger: review.Trigger, State: state,
		SourcePacketID: review.SourcePacketID, SourcePacketSHA256: review.SourcePacketSHA256,
		LeverageDecision: "high_leverage_trigger", Posture: "calm_constructive_advisory",
	}
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
