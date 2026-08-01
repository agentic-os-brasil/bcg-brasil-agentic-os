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
	Revision        string           `json:"revision"`
	TransitionID    string           `json:"transition_id,omitempty"`
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
	ID                      string           `json:"id"`
	State                   ObservationState `json:"state"`
	RecordedAt              time.Time        `json:"recorded_at"`
	StateChangedAt          time.Time        `json:"state_changed_at"`
	CanonicalDigest         string           `json:"canonical_digest,omitempty"`
	TransitionID            string           `json:"transition_id,omitempty"`
	ExpectedState           ObservationState `json:"expected_state,omitempty"`
	ExpectedRevision        string           `json:"expected_revision,omitempty"`
	ExpectedCanonicalDigest string           `json:"expected_canonical_digest,omitempty"`
	Tombstone               bool             `json:"tombstone,omitempty"`
}

// ObservationTransitionInput is the closed CAS contract for every lifecycle
// transition. TransitionID is the caller's stable occurrence key: retries of
// the same transition are idempotent, while a stale or conflicting transition
// fails closed under the cross-process observation lock.
type ObservationTransitionInput struct {
	ObservationID           string
	TransitionID            string
	Next                    ObservationState
	ExpectedState           ObservationState
	ExpectedRevision        string
	ExpectedCanonicalDigest string
	OwnerAction             bool
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
	if (input.Signal == SignalExplicitInstruction || input.Signal == SignalExplicitCorrection) && !input.OwnerConfirmed {
		evaluation.Reason = "explicit_owner_confirmation_missing"
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
	record := observationRecord{ObservationInput: input, ID: observationID(input), State: ObservationCaptured, RecordedAt: now, StateChangedAt: now}
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

// GetObservation returns the current metadata-only receipt for one observation.
func GetObservation(root, id string) (ObservationReceipt, error) {
	if !observationIdentifier.MatchString(id) {
		return ObservationReceipt{}, errors.New("observation ID is invalid")
	}
	receipts, err := ListObservations(root)
	if err != nil {
		return ObservationReceipt{}, err
	}
	for _, receipt := range receipts {
		if receipt.ID == id {
			return receipt, nil
		}
	}
	return ObservationReceipt{}, os.ErrNotExist
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
// previous event. The full read, validation, CAS and append sequence is held
// under one cross-process lock.
func TransitionObservation(root string, input ObservationTransitionInput) (ObservationReceipt, error) {
	var receipt ObservationReceipt
	err := withObservationLock(root, func(path string) error {
		if !observationIdentifier.MatchString(input.ObservationID) || !observationIdentifier.MatchString(input.TransitionID) || input.ExpectedState == "" || !validDigest(input.ExpectedRevision) {
			return errors.New("observation transition CAS identity is invalid")
		}
		records, err := readObservationRecords(root)
		if err != nil {
			return err
		}
		for _, existing := range records {
			if existing.TransitionID != input.TransitionID {
				continue
			}
			if existing.ID != input.ObservationID || existing.State != input.Next || existing.ExpectedState != input.ExpectedState || existing.ExpectedRevision != input.ExpectedRevision || existing.ExpectedCanonicalDigest != input.ExpectedCanonicalDigest {
				return errors.New("observation transition occurrence was reused with different content")
			}
			receipt = observationReceipt(existing, "state_transition_idempotent")
			return nil
		}
		current, ok := latestObservations(records)[input.ObservationID]
		if !ok {
			return os.ErrNotExist
		}
		if current.State != input.ExpectedState || observationRevision(current) != input.ExpectedRevision {
			return ErrRevisionConflict
		}
		if err := validateTransition(current, input.Next, records); err != nil {
			return err
		}
		if input.Next == ObservationPromoted {
			if !input.OwnerAction || !current.OwnerConfirmed {
				return errors.New("self promotion requires explicit owner action")
			}
			definition, err := facetDefinition(root, current.Facet)
			if err != nil {
				return err
			}
			if definition.Refinement == "proposal_only" {
				return errors.New("this self facet is proposal-only and requires an explicit facet proposal")
			}
			if current.ScopeKind == "global" && !current.DeclassifiedGlobal {
				return errors.New("global self promotion requires explicit declassification")
			}
			canonical, err := canonicalDigestForFacet(root, current.Facet)
			if err != nil {
				return err
			}
			if input.ExpectedCanonicalDigest == "" || canonical != input.ExpectedCanonicalDigest {
				return ErrRevisionConflict
			}
			current.CanonicalDigest = canonical
		}
		current.State = input.Next
		current.TransitionID = input.TransitionID
		current.ExpectedState = input.ExpectedState
		current.ExpectedRevision = input.ExpectedRevision
		current.ExpectedCanonicalDigest = input.ExpectedCanonicalDigest
		current.Tombstone = input.Next == ObservationRedacted
		current.StateChangedAt = time.Now().UTC()
		if err := appendObservationRecord(path, current); err != nil {
			return err
		}
		receipt = observationReceipt(current, "state_transition")
		return nil
	})
	return receipt, err
}

func RejectObservation(root string, input ObservationTransitionInput) (ObservationReceipt, error) {
	if input.Next != ObservationRejected && input.Next != ObservationContradicted && input.Next != ObservationExpired && input.Next != ObservationRedacted {
		return ObservationReceipt{}, errors.New("invalid observation terminal state")
	}
	return TransitionObservation(root, input)
}

// ResetDerivedSelf redacts provisional observations through append-only
// tombstones and removes only derived snapshot projections. Promoted facets
// are canonical and therefore require the normal owner facet revert path.
func ResetDerivedSelf(root string, confirmed bool) error {
	if !confirmed {
		return ErrConfirmationRequired
	}
	return withObservationLock(root, func(path string) error {
		records, err := readObservationRecords(root)
		if err != nil {
			return err
		}
		for _, record := range latestObservations(records) {
			if record.State == ObservationPromoted {
				return errors.New("reset requires reverting promoted canonical facets first")
			}
		}
		for _, record := range latestObservations(records) {
			if record.State == ObservationRejected || record.State == ObservationContradicted || record.State == ObservationExpired || record.State == ObservationRedacted {
				continue
			}
			expectedState := record.State
			expectedRevision := observationRevision(record)
			record.State = ObservationRedacted
			record.TransitionID = "reset-" + digest(record.ID + "\x00reset")[:32]
			record.ExpectedState = expectedState
			record.ExpectedRevision = expectedRevision
			record.Tombstone = true
			record.StateChangedAt = time.Now().UTC()
			if err := appendObservationRecord(path, record); err != nil {
				return err
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
	})
}

func observationID(input ObservationInput) string {
	body, _ := json.Marshal(input)
	return "observation-" + digest(string(body))[:40]
}

func validateObservationInput(input ObservationInput) error {
	if input.SchemaVersion != 1 || !validSignal(input.Signal) || !observationClaim.MatchString(input.Claim) || !observationIdentifier.MatchString(input.SourceEvent) || !observationIdentifier.MatchString(input.EpisodeID) {
		return errors.New("self observation identity or signal is invalid")
	}
	if input.Facet != "" && (!observationIdentifier.MatchString(input.Facet) || !validObservationFacet(input.Facet)) {
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
	if input.Signal == SignalExplicitEndorsement && (strings.EqualFold(input.Claim, "ok") || strings.EqualFold(input.Claim, "okay")) {
		return errors.New("silence or generic acceptance is not explicit endorsement")
	}
	if input.EvidenceType == "generated_output" || input.EvidenceType == "client_document" || input.EvidenceType == "agent_output" {
		return errors.New("generated or client content cannot evidence the self")
	}
	return nil
}

func validObservationFacet(facet string) bool {
	switch facet {
	case "professional-role", "communication-style", "voice", "preferences", "decision-rules", "working-boundaries":
		return true
	default:
		return false
	}
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
			if record.ID != current.ID && record.EpisodeID != current.EpisodeID && record.Facet == current.Facet && record.Claim == current.Claim && record.SourceDigest != current.SourceDigest && record.State != ObservationRejected && record.State != ObservationRedacted && record.State != ObservationContradicted {
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

func withObservationLock(root string, operation func(path string) error) error {
	rootPath, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	if err := ensurePromptDirectory(rootPath, false); err != nil {
		return err
	}
	ownerPath := filepath.Join(rootPath, "owner")
	if err := ensurePromptDirectory(ownerPath, true); err != nil {
		return err
	}
	directory := filepath.Join(ownerPath, "observations")
	if err := ensurePromptDirectory(directory, true); err != nil {
		return err
	}
	lock, err := acquirePromptHistoryLock(filepath.Join(directory, promptHistoryLockName))
	if err != nil {
		return err
	}
	operationErr := operation(filepath.Join(directory, "observations.jsonl"))
	unlockErr := lock()
	if operationErr != nil {
		return operationErr
	}
	return unlockErr
}

func appendObservation(root string, record observationRecord) error {
	return appendObservationWithMode(root, record, true)
}

func appendObservationTransition(root string, record observationRecord) error {
	return appendObservationWithMode(root, record, false)
}

func appendObservationWithMode(root string, record observationRecord, idempotent bool) error {
	return withObservationLock(root, func(path string) error {
		records, err := readObservationRecords(root)
		if err != nil {
			return err
		}
		for _, existing := range records {
			if existing.ID != record.ID {
				continue
			}
			if idempotent {
				if sameObservationInput(existing.ObservationInput, record.ObservationInput) {
					return nil
				}
				return errors.New("observation occurrence was reused with different content")
			}
		}
		return appendObservationRecord(path, record)
	})
}

func appendObservationRecord(path string, record observationRecord) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	body, err := json.Marshal(record)
	if err != nil {
		_ = file.Close()
		return err
	}
	if _, err := file.Write(append(body, '\n')); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func sameObservationInput(left, right ObservationInput) bool {
	return left.SchemaVersion == right.SchemaVersion && left.Signal == right.Signal && left.Facet == right.Facet && left.Claim == right.Claim && left.EvidenceType == right.EvidenceType && left.SourceEvent == right.SourceEvent && left.SourceDigest == right.SourceDigest && left.EpisodeID == right.EpisodeID && left.ScopeKind == right.ScopeKind && left.ScopeID == right.ScopeID && left.Confidence == right.Confidence && left.Sensitivity == right.Sensitivity && left.ExpiresAt.Equal(right.ExpiresAt) && left.AuthenticatedOwner == right.AuthenticatedOwner && left.Material == right.Material && left.OwnerConfirmed == right.OwnerConfirmed && left.DeclassifiedGlobal == right.DeclassifiedGlobal
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
	return ObservationReceipt{ID: record.ID, State: record.State, Signal: record.Signal, Facet: record.Facet, ScopeKind: record.ScopeKind, ScopeID: record.ScopeID, Confidence: record.Confidence, Persisted: true, Reason: reason, CanonicalDigest: record.CanonicalDigest, Revision: observationRevision(record), TransitionID: record.TransitionID}
}

func observationRevision(record observationRecord) string {
	body, _ := json.Marshal(record)
	return digest(string(body))
}

func validDigest(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
