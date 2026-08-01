// Package userintent defines the bounded contract between Maestro, Walter and
// the user-owned self layers. Raw prompt and draft fields are ephemeral packet
// inputs; durable observations and receipts contain only metadata and digests.
package userintent

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	SchemaVersion       = 1
	MaxPacketTextBytes  = 32768
	MaxHypothesisBytes  = 4000
	MaxObservationCount = 32
)

type SelfSection string

const (
	SectionPreferences   SelfSection = "preferences"
	SectionPrinciples    SelfSection = "principles"
	SectionDecisionRules SelfSection = "decision_rules"
	SectionCommunication SelfSection = "communication_style"
	SectionMotivations   SelfSection = "motivations"
	SectionBoundaries    SelfSection = "boundaries"
)

type SignalKind string

const (
	ExplicitInstruction SignalKind = "explicit_instruction"
	ExplicitCorrection  SignalKind = "explicit_correction"
	ExplicitEndorsement SignalKind = "explicit_endorsement"
	ObservedPattern     SignalKind = "observed_pattern"
	InferredHypothesis  SignalKind = "inferred_hypothesis"
)

type Confidence string

const (
	ConfidenceLow    Confidence = "low"
	ConfidenceMedium Confidence = "medium"
	ConfidenceHigh   Confidence = "high"
)

type Sensitivity string

const (
	SensitivityNormal     Sensitivity = "normal"
	SensitivitySensitive  Sensitivity = "sensitive"
	SensitivityRestricted Sensitivity = "restricted"
)

type ObservationScope string

const (
	ScopeGlobal      ObservationScope = "global"
	ScopeWorkspace   ObservationScope = "workspace"
	ScopeAccount     ObservationScope = "account"
	ScopeCase        ObservationScope = "case"
	ScopeInteraction ObservationScope = "interaction"
	ScopeSkill       ObservationScope = "skill"
)

type EvidenceType string

const (
	EvidenceOwnerSpeech EvidenceType = "owner_speech"
	EvidenceOwnerAction EvidenceType = "owner_action"
)

type ObservationLifecycle string

const (
	LifecycleCaptured     ObservationLifecycle = "captured"
	LifecycleEligible     ObservationLifecycle = "eligible"
	LifecycleCorroborated ObservationLifecycle = "corroborated"
	LifecycleProposed     ObservationLifecycle = "proposed"
	LifecycleRejected     ObservationLifecycle = "rejected"
	LifecycleContradicted ObservationLifecycle = "contradicted"
	LifecycleExpired      ObservationLifecycle = "expired"
	LifecycleRedacted     ObservationLifecycle = "redacted"
	LifecyclePromoted     ObservationLifecycle = "promoted"
)

type Audience string

const (
	AudienceUser      Audience = "user"
	AudienceInternal  Audience = "internal"
	AudienceClient    Audience = "client"
	AudienceExecutive Audience = "executive"
	AudienceExternal  Audience = "external"
)

type Consequence string

const (
	ConsequenceLow      Consequence = "low"
	ConsequenceMaterial Consequence = "material"
	ConsequenceHigh     Consequence = "high"
)

type Reversibility string

const (
	Reversible    Reversibility = "reversible"
	HardToReverse Reversibility = "hard_to_reverse"
)

type PurposeSatisfaction string

const (
	PurposeYes     PurposeSatisfaction = "yes"
	PurposePartial PurposeSatisfaction = "partial"
	PurposeNo      PurposeSatisfaction = "no"
	PurposeUnknown PurposeSatisfaction = "unknown"
)

type IntentVerdict string

const (
	VerdictApprove         IntentVerdict = "approve"
	VerdictRefine          IntentVerdict = "refine"
	VerdictClarify         IntentVerdict = "clarify"
	VerdictHoldExceptional IntentVerdict = "hold_exceptional"
)

type RefinementKind string

const (
	RefinePurpose       RefinementKind = "purpose"
	RefinePreference    RefinementKind = "preference"
	RefineDecision      RefinementKind = "decision_rule"
	RefineCommunication RefinementKind = "communication"
	RefineReadiness     RefinementKind = "readiness"
	RefineGovernance    RefinementKind = "governance_blocker"
)

var (
	idPattern     = regexp.MustCompile(`^[a-z0-9][a-z0-9._:-]{0,63}$`)
	digestPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)
	refPattern    = regexp.MustCompile(`^[a-z0-9][a-z0-9._:/-]{0,127}$`)
)

// UserSelfSnapshot is a user-owned version pointer. The section bodies live
// in the private local store and never in the public repository or Darwin
// receipts.
type UserSelfSnapshot struct {
	SchemaVersion    int           `json:"schema_version"`
	Version          int           `json:"version"`
	Digest           string        `json:"digest"`
	Owner            string        `json:"owner"`
	ProjectionOf     string        `json:"projection_of"`
	PrecedencePolicy []string      `json:"precedence_policy"`
	Sections         []SelfSection `json:"sections"`
}

func (snapshot UserSelfSnapshot) Validate() error {
	if snapshot.SchemaVersion != SchemaVersion || snapshot.Version < 1 || !digestPattern.MatchString(snapshot.Digest) || snapshot.Owner != "user" || snapshot.ProjectionOf != "owner_context" || len(snapshot.Sections) == 0 {
		return errors.New("invalid user self snapshot reference")
	}
	if len(snapshot.PrecedencePolicy) != 5 || strings.Join(snapshot.PrecedencePolicy, ">") != "explicit_instruction>explicit_correction>canonical_snapshot>recent_observation>walter_hypothesis" {
		return errors.New("user self snapshot precedence policy is not canonical")
	}
	previous := 0
	for _, section := range snapshot.Sections {
		order, ok := sectionOrder[section]
		if !ok || order <= previous {
			return errors.New("user self snapshot sections must be closed and sorted")
		}
		previous = order
	}
	return nil
}

type ObservationRef struct {
	ObservationID     string           `json:"observation_id"`
	SourceEventSHA256 string           `json:"source_event_sha256"`
	Kind              SignalKind       `json:"kind"`
	Scope             ObservationScope `json:"scope"`
	Confidence        Confidence       `json:"confidence"`
}

func (ref ObservationRef) Validate() error {
	if !idPattern.MatchString(ref.ObservationID) || !digestPattern.MatchString(ref.SourceEventSHA256) || !validSignal(ref.Kind) || !validScope(ref.Scope) || !validConfidence(ref.Confidence) {
		return errors.New("invalid intent observation reference")
	}
	return nil
}

// IntentReviewPacket is ephemeral. It binds Walter's review to the exact
// prompt, plan, draft and user-self snapshot without persisting their bodies.
type IntentReviewPacket struct {
	SchemaVersion        int              `json:"schema_version"`
	PacketID             string           `json:"packet_id"`
	PromptLiteral        string           `json:"prompt_literal"`
	PromptSHA256         string           `json:"prompt_sha256"`
	PlanRoute            string           `json:"plan_route"`
	PlanSHA256           string           `json:"plan_sha256"`
	DraftOutput          string           `json:"draft_output"`
	DraftSHA256          string           `json:"draft_sha256"`
	ContextRefs          []string         `json:"context_refs,omitempty"`
	SelfSnapshot         UserSelfSnapshot `json:"user_self_snapshot"`
	Observations         []ObservationRef `json:"observations,omitempty"`
	AccountReceiptSHA256 string           `json:"account_receipt_sha256,omitempty"`
	Audience             Audience         `json:"audience"`
	Consequence          Consequence      `json:"consequence"`
	Reversibility        Reversibility    `json:"reversibility"`
}

// SelfReviewPacket is the semantic name used by the Walter adapter.
type SelfReviewPacket = IntentReviewPacket

func NewIntentReviewPacket(packetID, prompt, route, plan, draft string, snapshot UserSelfSnapshot, refs []string, observations []ObservationRef, audience Audience, consequence Consequence, reversibility Reversibility) (IntentReviewPacket, error) {
	promptDigest := sha256.Sum256([]byte(prompt))
	draftDigest := sha256.Sum256([]byte(draft))
	packet := IntentReviewPacket{
		SchemaVersion: SchemaVersion, PacketID: packetID, PromptLiteral: prompt,
		PromptSHA256: hex.EncodeToString(promptDigest[:]), PlanRoute: route,
		PlanSHA256: sha256Hex(plan), DraftOutput: draft,
		DraftSHA256: hex.EncodeToString(draftDigest[:]), ContextRefs: append([]string(nil), refs...),
		SelfSnapshot: snapshot, Observations: append([]ObservationRef(nil), observations...),
		Audience: audience, Consequence: consequence, Reversibility: reversibility,
	}
	return packet, packet.Validate()
}

func (packet IntentReviewPacket) Validate() error {
	if packet.SchemaVersion != SchemaVersion || !idPattern.MatchString(packet.PacketID) || strings.TrimSpace(packet.PromptLiteral) == "" || strings.TrimSpace(packet.DraftOutput) == "" || len([]byte(packet.PromptLiteral)) > MaxPacketTextBytes || len([]byte(packet.DraftOutput)) > MaxPacketTextBytes || !refPattern.MatchString(packet.PlanRoute) || !digestPattern.MatchString(packet.PlanSHA256) || !validAudience(packet.Audience) || !validConsequence(packet.Consequence) || !validReversibility(packet.Reversibility) {
		return errors.New("invalid intent review packet header")
	}
	if packet.PromptSHA256 != sha256Hex(packet.PromptLiteral) || packet.DraftSHA256 != sha256Hex(packet.DraftOutput) {
		return errors.New("intent review packet digest is stale")
	}
	if packet.AccountReceiptSHA256 != "" && !digestPattern.MatchString(packet.AccountReceiptSHA256) {
		return errors.New("intent review packet account receipt digest is invalid")
	}
	if err := packet.SelfSnapshot.Validate(); err != nil {
		return err
	}
	if len(packet.ContextRefs) > 12 || len(packet.Observations) > MaxObservationCount {
		return errors.New("intent review packet exceeds its bounded context")
	}
	seen := map[string]bool{}
	for _, ref := range packet.ContextRefs {
		if !refPattern.MatchString(ref) || seen[ref] {
			return errors.New("intent review packet contains an invalid or duplicate context reference")
		}
		seen[ref] = true
	}
	seen = map[string]bool{}
	for _, observation := range packet.Observations {
		if err := observation.Validate(); err != nil || seen[observation.ObservationID] {
			return errors.New("intent review packet contains an invalid or duplicate observation")
		}
		seen[observation.ObservationID] = true
	}
	return nil
}

func (packet IntentReviewPacket) Digest() string {
	body, err := json.Marshal(packet)
	if err != nil {
		return ""
	}
	return sha256Hex(string(body))
}

type IntentHypothesis struct {
	Text       string     `json:"text"`
	Confidence Confidence `json:"confidence"`
	Status     string     `json:"status"`
}

type ConstructiveRefinement struct {
	Kind               RefinementKind `json:"kind"`
	Summary            string         `json:"summary"`
	AcceptanceCriteria string         `json:"acceptance_criteria"`
	PreservesIntent    bool           `json:"preserves_intent"`
	LoadBearing        bool           `json:"load_bearing"`
	Blocking           bool           `json:"blocking"`
}

type IntentReview struct {
	SchemaVersion             int                     `json:"schema_version"`
	LiteralRequest            string                  `json:"literal_request"`
	IntrinsicIntentHypothesis IntentHypothesis        `json:"intrinsic_intent_hypothesis"`
	EvidenceRefs              []string                `json:"evidence_refs"`
	Confidence                Confidence              `json:"confidence"`
	Alternatives              []string                `json:"alternatives,omitempty"`
	Materiality               Consequence             `json:"materiality"`
	DisconfirmationCondition  string                  `json:"disconfirmation_condition"`
	PurposeSatisfied          PurposeSatisfaction     `json:"purpose_satisfied"`
	ConstructiveRefinement    *ConstructiveRefinement `json:"constructive_refinement,omitempty"`
	UnresolvedUncertainty     string                  `json:"unresolved_uncertainty,omitempty"`
	Verdict                   IntentVerdict           `json:"verdict"`
}

func (review IntentReview) Validate(packet IntentReviewPacket) error {
	if err := packet.Validate(); err != nil {
		return err
	}
	if review.SchemaVersion != SchemaVersion || strings.TrimSpace(review.LiteralRequest) == "" || len([]byte(review.LiteralRequest)) > MaxPacketTextBytes || strings.TrimSpace(review.IntrinsicIntentHypothesis.Text) == "" || len([]byte(review.IntrinsicIntentHypothesis.Text)) > MaxHypothesisBytes || review.IntrinsicIntentHypothesis.Status != "hypothesis" || !validConfidence(review.Confidence) || !validConfidence(review.IntrinsicIntentHypothesis.Confidence) || !validConsequence(review.Materiality) || !validPurpose(review.PurposeSatisfied) || !validVerdict(review.Verdict) || strings.TrimSpace(review.DisconfirmationCondition) == "" {
		return errors.New("invalid Walter intent review")
	}
	if len(review.EvidenceRefs) == 0 || len(review.EvidenceRefs) > 12 {
		return errors.New("intent review requires bounded evidence references")
	}
	seen := map[string]bool{}
	for _, ref := range review.EvidenceRefs {
		if !refPattern.MatchString(ref) || seen[ref] {
			return errors.New("intent review evidence references are invalid or duplicated")
		}
		seen[ref] = true
	}
	if len(review.Alternatives) > 4 {
		return errors.New("intent review alternatives exceed the bounded limit")
	}
	for _, alternative := range review.Alternatives {
		if strings.TrimSpace(alternative) == "" || len([]byte(alternative)) > MaxHypothesisBytes {
			return errors.New("intent review alternative is invalid")
		}
	}
	if len([]byte(review.UnresolvedUncertainty)) > MaxHypothesisBytes {
		return errors.New("intent review uncertainty is oversized")
	}
	if review.Verdict == VerdictClarify && strings.TrimSpace(review.UnresolvedUncertainty) == "" {
		return errors.New("clarify verdict requires unresolved uncertainty")
	}
	if review.Verdict == VerdictHoldExceptional && (review.ConstructiveRefinement == nil || review.ConstructiveRefinement.Kind != RefineGovernance || !review.ConstructiveRefinement.Blocking) {
		return errors.New("exceptional hold requires a material governance blocker")
	}
	if review.Verdict == VerdictRefine {
		if review.ConstructiveRefinement == nil || !review.ConstructiveRefinement.LoadBearing || review.ConstructiveRefinement.Blocking || !review.ConstructiveRefinement.PreservesIntent || strings.TrimSpace(review.ConstructiveRefinement.Summary) == "" || strings.TrimSpace(review.ConstructiveRefinement.AcceptanceCriteria) == "" {
			return errors.New("refine verdict requires a concrete non-blocking intent-preserving refinement")
		}
	}
	if review.ConstructiveRefinement != nil {
		if !validRefinement(review.ConstructiveRefinement.Kind) || strings.TrimSpace(review.ConstructiveRefinement.Summary) == "" || strings.TrimSpace(review.ConstructiveRefinement.AcceptanceCriteria) == "" {
			return errors.New("constructive refinement is incomplete")
		}
		if review.ConstructiveRefinement.Blocking && review.Verdict != VerdictHoldExceptional {
			return errors.New("only exceptional hold may contain a blocking refinement")
		}
	}
	if review.Verdict == VerdictClarify && review.Confidence == ConfidenceHigh {
		return errors.New("high-confidence review cannot request clarification")
	}
	return nil
}

type IntentReviewReceipt struct {
	SchemaVersion       int                 `json:"schema_version"`
	ReviewID            string              `json:"review_id"`
	PacketID            string              `json:"packet_id"`
	PacketSHA256        string              `json:"packet_sha256"`
	PromptSHA256        string              `json:"prompt_sha256"`
	DraftSHA256         string              `json:"draft_sha256"`
	SelfSnapshotVersion int                 `json:"self_snapshot_version"`
	SelfSnapshotSHA256  string              `json:"self_snapshot_sha256"`
	Verdict             IntentVerdict       `json:"verdict"`
	PurposeSatisfied    PurposeSatisfaction `json:"purpose_satisfied"`
	Confidence          Confidence          `json:"confidence"`
	ReviewerID          string              `json:"reviewer_id"`
	Cycle               int                 `json:"cycle"`
	RecordedAt          time.Time           `json:"recorded_at"`
}

func NewIntentReviewReceipt(reviewID string, packet IntentReviewPacket, review IntentReview, cycle int, recordedAt time.Time) (IntentReviewReceipt, error) {
	if err := review.Validate(packet); err != nil {
		return IntentReviewReceipt{}, err
	}
	receipt := IntentReviewReceipt{SchemaVersion: SchemaVersion, ReviewID: reviewID, PacketID: packet.PacketID, PacketSHA256: packet.Digest(), PromptSHA256: packet.PromptSHA256, DraftSHA256: packet.DraftSHA256, SelfSnapshotVersion: packet.SelfSnapshot.Version, SelfSnapshotSHA256: packet.SelfSnapshot.Digest, Verdict: review.Verdict, PurposeSatisfied: review.PurposeSatisfied, Confidence: review.Confidence, ReviewerID: "walter", Cycle: cycle, RecordedAt: recordedAt.UTC()}
	return receipt, receipt.Validate(packet)
}

func (receipt IntentReviewReceipt) Validate(packet IntentReviewPacket) error {
	if err := packet.Validate(); err != nil {
		return err
	}
	if receipt.SchemaVersion != SchemaVersion || !idPattern.MatchString(receipt.ReviewID) || receipt.PacketID != packet.PacketID || receipt.PacketSHA256 != packet.Digest() || receipt.PromptSHA256 != packet.PromptSHA256 || receipt.DraftSHA256 != packet.DraftSHA256 || receipt.SelfSnapshotVersion != packet.SelfSnapshot.Version || receipt.SelfSnapshotSHA256 != packet.SelfSnapshot.Digest || receipt.ReviewerID != "walter" || receipt.Cycle < 0 || receipt.RecordedAt.IsZero() || !validVerdict(receipt.Verdict) || !validPurpose(receipt.PurposeSatisfied) || !validConfidence(receipt.Confidence) {
		return errors.New("intent review receipt is not bound to its packet")
	}
	return nil
}

// InteractionObservation is the append-only provisional absorption record.
// ClaimDigest is intentionally the only representation of the learned claim.
type InteractionObservation struct {
	SchemaVersion      int                  `json:"schema_version"`
	ObservationID      string               `json:"observation_id"`
	SourceEventSHA256  string               `json:"source_event_sha256"`
	Kind               SignalKind           `json:"kind"`
	Facet              SelfSection          `json:"facet"`
	SignalKey          string               `json:"signal_key"`
	ClaimDigest        string               `json:"claim_digest"`
	EpisodeSHA256      string               `json:"episode_sha256"`
	EvidenceType       EvidenceType         `json:"evidence_type"`
	OwnerAuthenticated bool                 `json:"owner_authenticated"`
	Material           bool                 `json:"material"`
	Declassified       bool                 `json:"declassified"`
	Scope              ObservationScope     `json:"scope"`
	ConfidenceBasisPts int                  `json:"confidence_basis_points"`
	Sensitivity        Sensitivity          `json:"sensitivity"`
	Lifecycle          ObservationLifecycle `json:"lifecycle"`
	RecordedAt         time.Time            `json:"recorded_at"`
	ExpiresAt          time.Time            `json:"expires_at"`
	RecheckAt          time.Time            `json:"recheck_at"`
	UserConfirmed      bool                 `json:"user_confirmed"`
	SupersedesIDs      []string             `json:"supersedes_ids,omitempty"`
}

func (observation InteractionObservation) Validate() error {
	if observation.SchemaVersion != SchemaVersion || !idPattern.MatchString(observation.ObservationID) || !digestPattern.MatchString(observation.SourceEventSHA256) || !validSignal(observation.Kind) || !validSection(observation.Facet) || !refPattern.MatchString(observation.SignalKey) || !digestPattern.MatchString(observation.ClaimDigest) || !digestPattern.MatchString(observation.EpisodeSHA256) || !validEvidenceType(observation.EvidenceType) || !observation.OwnerAuthenticated || !observation.Material || !validScope(observation.Scope) || observation.ConfidenceBasisPts < 0 || observation.ConfidenceBasisPts > 10000 || !validSensitivity(observation.Sensitivity) || !validLifecycle(observation.Lifecycle) || observation.Lifecycle == LifecycleProposed || observation.Lifecycle == LifecycleRedacted || observation.Lifecycle == LifecyclePromoted || observation.RecordedAt.IsZero() || observation.ExpiresAt.IsZero() || observation.RecheckAt.IsZero() || !observation.ExpiresAt.After(observation.RecordedAt) || !observation.RecheckAt.After(observation.RecordedAt) || (observation.UserConfirmed && observation.Kind != ExplicitInstruction && observation.Kind != ExplicitCorrection && observation.Kind != ExplicitEndorsement) || (observation.Kind == ExplicitCorrection && len(observation.SupersedesIDs) == 0) {
		return errors.New("invalid interaction observation")
	}
	if observation.Scope == ScopeGlobal && (!observation.Declassified || !observation.UserConfirmed) {
		return errors.New("global self signal requires owner confirmation and declassification")
	}
	return nil
}

type AppendResult struct {
	Duplicate     bool
	Contradiction bool
}

type InteractionEvaluation struct {
	Persistable bool   `json:"persistable"`
	ReasonCode  string `json:"reason_code"`
}

// EvaluateInteraction runs for every interaction before the caller decides
// whether to append a provisional observation. It never stores content and it
// never promotes a claim.
func EvaluateInteraction(observation InteractionObservation) InteractionEvaluation {
	if !observation.OwnerAuthenticated || !observation.Material {
		return InteractionEvaluation{ReasonCode: "not_material_authenticated_owner_signal"}
	}
	if observation.Scope == ScopeGlobal && (!observation.Declassified || !observation.UserConfirmed) {
		return InteractionEvaluation{ReasonCode: "global_requires_owner_declassification"}
	}
	if observation.EvidenceType != EvidenceOwnerSpeech && observation.EvidenceType != EvidenceOwnerAction {
		return InteractionEvaluation{ReasonCode: "unsupported_owner_provenance"}
	}
	switch observation.Kind {
	case ExplicitInstruction, ExplicitCorrection, ExplicitEndorsement:
		return InteractionEvaluation{Persistable: true, ReasonCode: "authenticated_owner_signal"}
	default:
		return InteractionEvaluation{ReasonCode: "hypothesis_or_pattern_is_ephemeral"}
	}
}

type AbsorptionLog struct {
	SchemaVersion  int                      `json:"schema_version"`
	Observations   []InteractionObservation `json:"observations"`
	DuplicateCount int                      `json:"duplicate_count"`
}

func (log AbsorptionLog) Validate() error {
	if log.SchemaVersion != SchemaVersion || len(log.Observations) > 10000 || log.DuplicateCount < 0 {
		return errors.New("invalid provisional absorption log")
	}
	seen := map[string]bool{}
	for _, observation := range log.Observations {
		if err := observation.Validate(); err != nil || seen[observation.ObservationID] {
			return errors.New("absorption log contains invalid or duplicate observation")
		}
		seen[observation.ObservationID] = true
	}
	return nil
}

func (log *AbsorptionLog) Append(observation InteractionObservation) (AppendResult, error) {
	if err := observation.Validate(); err != nil {
		return AppendResult{}, err
	}
	if decision := EvaluateInteraction(observation); !decision.Persistable {
		return AppendResult{}, errors.New("interaction is not a persistable authenticated owner signal: " + decision.ReasonCode)
	}
	if err := log.Validate(); err != nil && log.SchemaVersion != 0 {
		return AppendResult{}, err
	}
	if log.SchemaVersion == 0 {
		log.SchemaVersion = SchemaVersion
	}
	result := AppendResult{}
	for _, existing := range log.Observations {
		if existing.ObservationID == observation.ObservationID {
			if reflect.DeepEqual(existing, observation) {
				result.Duplicate = true
				log.DuplicateCount++
				return result, nil
			}
			return result, errors.New("observation ID replay conflicts with existing record")
		}
		if existing.Scope == observation.Scope && existing.SignalKey == observation.SignalKey && existing.ClaimDigest != observation.ClaimDigest && existing.EpisodeSHA256 != observation.EpisodeSHA256 {
			result.Contradiction = true
		}
	}
	if observation.Kind == ExplicitCorrection {
		known := map[string]bool{}
		for _, existing := range log.Observations {
			known[existing.ObservationID] = true
		}
		for _, superseded := range observation.SupersedesIDs {
			if !known[superseded] {
				return result, errors.New("correction must supersede an existing observation")
			}
		}
	}
	log.Observations = append(log.Observations, observation)
	return result, nil
}

type SelfProposalReceipt struct {
	ProposalID             string      `json:"proposal_id"`
	Kind                   string      `json:"kind"`
	Facet                  SelfSection `json:"facet"`
	EvidenceObservationIDs []string    `json:"evidence_observation_ids"`
	EvidenceSHA256         string      `json:"evidence_sha256"`
	Status                 string      `json:"status"`
	OwnerActionRequired    bool        `json:"owner_action_required"`
	MayPromoteCanonical    bool        `json:"may_promote_canonical"`
	BaseSnapshotVersion    int         `json:"base_snapshot_version"`
	BaseSnapshotSHA256     string      `json:"base_snapshot_sha256"`
	RecordedAt             time.Time   `json:"recorded_at"`
}

type MaintenanceReport struct {
	SchemaVersion              int                   `json:"schema_version"`
	SnapshotVersion            int                   `json:"snapshot_version"`
	SnapshotSHA256             string                `json:"snapshot_sha256"`
	ObservationCount           int                   `json:"observation_count"`
	DuplicateCount             int                   `json:"duplicate_count"`
	ContradictionCount         int                   `json:"contradiction_count"`
	RecheckDue                 int                   `json:"recheck_due"`
	DecayCandidates            int                   `json:"decay_candidates"`
	OwnerConfirmedSignals      int                   `json:"owner_confirmed_signals"`
	CanonicalMutationsByDarwin int                   `json:"canonical_mutations_by_darwin"`
	ReevaluationProposals      int                   `json:"reevaluation_proposals"`
	ProposalReceipts           []SelfProposalReceipt `json:"proposal_receipts"`
}

func (log AbsorptionLog) Analyze(snapshot UserSelfSnapshot, now time.Time) (MaintenanceReport, error) {
	if err := snapshot.Validate(); err != nil {
		return MaintenanceReport{}, err
	}
	if err := log.Validate(); err != nil {
		return MaintenanceReport{}, err
	}
	if now.IsZero() {
		return MaintenanceReport{}, errors.New("self maintenance time is required")
	}
	result := MaintenanceReport{SchemaVersion: SchemaVersion, SnapshotVersion: snapshot.Version, SnapshotSHA256: snapshot.Digest, ObservationCount: len(log.Observations), DuplicateCount: log.DuplicateCount, ProposalReceipts: []SelfProposalReceipt{}}
	groups := map[string][]InteractionObservation{}
	for _, observation := range log.Observations {
		key := string(observation.Scope) + ":" + observation.SignalKey
		groups[key] = append(groups[key], observation)
		if !now.Before(observation.RecheckAt) {
			result.RecheckDue++
		}
		if EffectiveConfidence(observation, now) < observation.ConfidenceBasisPts {
			result.DecayCandidates++
		}
		if observation.UserConfirmed && (observation.Kind == ExplicitInstruction || observation.Kind == ExplicitCorrection || observation.Kind == ExplicitEndorsement) {
			result.OwnerConfirmedSignals++
		}
	}
	for key, observations := range groups {
		values := map[string]bool{}
		episodes := map[string]map[string]bool{}
		ids := make([]string, 0, len(observations))
		for _, observation := range observations {
			values[observation.ClaimDigest] = true
			if episodes[observation.ClaimDigest] == nil {
				episodes[observation.ClaimDigest] = map[string]bool{}
			}
			episodes[observation.ClaimDigest][observation.EpisodeSHA256] = true
			ids = append(ids, observation.ObservationID)
		}
		claims := make([]string, 0, len(episodes))
		for claim := range episodes {
			claims = append(claims, claim)
		}
		independentContradiction := false
		for left := 0; left < len(claims) && !independentContradiction; left++ {
			for right := left + 1; right < len(claims) && !independentContradiction; right++ {
				for episode := range episodes[claims[left]] {
					if !episodes[claims[right]][episode] {
						independentContradiction = true
						break
					}
				}
			}
		}
		if len(values) > 1 && independentContradiction {
			result.ContradictionCount++
			result.ProposalReceipts = append(result.ProposalReceipts, proposal("reevaluate_facet", observations[0].Facet, ids, key, snapshot, now))
		}
	}
	for _, kind := range []string{"recheck", "confidence_decay"} {
		ids := []string{}
		for _, observation := range log.Observations {
			include := kind == "recheck" && !now.Before(observation.RecheckAt)
			include = include || kind == "confidence_decay" && EffectiveConfidence(observation, now) < observation.ConfidenceBasisPts
			if include {
				ids = append(ids, observation.ObservationID)
			}
		}
		if len(ids) > 0 {
			result.ProposalReceipts = append(result.ProposalReceipts, proposal("reevaluate_facet", log.Observations[0].Facet, ids, kind+snapshot.Digest, snapshot, now))
		}
	}
	result.ReevaluationProposals = len(result.ProposalReceipts)
	sort.Slice(result.ProposalReceipts, func(i, j int) bool {
		return result.ProposalReceipts[i].ProposalID < result.ProposalReceipts[j].ProposalID
	})
	return result, nil
}

func EffectiveConfidence(observation InteractionObservation, now time.Time) int {
	if now.Before(observation.RecheckAt) {
		return observation.ConfidenceBasisPts
	}
	days := int(now.Sub(observation.RecheckAt) / (24 * time.Hour))
	decay := (days/30 + 1) * 1000
	if decay >= observation.ConfidenceBasisPts {
		return 0
	}
	return observation.ConfidenceBasisPts - decay
}

func proposal(kind string, facet SelfSection, ids []string, salt string, snapshot UserSelfSnapshot, at time.Time) SelfProposalReceipt {
	sortedIDs := append([]string(nil), ids...)
	sort.Strings(sortedIDs)
	evidence := sha256Hex(strings.Join(append(sortedIDs, salt), "\x00"))
	return SelfProposalReceipt{ProposalID: "self-" + evidence[:32], Kind: kind, Facet: facet, EvidenceObservationIDs: sortedIDs, EvidenceSHA256: evidence, Status: "proposal_only", OwnerActionRequired: true, MayPromoteCanonical: false, BaseSnapshotVersion: snapshot.Version, BaseSnapshotSHA256: snapshot.Digest, RecordedAt: at.UTC()}
}

func sha256Hex(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

func validSection(value SelfSection) bool {
	_, ok := sectionOrder[value]
	return ok
}

var sectionOrder = map[SelfSection]int{
	SectionPreferences: 1, SectionPrinciples: 2, SectionDecisionRules: 3,
	SectionCommunication: 4, SectionMotivations: 5, SectionBoundaries: 6,
}

func validSignal(value SignalKind) bool {
	return value == ExplicitInstruction || value == ExplicitCorrection || value == ExplicitEndorsement || value == ObservedPattern || value == InferredHypothesis
}
func validConfidence(value Confidence) bool {
	return value == ConfidenceLow || value == ConfidenceMedium || value == ConfidenceHigh
}
func validSensitivity(value Sensitivity) bool {
	return value == SensitivityNormal || value == SensitivitySensitive || value == SensitivityRestricted
}
func validScope(value ObservationScope) bool {
	return value == ScopeGlobal || value == ScopeWorkspace || value == ScopeAccount || value == ScopeCase || value == ScopeInteraction || value == ScopeSkill
}
func validEvidenceType(value EvidenceType) bool {
	return value == EvidenceOwnerSpeech || value == EvidenceOwnerAction
}
func validLifecycle(value ObservationLifecycle) bool {
	return value == LifecycleCaptured || value == LifecycleEligible || value == LifecycleCorroborated || value == LifecycleProposed || value == LifecycleRejected || value == LifecycleContradicted || value == LifecycleExpired || value == LifecycleRedacted
}
func validAudience(value Audience) bool {
	return value == AudienceUser || value == AudienceInternal || value == AudienceClient || value == AudienceExecutive || value == AudienceExternal
}
func validConsequence(value Consequence) bool {
	return value == ConsequenceLow || value == ConsequenceMaterial || value == ConsequenceHigh
}
func validReversibility(value Reversibility) bool {
	return value == Reversible || value == HardToReverse
}
func validPurpose(value PurposeSatisfaction) bool {
	return value == PurposeYes || value == PurposePartial || value == PurposeNo || value == PurposeUnknown
}
func validVerdict(value IntentVerdict) bool {
	return value == VerdictApprove || value == VerdictRefine || value == VerdictClarify || value == VerdictHoldExceptional
}
func validRefinement(value RefinementKind) bool {
	return value == RefinePurpose || value == RefinePreference || value == RefineDecision || value == RefineCommunication || value == RefineReadiness || value == RefineGovernance
}
