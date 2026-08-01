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

type HistoricalPrompt struct {
	ID              string    `json:"id"`
	OriginalText    string    `json:"original_text"`
	NormalizedText  string    `json:"normalized_text"`
	SourceLanguage  string    `json:"source_language"`
	WorkingLanguage string    `json:"working_language"`
	RecordedAt      time.Time `json:"recorded_at"`
	ScopeKind       string    `json:"scope_kind"`
	ScopeID         string    `json:"scope_id"`
	SHA256          string    `json:"sha256"`
	QuotedData      bool      `json:"quoted_data"`
}

type PromptHistoryPolicy struct {
	MaxCount              int    `json:"max_count"`
	MaxBytes              int    `json:"max_bytes"`
	MaxAgeSeconds         int64  `json:"max_age_seconds"`
	ScopeKind             string `json:"scope_kind"`
	ScopeID               string `json:"scope_id"`
	WorkingLanguage       string `json:"working_language"`
	CurrentPromptPrecedes bool   `json:"current_prompt_precedes"`
}

type PromptTranslator func(original, sourceLanguage, workingLanguage string) (string, error)

// IntentReviewPacket is ephemeral and sealed by Maestro. Literal request and
// draft are available to Walter for review, but receipts retain only digests.
type IntentReviewPacket struct {
	SchemaVersion        int                               `json:"schema_version"`
	PacketVersion        string                            `json:"packet_version"`
	PacketID             string                            `json:"packet_id"`
	LiteralRequest       string                            `json:"literal_request"`
	CurrentPrompt        string                            `json:"current_prompt"`
	PlanDigest           string                            `json:"plan_digest"`
	PlanRoute            string                            `json:"plan_route"`
	DraftOutput          string                            `json:"draft_output"`
	RelevantContextRefs  []string                          `json:"relevant_context_refs,omitempty"`
	SelfSnapshotVersion  string                            `json:"self_snapshot_version"`
	SelfSnapshotDigest   string                            `json:"self_snapshot_digest"`
	SelfFacets           map[string]ownerctx.SnapshotFacet `json:"self_facets"`
	Observations         []RelevantObservation             `json:"observations,omitempty"`
	PriorPrompts         []HistoricalPrompt                `json:"prior_prompts,omitempty"`
	PromptHistory        PromptHistoryPolicy               `json:"prompt_history"`
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
		CurrentPrompt: prompt,
		PlanDigest:    plan.PlanDigest, PlanRoute: string(plan.CaseEntry), DraftOutput: draft,
		RelevantContextRefs: append([]string(nil), contextRefs...), SelfSnapshotVersion: snapshot.Version,
		SelfSnapshotDigest: snapshot.CanonicalSourceDigest, SelfFacets: snapshot.Facets,
		Observations: append([]RelevantObservation(nil), observations...), Audience: audience,
		Consequence: consequence, Reversibility: reversibility, AccountReceiptDigest: accountReceiptDigest,
		PromptHistory: PromptHistoryPolicy{MaxCount: 8, MaxBytes: 32 << 10, MaxAgeSeconds: int64((30 * 24 * time.Hour) / time.Second), ScopeKind: "global", ScopeID: "owner", WorkingLanguage: "und", CurrentPromptPrecedes: true},
	}
	packet.PacketDigest = digestIntentPacket(packet)
	return packet, nil
}

// BuildIntentReviewPacketWithPromptHistory keeps history selection and
// translation before the Walter review stage. The prompt bodies remain only
// in the ephemeral packet; all durable receipts retain digests only.
func BuildIntentReviewPacketWithPromptHistory(prompt string, plan Plan, draft string, contextRefs []string, snapshot ownerctx.UserSelfSnapshot, observations []RelevantObservation, audience, consequence, reversibility, accountReceiptDigest, historyRoot string, limits ownerctx.PromptHistorySelectionLimits, workingLanguage string, translator PromptTranslator, now time.Time) (IntentReviewPacket, error) {
	packet, err := BuildIntentReviewPacket(prompt, plan, draft, contextRefs, snapshot, observations, audience, consequence, reversibility, accountReceiptDigest)
	if err != nil {
		return IntentReviewPacket{}, err
	}
	entries, err := ownerctx.SelectPromptHistory(historyRoot, limits, now)
	if err != nil {
		return IntentReviewPacket{}, err
	}
	return AttachPromptHistory(packet, entries, limits, workingLanguage, translator)
}

func AttachPromptHistory(packet IntentReviewPacket, entries []ownerctx.PromptHistoryEntry, limits ownerctx.PromptHistorySelectionLimits, workingLanguage string, translator PromptTranslator) (IntentReviewPacket, error) {
	if err := packet.Validate(); err != nil {
		return IntentReviewPacket{}, err
	}
	if strings.TrimSpace(workingLanguage) == "" || len(entries) > limits.MaxCount || limits.MaxCount < 1 || limits.MaxBytes < 1 {
		return IntentReviewPacket{}, errors.New("prompt history packet limits or working language are invalid")
	}
	prior := make([]HistoricalPrompt, 0, len(entries))
	bytes := 0
	for _, entry := range entries {
		if bytes+len([]byte(entry.Prompt)) > limits.MaxBytes {
			return IntentReviewPacket{}, errors.New("selected prompt history exceeds packet byte limit")
		}
		normalized, err := normalizePrompt(entry.Prompt, entry.Language, workingLanguage, translator)
		if err != nil {
			return IntentReviewPacket{}, err
		}
		prior = append(prior, HistoricalPrompt{ID: entry.ID, OriginalText: entry.Prompt, NormalizedText: normalized, SourceLanguage: entry.Language, WorkingLanguage: workingLanguage, RecordedAt: entry.RecordedAt, ScopeKind: string(entry.ScopeKind), ScopeID: entry.ScopeID, SHA256: entry.SHA256, QuotedData: true})
		bytes += len([]byte(entry.Prompt))
	}
	packet.PriorPrompts = prior
	packet.PromptHistory = PromptHistoryPolicy{MaxCount: limits.MaxCount, MaxBytes: limits.MaxBytes, MaxAgeSeconds: int64(limits.MaxAge / time.Second), ScopeKind: string(limits.ScopeKind), ScopeID: limits.ScopeID, WorkingLanguage: workingLanguage, CurrentPromptPrecedes: true}
	packet.PacketDigest = digestIntentPacket(packet)
	return packet, nil
}

func normalizePrompt(original, sourceLanguage, workingLanguage string, translator PromptTranslator) (string, error) {
	if strings.EqualFold(sourceLanguage, workingLanguage) {
		return strings.TrimSpace(original), nil
	}
	if translator == nil {
		return "", errors.New("prompt history translation requires a configured translator")
	}
	translated, err := translator(original, sourceLanguage, workingLanguage)
	if err != nil || strings.TrimSpace(translated) == "" {
		if err != nil {
			return "", err
		}
		return "", errors.New("prompt history translation returned empty text")
	}
	return strings.TrimSpace(translated), nil
}

func DeriveIntentHypothesis(packet IntentReviewPacket, expressedObjective, latentIntent string, evidenceRefs []string, confidence float64, alternatives []string, materiality, disconfirmation string) (IntentHypothesis, error) {
	if err := packet.Validate(); err != nil {
		return IntentHypothesis{}, err
	}
	if strings.TrimSpace(expressedObjective) == "" || strings.TrimSpace(latentIntent) == "" || strings.TrimSpace(disconfirmation) == "" || confidence < 0 || confidence > 1 {
		return IntentHypothesis{}, errors.New("intent hypothesis is incomplete")
	}
	allowed := map[string]bool{"current_prompt": true}
	for _, prompt := range packet.PriorPrompts {
		allowed[prompt.ID] = true
	}
	for _, ref := range evidenceRefs {
		if !allowed[ref] {
			return IntentHypothesis{}, errors.New("intent hypothesis evidence is not bound to the packet")
		}
	}
	return IntentHypothesis{ExpressedObjective: expressedObjective, LatentIntentHypothesis: latentIntent, EvidenceRefs: append([]string(nil), evidenceRefs...), Confidence: confidence, Alternatives: append([]string(nil), alternatives...), Materiality: materiality, DisconfirmationCondition: disconfirmation}, nil
}

func (packet IntentReviewPacket) Validate() error {
	if packet.SchemaVersion != IntentReviewSchemaVersion || packet.PacketVersion != "intent-review-v1" || packet.PacketID == "" || packet.PacketDigest == "" || packet.PacketDigest != digestIntentPacket(packet) {
		return errors.New("intent review packet integrity is invalid")
	}
	if strings.TrimSpace(packet.LiteralRequest) == "" || packet.CurrentPrompt != packet.LiteralRequest || !packet.PromptHistory.CurrentPromptPrecedes || strings.TrimSpace(packet.DraftOutput) == "" || strings.TrimSpace(packet.Audience) == "" || strings.TrimSpace(packet.Consequence) == "" || strings.TrimSpace(packet.Reversibility) == "" {
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
	if len(packet.PriorPrompts) > packet.PromptHistory.MaxCount && len(packet.PriorPrompts) > 0 {
		return errors.New("intent review packet contains too many prior prompts")
	}
	bytes := 0
	for _, prompt := range packet.PriorPrompts {
		if prompt.ID == "" || strings.TrimSpace(prompt.OriginalText) == "" || strings.TrimSpace(prompt.NormalizedText) == "" || !prompt.QuotedData || prompt.SHA256 != SHA256Hex(prompt.OriginalText) || prompt.WorkingLanguage != packet.PromptHistory.WorkingLanguage {
			return errors.New("intent review packet contains an invalid prior prompt")
		}
		bytes += len([]byte(prompt.OriginalText))
	}
	if len(packet.PriorPrompts) > 0 && (packet.PromptHistory.MaxBytes < 1 || bytes > packet.PromptHistory.MaxBytes) {
		return errors.New("intent review packet exceeds its prior prompt byte limit")
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
