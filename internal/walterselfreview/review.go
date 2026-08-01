// Package walterselfreview defines Walter's weekly, proposal-only self review.
// It is deliberately independent of the scheduler: a qualified runtime may
// invoke the typed handler, but no scheduler/catalog surface is activated here.
package walterselfreview

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/selfmodel"
)

const (
	SchemaVersion       = 1
	MaxPromptEntries    = 8
	MaxPromptBytes      = 2000
	MaxObservations     = 32
	MaxEvidenceEpisodes = 16
)

var ErrUnavailable = errors.New("Walter weekly self review is unavailable without an approved model adapter and authority")
var ErrProposalOnly = errors.New("Walter self review produces proposals only; canonical mutation belongs to Owner Context policy")

type ReceiptState string

const (
	ReceiptProposed    ReceiptState = "proposed"
	ReceiptDuplicate   ReceiptState = "duplicate"
	ReceiptUnavailable ReceiptState = "unavailable"
	ReceiptRejected    ReceiptState = "rejected"
)

type PromptWindowEntry struct {
	Sequence          int       `json:"sequence"`
	OriginalText      string    `json:"original_text"`
	OriginalSHA256    string    `json:"original_sha256"`
	WorkingText       string    `json:"working_text"`
	WorkingSHA256     string    `json:"working_sha256"`
	SourceEventSHA256 string    `json:"source_event_sha256"`
	OccurredAt        time.Time `json:"occurred_at"`
	Current           bool      `json:"current"`
}

type PromptWindow struct {
	SchemaVersion int                 `json:"schema_version"`
	Entries       []PromptWindowEntry `json:"entries"`
}

type EvidenceEpisode struct {
	ObservationID     string                `json:"observation_id"`
	SourceEventSHA256 string                `json:"source_event_sha256"`
	ClaimSHA256       string                `json:"claim_sha256"`
	ScopeKind         string                `json:"scope_kind"`
	ScopeID           string                `json:"scope_id"`
	Confidence        selfmodel.Confidence  `json:"confidence"`
	Sensitivity       string                `json:"sensitivity"`
	Materiality       selfmodel.Materiality `json:"materiality"`
}

type Request struct {
	SchemaVersion     int                         `json:"schema_version"`
	WeekID            string                      `json:"week_id"`
	PromptWindow      PromptWindow                `json:"prompt_window"`
	Observations      []selfmodel.Observation     `json:"observations"`
	CanonicalSnapshot selfmodel.CanonicalSnapshot `json:"canonical_snapshot"`
}

// InferenceInput keeps current prompt separate from historical evidence. A
// model adapter receives typed fields, never a concatenated instruction blob;
// historical text is evidence and cannot become an instruction by position.
type InferenceInput struct {
	CurrentPrompt        string
	HistoryEvidence      []PromptEvidence
	MaterialObservations []EvidenceEpisode
	CanonicalSnapshot    selfmodel.CanonicalSnapshot
}

type PromptEvidence struct {
	Sequence       int
	WorkingText    string
	OriginalSHA256 string
	WorkingSHA256  string
	OccurredAt     time.Time
	Instructional  bool
}

type ModelAdapter interface {
	ID() string
	Review(context.Context, InferenceInput) (SelfRefinementProposal, error)
}

type Authority interface {
	ID() string
	Approved(scope string) bool
}

type ConfirmationRequirement string

const (
	ConfirmationAutomaticWithAudit ConfirmationRequirement = "owner_confirmation_before_automatic_audit"
	ConfirmationProposalOnly       ConfirmationRequirement = "owner_confirmation_proposal_only"
	ConfirmationExplicit           ConfirmationRequirement = "explicit_owner_confirmation"
)

type SelfRefinementProposal struct {
	SchemaVersion            int                     `json:"schema_version"`
	ProposalID               string                  `json:"proposal_id"`
	State                    string                  `json:"state"`
	WeekID                   string                  `json:"week_id"`
	Facet                    string                  `json:"facet"`
	PriorClaim               string                  `json:"prior_claim"`
	PriorClaimSHA256         string                  `json:"prior_claim_sha256"`
	ProposedRefinement       string                  `json:"proposed_refinement"`
	ProposedRefinementSHA256 string                  `json:"proposed_refinement_sha256"`
	EvidenceEpisodes         []EvidenceEpisode       `json:"evidence_episodes"`
	Confidence               selfmodel.Confidence    `json:"confidence"`
	Sensitivity              string                  `json:"sensitivity"`
	ConfirmationRequirement  ConfirmationRequirement `json:"confirmation_requirement"`
	CanonicalSnapshotVersion int                     `json:"canonical_snapshot_version"`
	CanonicalSnapshotSHA256  string                  `json:"canonical_snapshot_sha256"`
	PromptWindowSHA256       string                  `json:"prompt_window_sha256"`
	InputSHA256              string                  `json:"input_sha256"`
	IntentHypothesisSHA256   string                  `json:"intent_hypothesis_sha256"`
}

// Receipt is metadata-only. The proposal text, prompt text and observation
// bodies are never copied into this durable surface.
type Receipt struct {
	SchemaVersion            int          `json:"schema_version"`
	ReceiptID                string       `json:"receipt_id"`
	State                    ReceiptState `json:"state"`
	ReasonCode               string       `json:"reason_code,omitempty"`
	WeekID                   string       `json:"week_id"`
	IdempotencyKey           string       `json:"idempotency_key"`
	InputSHA256              string       `json:"input_sha256"`
	PromptWindowSHA256       string       `json:"prompt_window_sha256"`
	CanonicalSnapshotVersion int          `json:"canonical_snapshot_version"`
	CanonicalSnapshotSHA256  string       `json:"canonical_snapshot_sha256"`
	ObservationDigests       []string     `json:"observation_digests,omitempty"`
	ProposalSHA256           string       `json:"proposal_sha256,omitempty"`
	AdapterID                string       `json:"adapter_id,omitempty"`
	AuthorityID              string       `json:"authority_id,omitempty"`
	RecordedAt               time.Time    `json:"recorded_at"`
}

func ValidateRequest(request Request) error {
	if request.SchemaVersion != SchemaVersion || !validWeekID(request.WeekID) || len(request.Observations) > MaxObservations {
		return errors.New("Walter weekly self-review request is invalid")
	}
	if err := ValidatePromptWindow(request.PromptWindow); err != nil {
		return err
	}
	if err := selfmodel.ValidateSnapshot(request.CanonicalSnapshot); err != nil {
		return err
	}
	for _, observation := range request.Observations {
		if err := selfmodel.ValidateObservation(observation); err != nil {
			return err
		}
		if observation.Materiality == selfmodel.MaterialityLow {
			return errors.New("Walter weekly self review accepts material observations only")
		}
	}
	return nil
}

func ValidatePromptWindow(window PromptWindow) error {
	if window.SchemaVersion != SchemaVersion || len(window.Entries) == 0 || len(window.Entries) > MaxPromptEntries {
		return errors.New("Walter prompt window is outside its bounded contract")
	}
	current := 0
	for index, entry := range window.Entries {
		if entry.Sequence != index+1 || strings.TrimSpace(entry.OriginalText) == "" || strings.TrimSpace(entry.WorkingText) == "" ||
			len([]byte(entry.OriginalText)) > MaxPromptBytes || len([]byte(entry.WorkingText)) > MaxPromptBytes ||
			entry.OriginalSHA256 != Digest(entry.OriginalText) || entry.WorkingSHA256 != Digest(entry.WorkingText) ||
			!validSHA256(entry.SourceEventSHA256) || entry.OccurredAt.IsZero() {
			return errors.New("Walter prompt window must preserve and digest-bind original and working text before inference")
		}
		if entry.Current {
			current++
		}
	}
	if current != 1 || !window.Entries[len(window.Entries)-1].Current {
		return errors.New("Walter prompt window requires exactly one current prompt at the end")
	}
	return nil
}

func BuildInferenceInput(request Request) (InferenceInput, error) {
	if err := ValidateRequest(request); err != nil {
		return InferenceInput{}, err
	}
	input := InferenceInput{CanonicalSnapshot: request.CanonicalSnapshot}
	for _, entry := range request.PromptWindow.Entries {
		if entry.Current {
			input.CurrentPrompt = entry.WorkingText
			continue
		}
		input.HistoryEvidence = append(input.HistoryEvidence, PromptEvidence{
			Sequence: entry.Sequence, WorkingText: entry.WorkingText, OriginalSHA256: entry.OriginalSHA256,
			WorkingSHA256: entry.WorkingSHA256, OccurredAt: entry.OccurredAt, Instructional: false,
		})
	}
	for _, observation := range request.Observations {
		input.MaterialObservations = append(input.MaterialObservations, EvidenceEpisode{
			ObservationID: observation.ObservationID, SourceEventSHA256: observation.SourceEventSHA256,
			ClaimSHA256: observation.ClaimSHA256, ScopeKind: observation.ScopeKind, ScopeID: observation.ScopeID,
			Confidence: observation.Confidence, Sensitivity: observation.Sensitivity, Materiality: observation.Materiality,
		})
	}
	return input, nil
}

func Review(ctx context.Context, request Request, adapter ModelAdapter, authority Authority, store *ReceiptStore, now time.Time) (SelfRefinementProposal, Receipt, error) {
	input, err := BuildInferenceInput(request)
	if err != nil {
		return SelfRefinementProposal{}, rejectedReceipt(request, err.Error(), now), err
	}
	receipt := baseReceipt(request, now)
	if adapter == nil || authority == nil || !authority.Approved("walter/self-review") {
		receipt.State = ReceiptUnavailable
		receipt.ReasonCode = "approved_model_adapter_or_authority_unavailable"
		return SelfRefinementProposal{}, receipt, ErrUnavailable
	}
	if store != nil {
		if previous, found, lookupErr := store.Lookup(receipt.IdempotencyKey); lookupErr != nil {
			return SelfRefinementProposal{}, receipt, lookupErr
		} else if found {
			previous.State = ReceiptDuplicate
			return SelfRefinementProposal{}, previous, nil
		}
	}
	proposal, err := adapter.Review(ctx, input)
	if err != nil {
		receipt.State = ReceiptRejected
		receipt.ReasonCode = "model_adapter_error"
		return SelfRefinementProposal{}, receipt, err
	}
	if err := ValidateProposal(request, proposal); err != nil {
		receipt.State = ReceiptRejected
		receipt.ReasonCode = "proposal_invalid_or_stale"
		return SelfRefinementProposal{}, receipt, err
	}
	receipt.State = ReceiptProposed
	receipt.AdapterID = adapter.ID()
	receipt.AuthorityID = authority.ID()
	receipt.ProposalSHA256 = DigestJSON(proposal)
	if store != nil {
		if err := store.Append(receipt); err != nil {
			return SelfRefinementProposal{}, receipt, err
		}
	}
	return proposal, receipt, nil
}

func ValidateProposal(request Request, proposal SelfRefinementProposal) error {
	if err := ValidateRequest(request); err != nil {
		return err
	}
	if proposal.SchemaVersion != SchemaVersion || proposal.State != "proposed" || strings.TrimSpace(proposal.ProposalID) == "" ||
		!validFacet(proposal.Facet) || strings.TrimSpace(proposal.PriorClaim) == "" || strings.TrimSpace(proposal.ProposedRefinement) == "" ||
		len([]byte(proposal.PriorClaim)) > MaxPromptBytes || len([]byte(proposal.ProposedRefinement)) > MaxPromptBytes ||
		proposal.PriorClaimSHA256 != Digest(proposal.PriorClaim) || proposal.ProposedRefinementSHA256 != Digest(proposal.ProposedRefinement) ||
		proposal.WeekID != request.WeekID || proposal.CanonicalSnapshotVersion != request.CanonicalSnapshot.Version ||
		proposal.CanonicalSnapshotSHA256 != request.CanonicalSnapshot.Digest || proposal.PromptWindowSHA256 != PromptWindowDigest(request.PromptWindow) ||
		proposal.InputSHA256 != RequestDigest(request) || !validSHA256(proposal.IntentHypothesisSHA256) || !validConfidence(proposal.Confidence) || strings.TrimSpace(proposal.Sensitivity) == "" ||
		!validConfirmation(proposal.ConfirmationRequirement) || len(proposal.EvidenceEpisodes) == 0 || len(proposal.EvidenceEpisodes) > MaxEvidenceEpisodes {
		return errors.New("Walter self-refinement proposal is incomplete or stale")
	}
	available := make(map[string]selfmodel.Observation, len(request.Observations))
	for _, observation := range request.Observations {
		available[observation.ObservationID] = observation
	}
	for _, episode := range proposal.EvidenceEpisodes {
		observation, ok := available[episode.ObservationID]
		if !ok || episode.SourceEventSHA256 != observation.SourceEventSHA256 || episode.ClaimSHA256 != observation.ClaimSHA256 || episode.ScopeKind != observation.ScopeKind || episode.ScopeID != observation.ScopeID || episode.Confidence != observation.Confidence || episode.Sensitivity != observation.Sensitivity || episode.Materiality != observation.Materiality {
			return errors.New("Walter proposal contains an unbound evidence episode")
		}
	}
	if proposal.Facet == "working-boundaries" || proposal.Facet == "psychological-profile" || proposal.Facet == "intrinsic-intent" {
		if proposal.ConfirmationRequirement != ConfirmationExplicit {
			return errors.New("sensitive self facet or motive claim requires explicit owner confirmation")
		}
	}
	return nil
}

func ApplyCanonicalMutation(SelfRefinementProposal, selfmodel.CanonicalSnapshot) error {
	return ErrProposalOnly
}

type ReceiptStore struct {
	Root string
	Now  func() time.Time
}

func (store ReceiptStore) Append(receipt Receipt) error {
	if receipt.SchemaVersion != SchemaVersion || strings.TrimSpace(store.Root) == "" {
		return errors.New("Walter self-review receipt store is invalid")
	}
	if err := os.MkdirAll(store.Root, 0o700); err != nil {
		return err
	}
	file, err := os.OpenFile(filepath.Join(store.Root, "weekly-receipts.jsonl"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	return json.NewEncoder(file).Encode(receipt)
}

func (store ReceiptStore) Lookup(key string) (Receipt, bool, error) {
	file, err := os.Open(filepath.Join(store.Root, "weekly-receipts.jsonl"))
	if errors.Is(err, os.ErrNotExist) {
		return Receipt{}, false, nil
	}
	if err != nil {
		return Receipt{}, false, err
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	for {
		var receipt Receipt
		if err := decoder.Decode(&receipt); err != nil {
			if errors.Is(err, io.EOF) {
				return Receipt{}, false, nil
			}
			return Receipt{}, false, err
		}
		if receipt.IdempotencyKey == key {
			return receipt, true, nil
		}
	}
}

func baseReceipt(request Request, now time.Time) Receipt {
	return Receipt{SchemaVersion: SchemaVersion, ReceiptID: Digest(request.WeekID + ":" + RequestDigest(request)), State: ReceiptRejected,
		WeekID: request.WeekID, IdempotencyKey: request.WeekID + ":" + RequestDigest(request), InputSHA256: RequestDigest(request),
		PromptWindowSHA256: PromptWindowDigest(request.PromptWindow), CanonicalSnapshotVersion: request.CanonicalSnapshot.Version,
		CanonicalSnapshotSHA256: request.CanonicalSnapshot.Digest, ObservationDigests: observationDigests(request.Observations), RecordedAt: now.UTC()}
}

func rejectedReceipt(request Request, reason string, now time.Time) Receipt {
	receipt := baseReceipt(request, now)
	receipt.ReasonCode = reason
	return receipt
}

func PromptWindowDigest(window PromptWindow) string { return DigestJSON(window) }
func RequestDigest(request Request) string          { return DigestJSON(request) }
func Digest(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}
func DigestJSON(value any) string { body, _ := json.Marshal(value); return Digest(string(body)) }

func observationDigests(observations []selfmodel.Observation) []string {
	digests := make([]string, 0, len(observations))
	for _, observation := range observations {
		digests = append(digests, DigestJSON(observation))
	}
	sort.Strings(digests)
	return digests
}

func validWeekID(value string) bool {
	if len(value) != 8 || value[4] != '-' || value[5] != 'W' {
		return false
	}
	for _, char := range value[:4] + value[6:] {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}
func validSHA256(value string) bool {
	return len(value) == 64 && strings.Trim(value, "0123456789abcdef") == ""
}
func validConfidence(value selfmodel.Confidence) bool {
	return value == selfmodel.ConfidenceLow || value == selfmodel.ConfidenceMedium || value == selfmodel.ConfidenceHigh
}
func validConfirmation(value ConfirmationRequirement) bool {
	return value == ConfirmationAutomaticWithAudit || value == ConfirmationProposalOnly || value == ConfirmationExplicit
}
func validFacet(value string) bool {
	switch value {
	case "professional-role", "communication-style", "voice", "preferences", "decision-rules", "working-boundaries", "psychological-profile", "intrinsic-intent":
		return true
	default:
		return false
	}
}
