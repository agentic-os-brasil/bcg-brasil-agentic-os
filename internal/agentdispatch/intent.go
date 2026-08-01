package agentdispatch

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/agentorchestration"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/selfmodel"
)

const IntentReviewSchemaVersion = 1

type IntentConsequence string

const (
	IntentConsequenceLow    IntentConsequence = "low"
	IntentConsequenceMedium IntentConsequence = "medium"
	IntentConsequenceHigh   IntentConsequence = "high"
)

type IntentReversibility string

const (
	IntentReversible           IntentReversibility = "reversible"
	IntentLimitedReversibility IntentReversibility = "limited"
	IntentIrreversible         IntentReversibility = "irreversible"
)

// UserSelfSnapshotRef is the single-authority Owner Context projection that a
// Walter packet may carry. It contains facet digests and no facet bodies.
type UserSelfSnapshotRef struct {
	Version      int               `json:"version"`
	Digest       string            `json:"digest"`
	Scope        string            `json:"scope"`
	FacetDigests map[string]string `json:"facet_digests,omitempty"`
}

type SelfObservationRef struct {
	ObservationID     string     `json:"observation_id"`
	Signal            string     `json:"signal"`
	SourceEventSHA256 string     `json:"source_event_sha256"`
	ClaimSHA256       string     `json:"claim_sha256"`
	OccurredAt        time.Time  `json:"occurred_at"`
	ScopeKind         string     `json:"scope_kind"`
	ScopeID           string     `json:"scope_id"`
	Confidence        string     `json:"confidence"`
	Sensitivity       string     `json:"sensitivity"`
	EvidenceType      string     `json:"evidence_type"`
	ExpiresAt         *time.Time `json:"expires_at,omitempty"`
	RecheckAfter      *time.Time `json:"recheck_after,omitempty"`
}

// IntentReviewPacket is ephemeral and bounded. Prompt/output bodies are
// present only for the active Walter call; durable receipts retain digests.
type IntentReviewPacket struct {
	SchemaVersion                  int                  `json:"schema_version"`
	PacketVersion                  int                  `json:"packet_version"`
	LiteralPrompt                  string               `json:"literal_prompt"`
	LiteralPromptSHA256            string               `json:"literal_prompt_sha256"`
	MaestroPlan                    string               `json:"maestro_plan"`
	MaestroPlanSHA256              string               `json:"maestro_plan_sha256"`
	DraftOutput                    string               `json:"draft_output"`
	DraftOutputSHA256              string               `json:"draft_output_sha256"`
	RelevantContextRefs            []string             `json:"relevant_context_refs,omitempty"`
	UserSelfSnapshot               UserSelfSnapshotRef  `json:"user_self_snapshot"`
	RecentObservations             []SelfObservationRef `json:"recent_observations,omitempty"`
	Audience                       string               `json:"audience"`
	Consequence                    IntentConsequence    `json:"consequence"`
	Reversibility                  IntentReversibility  `json:"reversibility"`
	ArtifactDigest                 string               `json:"artifact_digest,omitempty"`
	AccountValidationReceiptSHA256 string               `json:"account_validation_receipt_sha256,omitempty"`
}

type IntentPurposeSatisfaction string

const (
	PurposeYes     IntentPurposeSatisfaction = "yes"
	PurposePartial IntentPurposeSatisfaction = "partial"
	PurposeNo      IntentPurposeSatisfaction = "no"
	PurposeUnknown IntentPurposeSatisfaction = "unknown"
)

type IntentVerdict string

const (
	IntentApprove         IntentVerdict = "approve"
	IntentRefine          IntentVerdict = "refine"
	IntentClarify         IntentVerdict = "clarify"
	IntentHoldExceptional IntentVerdict = "hold_exceptional"
)

// IntentReviewBody separates the literal request from Walter's explicitly
// typed hypothesis about purpose. A hypothesis is never a self fact.
type IntentReviewBody struct {
	IntentPacketSHA256        string                    `json:"intent_packet_sha256"`
	LiteralRequest            string                    `json:"literal_request"`
	IntrinsicIntentHypothesis string                    `json:"intrinsic_intent_hypothesis"`
	EvidenceRefs              []string                  `json:"evidence_refs"`
	Confidence                selfmodel.Confidence      `json:"confidence"`
	PurposeSatisfied          IntentPurposeSatisfaction `json:"purpose_satisfied"`
	ConstructiveRefinement    string                    `json:"constructive_refinement,omitempty"`
	UnresolvedUncertainty     string                    `json:"unresolved_uncertainty,omitempty"`
	Verdict                   IntentVerdict             `json:"verdict"`
}

func NewIntentReviewPacket(prompt, plan, output, audience string, consequence IntentConsequence, reversibility IntentReversibility, snapshot UserSelfSnapshotRef, observations []SelfObservationRef, contextRefs []string) IntentReviewPacket {
	return IntentReviewPacket{
		SchemaVersion: IntentReviewSchemaVersion, PacketVersion: 1,
		LiteralPrompt: prompt, LiteralPromptSHA256: digestBodyString(prompt),
		MaestroPlan: plan, MaestroPlanSHA256: digestBodyString(plan),
		DraftOutput: output, DraftOutputSHA256: digestBodyString(output),
		RelevantContextRefs: append([]string(nil), contextRefs...), UserSelfSnapshot: snapshot,
		RecentObservations: append([]SelfObservationRef(nil), observations...), Audience: audience,
		Consequence: consequence, Reversibility: reversibility,
	}
}

func ValidateIntentReviewPacket(packet IntentReviewPacket) error {
	if packet.SchemaVersion != IntentReviewSchemaVersion || packet.PacketVersion < 1 ||
		!boundedIntent(packet.LiteralPrompt) || !boundedIntent(packet.MaestroPlan) || !boundedIntent(packet.DraftOutput) ||
		!validSHA256(packet.LiteralPromptSHA256) || packet.LiteralPromptSHA256 != digestBodyString(packet.LiteralPrompt) ||
		!validSHA256(packet.MaestroPlanSHA256) || packet.MaestroPlanSHA256 != digestBodyString(packet.MaestroPlan) ||
		!validSHA256(packet.DraftOutputSHA256) || packet.DraftOutputSHA256 != digestBodyString(packet.DraftOutput) ||
		strings.TrimSpace(packet.Audience) == "" || !validIntentConsequence(packet.Consequence) || !validIntentReversibility(packet.Reversibility) {
		return errors.New("Walter intent packet is invalid or not digest-bound")
	}
	snapshot := packet.UserSelfSnapshot
	if snapshot.Version < 1 || snapshot.Scope != selfmodel.OwnerScope || !validSHA256(snapshot.Digest) || len(snapshot.FacetDigests) == 0 {
		return errors.New("Walter intent packet is missing the canonical self projection")
	}
	for facet, digest := range snapshot.FacetDigests {
		if !validIntentSelfFacet(facet) || !validSHA256(digest) {
			return errors.New("Walter intent packet contains an invalid self facet digest")
		}
	}
	if len(packet.RelevantContextRefs) > maxPointers || len(packet.RecentObservations) > 8 {
		return errors.New("Walter intent packet exceeds its bounded reference budget")
	}
	seen := make(map[string]bool)
	for _, ref := range packet.RelevantContextRefs {
		normalized, ok := agentorchestration.NormalizeResource(ref)
		if !ok || normalized != ref || seen[ref] {
			return errors.New("Walter intent packet contains an invalid context reference")
		}
		seen[ref] = true
	}
	for _, observation := range packet.RecentObservations {
		if strings.TrimSpace(observation.ObservationID) == "" || !validSHA256(observation.SourceEventSHA256) || !validSHA256(observation.ClaimSHA256) || observation.OccurredAt.IsZero() ||
			strings.TrimSpace(observation.ScopeKind) == "" || strings.TrimSpace(observation.ScopeID) == "" ||
			strings.TrimSpace(observation.Signal) == "" || strings.TrimSpace(observation.Confidence) == "" ||
			strings.TrimSpace(observation.Sensitivity) == "" || !validIntentObservationEvidenceType(observation.EvidenceType) {
			return errors.New("Walter intent packet contains an invalid self observation reference")
		}
	}
	if packet.ArtifactDigest != "" && !validSHA256(packet.ArtifactDigest) {
		return errors.New("Walter intent packet artifact digest is invalid")
	}
	if packet.AccountValidationReceiptSHA256 != "" && !validSHA256(packet.AccountValidationReceiptSHA256) {
		return errors.New("Walter intent packet Account receipt digest is invalid")
	}
	return nil
}

func ValidateIntentReviewBody(body IntentReviewBody, packet IntentReviewPacket) error {
	if err := ValidateIntentReviewPacket(packet); err != nil {
		return err
	}
	if !boundedIntent(body.LiteralRequest) || body.LiteralRequest != packet.LiteralPrompt ||
		!validSHA256(body.IntentPacketSHA256) || body.IntentPacketSHA256 != IntentPacketDigest(packet) ||
		!boundedIntent(body.IntrinsicIntentHypothesis) || len(body.EvidenceRefs) == 0 || len(body.EvidenceRefs) > maxPointers ||
		!validIntentConfidence(body.Confidence) || !validPurpose(body.PurposeSatisfied) || !validIntentVerdict(body.Verdict) {
		return errors.New("Walter intent verdict is incomplete or untyped")
	}
	for _, ref := range body.EvidenceRefs {
		normalized, ok := agentorchestration.NormalizeResource(ref)
		if !ok || normalized != ref {
			return errors.New("Walter intent evidence is invalid")
		}
	}
	if body.Verdict == IntentRefine && strings.TrimSpace(body.ConstructiveRefinement) == "" {
		return errors.New("Walter intent refinement requires a concrete improvement")
	}
	if body.Verdict == IntentClarify && strings.TrimSpace(body.UnresolvedUncertainty) == "" {
		return errors.New("Walter intent clarification requires unresolved uncertainty")
	}
	if body.Verdict == IntentHoldExceptional && strings.TrimSpace(body.UnresolvedUncertainty) == "" && strings.TrimSpace(body.ConstructiveRefinement) == "" {
		return errors.New("Walter exceptional hold requires a material reason")
	}
	if packet.Consequence == IntentConsequenceHigh && body.Confidence == selfmodel.ConfidenceLow && body.Verdict != IntentClarify {
		return errors.New("low-confidence intent at high consequence must return to Maestro for clarification")
	}
	return nil
}

func IntentPacketDigest(packet IntentReviewPacket) string {
	return digestJSON(packet)
}

func validIntentConsequence(value IntentConsequence) bool {
	return value == IntentConsequenceLow || value == IntentConsequenceMedium || value == IntentConsequenceHigh
}
func validIntentReversibility(value IntentReversibility) bool {
	return value == IntentReversible || value == IntentLimitedReversibility || value == IntentIrreversible
}
func validIntentConfidence(value selfmodel.Confidence) bool {
	return value == selfmodel.ConfidenceLow || value == selfmodel.ConfidenceMedium || value == selfmodel.ConfidenceHigh
}
func validIntentSelfFacet(value string) bool {
	switch value {
	case "professional-role", "communication-style", "voice", "preferences", "decision-rules", "working-boundaries", "psychological-profile":
		return true
	default:
		return false
	}
}
func validIntentObservationEvidenceType(value string) bool {
	switch value {
	case "owner_speech", "owner_action", "owner_feedback", "owner_instruction", "owner_correction", "owner_endorsement":
		return true
	default:
		return false
	}
}
func validPurpose(value IntentPurposeSatisfaction) bool {
	return value == PurposeYes || value == PurposePartial || value == PurposeNo || value == PurposeUnknown
}
func validIntentVerdict(value IntentVerdict) bool {
	return value == IntentApprove || value == IntentRefine || value == IntentClarify || value == IntentHoldExceptional
}
func boundedIntent(value string) bool {
	return strings.TrimSpace(value) != "" && len([]byte(strings.TrimSpace(value))) <= maxReviewFieldBytes
}

func digestBodyString(value string) string { return digestBody(value) }

func digestJSON(value any) string {
	body, _ := json.Marshal(value)
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:])
}
