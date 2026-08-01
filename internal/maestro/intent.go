package maestro

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ownerctx"
)

const IntentReviewSchemaVersion = 1

type PurposeSatisfaction string

const (
	PurposeSatisfied PurposeSatisfaction = "yes"
	PurposePartial   PurposeSatisfaction = "partial"
	PurposeNo        PurposeSatisfaction = "no"
	PurposeUnknown   PurposeSatisfaction = "unknown"
)

type IntentVerdict string

const (
	IntentApprove IntentVerdict = "approve"
	IntentRefine  IntentVerdict = "refine"
	IntentClarify IntentVerdict = "clarify"
	IntentHold    IntentVerdict = "hold_exceptional"
)

type RelevantObservation struct {
	ID           string                    `json:"id"`
	State        ownerctx.ObservationState `json:"state"`
	Signal       ownerctx.SignalClass      `json:"signal"`
	Facet        string                    `json:"facet,omitempty"`
	Claim        string                    `json:"claim"`
	EvidenceType string                    `json:"evidence_type"`
	SourceDigest string                    `json:"source_digest"`
	ScopeKind    string                    `json:"scope_kind"`
	ScopeID      string                    `json:"scope_id"`
	Confidence   float64                   `json:"confidence"`
	ExpiresAt    time.Time                 `json:"expires_at"`
}

// IntentReviewPacket is ephemeral and sealed by Maestro. Literal request and
// draft are available to Walter for review, but receipts retain only digests.
type IntentReviewPacket struct {
	SchemaVersion        int                               `json:"schema_version"`
	PacketVersion        string                            `json:"packet_version"`
	PacketID             string                            `json:"packet_id"`
	LiteralRequest       string                            `json:"literal_request"`
	PlanDigest           string                            `json:"plan_digest"`
	PlanRoute            string                            `json:"plan_route"`
	DraftOutput          string                            `json:"draft_output"`
	RelevantContextRefs  []string                          `json:"relevant_context_refs,omitempty"`
	SelfSnapshotVersion  string                            `json:"self_snapshot_version"`
	SelfSnapshotDigest   string                            `json:"self_snapshot_digest"`
	SelfFacets           map[string]ownerctx.SnapshotFacet `json:"self_facets"`
	Observations         []RelevantObservation             `json:"observations,omitempty"`
	Audience             string                            `json:"audience"`
	Consequence          string                            `json:"consequence"`
	Reversibility        string                            `json:"reversibility"`
	AccountReceiptDigest string                            `json:"account_receipt_digest,omitempty"`
	PacketDigest         string                            `json:"packet_digest"`
}

type IntentHypothesis struct {
	ExpressedObjective       string   `json:"expressed_objective"`
	LatentIntentHypothesis   string   `json:"latent_intent_hypothesis"`
	EvidenceRefs             []string `json:"evidence_refs"`
	Confidence               float64  `json:"confidence"`
	Alternatives             []string `json:"alternatives,omitempty"`
	Materiality              string   `json:"materiality"`
	DisconfirmationCondition string   `json:"disconfirmation_condition"`
}

type IntentReviewResult struct {
	LiteralRequest            string              `json:"literal_request"`
	IntrinsicIntentHypothesis string              `json:"intrinsic_intent_hypothesis"`
	EvidenceRefs              []string            `json:"evidence_refs"`
	Confidence                float64             `json:"confidence"`
	PurposeSatisfied          PurposeSatisfaction `json:"purpose_satisfied"`
	ConstructiveRefinement    string              `json:"constructive_refinement,omitempty"`
	UnresolvedUncertainty     string              `json:"unresolved_uncertainty,omitempty"`
	Verdict                   IntentVerdict       `json:"verdict"`
	Hypothesis                IntentHypothesis    `json:"hypothesis"`
}

type IntentReviewReceipt struct {
	SchemaVersion       int                 `json:"schema_version"`
	PacketVersion       string              `json:"packet_version"`
	PacketDigest        string              `json:"packet_digest"`
	SelfSnapshotVersion string              `json:"self_snapshot_version"`
	SelfSnapshotDigest  string              `json:"self_snapshot_digest"`
	PromptDigest        string              `json:"prompt_digest"`
	OutputDigest        string              `json:"output_digest"`
	Confidence          float64             `json:"confidence"`
	PurposeSatisfied    PurposeSatisfaction `json:"purpose_satisfied"`
	Verdict             IntentVerdict       `json:"verdict"`
	RecordedAt          time.Time           `json:"recorded_at"`
}

func BuildIntentReviewPacket(prompt string, plan Plan, draft string, contextRefs []string, snapshot ownerctx.UserSelfSnapshot, observations []RelevantObservation, audience, consequence, reversibility, accountReceiptDigest string) (IntentReviewPacket, error) {
	if err := plan.Validate(); err != nil {
		return IntentReviewPacket{}, err
	}
	if strings.TrimSpace(prompt) == "" || strings.TrimSpace(draft) == "" || strings.TrimSpace(audience) == "" || strings.TrimSpace(consequence) == "" || strings.TrimSpace(reversibility) == "" {
		return IntentReviewPacket{}, errors.New("intent review packet requires bounded request, draft, audience, consequence and reversibility")
	}
	if err := snapshot.Validate(); err != nil {
		return IntentReviewPacket{}, err
	}
	packet := IntentReviewPacket{
		SchemaVersion: IntentReviewSchemaVersion, PacketVersion: "intent-review-v1",
		PacketID: packetID(plan.PlanDigest, prompt, draft), LiteralRequest: prompt,
		PlanDigest: plan.PlanDigest, PlanRoute: string(plan.CaseEntry), DraftOutput: draft,
		RelevantContextRefs: append([]string(nil), contextRefs...), SelfSnapshotVersion: snapshot.Version,
		SelfSnapshotDigest: snapshot.CanonicalSourceDigest, SelfFacets: snapshot.Facets,
		Observations: append([]RelevantObservation(nil), observations...), Audience: audience,
		Consequence: consequence, Reversibility: reversibility, AccountReceiptDigest: accountReceiptDigest,
	}
	packet.PacketDigest = digestIntentPacket(packet)
	return packet, nil
}

func (packet IntentReviewPacket) Validate() error {
	if packet.SchemaVersion != IntentReviewSchemaVersion || packet.PacketVersion != "intent-review-v1" || packet.PacketID == "" || packet.PacketDigest == "" || packet.PacketDigest != digestIntentPacket(packet) {
		return errors.New("intent review packet integrity is invalid")
	}
	if strings.TrimSpace(packet.LiteralRequest) == "" || strings.TrimSpace(packet.DraftOutput) == "" || strings.TrimSpace(packet.Audience) == "" || strings.TrimSpace(packet.Consequence) == "" || strings.TrimSpace(packet.Reversibility) == "" {
		return errors.New("intent review packet is incomplete")
	}
	if len(packet.SelfFacets) == 0 || packet.SelfSnapshotVersion == "" || !validSHA256(packet.SelfSnapshotDigest) || packet.PlanDigest == "" {
		return errors.New("intent review packet is missing self or plan binding")
	}
	for _, facet := range packet.SelfFacets {
		allowed := false
		for _, reader := range facet.Readers {
			if reader == "walter" {
				allowed = true
				break
			}
		}
		if !allowed {
			return errors.New("intent review packet contains a facet without Walter purpose authorization")
		}
	}
	for _, observation := range packet.Observations {
		if observation.EvidenceType == "generated_output" || observation.EvidenceType == "client_document" || observation.EvidenceType == "agent_output" || observation.Claim == "" || !validSHA256(observation.SourceDigest) {
			return errors.New("intent review packet contains an invalid self observation")
		}
	}
	return nil
}

func ValidateIntentReview(packet IntentReviewPacket, result IntentReviewResult) error {
	if err := packet.Validate(); err != nil {
		return err
	}
	if result.LiteralRequest != packet.LiteralRequest || strings.TrimSpace(result.IntrinsicIntentHypothesis) == "" || result.Confidence < 0 || result.Confidence > 1 || !validPurpose(result.PurposeSatisfied) || !validIntentVerdict(result.Verdict) {
		return errors.New("intent review result is incomplete or invalid")
	}
	if len(result.EvidenceRefs) > 8 || strings.TrimSpace(result.Hypothesis.ExpressedObjective) == "" || strings.TrimSpace(result.Hypothesis.LatentIntentHypothesis) == "" || strings.TrimSpace(result.Hypothesis.DisconfirmationCondition) == "" {
		return errors.New("intent hypothesis requires bounded evidence and disconfirmation")
	}
	if result.Verdict == IntentRefine && strings.TrimSpace(result.ConstructiveRefinement) == "" {
		return errors.New("intent refinement requires a concrete constructive refinement")
	}
	if result.Verdict == IntentClarify && strings.TrimSpace(result.UnresolvedUncertainty) == "" {
		return errors.New("intent clarification requires unresolved uncertainty")
	}
	if result.Verdict == IntentHold && strings.TrimSpace(result.UnresolvedUncertainty) == "" {
		return errors.New("exceptional intent hold requires a material uncertainty")
	}
	if result.Confidence < 0.5 && packet.Consequence != "low" && result.Verdict == IntentApprove {
		return errors.New("low-confidence high-consequence intent must return to Maestro for clarification")
	}
	return nil
}

func NewIntentReviewReceipt(packet IntentReviewPacket, result IntentReviewResult, now time.Time) (IntentReviewReceipt, error) {
	if err := ValidateIntentReview(packet, result); err != nil {
		return IntentReviewReceipt{}, err
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	return IntentReviewReceipt{SchemaVersion: IntentReviewSchemaVersion, PacketVersion: packet.PacketVersion, PacketDigest: packet.PacketDigest, SelfSnapshotVersion: packet.SelfSnapshotVersion, SelfSnapshotDigest: packet.SelfSnapshotDigest, PromptDigest: SHA256Hex(packet.LiteralRequest), OutputDigest: SHA256Hex(packet.DraftOutput), Confidence: result.Confidence, PurposeSatisfied: result.PurposeSatisfied, Verdict: result.Verdict, RecordedAt: now.UTC()}, nil
}

func SHA256Hex(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func packetID(planDigest, prompt, draft string) string {
	return "intent-" + SHA256Hex(planDigest + "\x00" + prompt + "\x00" + draft)[:24]
}

func digestIntentPacket(packet IntentReviewPacket) string {
	packet.PacketDigest = ""
	body, _ := json.Marshal(packet)
	return SHA256Hex(string(body))
}

func validSHA256(value string) bool { return len(value) == 64 && isHex(value) }
func isHex(value string) bool       { _, err := hex.DecodeString(value); return err == nil }

func validPurpose(value PurposeSatisfaction) bool {
	return value == PurposeSatisfied || value == PurposePartial || value == PurposeNo || value == PurposeUnknown
}

func validIntentVerdict(value IntentVerdict) bool {
	return value == IntentApprove || value == IntentRefine || value == IntentClarify || value == IntentHold
}
