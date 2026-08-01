package execution

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/reviewcustody"
)

type WalterReviewDecision string

const (
	WalterReviewApproved WalterReviewDecision = "approved"
	WalterReviewRejected WalterReviewDecision = "rejected"
)

var ErrWalterReviewUnsatisfied = errors.New("authenticated Walter review is not satisfied")

// WalterReviewEnvelope is signed outside the execution core. It binds one
// decision to the exact item, contract, attempt and pre-review ledger revision.
// The signature never enters transition history.
type WalterReviewEnvelope struct {
	SchemaVersion    int                  `json:"schema_version"`
	ItemID           string               `json:"item_id"`
	WorkspaceID      string               `json:"workspace_id"`
	AttemptID        string               `json:"attempt_id"`
	ReviewedRevision int                  `json:"reviewed_revision"`
	ContractSHA256   string               `json:"contract_sha256"`
	SignerKeyID      string               `json:"signer_key_id"`
	InstallationID   string               `json:"installation_id"`
	CustodyScope     string               `json:"custody_scope"`
	Decision         WalterReviewDecision `json:"decision"`
	Nonce            string               `json:"nonce"`
	IssuedAt         time.Time            `json:"issued_at"`
	Signature        string               `json:"signature"`
}

type walterReviewSigningPayload struct {
	SchemaVersion    int                  `json:"schema_version"`
	ItemID           string               `json:"item_id"`
	WorkspaceID      string               `json:"workspace_id"`
	AttemptID        string               `json:"attempt_id"`
	ReviewedRevision int                  `json:"reviewed_revision"`
	ContractSHA256   string               `json:"contract_sha256"`
	SignerKeyID      string               `json:"signer_key_id"`
	InstallationID   string               `json:"installation_id"`
	CustodyScope     string               `json:"custody_scope"`
	Decision         WalterReviewDecision `json:"decision"`
	Nonce            string               `json:"nonce"`
	IssuedAt         time.Time            `json:"issued_at"`
}

// WalterReviewSigningPayload returns the canonical bytes signed by a Walter
// adapter. The core verifies those bytes against the public key frozen in the
// immutable execution contract.
func WalterReviewSigningPayload(envelope WalterReviewEnvelope) ([]byte, error) {
	return json.Marshal(walterReviewSigningPayload{
		SchemaVersion:    envelope.SchemaVersion,
		ItemID:           envelope.ItemID,
		WorkspaceID:      envelope.WorkspaceID,
		AttemptID:        envelope.AttemptID,
		ReviewedRevision: envelope.ReviewedRevision,
		ContractSHA256:   envelope.ContractSHA256,
		SignerKeyID:      envelope.SignerKeyID,
		InstallationID:   envelope.InstallationID,
		CustodyScope:     envelope.CustodyScope,
		Decision:         envelope.Decision,
		Nonce:            envelope.Nonce,
		IssuedAt:         envelope.IssuedAt,
	})
}

// WalterReviewReceipt is metadata-only. It records what ledger revision was
// reviewed and which immutable key authenticated the decision, not rationale.
type WalterReviewReceipt struct {
	SchemaVersion    int                  `json:"schema_version"`
	ReviewID         string               `json:"review_id"`
	ItemID           string               `json:"item_id"`
	WorkspaceID      string               `json:"workspace_id"`
	AttemptID        string               `json:"attempt_id"`
	ReviewedRevision int                  `json:"reviewed_revision"`
	RecordedRevision int                  `json:"recorded_revision"`
	ContractSHA256   string               `json:"contract_sha256"`
	SignerKeyID      string               `json:"signer_key_id"`
	InstallationID   string               `json:"installation_id"`
	CustodyScope     string               `json:"custody_scope"`
	Decision         WalterReviewDecision `json:"decision"`
	SignerSHA256     string               `json:"signer_sha256"`
	EnvelopeSHA256   string               `json:"envelope_sha256"`
	ObservedAt       time.Time            `json:"observed_at"`
}

type WalterReviewInput struct {
	ExpectedRevision int
	AttemptID        string
	Envelope         WalterReviewEnvelope
}

type WalterReviewResult struct {
	Item    Item
	Receipt WalterReviewReceipt
}

func (store Store) RecordWalterReview(workspaceID, itemID string, input WalterReviewInput) (WalterReviewResult, error) {
	if err := store.validateItemInput(workspaceID, itemID); err != nil {
		return WalterReviewResult{}, err
	}
	if input.ExpectedRevision < 1 {
		return WalterReviewResult{}, errors.New("Walter review expected revision must be positive")
	}
	if err := validateID("attempt", input.AttemptID); err != nil {
		return WalterReviewResult{}, err
	}
	unlock, err := store.lock(workspaceID, itemID)
	if err != nil {
		return WalterReviewResult{}, err
	}
	defer unlock()

	item, err := store.inspectUnlocked(workspaceID, itemID)
	if err != nil {
		return WalterReviewResult{}, err
	}
	if item.State.StateRevision != input.ExpectedRevision {
		return WalterReviewResult{}, ErrRevisionConflict
	}
	if item.State.State != StateRunning || item.Attempt == nil ||
		item.Attempt.State != AttemptActive ||
		item.State.ActiveAttemptID != input.AttemptID ||
		item.Attempt.AttemptID != input.AttemptID {
		return WalterReviewResult{}, ErrAttemptConflict
	}
	if !item.Contract.RequireWalterReview {
		return WalterReviewResult{}, errors.New("execution contract does not require Walter review")
	}
	if err := verifyWalterReviewEnvelope(item, input.Envelope, store.now()); err != nil {
		return WalterReviewResult{}, err
	}

	envelopeBody, err := json.Marshal(input.Envelope)
	if err != nil {
		return WalterReviewResult{}, err
	}
	envelopeDigest := sha256.Sum256(envelopeBody)
	envelopeSHA256 := hex.EncodeToString(envelopeDigest[:])
	reviews, err := store.walterReviews(workspaceID, itemID)
	if err != nil {
		return WalterReviewResult{}, err
	}
	for _, review := range reviews {
		if review.EnvelopeSHA256 == envelopeSHA256 {
			return WalterReviewResult{}, errors.New("Walter review envelope was already recorded")
		}
	}

	reviewID, err := store.newID("review")
	if err != nil {
		return WalterReviewResult{}, err
	}
	if err := validateID("review", reviewID); err != nil {
		return WalterReviewResult{}, err
	}
	publicKey, err := decodeWalterPublicKey(item.Contract.WalterPublicKey)
	if err != nil {
		return WalterReviewResult{}, err
	}
	keyDigest := sha256.Sum256(publicKey)
	now := store.now()
	state := item.State
	state.StateRevision++
	state.UpdatedAt = now
	receipt := WalterReviewReceipt{
		SchemaVersion:    1,
		ReviewID:         reviewID,
		ItemID:           itemID,
		WorkspaceID:      workspaceID,
		AttemptID:        input.AttemptID,
		ReviewedRevision: input.ExpectedRevision,
		RecordedRevision: state.StateRevision,
		ContractSHA256:   item.State.ContractSHA256,
		SignerKeyID:      input.Envelope.SignerKeyID,
		InstallationID:   input.Envelope.InstallationID,
		CustodyScope:     input.Envelope.CustodyScope,
		Decision:         input.Envelope.Decision,
		SignerSHA256:     hex.EncodeToString(keyDigest[:]),
		EnvelopeSHA256:   envelopeSHA256,
		ObservedAt:       now,
	}
	transition := Transition{
		SchemaVersion: 1, ItemID: itemID, WorkspaceID: workspaceID,
		AttemptID: input.AttemptID, State: StateRunning,
		StateRevision: state.StateRevision, OccurredAt: now,
	}
	revision := Revision{
		SchemaVersion: 1, State: state, Attempt: item.Attempt,
		Checkpoint: item.Checkpoint, WalterReview: &receipt, Transition: transition,
	}
	if err := validateRevision(revision); err != nil {
		return WalterReviewResult{}, err
	}
	if err := store.commitRevision(workspaceID, itemID, revision); err != nil {
		return WalterReviewResult{}, err
	}
	return WalterReviewResult{
		Item: Item{
			Contract: item.Contract, State: state, Attempt: item.Attempt,
			Checkpoint: item.Checkpoint,
		},
		Receipt: receipt,
	}, nil
}

func verifyWalterReviewEnvelope(item Item, envelope WalterReviewEnvelope, now time.Time) error {
	if envelope.SchemaVersion != 1 || envelope.IssuedAt.IsZero() {
		return errors.New("invalid Walter review envelope header")
	}
	if envelope.ItemID != item.State.ItemID ||
		envelope.WorkspaceID != item.State.WorkspaceID ||
		envelope.AttemptID != item.State.ActiveAttemptID ||
		envelope.ReviewedRevision != item.State.StateRevision ||
		envelope.ContractSHA256 != item.State.ContractSHA256 {
		return errors.New("Walter review envelope does not match the current ledger")
	}
	if envelope.SignerKeyID != item.Contract.WalterKeyID ||
		envelope.InstallationID != item.Contract.WalterInstallationID ||
		envelope.CustodyScope != reviewcustody.WalterReviewScope {
		return errors.New("Walter review envelope is outside the installation custody scope")
	}
	if envelope.Decision != WalterReviewApproved && envelope.Decision != WalterReviewRejected {
		return errors.New("invalid Walter review decision")
	}
	if err := validateID("nonce", envelope.Nonce); err != nil {
		return err
	}
	if envelope.IssuedAt.After(now.Add(5 * time.Minute)) {
		return errors.New("Walter review envelope is issued in the future")
	}
	publicKey, err := decodeWalterPublicKey(item.Contract.WalterPublicKey)
	if err != nil {
		return err
	}
	signature, err := base64.RawStdEncoding.DecodeString(envelope.Signature)
	if err != nil || len(signature) != ed25519.SignatureSize {
		return errors.New("invalid Walter review signature")
	}
	payload, err := WalterReviewSigningPayload(envelope)
	if err != nil {
		return err
	}
	if !ed25519.Verify(publicKey, payload, signature) {
		return errors.New("Walter review signature verification failed")
	}
	return nil
}

func validateWalterReviewReceipt(receipt WalterReviewReceipt) error {
	if receipt.SchemaVersion != 1 || receipt.ObservedAt.IsZero() ||
		receipt.ReviewedRevision < 1 ||
		receipt.RecordedRevision != receipt.ReviewedRevision+1 {
		return errors.New("invalid Walter review receipt header")
	}
	for kind, id := range map[string]string{
		"workspace": receipt.WorkspaceID,
		"item":      receipt.ItemID,
		"attempt":   receipt.AttemptID,
		"review":    receipt.ReviewID,
	} {
		if err := validateID(kind, id); err != nil {
			return err
		}
	}
	if !safeCustodyIdentity(receipt.SignerKeyID) || !safeCustodyIdentity(receipt.InstallationID) ||
		receipt.CustodyScope != reviewcustody.WalterReviewScope {
		return errors.New("invalid Walter review custody identity")
	}
	if receipt.Decision != WalterReviewApproved && receipt.Decision != WalterReviewRejected {
		return errors.New("invalid Walter review receipt decision")
	}
	for label, digest := range map[string]string{
		"contract": receipt.ContractSHA256,
		"signer":   receipt.SignerSHA256,
		"envelope": receipt.EnvelopeSHA256,
	} {
		if len(digest) != 64 {
			return fmt.Errorf("invalid Walter review %s digest", label)
		}
	}
	return nil
}

func decodeWalterPublicKey(encoded string) (ed25519.PublicKey, error) {
	key, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil || len(key) != ed25519.PublicKeySize {
		return nil, errors.New("invalid Walter Ed25519 public key")
	}
	return ed25519.PublicKey(key), nil
}

func validateWalterContractSettings(required bool, encodedPublicKey, keyID, installationID string) error {
	if !required {
		if strings.TrimSpace(encodedPublicKey) != "" || strings.TrimSpace(keyID) != "" || strings.TrimSpace(installationID) != "" {
			return errors.New("Walter public key requires the review gate")
		}
		return nil
	}
	if !safeCustodyIdentity(keyID) || !safeCustodyIdentity(installationID) {
		return errors.New("Walter review custody identity is required")
	}
	_, err := decodeWalterPublicKey(encodedPublicKey)
	return err
}

func safeCustodyIdentity(value string) bool {
	if value == "" || len(value) > 96 {
		return false
	}
	for _, character := range value {
		if (character < 'a' || character > 'z') && (character < 'A' || character > 'Z') &&
			(character < '0' || character > '9') && character != '_' && character != '-' {
			return false
		}
	}
	return true
}

func (store Store) walterReviews(workspaceID, itemID string) ([]WalterReviewReceipt, error) {
	root := filepath.Join(store.itemRoot(workspaceID, itemID), "revisions")
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(left, right int) bool { return entries[left].Name() < entries[right].Name() })
	result := make([]WalterReviewReceipt, 0)
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		var revision Revision
		if err := readStrictJSON(filepath.Join(root, entry.Name()), &revision); err != nil {
			return nil, err
		}
		if err := validateRevision(revision); err != nil ||
			entry.Name() != revisionName(revision.State.StateRevision) {
			return nil, errors.New("Walter review belongs to an invalid revision")
		}
		if revision.WalterReview != nil {
			result = append(result, *revision.WalterReview)
		}
	}
	return result, nil
}

func (store Store) currentWalterApproval(workspaceID, itemID string, state State) error {
	reviews, err := store.walterReviews(workspaceID, itemID)
	if err != nil {
		return err
	}
	if len(reviews) == 0 {
		return ErrWalterReviewUnsatisfied
	}
	review := reviews[len(reviews)-1]
	if review.Decision != WalterReviewApproved ||
		review.RecordedRevision != state.StateRevision ||
		review.ContractSHA256 != state.ContractSHA256 ||
		review.AttemptID != state.ActiveAttemptID {
		return ErrWalterReviewUnsatisfied
	}
	return nil
}
