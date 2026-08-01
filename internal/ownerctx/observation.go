package ownerctx

import (
	"bufio"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// SignalClass is deliberately closed. An inferred hypothesis is useful to a
// task-local evaluator but can never be promoted from this log by itself.
type SignalClass string

const (
	SignalExplicitInstruction SignalClass = "explicit_instruction"
	SignalExplicitCorrection  SignalClass = "explicit_correction"
	SignalExplicitEndorsement SignalClass = "explicit_endorsement"
	SignalObservedPattern     SignalClass = "observed_pattern"
	SignalInferredHypothesis  SignalClass = "inferred_hypothesis"
)

type ObservationState string

const (
	ObservationCaptured     ObservationState = "captured"
	ObservationEligible     ObservationState = "eligible"
	ObservationCorroborated ObservationState = "corroborated"
	ObservationProposed     ObservationState = "proposed"
	ObservationPromoted     ObservationState = "promoted"
	ObservationRejected     ObservationState = "rejected"
	ObservationContradicted ObservationState = "contradicted"
	ObservationExpired      ObservationState = "expired"
	ObservationRedacted     ObservationState = "redacted"
)

var observationIdentifier = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,127}$`)
var observationClaim = regexp.MustCompile(`^[a-z0-9][a-z0-9_.:-]{0,127}$`)

// ObservationInput is supplied by Maestro after every interaction. It is a
// metadata contract: Claim is a short normalized code, never a prompt,
// transcript, document excerpt or generated output.
type ObservationInput struct {
	SchemaVersion      int         `json:"schema_version"`
	Signal             SignalClass `json:"signal"`
	Facet              string      `json:"facet,omitempty"`
	Claim              string      `json:"claim"`
	EvidenceType       string      `json:"evidence_type"`
	SourceEvent        string      `json:"source_event"`
	SourceDigest       string      `json:"source_digest"`
	EpisodeID          string      `json:"episode_id"`
	ScopeKind          string      `json:"scope_kind"`
	ScopeID            string      `json:"scope_id"`
	Confidence         float64     `json:"confidence"`
	Sensitivity        string      `json:"sensitivity"`
	ExpiresAt          time.Time   `json:"expires_at"`
	AuthenticatedOwner bool        `json:"authenticated_owner"`
	Material           bool        `json:"material"`
	OwnerConfirmed     bool        `json:"owner_confirmed"`
	DeclassifiedGlobal bool        `json:"declassified_global"`
}

type InteractionEvaluation struct {
	Evaluated bool             `json:"evaluated"`
	Persist   bool             `json:"persist"`
	Reason    string           `json:"reason"`
	State     ObservationState `json:"state"`
}

type ObservationReceipt struct {
	ID              string           `json:"id"`
	State           ObservationState `json:"state"`
	Signal          SignalClass      `json:"signal"`
	Facet           string           `json:"facet,omitempty"`
	ScopeKind       string           `json:"scope_kind"`
	ScopeID         string           `json:"scope_id"`
	Confidence      float64          `json:"confidence"`
	Persisted       bool             `json:"persisted"`
	Reason          string           `json:"reason,omitempty"`
	CanonicalDigest string           `json:"canonical_digest,omitempty"`
}

// ObservationMetadataReport is the only observation surface exposed to a
// metadata-only Darwin analysis. It contains counts, digests and facet IDs,
// never claim text or source content.
type ObservationMetadataReport struct {
	Total                  int            `json:"total"`
	ByState                map[string]int `json:"by_state"`
	DuplicateSourceDigests int            `json:"duplicate_source_digests"`
	ContradictionCount     int            `json:"contradiction_count"`
	ExpiringWithin         int            `json:"expiring_within"`
	ReevaluateFacets       []string       `json:"reevaluate_facets,omitempty"`
}

type observationRecord struct {
	ObservationInput
	ID              string           `json:"id"`
	State           ObservationState `json:"state"`
	RecordedAt      time.Time        `json:"recorded_at"`
	StateChangedAt  time.Time        `json:"state_changed_at"`
	CanonicalDigest string           `json:"canonical_digest,omitempty"`
	Tombstone       bool             `json:"tombstone,omitempty"`
}

// EvaluateInteraction runs for every interaction. Only an authenticated,
// material owner speech/action can be persisted; the evaluator itself is
// intentionally side-effect free.
func EvaluateInteraction(input ObservationInput) (InteractionEvaluation, error) {
	if err := validateObservationInput(input); err != nil {
		return InteractionEvaluation{}, err
	}
	evaluation := InteractionEvaluation{Evaluated: true, State: ObservationCaptured}
	if !input.AuthenticatedOwner {
		evaluation.Reason = "owner_attestation_missing"
		return evaluation, nil
	}
	if !input.Material {
		evaluation.Reason = "signal_not_material"
		return evaluation, nil
	}
	if input.Signal == SignalInferredHypothesis {
		evaluation.Reason = "hypothesis_is_task_local"
		return evaluation, nil
	}
	if input.Signal == SignalExplicitEndorsement && (input.Claim == "ok" || input.Claim == "okay" || !input.OwnerConfirmed) {
		evaluation.Reason = "generic_acknowledgement_is_not_endorsement"
		return evaluation, nil
	}
	evaluation.Persist = true
	evaluation.Reason = "material_owner_attested_signal"
	return evaluation, nil
}

// AppendObservation evaluates first and writes only when the contract says
// Persist. Non-persisted interactions remain represented by the returned
// evaluation, not by a durable self claim.
func AppendObservation(root string, input ObservationInput) (ObservationReceipt, InteractionEvaluation, error) {
	evaluation, err := EvaluateInteraction(input)
	if err != nil {
		return ObservationReceipt{}, evaluation, err
	}
	if !evaluation.Persist {
		return ObservationReceipt{State: evaluation.State, Signal: input.Signal, Facet: input.Facet, ScopeKind: input.ScopeKind, ScopeID: input.ScopeID, Confidence: input.Confidence, Persisted: false, Reason: evaluation.Reason}, evaluation, nil
	}
	now := time.Now().UTC()
	record := observationRecord{ObservationInput: input, ID: observationID(input, now), State: ObservationCaptured, RecordedAt: now, StateChangedAt: now}
	if err := appendObservation(root, record); err != nil {
		return ObservationReceipt{}, evaluation, err
	}
	return observationReceipt(record, evaluation.Reason), evaluation, nil
}

func ListObservations(root string) ([]ObservationReceipt, error) {
	records, err := readObservationRecords(root)
	if err != nil {
		return nil, err
	}
	latest := latestObservations(records)
	result := make([]ObservationReceipt, 0, len(latest))
	for _, record := range latest {
		result = append(result, observationReceipt(record, ""))
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}

// AnalyzeObservationMetadata gives Darwin enough signal to propose review of
// an observation facet without giving it semantic authority over the self.
func AnalyzeObservationMetadata(root string, now time.Time) (ObservationMetadataReport, error) {
	records, err := readObservationRecords(root)
	if err != nil {
		return ObservationMetadataReport{}, err
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	report := ObservationMetadataReport{ByState: map[string]int{}}
	digests := map[string]int{}
	facets := map[string]bool{}
	for _, record := range latestObservations(records) {
		report.Total++
		report.ByState[string(record.State)]++
		digests[record.SourceDigest]++
		if record.State == ObservationContradicted {
			report.ContradictionCount++
			facets[record.Facet] = true
		}
		if record.ExpiresAt.Sub(now) <= 24*time.Hour && record.ExpiresAt.After(now) {
			report.ExpiringWithin++
			facets[record.Facet] = true
		}
	}
	for _, count := range digests {
		if count > 1 {
			report.DuplicateSourceDigests++
		}
	}
	for facet := range facets {
		if facet != "" {
			report.ReevaluateFacets = append(report.ReevaluateFacets, facet)
		}
	}
	sort.Strings(report.ReevaluateFacets)
	return report, nil
}

// TransitionObservation appends a new state record and never rewrites the
// previous event. expectedCanonicalDigest is the CAS token for promotion.
func TransitionObservation(root, id string, next ObservationState, expectedCanonicalDigest string, ownerAction bool) (ObservationReceipt, error) {
	records, err := readObservationRecords(root)
	if err != nil {
		return ObservationReceipt{}, err
	}
	current, ok := latestObservations(records)[id]
	if !ok {
		return ObservationReceipt{}, os.ErrNotExist
	}
	if err := validateTransition(current, next, records); err != nil {
		return ObservationReceipt{}, err
	}
	if next == ObservationPromoted {
		if !ownerAction || !current.OwnerConfirmed {
			return ObservationReceipt{}, errors.New("self promotion requires explicit owner action")
		}
		definition, err := facetDefinition(root, current.Facet)
		if err != nil {
			return ObservationReceipt{}, err
		}
		if definition.Refinement == "proposal_only" {
			return ObservationReceipt{}, errors.New("this self facet is proposal-only and requires an explicit facet proposal")
		}
		if current.ScopeKind == "global" && !current.DeclassifiedGlobal {
			return ObservationReceipt{}, errors.New("global self promotion requires explicit declassification")
		}
		canonical, err := canonicalDigestForFacet(root, current.Facet)
		if err != nil {
			return ObservationReceipt{}, err
		}
		if expectedCanonicalDigest == "" || canonical != expectedCanonicalDigest {
			return ObservationReceipt{}, ErrRevisionConflict
		}
		current.CanonicalDigest = canonical
	}
	now := time.Now().UTC()
	current.State = next
	current.Tombstone = next == ObservationRedacted
	current.StateChangedAt = now
	if err := appendObservation(root, current); err != nil {
		return ObservationReceipt{}, err
	}
	return observationReceipt(current, "state_transition"), nil
}

func RejectObservation(root, id string, state ObservationState, ownerAction bool) (ObservationReceipt, error) {
	if state != ObservationRejected && state != ObservationContradicted && state != ObservationExpired && state != ObservationRedacted {
		return ObservationReceipt{}, errors.New("invalid observation terminal state")
	}
	return TransitionObservation(root, id, state, "", ownerAction)
}

// ResetDerivedSelf redacts provisional observations through append-only
// tombstones and removes only derived snapshot projections. Promoted facets
// are canonical and therefore require the normal owner facet revert path.
func ResetDerivedSelf(root string, confirmed bool) error {
	if !confirmed {
		return ErrConfirmationRequired
	}
	records, err := readObservationRecords(root)
	if err != nil {
		return err
	}
	for id, record := range latestObservations(records) {
		if record.State == ObservationPromoted {
			return errors.New("reset requires reverting promoted canonical facets first")
		}
		if record.State != ObservationRejected && record.State != ObservationContradicted && record.State != ObservationExpired && record.State != ObservationRedacted {
			if _, err := RejectObservation(root, id, ObservationRedacted, true); err != nil {
				return err
			}
		}
	}
	projectionRoot := filepath.Join(root, "owner", "self", "projections")
	entries, err := os.ReadDir(projectionRoot)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		if err := os.Remove(filepath.Join(projectionRoot, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func observationID(input ObservationInput, now time.Time) string {
	return now.Format("20060102T150405.000000000Z") + "-" + input.SourceDigest[:12] + "-" + input.Claim
}

func validateObservationInput(input ObservationInput) error {
	if input.SchemaVersion != 1 || !validSignal(input.Signal) || !observationClaim.MatchString(input.Claim) || !observationIdentifier.MatchString(input.SourceEvent) || !observationIdentifier.MatchString(input.EpisodeID) {
		return errors.New("self observation identity or signal is invalid")
	}
	if input.Facet != "" && !observationIdentifier.MatchString(input.Facet) {
		return errors.New("self observation facet is invalid")
	}
	if !validDigest(input.SourceDigest) || input.Confidence < 0 || input.Confidence > 1 || input.ExpiresAt.IsZero() {
		return errors.New("self observation provenance, confidence or expiry is invalid")
	}
	if input.ScopeID == "" || !validScope(input.ScopeKind) || !observationIdentifier.MatchString(input.ScopeID) {
		return errors.New("self observation scope is invalid")
	}
	if input.Sensitivity != "professional" && input.Sensitivity != "sensitive" && input.Sensitivity != "restricted" {
		return errors.New("self observation sensitivity is invalid")
	}
	if input.Signal == SignalExplicitEndorsement && strings.EqualFold(input.Claim, "ok") {
		return errors.New("silence or generic acceptance is not explicit endorsement")
	}
	if input.EvidenceType == "generated_output" || input.EvidenceType == "client_document" || input.EvidenceType == "agent_output" {
		return errors.New("generated or client content cannot evidence the self")
	}
	return nil
}

func validSignal(signal SignalClass) bool {
	switch signal {
	case SignalExplicitInstruction, SignalExplicitCorrection, SignalExplicitEndorsement, SignalObservedPattern, SignalInferredHypothesis:
		return true
	default:
		return false
	}
}

func validScope(scope string) bool {
	switch scope {
	case "global", "workspace", "account", "case":
		return true
	default:
		return false
	}
}

func validateTransition(current observationRecord, next ObservationState, records []observationRecord) error {
	switch next {
	case ObservationEligible:
		if current.State != ObservationCaptured || time.Now().UTC().After(current.ExpiresAt) {
			return errors.New("observation is not eligible")
		}
	case ObservationCorroborated:
		if current.State != ObservationEligible && current.State != ObservationCorroborated {
			return errors.New("observation must be eligible before corroboration")
		}
		episodes := map[string]bool{}
		for _, record := range records {
			if record.ID != current.ID && record.Facet == current.Facet && record.Claim == current.Claim && record.SourceDigest != current.SourceDigest && record.State != ObservationRejected && record.State != ObservationRedacted && record.State != ObservationContradicted {
				episodes[record.EpisodeID] = true
			}
		}
		if len(episodes) == 0 {
			return errors.New("corroboration requires an independent episode")
		}
	case ObservationProposed:
		if current.State != ObservationCorroborated && !(current.Signal == SignalExplicitInstruction || current.Signal == SignalExplicitCorrection || current.Signal == SignalExplicitEndorsement) {
			return errors.New("observation is not eligible for promotion proposal")
		}
	case ObservationPromoted:
		if current.State != ObservationProposed && current.State != ObservationCorroborated {
			return errors.New("observation is not ready for promotion")
		}
	case ObservationRejected, ObservationContradicted, ObservationExpired, ObservationRedacted:
		if current.State == ObservationPromoted || current.State == ObservationRedacted {
			return errors.New("observation is already terminal")
		}
	default:
		return errors.New("unknown observation lifecycle state")
	}
	return nil
}

func canonicalDigestForFacet(root, facet string) (string, error) {
	definition, err := facetDefinition(root, facet)
	if err != nil {
		return "", err
	}
	body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(definition.Path)))
	if err != nil {
		return "", err
	}
	return digest(string(body)), nil
}

func appendObservation(root string, record observationRecord) error {
	path := filepath.Join(root, "owner", "observations", "observations.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	body, err := json.Marshal(record)
	if err != nil {
		return err
	}
	if _, err := file.Write(append(body, '\n')); err != nil {
		return err
	}
	return file.Sync()
}

func readObservationRecords(root string) ([]observationRecord, error) {
	path := filepath.Join(root, "owner", "observations", "observations.jsonl")
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var records []observationRecord
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var record observationRecord
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			return nil, err
		}
		record.SourceDigest = strings.ToLower(record.SourceDigest)
		record.ID = strings.TrimSpace(record.ID)
		records = append(records, record)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return records, nil
}

func latestObservations(records []observationRecord) map[string]observationRecord {
	latest := make(map[string]observationRecord)
	for _, record := range records {
		latest[record.ID] = record
	}
	return latest
}

func observationReceipt(record observationRecord, reason string) ObservationReceipt {
	return ObservationReceipt{ID: record.ID, State: record.State, Signal: record.Signal, Facet: record.Facet, ScopeKind: record.ScopeKind, ScopeID: record.ScopeID, Confidence: record.Confidence, Persisted: true, Reason: reason, CanonicalDigest: record.CanonicalDigest}
}

func validDigest(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
