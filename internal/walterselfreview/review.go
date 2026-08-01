// Package walterselfreview implements Walter's periodic, proposal-only self
// review on top of the canonical ownerctx and Maestro contracts.
package walterselfreview

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/maestro"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/maintenance"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ownerctx"
)

const (
	SchemaVersion       = 1
	WeeklyJobID         = "walter-self-review-weekly"
	MaxPromptEntries    = 8
	MaxPromptBytes      = 32 << 10
	MaxObservations     = 32
	MaxProposalBytes    = 2000
	MaxReceiptLineBytes = 32 << 10
)

var (
	ErrUnavailable    = errors.New("Walter weekly self review is unavailable without an approved model adapter and authority")
	ErrProposalOnly   = errors.New("Walter self review produces proposals only; canonical mutation belongs to Owner Context policy")
	ErrOccurrenceBusy = errors.New("Walter weekly self-review occurrence is already reserved")
)

type ReceiptState string

const (
	ReceiptReserved    ReceiptState = "reserved"
	ReceiptProposal    ReceiptState = "proposal_emitted"
	ReceiptUnavailable ReceiptState = "unavailable"
	ReceiptFailed      ReceiptState = "failed"
)

type ConfirmationRequirement string

const (
	ConfirmationAutomaticWithAudit ConfirmationRequirement = "automatic_with_audit"
	ConfirmationProposalOnly       ConfirmationRequirement = "proposal_only"
	ConfirmationExplicit           ConfirmationRequirement = "confirmation_required"
)

type PromptEvidence struct {
	ID               string `json:"id"`
	OriginalText     string `json:"original_text"`
	NormalizedText   string `json:"normalized_text"`
	SourceLanguage   string `json:"source_language"`
	WorkingLanguage  string `json:"working_language"`
	OriginalSHA256   string `json:"original_sha256"`
	NormalizedSHA256 string `json:"normalized_sha256"`
	RelevanceScore   int    `json:"relevance_score"`
	QuotedData       bool   `json:"quoted_data"`
	Instructional    bool   `json:"instructional"`
}

type TranslationReceipt struct {
	TranslatorID      string `json:"translator_id"`
	TranslatorVersion string `json:"translator_version"`
	SourceLanguage    string `json:"source_language"`
	WorkingLanguage   string `json:"working_language"`
	OriginalSHA256    string `json:"original_sha256"`
	WorkingSHA256     string `json:"working_sha256"`
	HistorySHA256     string `json:"history_sha256"`
	ReceiptSHA256     string `json:"receipt_sha256"`
}

// Request contains only the already selected canonical PromptHistory entries,
// metadata-only owner observations and a read-only ownerctx projection. The
// intrinsic-purpose hypothesis is optional and task-local; it is never stored.
type Request struct {
	SchemaVersion     int                           `json:"schema_version"`
	WeekID            string                        `json:"week_id"`
	OccurrenceID      string                        `json:"occurrence_id"`
	OwnerID           string                        `json:"owner_id"`
	ScopeKind         ownerctx.PromptScopeKind      `json:"scope_kind"`
	ScopeID           string                        `json:"scope_id"`
	CurrentOriginal   string                        `json:"current_original"`
	CurrentNormalized string                        `json:"current_normalized"`
	PromptHistory     []PromptEvidence              `json:"prompt_history"`
	Translation       TranslationReceipt            `json:"translation"`
	Observations      []ownerctx.ObservationReceipt `json:"observations"`
	CanonicalSnapshot ownerctx.UserSelfSnapshot     `json:"canonical_snapshot"`
	IntentHypothesis  *maestro.IntentHypothesis     `json:"-"`
	OwnerContextRoot  string                        `json:"-"`
}

type ModelInput struct {
	CurrentOriginal   string
	CurrentNormalized string
	History           []PromptEvidence
	Translation       TranslationReceipt
	Observations      []ownerctx.ObservationReceipt
	CanonicalSnapshot ownerctx.UserSelfSnapshot
	IntentHypothesis  *maestro.IntentHypothesis
}

type ModelAdapter interface {
	ID() string
	Review(context.Context, ModelInput) (SelfRefinementProposal, error)
}

type Authority interface {
	ID() string
	Approved(string) bool
}

type SelfRefinementProposal struct {
	SchemaVersion            int                     `json:"schema_version"`
	ProposalID               string                  `json:"proposal_id"`
	State                    string                  `json:"state"`
	WeekID                   string                  `json:"week_id"`
	Facet                    string                  `json:"facet"`
	PriorClaim               string                  `json:"prior_claim"`
	ProposedRefinement       string                  `json:"proposed_refinement"`
	EvidenceObservationIDs   []string                `json:"evidence_observation_ids"`
	Confidence               float64                 `json:"confidence"`
	Sensitivity              string                  `json:"sensitivity"`
	ConfirmationRequirement  ConfirmationRequirement `json:"confirmation_requirement"`
	CanonicalSnapshotVersion string                  `json:"canonical_snapshot_version"`
	CanonicalSnapshotSHA256  string                  `json:"canonical_snapshot_sha256"`
	PromptHistorySHA256      string                  `json:"prompt_history_sha256"`
	TranslationReceiptSHA256 string                  `json:"translation_receipt_sha256"`
	IntentHypothesisSHA256   string                  `json:"intent_hypothesis_sha256,omitempty"`
}

// Receipt is metadata-only and is fenced by the reservation token. It never
// contains prompt, observation claim, proposal or intrinsic-purpose text.
type Receipt struct {
	SchemaVersion            int          `json:"schema_version"`
	ReceiptID                string       `json:"receipt_id"`
	State                    ReceiptState `json:"state"`
	WeekID                   string       `json:"week_id"`
	OccurrenceID             string       `json:"occurrence_id"`
	FencingToken             string       `json:"fencing_token"`
	RequestSHA256            string       `json:"request_sha256"`
	PromptHistorySHA256      string       `json:"prompt_history_sha256"`
	TranslationReceiptSHA256 string       `json:"translation_receipt_sha256"`
	CanonicalSnapshotSHA256  string       `json:"canonical_snapshot_sha256"`
	ProposalID               string       `json:"proposal_id,omitempty"`
	ProposalSHA256           string       `json:"proposal_sha256,omitempty"`
	AdapterID                string       `json:"adapter_id,omitempty"`
	AuthorityID              string       `json:"authority_id,omitempty"`
	ReasonCode               string       `json:"reason_code,omitempty"`
	RecordedAt               time.Time    `json:"recorded_at"`
}

type Reservation struct {
	Receipt  Receipt
	Existing bool
}

type ReceiptStore struct{ Root string }

func (store ReceiptStore) Reserve(request Request, now time.Time) (Reservation, error) {
	if err := ValidateRequest(request); err != nil {
		return Reservation{}, err
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	var reservation Reservation
	err := store.withLock(func(path string) error {
		receipts, err := store.read(path)
		if err != nil {
			return err
		}
		for _, receipt := range receipts {
			if receipt.OccurrenceID == request.OccurrenceID {
				if receipt.RequestSHA256 != RequestDigest(request) {
					return errors.New("weekly occurrence was reused with different input")
				}
				reservation = Reservation{Receipt: receipt, Existing: true}
				return nil
			}
		}
		token := randomToken()
		receipt := Receipt{SchemaVersion: SchemaVersion, ReceiptID: Digest(request.OccurrenceID + ":" + RequestDigest(request)), State: ReceiptReserved, WeekID: request.WeekID, OccurrenceID: request.OccurrenceID, FencingToken: token, RequestSHA256: RequestDigest(request), PromptHistorySHA256: PromptHistoryDigest(request.PromptHistory), TranslationReceiptSHA256: request.Translation.ReceiptSHA256, CanonicalSnapshotSHA256: request.CanonicalSnapshot.CanonicalSourceDigest, RecordedAt: now.UTC()}
		if err := store.append(path, receipt); err != nil {
			return err
		}
		reservation = Reservation{Receipt: receipt}
		return nil
	})
	return reservation, err
}

func (store ReceiptStore) Finalize(reservation Reservation, state ReceiptState, proposal SelfRefinementProposal, adapterID, authorityID, reason string, now time.Time) (Receipt, error) {
	if reservation.Existing && reservation.Receipt.State != ReceiptReserved {
		return reservation.Receipt, nil
	}
	if state != ReceiptProposal && state != ReceiptUnavailable && state != ReceiptFailed {
		return Receipt{}, errors.New("invalid weekly self-review final state")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	final := reservation.Receipt
	final.State, final.AdapterID, final.AuthorityID, final.ReasonCode, final.RecordedAt = state, adapterID, authorityID, reason, now.UTC()
	if state == ReceiptProposal {
		final.ProposalID = proposal.ProposalID
		final.ProposalSHA256 = DigestJSON(proposal)
	}
	err := store.withLock(func(path string) error {
		receipts, err := store.read(path)
		if err != nil {
			return err
		}
		for _, existing := range receipts {
			if existing.OccurrenceID != reservation.Receipt.OccurrenceID {
				continue
			}
			if existing.FencingToken != reservation.Receipt.FencingToken {
				return errors.New("weekly receipt fencing token mismatch")
			}
			if existing.State != ReceiptReserved {
				final = existing
				return nil
			}
			return store.append(path, final)
		}
		return errors.New("weekly reservation is missing")
	})
	return final, err
}

func (store ReceiptStore) withLock(operation func(string) error) error {
	root, err := ensurePrivateRoot(store.Root)
	if err != nil {
		return err
	}
	lockPath := filepath.Join(root, "weekly-receipts.lock")
	token := randomToken()
	deadline := time.Now().Add(3 * time.Second)
	for {
		if info, statErr := os.Lstat(lockPath); statErr == nil {
			if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
				return errors.New("weekly receipt lock is not a regular file")
			}
		} else if !errors.Is(statErr, os.ErrNotExist) {
			return statErr
		}
		file, createErr := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if createErr == nil {
			if _, createErr = file.WriteString(token); createErr == nil {
				createErr = file.Sync()
			}
			closeErr := file.Close()
			if createErr != nil {
				_ = os.Remove(lockPath)
				return createErr
			}
			if closeErr != nil {
				_ = os.Remove(lockPath)
				return closeErr
			}
			err := operation(filepath.Join(root, "weekly-receipts.jsonl"))
			releaseErr := releaseLock(lockPath, token)
			if err != nil {
				return err
			}
			return releaseErr
		}
		if !errors.Is(createErr, os.ErrExist) {
			return createErr
		}
		if time.Now().After(deadline) {
			return ErrOccurrenceBusy
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func (store ReceiptStore) read(path string) ([]Receipt, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, errors.New("weekly receipt file is not a private regular file")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(body) == 0 {
		return nil, nil
	}
	lines := strings.Split(string(body), "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	receipts := make([]Receipt, 0, len(lines))
	for _, line := range lines {
		if len([]byte(line)) > MaxReceiptLineBytes {
			return nil, errors.New("weekly receipt line exceeds bound")
		}
		var receipt Receipt
		decoder := json.NewDecoder(strings.NewReader(line))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&receipt); err != nil || !strings.HasPrefix(line, "{") {
			return nil, errors.New("weekly receipt log contains a partial or invalid line")
		}
		if err := validateReceipt(receipt); err != nil {
			return nil, err
		}
		receipts = append(receipts, receipt)
	}
	return receipts, nil
}

func (store ReceiptStore) append(path string, receipt Receipt) error {
	if err := validateReceipt(receipt); err != nil {
		return err
	}
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return errors.New("weekly receipt file is not a private regular file")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	body, err := json.Marshal(receipt)
	if err == nil {
		_, err = file.Write(append(body, '\n'))
	}
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil {
		return err
	}
	return closeErr
}

func Review(ctx context.Context, request Request, adapter ModelAdapter, authority Authority, store ReceiptStore, now time.Time) (SelfRefinementProposal, Receipt, error) {
	reservation, err := store.Reserve(request, now)
	if err != nil {
		return SelfRefinementProposal{}, Receipt{}, err
	}
	if reservation.Existing {
		return SelfRefinementProposal{}, reservation.Receipt, nil
	}
	if adapter == nil || authority == nil || !authority.Approved("walter/self-review-weekly") {
		receipt, finalizeErr := store.Finalize(reservation, ReceiptUnavailable, SelfRefinementProposal{}, "", "", "approved_model_adapter_or_authority_unavailable", now)
		if finalizeErr != nil {
			return SelfRefinementProposal{}, receipt, finalizeErr
		}
		return SelfRefinementProposal{}, receipt, ErrUnavailable
	}
	proposal, err := adapter.Review(ctx, ModelInput{CurrentOriginal: request.CurrentOriginal, CurrentNormalized: request.CurrentNormalized, History: append([]PromptEvidence(nil), request.PromptHistory...), Translation: request.Translation, Observations: append([]ownerctx.ObservationReceipt(nil), request.Observations...), CanonicalSnapshot: request.CanonicalSnapshot, IntentHypothesis: request.IntentHypothesis})
	if err != nil {
		receipt, finalizeErr := store.Finalize(reservation, ReceiptFailed, SelfRefinementProposal{}, adapter.ID(), authority.ID(), "model_adapter_error", now)
		if finalizeErr != nil {
			return SelfRefinementProposal{}, receipt, finalizeErr
		}
		return SelfRefinementProposal{}, receipt, err
	}
	if err := ValidateProposal(request, proposal); err != nil {
		receipt, finalizeErr := store.Finalize(reservation, ReceiptFailed, SelfRefinementProposal{}, adapter.ID(), authority.ID(), "proposal_invalid_or_stale", now)
		if finalizeErr != nil {
			return SelfRefinementProposal{}, receipt, finalizeErr
		}
		return SelfRefinementProposal{}, receipt, err
	}
	if _, err := ownerctx.SubmitRefinement(request.OwnerContextRoot, ownerctx.RefinementInput{Facet: proposal.Facet, Evidence: "walter-weekly:" + DigestJSON(proposal.EvidenceObservationIDs), ProposedBody: proposal.ProposedRefinement}); err != nil {
		return SelfRefinementProposal{}, Receipt{}, err
	}
	receipt, err := store.Finalize(reservation, ReceiptProposal, proposal, adapter.ID(), authority.ID(), "proposal_durable", now)
	if err != nil {
		return SelfRefinementProposal{}, receipt, err
	}
	return proposal, receipt, nil
}

// Handler is the maintenance worker seam. It assembles from canonical
// ownerctx, reserves before invoking the model, and reports proposal_emitted
// only after both the ownerctx proposal and fenced receipt are durable.
type Handler struct {
	Root              string
	OwnerID           string
	ScopeKind         ownerctx.PromptScopeKind
	ScopeID           string
	CurrentPrompt     string
	CurrentLanguage   string
	WorkingLanguage   string
	RelevanceKeys     []string
	TranslatorID      string
	TranslatorVersion string
	Translator        maestro.PromptTranslator
	Adapter           ModelAdapter
	Authority         Authority
	Store             ReceiptStore
	MaintenanceStore  maintenance.Store
	Now               func() time.Time
}

var _ maintenance.Handler = Handler{}

func (handler Handler) Handle(ctx context.Context, command maintenance.Command) (maintenance.Receipt, error) {
	now := time.Now().UTC()
	if handler.Now != nil {
		now = handler.Now().UTC()
	}
	if err := command.Validate(now); err != nil || command.JobID != WeeklyJobID || !command.ProposalOnly || command.Trigger != maintenance.TriggerWeekly {
		return maintenance.Receipt{}, errors.New("invalid Walter weekly self-review maintenance command")
	}
	request, err := handler.BuildRequest(command, now)
	if err != nil {
		return maintenance.Receipt{}, err
	}
	_, receipt, reviewErr := Review(ctx, request, handler.Adapter, handler.Authority, handler.Store, now)
	maintenanceReceipt := handler.toMaintenanceReceipt(command, receipt, reviewErr, now)
	if appendErr := handler.MaintenanceStore.AppendReceipt(maintenanceReceipt); appendErr != nil {
		return maintenanceReceipt, appendErr
	}
	if reviewErr != nil {
		return maintenanceReceipt, reviewErr
	}
	return maintenanceReceipt, nil
}

func (handler Handler) BuildRequest(command maintenance.Command, now time.Time) (Request, error) {
	if strings.TrimSpace(handler.OwnerID) == "" || strings.TrimSpace(handler.CurrentPrompt) == "" || strings.TrimSpace(handler.WorkingLanguage) == "" || strings.TrimSpace(handler.TranslatorID) == "" || strings.TrimSpace(handler.TranslatorVersion) == "" || handler.Translator == nil {
		return Request{}, errors.New("Walter weekly self-review requires canonical prompt history and translator configuration")
	}
	if handler.ScopeKind == "" {
		handler.ScopeKind = ownerctx.PromptScopeGlobal
	}
	if handler.ScopeID == "" {
		handler.ScopeID = "owner"
	}
	snapshot, err := ownerctx.ProjectSnapshot(handler.Root, nil)
	if err != nil {
		return Request{}, err
	}
	observations, err := ownerctx.ListObservations(handler.Root)
	if err != nil {
		return Request{}, err
	}
	filtered := make([]ownerctx.ObservationReceipt, 0, len(observations))
	for _, observation := range observations {
		if observation.State == ownerctx.ObservationCorroborated || observation.Signal == ownerctx.SignalExplicitInstruction || observation.Signal == ownerctx.SignalExplicitCorrection || (observation.Signal == ownerctx.SignalExplicitEndorsement && observation.OwnerConfirmed) {
			filtered = append(filtered, observation)
		}
	}
	if len(filtered) > MaxObservations {
		filtered = filtered[:MaxObservations]
	}
	selected, err := ownerctx.SelectRelevantPromptHistory(handler.Root, ownerctx.PromptHistorySelectionLimits{OwnerID: handler.OwnerID, MaxCount: MaxPromptEntries, MaxBytes: MaxPromptBytes, MaxAge: 90 * 24 * time.Hour, ScopeKind: handler.ScopeKind, ScopeID: handler.ScopeID, CurrentPrompt: handler.CurrentPrompt, RelevanceKeys: append([]string(nil), handler.RelevanceKeys...), CurrentLanguage: handler.CurrentLanguage}, now)
	if err != nil {
		return Request{}, err
	}
	currentNormalized, err := handler.Translator(handler.CurrentPrompt, handler.CurrentLanguage, handler.WorkingLanguage)
	if err != nil || strings.TrimSpace(currentNormalized) == "" {
		return Request{}, errors.New("Walter current prompt translation is unavailable")
	}
	currentNormalized = strings.TrimSpace(currentNormalized)
	history := make([]PromptEvidence, 0, len(selected))
	historyDigests := make([]string, 0, len(selected))
	for _, item := range selected {
		normalized, translateErr := handler.Translator(item.Entry.Prompt, item.Entry.Language, handler.WorkingLanguage)
		if translateErr != nil || strings.TrimSpace(normalized) == "" {
			return Request{}, errors.New("Walter prompt history translation is unavailable")
		}
		normalized = strings.TrimSpace(normalized)
		history = append(history, PromptEvidence{ID: item.Entry.ID, OriginalText: item.Entry.Prompt, NormalizedText: normalized, SourceLanguage: item.Entry.Language, WorkingLanguage: handler.WorkingLanguage, OriginalSHA256: maestro.SHA256Hex(item.Entry.Prompt), NormalizedSHA256: maestro.SHA256Hex(normalized), RelevanceScore: item.Score, QuotedData: true, Instructional: false})
		historyDigests = append(historyDigests, item.Entry.SHA256+":"+maestro.SHA256Hex(normalized))
	}
	sort.Strings(historyDigests)
	translation := TranslationReceipt{TranslatorID: handler.TranslatorID, TranslatorVersion: handler.TranslatorVersion, SourceLanguage: handler.CurrentLanguage, WorkingLanguage: handler.WorkingLanguage, OriginalSHA256: maestro.SHA256Hex(handler.CurrentPrompt), WorkingSHA256: maestro.SHA256Hex(currentNormalized), HistorySHA256: Digest(strings.Join(historyDigests, "\n"))}
	translation.ReceiptSHA256 = DigestJSON(translation)
	return Request{SchemaVersion: SchemaVersion, WeekID: weekID(now), OccurrenceID: command.CommandID, OwnerID: handler.OwnerID, ScopeKind: handler.ScopeKind, ScopeID: handler.ScopeID, CurrentOriginal: handler.CurrentPrompt, CurrentNormalized: currentNormalized, PromptHistory: history, Translation: translation, Observations: filtered, CanonicalSnapshot: snapshot, OwnerContextRoot: handler.Root}, nil
}

func (handler Handler) toMaintenanceReceipt(command maintenance.Command, receipt Receipt, reviewErr error, now time.Time) maintenance.Receipt {
	state := maintenance.ReceiptFailed
	if receipt.State == ReceiptUnavailable {
		state = maintenance.ReceiptUnavailable
	}
	if receipt.State == ReceiptProposal {
		state = maintenance.ReceiptProposalEmitted
	}
	diagnostic := receipt.ReasonCode
	if reviewErr != nil && diagnostic == "" {
		diagnostic = "walter_self_review_unavailable_or_failed"
	}
	value := maintenance.Receipt{SchemaVersion: maintenance.CommandSchemaVersion, AttemptID: Digest(command.OccurrenceDigest() + receipt.FencingToken)[:32], OccurrenceDigest: command.OccurrenceDigest(), CommandID: command.CommandID, JobID: command.JobID, WorkspaceID: command.WorkspaceID, Trigger: command.Trigger, State: state, RecordedAt: now, Deadline: command.Deadline, ProposalOnly: true, Diagnostic: diagnostic}
	if state == maintenance.ReceiptProposalEmitted {
		value.ProposalCount = 1
		value.ProposalDigest = receipt.ProposalSHA256
	}
	return value
}

func ValidateRequest(request Request) error {
	if request.SchemaVersion != SchemaVersion || !validID(request.WeekID) || !validID(request.OccurrenceID) || strings.TrimSpace(request.OwnerID) == "" || strings.TrimSpace(request.CurrentOriginal) == "" || strings.TrimSpace(request.CurrentNormalized) == "" || len(request.PromptHistory) > MaxPromptEntries || len(request.Observations) > MaxObservations {
		return errors.New("Walter weekly self-review request is invalid")
	}
	if err := request.CanonicalSnapshot.Validate(); err != nil {
		return err
	}
	if request.Translation.ReceiptSHA256 != DigestJSON(TranslationReceipt{TranslatorID: request.Translation.TranslatorID, TranslatorVersion: request.Translation.TranslatorVersion, SourceLanguage: request.Translation.SourceLanguage, WorkingLanguage: request.Translation.WorkingLanguage, OriginalSHA256: request.Translation.OriginalSHA256, WorkingSHA256: request.Translation.WorkingSHA256, HistorySHA256: request.Translation.HistorySHA256}) || request.Translation.OriginalSHA256 != maestro.SHA256Hex(request.CurrentOriginal) || request.Translation.WorkingSHA256 != maestro.SHA256Hex(request.CurrentNormalized) {
		return errors.New("Walter translation receipt is stale or incomplete")
	}
	for _, prompt := range request.PromptHistory {
		if !prompt.QuotedData || prompt.Instructional || prompt.OriginalSHA256 != maestro.SHA256Hex(prompt.OriginalText) || prompt.NormalizedSHA256 != maestro.SHA256Hex(prompt.NormalizedText) || prompt.WorkingLanguage != request.Translation.WorkingLanguage || strings.TrimSpace(prompt.OriginalText) == "" || strings.TrimSpace(prompt.NormalizedText) == "" {
			return errors.New("Walter prompt history is not quoted, normalized and digest-bound")
		}
	}
	for _, observation := range request.Observations {
		if observation.ID == "" || observation.SourceDigest == "" || observation.Claim == "" {
			return errors.New("Walter observation evidence is incomplete")
		}
	}
	return nil
}

func ValidateProposal(request Request, proposal SelfRefinementProposal) error {
	if err := ValidateRequest(request); err != nil {
		return err
	}
	if proposal.SchemaVersion != SchemaVersion || proposal.State != "proposed" || strings.TrimSpace(proposal.ProposalID) == "" || !validFacet(proposal.Facet) || strings.TrimSpace(proposal.PriorClaim) == "" || strings.TrimSpace(proposal.ProposedRefinement) == "" || len([]byte(proposal.PriorClaim)) > MaxProposalBytes || len([]byte(proposal.ProposedRefinement)) > MaxProposalBytes || proposal.WeekID != request.WeekID || proposal.CanonicalSnapshotVersion != request.CanonicalSnapshot.Version || proposal.CanonicalSnapshotSHA256 != request.CanonicalSnapshot.CanonicalSourceDigest || proposal.PromptHistorySHA256 != DigestJSON(request.PromptHistory) || proposal.TranslationReceiptSHA256 != request.Translation.ReceiptSHA256 || !validConfidence(proposal.Confidence) || strings.TrimSpace(proposal.Sensitivity) == "" || len(proposal.EvidenceObservationIDs) == 0 || len(proposal.EvidenceObservationIDs) > MaxObservations || !validConfirmation(proposal.ConfirmationRequirement) {
		return errors.New("Walter self-refinement proposal is incomplete or stale")
	}
	if proposal.IntentHypothesisSHA256 != "" && proposal.IntentHypothesisSHA256 != DigestJSON(request.IntentHypothesis) {
		return errors.New("Walter intrinsic-purpose hypothesis binding is stale")
	}
	available := map[string]ownerctx.ObservationReceipt{}
	for _, observation := range request.Observations {
		available[observation.ID] = observation
	}
	seen := map[string]bool{}
	for _, id := range proposal.EvidenceObservationIDs {
		observation, ok := available[id]
		if !ok || seen[id] || !(observation.State == ownerctx.ObservationCorroborated || observation.Signal == ownerctx.SignalExplicitInstruction || observation.Signal == ownerctx.SignalExplicitCorrection || (observation.Signal == ownerctx.SignalExplicitEndorsement && observation.OwnerConfirmed)) {
			return errors.New("Walter proposal evidence is not independently corroborated or explicitly owner-attested")
		}
		seen[id] = true
	}
	switch proposal.Facet {
	case "professional-role", "decision-rules":
		if proposal.ConfirmationRequirement != ConfirmationProposalOnly {
			return errors.New("professional role and decision rules are proposal-only")
		}
	case "working-boundaries", "psychological-profile":
		if proposal.ConfirmationRequirement != ConfirmationExplicit {
			return errors.New("sensitive self facets require explicit confirmation")
		}
	case "communication-style", "voice", "preferences":
		if proposal.ConfirmationRequirement != ConfirmationAutomaticWithAudit && proposal.ConfirmationRequirement != ConfirmationProposalOnly {
			return errors.New("communication facets require the canonical low-risk policy")
		}
	default:
		return errors.New("intrinsic-purpose hypotheses are ephemeral and cannot become a self facet")
	}
	return nil
}

func ApplyCanonicalMutation(SelfRefinementProposal, ownerctx.UserSelfSnapshot) error {
	return ErrProposalOnly
}

func validFacet(value string) bool {
	switch value {
	case "professional-role", "communication-style", "voice", "preferences", "decision-rules", "working-boundaries", "psychological-profile":
		return true
	default:
		return false
	}
}
func validConfirmation(value ConfirmationRequirement) bool {
	return value == ConfirmationAutomaticWithAudit || value == ConfirmationProposalOnly || value == ConfirmationExplicit
}
func validConfidence(value float64) bool { return value >= 0 && value <= 1 }
func validID(value string) bool          { return strings.TrimSpace(value) != "" && len([]byte(value)) <= 128 }
func RequestDigest(request Request) string {
	body, _ := json.Marshal(request)
	return Digest(string(body))
}
func PromptHistoryDigest(history []PromptEvidence) string { return DigestJSON(history) }
func DigestJSON(value any) string                         { body, _ := json.Marshal(value); return Digest(string(body)) }
func Digest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
func randomToken() string {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return Digest(time.Now().UTC().String())[:32]
	}
	return hex.EncodeToString(bytes)
}
func weekID(now time.Time) string {
	year, week := now.ISOWeek()
	return fmt.Sprintf("%04d-W%02d", year, week)
}

func validateReceipt(receipt Receipt) error {
	if receipt.SchemaVersion != SchemaVersion || !validID(receipt.ReceiptID) || !validID(receipt.WeekID) || !validID(receipt.OccurrenceID) || len(receipt.FencingToken) != 32 || receipt.RecordedAt.IsZero() || receipt.RequestSHA256 == "" || receipt.PromptHistorySHA256 == "" || receipt.TranslationReceiptSHA256 == "" || receipt.CanonicalSnapshotSHA256 == "" {
		return errors.New("weekly Walter receipt is invalid")
	}
	switch receipt.State {
	case ReceiptReserved, ReceiptProposal, ReceiptUnavailable, ReceiptFailed:
		return nil
	default:
		return errors.New("weekly Walter receipt state is invalid")
	}
}

func ensurePrivateRoot(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", errors.New("weekly receipt root is required")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	abs = filepath.Clean(abs)
	volume := filepath.VolumeName(abs)
	current := volume + string(os.PathSeparator)
	rest := strings.TrimPrefix(abs, current)
	parts := strings.Split(rest, string(os.PathSeparator))
	for index, part := range parts {
		if part == "" {
			continue
		}
		current = filepath.Join(current, part)
		info, statErr := os.Lstat(current)
		if errors.Is(statErr, os.ErrNotExist) {
			if statErr = os.Mkdir(current, 0o700); statErr != nil {
				return "", statErr
			}
			info, statErr = os.Lstat(current)
		}
		if statErr != nil {
			return "", fmt.Errorf("weekly receipt parent is not a private directory: %s: %v", current, statErr)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			// macOS exposes the system temporary tree through /var -> /private/var.
			// Other symlinked ancestors, including the store root itself, fail closed.
			if index == len(parts)-1 || filepath.Clean(current) != string(os.PathSeparator)+"var" {
				return "", errors.New("weekly receipt parent is not a private directory")
			}
			resolved, resolveErr := os.Stat(current)
			if resolveErr != nil || !resolved.IsDir() {
				return "", errors.New("weekly receipt symlink ancestor is not a directory")
			}
			continue
		}
		if !info.IsDir() {
			return "", errors.New("weekly receipt parent is not a private directory")
		}
		if index == len(parts)-1 && info.Mode().Perm() != 0o700 {
			if err := os.Chmod(current, 0o700); err != nil {
				return "", err
			}
		}
	}
	return abs, nil
}

func releaseLock(path, token string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return errors.New("weekly receipt lock changed")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if string(body) != token {
		return errors.New("weekly receipt lock ownership changed")
	}
	return os.Remove(path)
}
