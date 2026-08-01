// Package selfmodel defines the local, runtime-neutral boundary between the
// Owner Context and ephemeral Walter intent review. It stores metadata and
// digests only; owner facet bodies remain in the user-local Owner Context.
package selfmodel

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	SchemaVersion           = 1
	OwnerScope              = "local_user"
	CanonicalAuthorityLevel = 3
	maxIDBytes              = 96
	maxDigestBytes          = 64
	maxEvidence             = 8
)

type SignalKind string

const (
	ExplicitInstruction SignalKind = "explicit_instruction"
	ExplicitCorrection  SignalKind = "explicit_correction"
	ExplicitEndorsement SignalKind = "explicit_endorsement"
	ObservedPattern     SignalKind = "observed_pattern"
	InferredHypothesis  SignalKind = "inferred_hypothesis"
)

// AuthorityLevel is the deterministic precedence used when a provisional
// signal is compared with another source. Current owner instruction wins over
// correction, canon, recent observations and Walter hypotheses.
func AuthorityLevel(signal SignalKind) int {
	switch signal {
	case ExplicitInstruction:
		return 5
	case ExplicitCorrection, ExplicitEndorsement:
		return 4
	case ObservedPattern:
		return 2
	case InferredHypothesis:
		return 1
	default:
		return 0
	}
}

type Confidence string

const (
	ConfidenceLow    Confidence = "low"
	ConfidenceMedium Confidence = "medium"
	ConfidenceHigh   Confidence = "high"
)

type Materiality string

const (
	MaterialityLow    Materiality = "low"
	MaterialityMedium Materiality = "medium"
	MaterialityHigh   Materiality = "high"
)

type Lifecycle string

const (
	Captured     Lifecycle = "captured"
	Eligible     Lifecycle = "eligible"
	Corroborated Lifecycle = "corroborated"
	Proposed     Lifecycle = "proposed"
	Promoted     Lifecycle = "promoted"
	Rejected     Lifecycle = "rejected"
	Contradicted Lifecycle = "contradicted"
	Expired      Lifecycle = "expired"
	Redacted     Lifecycle = "redacted"
)

// CanonicalSnapshot is only a digest projection of Owner Context facets. It
// intentionally has no facet body, claim text or client/workspace content.
type CanonicalSnapshot struct {
	SchemaVersion int               `json:"schema_version"`
	Version       int               `json:"version"`
	Digest        string            `json:"digest"`
	Scope         string            `json:"scope"`
	FacetDigests  map[string]string `json:"facet_digests"`
	UpdatedAt     time.Time         `json:"updated_at"`
}

func NewCanonicalSnapshot(version int, facetDigests map[string]string, now time.Time) (CanonicalSnapshot, error) {
	snapshot := CanonicalSnapshot{SchemaVersion: SchemaVersion, Version: version, Scope: OwnerScope, FacetDigests: cloneStrings(facetDigests), UpdatedAt: now.UTC()}
	snapshot.Digest = Digest(fmt.Sprintf("%d:%s", version, digestMap(snapshot.FacetDigests)))
	if err := ValidateSnapshot(snapshot); err != nil {
		return CanonicalSnapshot{}, err
	}
	return snapshot, nil
}

// Observation contains only the minimum evidence needed to inspect or govern
// a self signal. The actual prompt, transcript, client text and generated
// artifact are deliberately represented by digests and typed references.
type Observation struct {
	SchemaVersion                int         `json:"schema_version"`
	ObservationID                string      `json:"observation_id"`
	Signal                       SignalKind  `json:"signal"`
	Lifecycle                    Lifecycle   `json:"lifecycle"`
	SourceEvent                  string      `json:"source_event"`
	SourceEventSHA256            string      `json:"source_event_sha256"`
	OccurredAt                   time.Time   `json:"occurred_at"`
	ScopeKind                    string      `json:"scope_kind"`
	ScopeID                      string      `json:"scope_id"`
	ClaimSHA256                  string      `json:"claim_sha256"`
	EvidenceType                 string      `json:"evidence_type"`
	ProvenanceSHA256             string      `json:"provenance_sha256"`
	EvidenceRefs                 []string    `json:"evidence_refs,omitempty"`
	Confidence                   Confidence  `json:"confidence"`
	Sensitivity                  string      `json:"sensitivity"`
	ExpiresAt                    *time.Time  `json:"expires_at,omitempty"`
	RecheckAfter                 *time.Time  `json:"recheck_after,omitempty"`
	ExpressedObjectiveSHA256     string      `json:"expressed_objective_sha256,omitempty"`
	LatentIntentHypothesisSHA256 string      `json:"latent_intent_hypothesis_sha256,omitempty"`
	AlternativeDigests           []string    `json:"alternative_digests,omitempty"`
	DisconfirmationSHA256        string      `json:"disconfirmation_sha256,omitempty"`
	Materiality                  Materiality `json:"materiality"`
	OwnerAuthenticated           bool        `json:"owner_authenticated"`
	SupersedesObservationID      string      `json:"supersedes_observation_id,omitempty"`
}

// InteractionInput is evaluated after every interaction. The evaluator may
// return an ephemeral result for any interaction, but only an authenticated,
// material owner signal is eligible for append-only persistence.
type InteractionInput struct {
	ObservationID                string
	Signal                       SignalKind
	SourceEvent                  string
	SourceEventSHA256            string
	OccurredAt                   time.Time
	ScopeKind                    string
	ScopeID                      string
	ClaimSHA256                  string
	EvidenceType                 string
	ProvenanceSHA256             string
	EvidenceRefs                 []string
	Confidence                   Confidence
	Sensitivity                  string
	ExpiresAt                    *time.Time
	RecheckAfter                 *time.Time
	ExpressedObjectiveSHA256     string
	LatentIntentHypothesisSHA256 string
	AlternativeDigests           []string
	DisconfirmationSHA256        string
	Materiality                  Materiality
	OwnerAuthenticated           bool
	SupersedesObservationID      string
}

// EvaluateInteraction is independent of Walter invocation and must be called
// by Maestro after every interaction. It never promotes a signal or treats an
// intent hypothesis as a fact.
func EvaluateInteraction(input InteractionInput) (Observation, bool, error) {
	observation := Observation{
		SchemaVersion: SchemaVersion, ObservationID: input.ObservationID,
		Signal: input.Signal, Lifecycle: Captured, SourceEvent: input.SourceEvent,
		SourceEventSHA256: input.SourceEventSHA256, OccurredAt: input.OccurredAt,
		ScopeKind: input.ScopeKind, ScopeID: input.ScopeID, ClaimSHA256: input.ClaimSHA256,
		EvidenceType: input.EvidenceType, ProvenanceSHA256: input.ProvenanceSHA256,
		EvidenceRefs: append([]string(nil), input.EvidenceRefs...), Confidence: input.Confidence,
		Sensitivity: input.Sensitivity, ExpiresAt: input.ExpiresAt, RecheckAfter: input.RecheckAfter,
		ExpressedObjectiveSHA256:     input.ExpressedObjectiveSHA256,
		LatentIntentHypothesisSHA256: input.LatentIntentHypothesisSHA256,
		AlternativeDigests:           append([]string(nil), input.AlternativeDigests...),
		DisconfirmationSHA256:        input.DisconfirmationSHA256, Materiality: input.Materiality,
		OwnerAuthenticated:      input.OwnerAuthenticated,
		SupersedesObservationID: input.SupersedesObservationID,
	}
	if !input.OwnerAuthenticated {
		return observation, false, nil
	}
	if !validOwnerEvidenceType(input.EvidenceType) {
		return Observation{}, false, errors.New("self observation evidence is not an authenticated owner signal")
	}
	if err := ValidateObservation(observation); err != nil {
		return Observation{}, false, err
	}
	return observation, input.OwnerAuthenticated && input.Materiality != MaterialityLow, nil
}

func ValidateSnapshot(snapshot CanonicalSnapshot) error {
	if snapshot.SchemaVersion != SchemaVersion || snapshot.Version < 1 || snapshot.Scope != OwnerScope || !validDigest(snapshot.Digest) || len(snapshot.FacetDigests) == 0 {
		return errors.New("canonical self snapshot is invalid")
	}
	for facet, digest := range snapshot.FacetDigests {
		if !validFacet(facet) || !validDigest(digest) {
			return errors.New("canonical self snapshot contains an invalid facet digest")
		}
	}
	return nil
}

func ValidateObservation(observation Observation) error {
	if observation.SchemaVersion != SchemaVersion || !validID(observation.ObservationID) || !validSignal(observation.Signal) ||
		observation.Lifecycle != Captured || strings.TrimSpace(observation.SourceEvent) == "" ||
		!validDigest(observation.SourceEventSHA256) || observation.OccurredAt.IsZero() ||
		!validScope(observation.ScopeKind, observation.ScopeID) || !validDigest(observation.ClaimSHA256) ||
		!validOwnerEvidenceType(observation.EvidenceType) || !validDigest(observation.ProvenanceSHA256) ||
		!validConfidence(observation.Confidence) || strings.TrimSpace(observation.Sensitivity) == "" ||
		!validMateriality(observation.Materiality) || !observation.OwnerAuthenticated || len(observation.EvidenceRefs) > maxEvidence {
		return errors.New("self observation is invalid or unauthenticated")
	}
	if observation.SupersedesObservationID != "" && !validID(observation.SupersedesObservationID) {
		return errors.New("self observation supersession reference is invalid")
	}
	for _, ref := range observation.EvidenceRefs {
		if strings.TrimSpace(ref) == "" || len([]byte(ref)) > maxIDBytes {
			return errors.New("self observation evidence reference is invalid")
		}
	}
	for _, digest := range append(append([]string{}, observation.AlternativeDigests...), observation.ExpressedObjectiveSHA256, observation.LatentIntentHypothesisSHA256, observation.DisconfirmationSHA256) {
		if digest != "" && !validDigest(digest) {
			return errors.New("self observation contains an invalid digest")
		}
	}
	return nil
}

// PromotionDecision is intentionally explicit. Darwin may produce a proposal
// but cannot call Promote or mutate the canonical snapshot.
type PromotionDecision struct {
	ObservationID          string
	Facet                  string
	UserConfirmed          bool
	Episodes               int
	ExpectedSnapshotDigest string
}

func AuthorizePromotion(snapshot CanonicalSnapshot, observation Observation, decision PromotionDecision) (Lifecycle, error) {
	if err := ValidateSnapshot(snapshot); err != nil || decision.ExpectedSnapshotDigest == "" || decision.ExpectedSnapshotDigest != snapshot.Digest {
		return "", errors.New("promotion CAS does not match the current canonical self snapshot")
	}
	return PromotionLifecycle(observation, decision)
}

func PromotionLifecycle(observation Observation, decision PromotionDecision) (Lifecycle, error) {
	if err := ValidateObservation(observation); err != nil || observation.ObservationID != decision.ObservationID || !validPromotionFacet(decision.Facet) {
		return "", errors.New("promotion request is invalid")
	}
	if observation.Signal == InferredHypothesis || observation.Signal == ObservedPattern {
		if decision.Episodes < 2 || !decision.UserConfirmed {
			return Proposed, nil
		}
	}
	if decision.Facet == "professional-role" || decision.Facet == "decision-rules" {
		if !decision.UserConfirmed {
			return Proposed, nil
		}
	}
	if decision.Facet == "working-boundaries" || decision.Facet == "psychological-profile" {
		if !decision.UserConfirmed {
			return Proposed, nil
		}
	}
	if decision.Facet == "intrinsic-intent" && !decision.UserConfirmed {
		return Proposed, nil
	}
	return Promoted, nil
}

type logEvent struct {
	Kind          string       `json:"kind"`
	Observation   *Observation `json:"observation,omitempty"`
	ObservationID string       `json:"observation_id,omitempty"`
	Lifecycle     Lifecycle    `json:"lifecycle,omitempty"`
	RecordedAt    time.Time    `json:"recorded_at"`
}

// Store is a local-only append-only observation log. Redaction, rejection and
// expiry append tombstones; historical events are never rewritten.
type Store struct {
	Root string
	Now  func() time.Time
}

func (store Store) Append(observation Observation) error {
	if err := ValidateObservation(observation); err != nil {
		return err
	}
	now := time.Now().UTC()
	if store.Now != nil {
		now = store.Now().UTC()
	}
	if err := store.append(logEvent{Kind: "observation", Observation: &observation, RecordedAt: now}); err != nil {
		return err
	}
	if observation.SupersedesObservationID != "" {
		return store.Tombstone(observation.SupersedesObservationID, Contradicted)
	}
	return nil
}

func (store Store) Tombstone(observationID string, lifecycle Lifecycle) error {
	if !validID(observationID) || (lifecycle != Rejected && lifecycle != Contradicted && lifecycle != Expired && lifecycle != Redacted) {
		return errors.New("invalid observation tombstone")
	}
	now := time.Now().UTC()
	if store.Now != nil {
		now = store.Now().UTC()
	}
	return store.append(logEvent{Kind: "tombstone", ObservationID: observationID, Lifecycle: lifecycle, RecordedAt: now})
}

func (store Store) List() ([]Observation, error) {
	file, err := os.Open(store.logPath())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var events []logEvent
	decoder := json.NewDecoder(file)
	for {
		var event logEvent
		if err := decoder.Decode(&event); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}
		events = append(events, event)
	}
	states := make(map[string]Lifecycle)
	result := make(map[string]Observation)
	for _, event := range events {
		if event.Kind == "observation" && event.Observation != nil {
			result[event.Observation.ObservationID] = *event.Observation
			states[event.Observation.ObservationID] = event.Observation.Lifecycle
		}
		if event.Kind == "tombstone" {
			states[event.ObservationID] = event.Lifecycle
		}
	}
	ids := make([]string, 0, len(result))
	for id := range result {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	observations := make([]Observation, 0, len(ids))
	for _, id := range ids {
		observation := result[id]
		if lifecycle := states[id]; lifecycle == Rejected || lifecycle == Contradicted || lifecycle == Expired || lifecycle == Redacted {
			continue
		}
		observations = append(observations, observation)
	}
	return observations, nil
}

func (store Store) append(event logEvent) error {
	if strings.TrimSpace(store.Root) == "" {
		return errors.New("self observation store root is required")
	}
	if err := os.MkdirAll(store.Root, 0o700); err != nil {
		return err
	}
	file, err := os.OpenFile(store.logPath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	return json.NewEncoder(file).Encode(event)
}

func (store Store) logPath() string { return filepath.Join(store.Root, "observations.jsonl") }

func Digest(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

func validID(value string) bool {
	return strings.TrimSpace(value) != "" && len([]byte(value)) <= maxIDBytes
}
func validDigest(value string) bool {
	return len(value) == maxDigestBytes && strings.Trim(value, "0123456789abcdef") == ""
}
func validFacet(value string) bool {
	switch value {
	case "professional-role", "communication-style", "voice", "preferences", "decision-rules", "working-boundaries", "psychological-profile":
		return true
	default:
		return false
	}
}
func validPromotionFacet(value string) bool { return validFacet(value) || value == "intrinsic-intent" }
func validSignal(value SignalKind) bool {
	return value == ExplicitInstruction || value == ExplicitCorrection || value == ExplicitEndorsement || value == ObservedPattern || value == InferredHypothesis
}
func validConfidence(value Confidence) bool {
	return value == ConfidenceLow || value == ConfidenceMedium || value == ConfidenceHigh
}
func validMateriality(value Materiality) bool {
	return value == MaterialityLow || value == MaterialityMedium || value == MaterialityHigh
}
func validOwnerEvidenceType(value string) bool {
	switch value {
	case "owner_speech", "owner_action", "owner_feedback", "owner_instruction", "owner_correction", "owner_endorsement":
		return true
	default:
		return false
	}
}
func validScope(kind, id string) bool {
	if id == "" || len([]byte(id)) > maxIDBytes {
		return false
	}
	switch kind {
	case "global":
		return id == OwnerScope
	case "workspace", "account", "case":
		return true
	default:
		return false
	}
}

func cloneStrings(source map[string]string) map[string]string {
	copy := make(map[string]string, len(source))
	for key, value := range source {
		copy[key] = value
	}
	return copy
}

func digestMap(source map[string]string) string {
	keys := make([]string, 0, len(source))
	for key := range source {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var builder strings.Builder
	for _, key := range keys {
		builder.WriteString(key)
		builder.WriteByte('=')
		builder.WriteString(source[key])
		builder.WriteByte('\n')
	}
	return builder.String()
}

func (observation Observation) String() string {
	return fmt.Sprintf("%s:%s", observation.Signal, observation.ObservationID)
}
