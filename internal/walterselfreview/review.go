// Package walterselfreview defines Walter's periodic, bounded self-ingestion
// seam on top of the canonical ownerctx and Maestro contracts. The shipped
// model path remains unavailable; a qualified implementation must compact a
// bounded self projection without emitting a user-facing weekly artifact.
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
	SchemaVersion           = 1
	WeeklyJobID             = maintenance.WalterSelfReviewWeeklyJobID
	MaxPromptEntries        = 8
	MaxPromptBytes          = 32 << 10
	MaxContextBytes         = 32 << 10
	MaxTranslationExpansion = 4
	MaxTranslationOverhead  = 256
	MaxObservations         = 32
	MaxProposalBytes        = 2000
	MaxReceiptLineBytes     = 32 << 10
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
	SchemaVersion            int                           `json:"schema_version"`
	WeekID                   string                        `json:"week_id"`
	OccurrenceID             string                        `json:"occurrence_id"`
	OwnerID                  string                        `json:"owner_id"`
	ScopeKind                ownerctx.PromptScopeKind      `json:"scope_kind"`
	ScopeID                  string                        `json:"scope_id"`
	CurrentOriginal          string                        `json:"current_original"`
	CurrentNormalized        string                        `json:"current_normalized"`
	PromptHistory            []PromptEvidence              `json:"prompt_history"`
	Translation              TranslationReceipt            `json:"translation"`
	Observations             []ownerctx.ObservationReceipt `json:"observations"`
	CanonicalSnapshot        ownerctx.UserSelfSnapshot     `json:"canonical_snapshot"`
	ReviewFacets             []string                      `json:"review_facets"`
	SensitivePurposeSHA256   string                        `json:"sensitive_purpose_sha256,omitempty"`
	SensitiveOwnerAuthorized bool                          `json:"sensitive_owner_authorized,omitempty"`
	IntentHypothesisSHA256   string                        `json:"intent_hypothesis_sha256,omitempty"`
	IntentHypothesis         *maestro.IntentHypothesis     `json:"-"`
	OwnerContextRoot         string                        `json:"-"`
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
	Readers                  []string                `json:"readers"`
	Refinement               string                  `json:"refinement"`
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
	SchemaVersion                    int                     `json:"schema_version"`
	ReceiptID                        string                  `json:"receipt_id"`
	State                            ReceiptState            `json:"state"`
	WeekID                           string                  `json:"week_id"`
	OccurrenceID                     string                  `json:"occurrence_id"`
	FencingToken                     string                  `json:"fencing_token"`
	LeaseOwner                       string                  `json:"lease_owner,omitempty"`
	LeaseUntil                       time.Time               `json:"lease_until,omitempty"`
	RequestSHA256                    string                  `json:"request_sha256"`
	PromptHistorySHA256              string                  `json:"prompt_history_sha256"`
	TranslationReceiptSHA256         string                  `json:"translation_receipt_sha256"`
	CanonicalSnapshotSHA256          string                  `json:"canonical_snapshot_sha256"`
	ProposalID                       string                  `json:"proposal_id,omitempty"`
	ProposalSHA256                   string                  `json:"proposal_sha256,omitempty"`
	OwnerctxProposalID               string                  `json:"ownerctx_proposal_id,omitempty"`
	OwnerctxProposalSHA256           string                  `json:"ownerctx_proposal_sha256,omitempty"`
	OwnerctxPolicy                   string                  `json:"ownerctx_policy,omitempty"`
	CanonicalSensitivity             string                  `json:"canonical_sensitivity,omitempty"`
	CanonicalReaders                 []string                `json:"canonical_readers,omitempty"`
	CanonicalRefinement              string                  `json:"canonical_refinement,omitempty"`
	CanonicalConfirmationRequirement ConfirmationRequirement `json:"canonical_confirmation_requirement,omitempty"`
	AdapterID                        string                  `json:"adapter_id,omitempty"`
	AuthorityID                      string                  `json:"authority_id,omitempty"`
	ReasonCode                       string                  `json:"reason_code,omitempty"`
	RecordedAt                       time.Time               `json:"recorded_at"`
}

type Reservation struct {
	Receipt    Receipt
	Existing   bool
	Resumed    bool
	LeaseOwner string
}

type ReceiptStore struct {
	Root          string
	LeaseDuration time.Duration
	LockWait      time.Duration
}

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
		receipt, found := latestReceipt(receipts, request.OccurrenceID)
		if found {
			if receipt.RequestSHA256 != RequestDigest(request) {
				return errors.New("weekly occurrence was reused with different input")
			}
			if receipt.State != ReceiptReserved {
				reservation = Reservation{Receipt: receipt, Existing: true}
				return nil
			}
			if receipt.LeaseUntil.After(now.UTC()) {
				return ErrOccurrenceBusy
			}
			leaseOwner := randomToken()
			receipt.FencingToken = randomToken()
			receipt.LeaseOwner = leaseOwner
			receipt.LeaseUntil = now.UTC().Add(store.leaseDuration())
			receipt.RecordedAt = now.UTC()
			if err := store.append(path, receipt); err != nil {
				return err
			}
			reservation = Reservation{Receipt: receipt, Resumed: true, LeaseOwner: leaseOwner}
			return nil
		}
		token := randomToken()
		leaseOwner := randomToken()
		receipt = Receipt{SchemaVersion: SchemaVersion, ReceiptID: Digest(request.OccurrenceID + ":" + RequestDigest(request)), State: ReceiptReserved, WeekID: request.WeekID, OccurrenceID: request.OccurrenceID, FencingToken: token, LeaseOwner: leaseOwner, LeaseUntil: now.UTC().Add(store.leaseDuration()), RequestSHA256: RequestDigest(request), PromptHistorySHA256: PromptHistoryDigest(request.PromptHistory), TranslationReceiptSHA256: request.Translation.ReceiptSHA256, CanonicalSnapshotSHA256: request.CanonicalSnapshot.CanonicalSourceDigest, RecordedAt: now.UTC()}
		if err := store.append(path, receipt); err != nil {
			return err
		}
		reservation = Reservation{Receipt: receipt, LeaseOwner: leaseOwner}
		return nil
	})
	return reservation, err
}

func (store ReceiptStore) Finalize(reservation Reservation, state ReceiptState, proposal SelfRefinementProposal, ownerReceipt ownerctx.RefinementReceipt, adapterID, authorityID, reason string, now time.Time) (Receipt, error) {
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
		if ownerReceipt.WalterProposalSHA256 != "" {
			final.ProposalSHA256 = ownerReceipt.WalterProposalSHA256
		}
		final.OwnerctxProposalID = ownerReceipt.ID
		final.OwnerctxProposalSHA256 = ownerReceipt.ProposalSHA256
		final.OwnerctxPolicy = ownerReceipt.Policy
		final.CanonicalSensitivity = proposal.Sensitivity
		final.CanonicalReaders = append([]string(nil), proposal.Readers...)
		final.CanonicalRefinement = proposal.Refinement
		final.CanonicalConfirmationRequirement = proposal.ConfirmationRequirement
	}
	err := store.withLock(func(path string) error {
		receipts, err := store.read(path)
		if err != nil {
			return err
		}
		existing, found := latestReceipt(receipts, reservation.Receipt.OccurrenceID)
		if !found {
			return errors.New("weekly reservation is missing")
		}
		if existing.FencingToken != reservation.Receipt.FencingToken || existing.LeaseOwner != reservation.LeaseOwner {
			return errors.New("weekly receipt lease or fencing token mismatch")
		}
		if existing.State != ReceiptReserved {
			final = existing
			return nil
		}
		return store.append(path, final)
	})
	return final, err
}

func (store ReceiptStore) leaseDuration() time.Duration {
	if store.LeaseDuration <= 0 {
		return 30 * time.Second
	}
	return store.LeaseDuration
}

func latestReceipt(receipts []Receipt, occurrenceID string) (Receipt, bool) {
	for index := len(receipts) - 1; index >= 0; index-- {
		if receipts[index].OccurrenceID == occurrenceID {
			return receipts[index], true
		}
	}
	return Receipt{}, false
}

func (store ReceiptStore) Renew(reservation Reservation, now time.Time) error {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	return store.withLock(func(path string) error {
		receipts, err := store.read(path)
		if err != nil {
			return err
		}
		existing, found := latestReceipt(receipts, reservation.Receipt.OccurrenceID)
		if !found || existing.State != ReceiptReserved || existing.FencingToken != reservation.Receipt.FencingToken || existing.LeaseOwner != reservation.LeaseOwner || !existing.LeaseUntil.After(now.UTC()) {
			return errors.New("weekly occurrence lease cannot be renewed")
		}
		existing.LeaseUntil = now.UTC().Add(store.leaseDuration())
		existing.RecordedAt = now.UTC()
		return store.append(path, existing)
	})
}

func (store ReceiptStore) AssertLease(reservation Reservation, now time.Time) error {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	return store.withLock(func(path string) error {
		receipts, err := store.read(path)
		if err != nil {
			return err
		}
		existing, found := latestReceipt(receipts, reservation.Receipt.OccurrenceID)
		if !found || existing.State != ReceiptReserved || existing.FencingToken != reservation.Receipt.FencingToken || existing.LeaseOwner != reservation.LeaseOwner || !existing.LeaseUntil.After(now.UTC()) {
			return errors.New("weekly occurrence lease is stale or fenced")
		}
		return nil
	})
}

// CommitOwnerctxProposal is the Walter commit seam. The receipt advisory lock
// remains held while the current reserved lease/fence is reloaded and the
// occurrence-bound ownerctx proposal is created with create-if-absent/CAS
// semantics. A stale worker can therefore never write after takeover.
func (store ReceiptStore) CommitOwnerctxProposal(reservation Reservation, root string, input ownerctx.RefinementInput, now time.Time) (ownerctx.RefinementReceipt, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	var result ownerctx.RefinementReceipt
	err := store.withLock(func(path string) error {
		receipts, err := store.read(path)
		if err != nil {
			return err
		}
		existing, found := latestReceipt(receipts, reservation.Receipt.OccurrenceID)
		if !found || existing.State != ReceiptReserved || existing.FencingToken != reservation.Receipt.FencingToken || existing.LeaseOwner != reservation.LeaseOwner || !existing.LeaseUntil.After(now.UTC()) {
			return errors.New("weekly occurrence lease is stale or fenced before ownerctx commit")
		}
		input.WalterReviewFencingToken = reservation.Receipt.FencingToken
		result, err = ownerctx.SubmitRefinement(root, input)
		if err != nil {
			return err
		}
		if !existing.LeaseUntil.After(time.Now().UTC()) {
			return errors.New("weekly occurrence lease expired during ownerctx commit")
		}
		return nil
	})
	return result, err
}

func (store ReceiptStore) renewLoop(ctx context.Context, reservation Reservation, failures chan<- error) {
	interval := store.leaseDuration() / 3
	if interval < 10*time.Millisecond {
		interval = 10 * time.Millisecond
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			if err := store.Renew(reservation, now.UTC()); err != nil {
				select {
				case failures <- err:
				default:
				}
				return
			}
		}
	}
}

func (store ReceiptStore) withLock(operation func(string) error) error {
	root, err := ensurePrivateRoot(store.Root)
	if err != nil {
		return err
	}
	lockPath := filepath.Join(root, "weekly-receipts.lock")
	deadline := time.Now().Add(store.lockWait())
	for {
		if info, statErr := os.Lstat(lockPath); statErr == nil {
			if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
				return errors.New("weekly receipt lock is not a regular file")
			}
		} else if !errors.Is(statErr, os.ErrNotExist) {
			return statErr
		}
		file, openErr := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
		if openErr == nil {
			lockErr := tryLockWeeklyReceiptFile(file)
			if lockErr == nil {
				err := operation(filepath.Join(root, "weekly-receipts.jsonl"))
				unlockErr := unlockWeeklyReceiptFile(file)
				closeErr := file.Close()
				if err != nil {
					return err
				}
				if unlockErr != nil {
					return unlockErr
				}
				return closeErr
			}
			_ = file.Close()
			if !errors.Is(lockErr, errWeeklyReceiptLockBusy) {
				return lockErr
			}
		}
		if openErr != nil && !errors.Is(openErr, os.ErrNotExist) {
			return openErr
		}
		if time.Now().After(deadline) {
			return ErrOccurrenceBusy
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func (store ReceiptStore) lockWait() time.Duration {
	if store.LockWait <= 0 {
		return 3 * time.Second
	}
	return store.LockWait
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
	if err := validateCurrentSnapshot(request); err != nil {
		receipt, finalizeErr := store.Finalize(reservation, ReceiptFailed, SelfRefinementProposal{}, ownerctx.RefinementReceipt{}, "", "", "canonical_snapshot_stale", now)
		if finalizeErr != nil {
			return SelfRefinementProposal{}, receipt, finalizeErr
		}
		return SelfRefinementProposal{}, receipt, err
	}
	if reservation.Resumed {
		ownerReceipt, found, findErr := ownerctx.FindOccurrenceRefinement(request.OwnerContextRoot, request.OccurrenceID)
		if findErr != nil {
			return SelfRefinementProposal{}, Receipt{}, findErr
		}
		if found {
			// Recovery is bound to the immutable occurrence and request, not the
			// lease fence held by the worker that may have crashed after the
			// ownerctx commit. A takeover rotates the fence by design.
			if ownerReceipt.WalterRequestSHA256 != RequestDigest(request) || ownerReceipt.WalterProposalID == "" || ownerReceipt.WalterProposalSHA256 == "" {
				return SelfRefinementProposal{}, Receipt{}, errors.New("Walter occurrence artifact is bound to a different request")
			}
			policy, policyErr := request.CanonicalSnapshot.Policy(ownerReceipt.Facet)
			if policyErr != nil || !contains(request.ReviewFacets, ownerReceipt.Facet) || ownerReceipt.WalterSensitivity != policy.Sensitivity || !sameStrings(ownerReceipt.WalterReaders, policy.Readers) || ownerReceipt.WalterRefinement != policy.Refinement || ownerReceipt.WalterConfirmation != policy.ConfirmationRequirement {
				return SelfRefinementProposal{}, Receipt{}, errors.New("Walter occurrence artifact policy is stale")
			}
			resumedProposal := SelfRefinementProposal{SchemaVersion: SchemaVersion, ProposalID: ownerReceipt.WalterProposalID, State: "proposed", WeekID: request.WeekID, Facet: ownerReceipt.Facet, Sensitivity: ownerReceipt.WalterSensitivity, Readers: append([]string(nil), ownerReceipt.WalterReaders...), Refinement: ownerReceipt.WalterRefinement, ConfirmationRequirement: ConfirmationRequirement(ownerReceipt.WalterConfirmation), CanonicalSnapshotVersion: request.CanonicalSnapshot.Version, CanonicalSnapshotSHA256: request.CanonicalSnapshot.CanonicalSourceDigest, PromptHistorySHA256: PromptHistoryDigest(request.PromptHistory), TranslationReceiptSHA256: request.Translation.ReceiptSHA256, IntentHypothesisSHA256: request.IntentHypothesisSHA256}
			terminal, finalizeErr := store.Finalize(reservation, ReceiptProposal, resumedProposal, ownerReceipt, ownerReceipt.WalterAdapterID, ownerReceipt.WalterAuthorityID, "proposal_resumed_without_model", now)
			if finalizeErr != nil {
				return SelfRefinementProposal{}, terminal, finalizeErr
			}
			return resumedProposal, terminal, nil
		}
	}
	if adapter == nil || authority == nil || !authority.Approved("walter/self-review-weekly") {
		receipt, finalizeErr := store.Finalize(reservation, ReceiptUnavailable, SelfRefinementProposal{}, ownerctx.RefinementReceipt{}, "", "", "approved_model_adapter_or_authority_unavailable", now)
		if finalizeErr != nil {
			return SelfRefinementProposal{}, receipt, finalizeErr
		}
		return SelfRefinementProposal{}, receipt, ErrUnavailable
	}
	renewCtx, cancelRenew := context.WithCancel(ctx)
	defer cancelRenew()
	renewFailures := make(chan error, 1)
	go store.renewLoop(renewCtx, reservation, renewFailures)
	type modelResult struct {
		proposal SelfRefinementProposal
		err      error
	}
	modelResults := make(chan modelResult, 1)
	go func() {
		proposal, err := adapter.Review(renewCtx, ModelInput{CurrentOriginal: request.CurrentOriginal, CurrentNormalized: request.CurrentNormalized, History: append([]PromptEvidence(nil), request.PromptHistory...), Translation: request.Translation, Observations: append([]ownerctx.ObservationReceipt(nil), request.Observations...), CanonicalSnapshot: request.CanonicalSnapshot, IntentHypothesis: request.IntentHypothesis})
		modelResults <- modelResult{proposal: proposal, err: err}
	}()
	var result modelResult
	select {
	case result = <-modelResults:
	case <-renewCtx.Done():
		receipt, finalizeErr := store.Finalize(reservation, ReceiptFailed, SelfRefinementProposal{}, ownerctx.RefinementReceipt{}, adapter.ID(), authority.ID(), "execution_deadline_exceeded", time.Now().UTC())
		if finalizeErr != nil {
			return SelfRefinementProposal{}, receipt, finalizeErr
		}
		return SelfRefinementProposal{}, receipt, renewCtx.Err()
	}
	proposal, err := result.proposal, result.err
	if err != nil {
		receipt, finalizeErr := store.Finalize(reservation, ReceiptFailed, SelfRefinementProposal{}, ownerctx.RefinementReceipt{}, adapter.ID(), authority.ID(), "model_adapter_error", now)
		if finalizeErr != nil {
			return SelfRefinementProposal{}, receipt, finalizeErr
		}
		return SelfRefinementProposal{}, receipt, err
	}
	if ctxErr := renewCtx.Err(); ctxErr != nil {
		receipt, finalizeErr := store.Finalize(reservation, ReceiptFailed, SelfRefinementProposal{}, ownerctx.RefinementReceipt{}, adapter.ID(), authority.ID(), "execution_deadline_exceeded", time.Now().UTC())
		if finalizeErr != nil {
			return SelfRefinementProposal{}, receipt, finalizeErr
		}
		return SelfRefinementProposal{}, receipt, ctxErr
	}
	if err := ValidateProposal(request, proposal); err != nil {
		receipt, finalizeErr := store.Finalize(reservation, ReceiptFailed, SelfRefinementProposal{}, ownerctx.RefinementReceipt{}, adapter.ID(), authority.ID(), "proposal_invalid_or_stale", now)
		if finalizeErr != nil {
			return SelfRefinementProposal{}, receipt, finalizeErr
		}
		return SelfRefinementProposal{}, receipt, err
	}
	if err := validateCurrentSnapshot(request); err != nil {
		receipt, finalizeErr := store.Finalize(reservation, ReceiptFailed, SelfRefinementProposal{}, ownerctx.RefinementReceipt{}, adapter.ID(), authority.ID(), "canonical_snapshot_stale", now)
		if finalizeErr != nil {
			return SelfRefinementProposal{}, receipt, finalizeErr
		}
		return SelfRefinementProposal{}, receipt, err
	}
	select {
	case renewErr := <-renewFailures:
		return SelfRefinementProposal{}, Receipt{}, renewErr
	default:
	}
	if ctxErr := renewCtx.Err(); ctxErr != nil {
		receipt, finalizeErr := store.Finalize(reservation, ReceiptFailed, SelfRefinementProposal{}, ownerctx.RefinementReceipt{}, adapter.ID(), authority.ID(), "execution_deadline_exceeded", time.Now().UTC())
		if finalizeErr != nil {
			return SelfRefinementProposal{}, receipt, finalizeErr
		}
		return SelfRefinementProposal{}, receipt, ctxErr
	}
	ownerReceipt, err := store.CommitOwnerctxProposal(reservation, request.OwnerContextRoot, ownerctx.RefinementInput{Facet: proposal.Facet, Evidence: "walter-weekly:" + DigestJSON(proposal.EvidenceObservationIDs), ProposedBody: proposal.ProposedRefinement, OccurrenceID: request.OccurrenceID, WalterReviewRequestSHA256: RequestDigest(request), WalterReviewProposalID: proposal.ProposalID, WalterReviewProposalSHA256: DigestJSON(proposal), WalterReviewSensitivity: proposal.Sensitivity, WalterReviewReaders: proposal.Readers, WalterReviewRefinement: proposal.Refinement, WalterReviewConfirmation: string(proposal.ConfirmationRequirement), WalterReviewAdapterID: adapter.ID(), WalterReviewAuthorityID: authority.ID()}, time.Now().UTC())
	if err != nil {
		receipt, finalizeErr := store.Finalize(reservation, ReceiptFailed, SelfRefinementProposal{}, ownerctx.RefinementReceipt{}, adapter.ID(), authority.ID(), "ownerctx_proposal_failed", now)
		if finalizeErr != nil {
			return SelfRefinementProposal{}, receipt, finalizeErr
		}
		return SelfRefinementProposal{}, receipt, err
	}
	policy, policyErr := request.CanonicalSnapshot.Policy(proposal.Facet)
	if policyErr != nil || ownerReceipt.ID == "" || ownerReceipt.ProposalSHA256 == "" || ownerReceipt.Policy != policy.Refinement || ownerReceipt.Sensitivity != policy.Sensitivity || !sameStrings(ownerReceipt.Readers, policy.Readers) {
		receipt, finalizeErr := store.Finalize(reservation, ReceiptFailed, SelfRefinementProposal{}, ownerctx.RefinementReceipt{}, adapter.ID(), authority.ID(), "ownerctx_policy_binding_mismatch", now)
		if finalizeErr != nil {
			return SelfRefinementProposal{}, receipt, finalizeErr
		}
		return SelfRefinementProposal{}, receipt, errors.New("ownerctx proposal policy binding is stale")
	}
	receipt, err := store.Finalize(reservation, ReceiptProposal, proposal, ownerReceipt, adapter.ID(), authority.ID(), "proposal_durable", now)
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
	ReviewFacets      []string
	SensitivePurpose  string
	OwnerAuthorized   bool
	IntentHypothesis  *maestro.IntentHypothesis
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

var _ maintenance.WalterWeeklyProposalHandler = Handler{}

func (handler Handler) ExecuteAuthorized(ctx context.Context, command maintenance.Command, grant maintenance.ExecutionGrant) (maintenance.HandlerResult, error) {
	if err := maintenance.ValidateExecutionGrant(grant, command); err != nil {
		return maintenance.HandlerResult{}, err
	}
	receipt, err := handler.execute(ctx, command)
	return maintenance.HandlerResult{
		State:              receipt.State,
		ProposalCount:      receipt.ProposalCount,
		ProposalDigest:     receipt.ProposalDigest,
		ProposalArtifactID: receipt.ProposalArtifactID,
		ReasonCode:         receipt.ReasonCode,
	}, err
}

func (handler Handler) ProposeWeekly(ctx context.Context, command maintenance.Command, grant maintenance.ExecutionGrant) (maintenance.HandlerResult, error) {
	return handler.ExecuteAuthorized(ctx, command, grant)
}

func (handler Handler) Execute(ctx context.Context, command maintenance.Command) (maintenance.HandlerResult, error) {
	receipt, err := handler.execute(ctx, command)
	return maintenance.HandlerResult{
		State:              receipt.State,
		ProposalCount:      receipt.ProposalCount,
		ProposalDigest:     receipt.ProposalDigest,
		ProposalArtifactID: receipt.ProposalArtifactID,
		ReasonCode:         receipt.ReasonCode,
	}, err
}

func (handler Handler) Handle(ctx context.Context, command maintenance.Command) (maintenance.Receipt, error) {
	receipt, err := handler.execute(ctx, command)
	if receipt.SchemaVersion == 0 {
		return receipt, err
	}
	if appendErr := handler.MaintenanceStore.AppendReceipt(receipt); appendErr != nil {
		return receipt, appendErr
	}
	return receipt, err
}

// execute is the publication-free Walter seam. The Worker owns the outer
// occurrence lease and the single terminal maintenance receipt; Handle keeps
// direct callers deterministic by publishing through the dedicated store.
func (handler Handler) execute(ctx context.Context, command maintenance.Command) (maintenance.Receipt, error) {
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
	executionCtx, cancel := context.WithDeadline(ctx, command.Deadline)
	defer cancel()
	_, receipt, reviewErr := Review(executionCtx, request, handler.Adapter, handler.Authority, handler.Store, now)
	maintenanceReceipt := handler.toMaintenanceReceipt(command, receipt, reviewErr, now)
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
	facets, err := resolveReviewFacets(handler.ReviewFacets, handler.SensitivePurpose, handler.OwnerAuthorized)
	if err != nil {
		return Request{}, err
	}
	snapshot, err := ownerctx.ProjectSnapshot(handler.Root, facets)
	if err != nil {
		return Request{}, err
	}
	observations, err := ownerctx.ListObservations(handler.Root)
	if err != nil {
		return Request{}, err
	}
	filtered := make([]ownerctx.ObservationReceipt, 0, len(observations))
	for _, observation := range observations {
		if contains(facets, observation.Facet) && ownerctx.IsWalterWeeklyEligible(observation) {
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
	if err := validateRawRequestInputs(handler.CurrentPrompt, selected, snapshot, handler.IntentHypothesis); err != nil {
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
	intentDigest := ""
	if handler.IntentHypothesis != nil {
		intentDigest = DigestJSON(handler.IntentHypothesis)
	}
	request := Request{SchemaVersion: SchemaVersion, WeekID: weekID(now), OccurrenceID: command.OccurrenceDigest(), OwnerID: handler.OwnerID, ScopeKind: handler.ScopeKind, ScopeID: handler.ScopeID, CurrentNormalized: currentNormalized, CurrentOriginal: handler.CurrentPrompt, PromptHistory: history, Translation: translation, Observations: filtered, CanonicalSnapshot: snapshot, ReviewFacets: facets, SensitiveOwnerAuthorized: handler.OwnerAuthorized, IntentHypothesis: handler.IntentHypothesis, IntentHypothesisSHA256: intentDigest, OwnerContextRoot: handler.Root}
	if strings.TrimSpace(handler.SensitivePurpose) != "" {
		request.SensitivePurposeSHA256 = maestro.SHA256Hex(handler.SensitivePurpose)
	}
	if err := ValidateRequest(request); err != nil {
		return Request{}, err
	}
	return request, nil
}

func (handler Handler) toMaintenanceReceipt(command maintenance.Command, receipt Receipt, _ error, now time.Time) maintenance.Receipt {
	state := maintenance.ReceiptFailed
	reasonCode := maintenance.ReasonHandlerFailure
	if receipt.State == ReceiptUnavailable {
		state = maintenance.ReceiptUnavailable
		reasonCode = maintenance.ReasonHandlerUnavailable
	}
	if receipt.State == ReceiptProposal {
		state = maintenance.ReceiptProposalEmitted
		reasonCode = maintenance.ReasonProposalEmitted
	}
	value := maintenance.Receipt{SchemaVersion: maintenance.CommandSchemaVersion, AttemptID: Digest(command.OccurrenceDigest() + receipt.FencingToken)[:32], OccurrenceDigest: command.OccurrenceDigest(), CommandID: command.CommandID, JobID: command.JobID, WorkspaceID: command.WorkspaceID, Trigger: command.Trigger, State: state, RecordedAt: now, Deadline: command.Deadline, ProposalOnly: true, ReasonCode: reasonCode}
	if state == maintenance.ReceiptProposalEmitted {
		value.ProposalCount = 1
		value.ProposalDigest = receipt.ProposalSHA256
		value.ProposalArtifactID = receipt.OwnerctxProposalSHA256
		if value.ProposalArtifactID == "" {
			value.ProposalArtifactID = receipt.ProposalSHA256
		}
	}
	return value
}

func ValidateRequest(request Request) error {
	if request.SchemaVersion != SchemaVersion || !validID(request.WeekID) || !validID(request.OccurrenceID) || strings.TrimSpace(request.OwnerID) == "" || strings.TrimSpace(request.CurrentOriginal) == "" || strings.TrimSpace(request.CurrentNormalized) == "" || len(request.PromptHistory) > MaxPromptEntries || len(request.Observations) > MaxObservations || len(request.ReviewFacets) == 0 {
		return errors.New("Walter weekly self-review request is invalid")
	}
	if err := request.CanonicalSnapshot.Validate(); err != nil {
		return err
	}
	hypothesisDigest := ""
	if request.IntentHypothesis != nil {
		if err := validateIntentHypothesis(request); err != nil {
			return err
		}
		hypothesisDigest = DigestJSON(request.IntentHypothesis)
	}
	if request.IntentHypothesisSHA256 != hypothesisDigest {
		return errors.New("Walter intent hypothesis digest is missing or stale")
	}
	if err := validateReviewFacets(request); err != nil {
		return err
	}
	if strings.TrimSpace(request.Translation.TranslatorID) == "" || strings.TrimSpace(request.Translation.TranslatorVersion) == "" || strings.TrimSpace(request.Translation.SourceLanguage) == "" || strings.TrimSpace(request.Translation.WorkingLanguage) == "" {
		return errors.New("Walter translation receipt is missing translator identity or language binding")
	}
	if request.Translation.ReceiptSHA256 != DigestJSON(TranslationReceipt{TranslatorID: request.Translation.TranslatorID, TranslatorVersion: request.Translation.TranslatorVersion, SourceLanguage: request.Translation.SourceLanguage, WorkingLanguage: request.Translation.WorkingLanguage, OriginalSHA256: request.Translation.OriginalSHA256, WorkingSHA256: request.Translation.WorkingSHA256, HistorySHA256: request.Translation.HistorySHA256}) || request.Translation.OriginalSHA256 != maestro.SHA256Hex(request.CurrentOriginal) || request.Translation.WorkingSHA256 != maestro.SHA256Hex(request.CurrentNormalized) {
		return errors.New("Walter translation receipt is stale or incomplete")
	}
	if err := validateTranslationPair(request.CurrentOriginal, request.CurrentNormalized); err != nil {
		return err
	}
	historyDigests := make([]string, 0, len(request.PromptHistory))
	for _, prompt := range request.PromptHistory {
		if !prompt.QuotedData || prompt.Instructional || prompt.OriginalSHA256 != maestro.SHA256Hex(prompt.OriginalText) || prompt.NormalizedSHA256 != maestro.SHA256Hex(prompt.NormalizedText) || prompt.WorkingLanguage != request.Translation.WorkingLanguage || strings.TrimSpace(prompt.OriginalText) == "" || strings.TrimSpace(prompt.NormalizedText) == "" || strings.TrimSpace(prompt.SourceLanguage) == "" {
			return errors.New("Walter prompt history is not quoted, normalized and digest-bound")
		}
		if err := validateTranslationPair(prompt.OriginalText, prompt.NormalizedText); err != nil {
			return err
		}
		historyDigests = append(historyDigests, prompt.OriginalSHA256+":"+prompt.NormalizedSHA256)
	}
	sort.Strings(historyDigests)
	if request.Translation.HistorySHA256 != Digest(strings.Join(historyDigests, "\n")) {
		return errors.New("Walter translation receipt history digest is stale or incomplete")
	}
	for _, observation := range request.Observations {
		if observation.ID == "" || observation.SourceDigest == "" || observation.Claim == "" {
			return errors.New("Walter observation evidence is incomplete")
		}
	}
	return validateContextBounds(request)
}

func ValidateProposal(request Request, proposal SelfRefinementProposal) error {
	if err := ValidateRequest(request); err != nil {
		return err
	}
	if proposal.SchemaVersion != SchemaVersion || proposal.State != "proposed" || strings.TrimSpace(proposal.ProposalID) == "" || !validFacet(proposal.Facet) || !contains(request.ReviewFacets, proposal.Facet) || strings.TrimSpace(proposal.PriorClaim) == "" || strings.TrimSpace(proposal.ProposedRefinement) == "" || len([]byte(proposal.PriorClaim)) > MaxProposalBytes || len([]byte(proposal.ProposedRefinement)) > MaxProposalBytes || proposal.WeekID != request.WeekID || proposal.CanonicalSnapshotVersion != request.CanonicalSnapshot.Version || proposal.CanonicalSnapshotSHA256 != request.CanonicalSnapshot.CanonicalSourceDigest || proposal.PromptHistorySHA256 != DigestJSON(request.PromptHistory) || proposal.TranslationReceiptSHA256 != request.Translation.ReceiptSHA256 || !validConfidence(proposal.Confidence) || strings.TrimSpace(proposal.Sensitivity) == "" || len(proposal.EvidenceObservationIDs) == 0 || len(proposal.EvidenceObservationIDs) > MaxObservations || !validConfirmation(proposal.ConfirmationRequirement) || strings.TrimSpace(proposal.Refinement) == "" || len(proposal.Readers) == 0 {
		return errors.New("Walter self-refinement proposal is incomplete or stale")
	}
	if proposal.IntentHypothesisSHA256 != request.IntentHypothesisSHA256 {
		return errors.New("Walter intrinsic-purpose hypothesis binding is stale")
	}
	available := map[string]ownerctx.ObservationReceipt{}
	for _, observation := range request.Observations {
		available[observation.ID] = observation
	}
	seen := map[string]bool{}
	for _, id := range proposal.EvidenceObservationIDs {
		observation, ok := available[id]
		if !ok || seen[id] || !ownerctx.IsWalterWeeklyEligible(observation) {
			return errors.New("Walter proposal evidence requires independent corroboration")
		}
		if observation.Facet != proposal.Facet {
			return errors.New("Walter proposal evidence facet does not match the proposed facet")
		}
		seen[id] = true
	}
	policy, err := request.CanonicalSnapshot.Policy(proposal.Facet)
	if err != nil || proposal.Sensitivity != policy.Sensitivity || !sameStrings(proposal.Readers, policy.Readers) || proposal.Refinement != policy.Refinement || string(proposal.ConfirmationRequirement) != policy.ConfirmationRequirement {
		return errors.New("Walter proposal policy does not exactly match the canonical self policy")
	}
	return nil
}

func ApplyCanonicalMutation(SelfRefinementProposal, ownerctx.UserSelfSnapshot) error {
	return ErrProposalOnly
}

func resolveReviewFacets(requested []string, sensitivePurpose string, ownerAuthorized bool) ([]string, error) {
	if len(requested) == 0 {
		return nil, errors.New("Walter review requires an explicit minimal facet allowlist")
	}
	seen := make(map[string]bool, len(requested))
	facets := make([]string, 0, len(requested))
	for _, facet := range requested {
		facet = strings.TrimSpace(facet)
		if !validFacet(facet) || seen[facet] {
			if !validFacet(facet) {
				return nil, errors.New("Walter review facet is not canonical")
			}
			continue
		}
		seen[facet] = true
		facets = append(facets, facet)
	}
	sort.Strings(facets)
	if contains(facets, "psychological-profile") && (strings.TrimSpace(sensitivePurpose) == "" || !ownerAuthorized) {
		return nil, errors.New("psychological self facet requires explicit purpose and owner authorization")
	}
	return facets, nil
}

func validateReviewFacets(request Request) error {
	if len(request.ReviewFacets) != len(request.CanonicalSnapshot.Facets) {
		return errors.New("Walter request facet allowlist does not match its snapshot")
	}
	seen := map[string]bool{}
	for _, facet := range request.ReviewFacets {
		if !validFacet(facet) || seen[facet] {
			return errors.New("Walter request facet allowlist is invalid")
		}
		if _, ok := request.CanonicalSnapshot.Facets[facet]; !ok {
			return errors.New("Walter request facet is absent from its snapshot")
		}
		seen[facet] = true
	}
	if _, ok := request.CanonicalSnapshot.Facets["psychological-profile"]; ok && (!request.SensitiveOwnerAuthorized || request.SensitivePurposeSHA256 == "") {
		return errors.New("Walter request includes a sensitive self facet without authorization binding")
	}
	return nil
}

func validateCurrentSnapshot(request Request) error {
	if strings.TrimSpace(request.OwnerContextRoot) == "" {
		return nil
	}
	current, err := ownerctx.ProjectSnapshot(request.OwnerContextRoot, append([]string(nil), request.ReviewFacets...))
	if err != nil {
		return err
	}
	if current.Version != request.CanonicalSnapshot.Version || current.CanonicalSourceDigest != request.CanonicalSnapshot.CanonicalSourceDigest {
		return ownerctx.ErrSnapshotStale
	}
	return nil
}

func validateTranslationPair(original, normalized string) error {
	originalBytes, normalizedBytes := len([]byte(original)), len([]byte(normalized))
	if originalBytes == 0 || normalizedBytes == 0 || originalBytes > MaxContextBytes || normalizedBytes > MaxContextBytes {
		return errors.New("Walter prompt field exceeds its UTF-8 bound")
	}
	if normalizedBytes > originalBytes*MaxTranslationExpansion+MaxTranslationOverhead {
		return errors.New("Walter translation expansion exceeds its bound")
	}
	return nil
}

func validateContextBounds(request Request) error {
	total := 0
	add := func(value string) error {
		bytes := len([]byte(value))
		if bytes > MaxContextBytes {
			return errors.New("Walter model input field exceeds its UTF-8 bound")
		}
		total += bytes
		if total > MaxContextBytes {
			return errors.New("Walter model input exceeds its combined UTF-8 bound")
		}
		return nil
	}
	if err := add(request.CurrentOriginal); err != nil {
		return err
	}
	if err := add(request.CurrentNormalized); err != nil {
		return err
	}
	for _, prompt := range request.PromptHistory {
		if err := add(prompt.OriginalText); err != nil {
			return err
		}
		if err := add(prompt.NormalizedText); err != nil {
			return err
		}
	}
	for _, facet := range request.CanonicalSnapshot.Facets {
		if err := add(facet.Content); err != nil {
			return err
		}
	}
	if request.IntentHypothesis != nil {
		hypothesis := request.IntentHypothesis
		for _, value := range []string{hypothesis.ExpressedObjective, hypothesis.LatentIntentHypothesis, hypothesis.Materiality, hypothesis.DisconfirmationCondition, hypothesis.WorkingPrompt} {
			if err := add(value); err != nil {
				return err
			}
		}
		for _, value := range append(append([]string(nil), hypothesis.EvidenceRefs...), hypothesis.Alternatives...) {
			if err := add(value); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateIntentHypothesis(request Request) error {
	hypothesis := request.IntentHypothesis
	if hypothesis == nil || strings.TrimSpace(hypothesis.ExpressedObjective) == "" || strings.TrimSpace(hypothesis.LatentIntentHypothesis) == "" || strings.TrimSpace(hypothesis.Materiality) == "" || strings.TrimSpace(hypothesis.DisconfirmationCondition) == "" || strings.TrimSpace(hypothesis.WorkingPrompt) == "" || hypothesis.WorkingPrompt != request.CurrentNormalized || !validConfidence(hypothesis.Confidence) || len(hypothesis.EvidenceRefs) == 0 || len(hypothesis.EvidenceRefs) > MaxObservations || len(hypothesis.Alternatives) > MaxObservations {
		return errors.New("Walter intent hypothesis is incomplete")
	}
	for _, value := range []string{hypothesis.ExpressedObjective, hypothesis.LatentIntentHypothesis, hypothesis.Materiality, hypothesis.DisconfirmationCondition, hypothesis.WorkingPrompt} {
		if len([]byte(value)) > MaxContextBytes {
			return errors.New("Walter intent hypothesis exceeds its UTF-8 bound")
		}
	}
	seen := make(map[string]bool, len(hypothesis.EvidenceRefs))
	allowed := map[string]bool{"current_prompt": true, maestro.SHA256Hex(request.CurrentOriginal): true, "current_prompt:" + maestro.SHA256Hex(request.CurrentOriginal): true}
	for _, prompt := range request.PromptHistory {
		for _, value := range []string{prompt.ID, prompt.OriginalSHA256, prompt.NormalizedSHA256, "prompt_history:" + prompt.ID, "prompt_history:" + prompt.OriginalSHA256, "prompt_history:" + prompt.NormalizedSHA256} {
			allowed[value] = true
		}
	}
	for _, observation := range request.Observations {
		allowed[observation.ID] = true
		allowed["observation:"+observation.ID] = true
	}
	for _, value := range hypothesis.EvidenceRefs {
		if strings.TrimSpace(value) == "" || len([]byte(value)) > MaxContextBytes || seen[value] || !allowed[value] {
			return errors.New("Walter intent hypothesis evidence reference is invalid or outside the request")
		}
		seen[value] = true
	}
	if !seen["current_prompt"] {
		return errors.New("Walter intent hypothesis must include the canonical current prompt evidence reference")
	}
	for _, value := range hypothesis.Alternatives {
		if strings.TrimSpace(value) == "" || len([]byte(value)) > MaxContextBytes {
			return errors.New("Walter intent hypothesis metadata is invalid")
		}
	}
	return nil
}

func validateRawRequestInputs(current string, selected []ownerctx.PromptHistorySelection, snapshot ownerctx.UserSelfSnapshot, hypothesis *maestro.IntentHypothesis) error {
	total := 0
	add := func(value string) error {
		bytes := len([]byte(value))
		if bytes > MaxContextBytes {
			return errors.New("Walter raw model input field exceeds its UTF-8 bound")
		}
		total += bytes
		if total > MaxContextBytes {
			return errors.New("Walter raw model input exceeds its combined UTF-8 bound")
		}
		return nil
	}
	if err := add(current); err != nil {
		return err
	}
	for _, item := range selected {
		if err := add(item.Entry.Prompt); err != nil {
			return err
		}
	}
	for _, facet := range snapshot.Facets {
		if err := add(facet.Content); err != nil {
			return err
		}
	}
	if hypothesis != nil {
		for _, value := range []string{hypothesis.ExpressedObjective, hypothesis.LatentIntentHypothesis, hypothesis.Materiality, hypothesis.DisconfirmationCondition, hypothesis.WorkingPrompt} {
			if err := add(value); err != nil {
				return err
			}
		}
		for _, value := range append(append([]string(nil), hypothesis.EvidenceRefs...), hypothesis.Alternatives...) {
			if err := add(value); err != nil {
				return err
			}
		}
	}
	return nil
}

func contains(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}

func sameStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	leftCopy, rightCopy := append([]string(nil), left...), append([]string(nil), right...)
	sort.Strings(leftCopy)
	sort.Strings(rightCopy)
	for index := range leftCopy {
		if leftCopy[index] != rightCopy[index] {
			return false
		}
	}
	return true
}

func validFacet(value string) bool {
	switch value {
	case "professional-role", "communication-style", "voice", "preferences", "motivations", "quality-bar", "decision-rules", "working-boundaries", "psychological-profile":
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
	// GeneratedAt is projection metadata, not request identity. Excluding it
	// makes retries of the same occurrence idempotent when a fresh projection
	// is assembled after a command receives a new attempt ID.
	stable := request
	stable.CanonicalSnapshot.GeneratedAt = time.Time{}
	body, _ := json.Marshal(stable)
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
	if len(receipt.LeaseOwner) != 32 || receipt.LeaseUntil.IsZero() {
		return errors.New("weekly Walter receipt lease binding is invalid")
	}
	switch receipt.State {
	case ReceiptReserved, ReceiptUnavailable, ReceiptFailed:
		return nil
	case ReceiptProposal:
		if receipt.ProposalID == "" || receipt.ProposalSHA256 == "" || receipt.OwnerctxProposalID == "" || receipt.OwnerctxProposalSHA256 == "" || receipt.OwnerctxPolicy == "" || receipt.CanonicalSensitivity == "" || len(receipt.CanonicalReaders) == 0 || receipt.CanonicalRefinement == "" || !validConfirmation(receipt.CanonicalConfirmationRequirement) {
			return errors.New("weekly Walter proposal receipt is missing canonical ownerctx binding")
		}
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
