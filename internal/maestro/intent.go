package maestro

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
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
	ID               string    `json:"id"`
	OriginalText     string    `json:"original_text"`
	NormalizedText   string    `json:"normalized_text"`
	SourceLanguage   string    `json:"source_language"`
	WorkingLanguage  string    `json:"working_language"`
	RecordedAt       time.Time `json:"recorded_at"`
	ScopeKind        string    `json:"scope_kind"`
	ScopeID          string    `json:"scope_id"`
	SHA256           string    `json:"sha256"`
	QuotedData       bool      `json:"quoted_data"`
	RelevanceScore   int       `json:"relevance_score"`
	RelevanceReasons []string  `json:"relevance_reasons"`
}

type PromptHistoryPolicy struct {
	MaxCount              int    `json:"max_count"`
	MaxBytes              int    `json:"max_bytes"`
	MaxAgeSeconds         int64  `json:"max_age_seconds"`
	ScopeKind             string `json:"scope_kind"`
	ScopeID               string `json:"scope_id"`
	OwnerID               string `json:"owner_id,omitempty"`
	WorkingLanguage       string `json:"working_language"`
	RelevanceScoring      string `json:"relevance_scoring"`
	RelevanceQuerySHA256  string `json:"relevance_query_sha256"`
	CurrentPromptPrecedes bool   `json:"current_prompt_precedes"`
}

type PromptTranslator func(original, sourceLanguage, workingLanguage string) (string, error)

// IntentReviewPacket is ephemeral and sealed by Maestro. Literal request and
// draft are available to Walter for review, but receipts retain only digests.
type IntentReviewPacket struct {
	SchemaVersion               int                               `json:"schema_version"`
	PacketVersion               string                            `json:"packet_version"`
	PacketID                    string                            `json:"packet_id"`
	LiteralRequest              string                            `json:"literal_request"`
	CurrentPrompt               string                            `json:"current_prompt"`
	CurrentPromptLanguage       string                            `json:"current_prompt_language"`
	CurrentPromptOriginalSHA256 string                            `json:"current_prompt_original_sha256"`
	WorkingCurrentPrompt        string                            `json:"working_current_prompt"`
	WorkingCurrentPromptSHA256  string                            `json:"working_current_prompt_sha256"`
	PlanDigest                  string                            `json:"plan_digest"`
	PlanRoute                   string                            `json:"plan_route"`
	DraftOutput                 string                            `json:"draft_output"`
	RelevantContextRefs         []string                          `json:"relevant_context_refs,omitempty"`
	SelfSnapshotVersion         string                            `json:"self_snapshot_version"`
	SelfSnapshotDigest          string                            `json:"self_snapshot_digest"`
	SelfFacets                  map[string]ownerctx.SnapshotFacet `json:"self_facets"`
	Observations                []RelevantObservation             `json:"observations,omitempty"`
	PriorPrompts                []HistoricalPrompt                `json:"prior_prompts,omitempty"`
	PromptHistory               PromptHistoryPolicy               `json:"prompt_history"`
	Audience                    string                            `json:"audience"`
	Consequence                 string                            `json:"consequence"`
	Reversibility               string                            `json:"reversibility"`
	AccountReceiptDigest        string                            `json:"account_receipt_digest,omitempty"`
	PacketDigest                string                            `json:"packet_digest"`
}

type IntentHypothesis struct {
	ExpressedObjective       string   `json:"expressed_objective"`
	LatentIntentHypothesis   string   `json:"latent_intent_hypothesis"`
	EvidenceRefs             []string `json:"evidence_refs"`
	Confidence               float64  `json:"confidence"`
	Alternatives             []string `json:"alternatives,omitempty"`
	Materiality              string   `json:"materiality"`
	DisconfirmationCondition string   `json:"disconfirmation_condition"`
	WorkingPrompt            string   `json:"working_prompt"`
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
		CurrentPrompt: prompt, CurrentPromptLanguage: "und",
		CurrentPromptOriginalSHA256: SHA256Hex(prompt), WorkingCurrentPrompt: strings.TrimSpace(prompt),
		WorkingCurrentPromptSHA256: SHA256Hex(strings.TrimSpace(prompt)),
		PlanDigest:                 plan.PlanDigest, PlanRoute: string(plan.CaseEntry), DraftOutput: draft,
		RelevantContextRefs: append([]string(nil), contextRefs...), SelfSnapshotVersion: snapshot.Version,
		SelfSnapshotDigest: snapshot.CanonicalSourceDigest, SelfFacets: snapshot.Facets,
		Observations: append([]RelevantObservation(nil), observations...), Audience: audience,
		Consequence: consequence, Reversibility: reversibility, AccountReceiptDigest: accountReceiptDigest,
		PromptHistory: PromptHistoryPolicy{MaxCount: 8, MaxBytes: 32 << 10, MaxAgeSeconds: int64((30 * 24 * time.Hour) / time.Second), ScopeKind: "global", ScopeID: "owner", WorkingLanguage: "und", RelevanceScoring: "lexical-v1", RelevanceQuerySHA256: SHA256Hex(prompt), CurrentPromptPrecedes: true},
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
	selected, err := ownerctx.SelectRelevantPromptHistory(historyRoot, ownerctx.PromptHistorySelectionLimits{
		OwnerID: limits.OwnerID, MaxCount: limits.MaxCount, MaxBytes: limits.MaxBytes, MaxAge: limits.MaxAge,
		ScopeKind: limits.ScopeKind, ScopeID: limits.ScopeID, CurrentPrompt: prompt,
		RelevanceKeys: append([]string(nil), limits.RelevanceKeys...), CurrentLanguage: limits.CurrentLanguage,
	}, now)
	if err != nil {
		return IntentReviewPacket{}, err
	}
	return AttachRelevantPromptHistory(packet, selected, limits, workingLanguage, translator)
}

func AttachPromptHistory(packet IntentReviewPacket, entries []ownerctx.PromptHistoryEntry, limits ownerctx.PromptHistorySelectionLimits, workingLanguage string, translator PromptTranslator) (IntentReviewPacket, error) {
	return attachPromptHistory(packet, entries, nil, limits, workingLanguage, translator)
}

func AttachRelevantPromptHistory(packet IntentReviewPacket, selected []ownerctx.PromptHistorySelection, limits ownerctx.PromptHistorySelectionLimits, workingLanguage string, translator PromptTranslator) (IntentReviewPacket, error) {
	entries := make([]ownerctx.PromptHistoryEntry, 0, len(selected))
	for _, item := range selected {
		entries = append(entries, item.Entry)
	}
	return attachPromptHistory(packet, entries, selected, limits, workingLanguage, translator)
}

func attachPromptHistory(packet IntentReviewPacket, entries []ownerctx.PromptHistoryEntry, selected []ownerctx.PromptHistorySelection, limits ownerctx.PromptHistorySelectionLimits, workingLanguage string, translator PromptTranslator) (IntentReviewPacket, error) {
	if err := packet.Validate(); err != nil {
		return IntentReviewPacket{}, err
	}
	if strings.TrimSpace(workingLanguage) == "" || len(entries) > 8 || len(entries) > limits.MaxCount || limits.MaxCount < 1 || limits.MaxCount > 8 || limits.MaxBytes < 1 || limits.MaxBytes > 32<<10 {
		return IntentReviewPacket{}, errors.New("prompt history packet limits or working language are invalid")
	}
	if workingLanguage != "und" && strings.TrimSpace(limits.CurrentLanguage) == "" {
		return IntentReviewPacket{}, errors.New("configured prompt working language requires current prompt language")
	}
	currentLanguage := limits.CurrentLanguage
	if currentLanguage == "" {
		currentLanguage = workingLanguage
	}
	currentWorking, err := normalizePrompt(packet.LiteralRequest, currentLanguage, workingLanguage, translator)
	if err != nil {
		return IntentReviewPacket{}, fmt.Errorf("current prompt working stage: %w", err)
	}
	packet.CurrentPromptLanguage = currentLanguage
	packet.CurrentPromptOriginalSHA256 = SHA256Hex(packet.LiteralRequest)
	packet.WorkingCurrentPrompt = currentWorking
	packet.WorkingCurrentPromptSHA256 = SHA256Hex(currentWorking)
	prior := make([]HistoricalPrompt, 0, len(entries))
	bytes := 0
	for index, entry := range entries {
		if bytes+len([]byte(entry.Prompt)) > limits.MaxBytes {
			return IntentReviewPacket{}, errors.New("selected prompt history exceeds packet byte limit")
		}
		normalized, err := normalizePrompt(entry.Prompt, entry.Language, workingLanguage, translator)
		if err != nil {
			return IntentReviewPacket{}, err
		}
		historical := HistoricalPrompt{ID: entry.ID, OriginalText: entry.Prompt, NormalizedText: normalized, SourceLanguage: entry.Language, WorkingLanguage: workingLanguage, RecordedAt: entry.RecordedAt, ScopeKind: string(entry.ScopeKind), ScopeID: entry.ScopeID, SHA256: entry.SHA256, QuotedData: true}
		if index < len(selected) {
			historical.RelevanceScore = selected[index].Score
			historical.RelevanceReasons = append([]string(nil), selected[index].Reasons...)
		}
		prior = append(prior, historical)
		bytes += len([]byte(entry.Prompt))
	}
	packet.PriorPrompts = prior
	packet.PromptHistory = PromptHistoryPolicy{MaxCount: limits.MaxCount, MaxBytes: limits.MaxBytes, MaxAgeSeconds: int64(limits.MaxAge / time.Second), ScopeKind: string(limits.ScopeKind), ScopeID: limits.ScopeID, OwnerID: limits.OwnerID, WorkingLanguage: workingLanguage, RelevanceScoring: "lexical-v1", RelevanceQuerySHA256: SHA256Hex(packet.LiteralRequest), CurrentPromptPrecedes: true}
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
	if !validEvidenceRefs(packet, evidenceRefs, true) {
		return IntentHypothesis{}, errors.New("intent hypothesis evidence is not bound to the packet or current prompt")
	}
	return IntentHypothesis{ExpressedObjective: expressedObjective, LatentIntentHypothesis: latentIntent, EvidenceRefs: append([]string(nil), evidenceRefs...), Confidence: confidence, Alternatives: append([]string(nil), alternatives...), Materiality: materiality, DisconfirmationCondition: disconfirmation, WorkingPrompt: packet.WorkingCurrentPrompt}, nil
}

func (packet IntentReviewPacket) Validate() error {
	if packet.SchemaVersion != IntentReviewSchemaVersion || packet.PacketVersion != "intent-review-v1" || packet.PacketID == "" || packet.PacketDigest == "" || packet.PacketDigest != digestIntentPacket(packet) {
		return errors.New("intent review packet integrity is invalid")
	}
	if strings.TrimSpace(packet.LiteralRequest) == "" || packet.CurrentPrompt != packet.LiteralRequest || packet.CurrentPromptOriginalSHA256 != SHA256Hex(packet.LiteralRequest) || strings.TrimSpace(packet.WorkingCurrentPrompt) == "" || packet.WorkingCurrentPromptSHA256 != SHA256Hex(packet.WorkingCurrentPrompt) || !packet.PromptHistory.CurrentPromptPrecedes || strings.TrimSpace(packet.DraftOutput) == "" || strings.TrimSpace(packet.Audience) == "" || strings.TrimSpace(packet.Consequence) == "" || strings.TrimSpace(packet.Reversibility) == "" {
		return errors.New("intent review packet is incomplete")
	}
	if packet.PromptHistory.WorkingLanguage == "" || packet.PromptHistory.RelevanceScoring != "lexical-v1" || packet.PromptHistory.RelevanceQuerySHA256 != SHA256Hex(packet.LiteralRequest) {
		return errors.New("intent review packet prompt working policy is invalid")
	}
	if packet.PromptHistory.WorkingLanguage != "und" && packet.CurrentPromptLanguage == "" {
		return errors.New("configured prompt working language requires a current prompt language")
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
	if packet.PromptHistory.MaxCount < 1 || packet.PromptHistory.MaxCount > 8 || packet.PromptHistory.MaxBytes < 1 || packet.PromptHistory.MaxBytes > 32<<10 || len(packet.PriorPrompts) > packet.PromptHistory.MaxCount && len(packet.PriorPrompts) > 0 {
		return errors.New("intent review packet contains too many prior prompts")
	}
	if len(packet.PriorPrompts) > 0 && packet.PromptHistory.OwnerID == "" {
		return errors.New("intent review packet history is missing owner binding")
	}
	bytes := 0
	for _, prompt := range packet.PriorPrompts {
		if prompt.ID == "" || strings.TrimSpace(prompt.OriginalText) == "" || strings.TrimSpace(prompt.NormalizedText) == "" || !prompt.QuotedData || prompt.SHA256 != SHA256Hex(prompt.OriginalText) || prompt.WorkingLanguage != packet.PromptHistory.WorkingLanguage || len(prompt.RelevanceReasons) > 8 {
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
	if len(result.EvidenceRefs) > 8 || strings.TrimSpace(result.Hypothesis.ExpressedObjective) == "" || strings.TrimSpace(result.Hypothesis.LatentIntentHypothesis) == "" || strings.TrimSpace(result.Hypothesis.DisconfirmationCondition) == "" || result.Hypothesis.WorkingPrompt != packet.WorkingCurrentPrompt || !validEvidenceRefs(packet, result.EvidenceRefs, true) || !validEvidenceRefs(packet, result.Hypothesis.EvidenceRefs, true) || !sameEvidenceRefs(result.EvidenceRefs, result.Hypothesis.EvidenceRefs) {
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

func validEvidenceRefs(packet IntentReviewPacket, refs []string, requireCurrent bool) bool {
	if len(refs) > 8 {
		return false
	}
	allowed := map[string]bool{"current_prompt": true}
	for _, ref := range packet.RelevantContextRefs {
		allowed[ref] = true
	}
	for _, prompt := range packet.PriorPrompts {
		allowed[prompt.ID] = true
	}
	seen := map[string]bool{}
	for _, ref := range refs {
		if !allowed[ref] || seen[ref] {
			return false
		}
		seen[ref] = true
	}
	return !requireCurrent || seen["current_prompt"]
}

func sameEvidenceRefs(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
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
