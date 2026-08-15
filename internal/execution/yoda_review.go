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

type YodaReviewDecision string

const (
	YodaReviewApproved YodaReviewDecision = "approved"
	YodaReviewRejected YodaReviewDecision = "rejected"
)

var ErrYodaReviewUnsatisfied = errors.New("authenticated Yoda review is not satisfied")

// YodaReviewEnvelope is signed outside the execution core. It binds one
// decision to the exact item, contract, attempt and pre-review ledger revision.
// The signature never enters transition history.
type YodaReviewEnvelope struct {
	SchemaVersion    int                `json:"schema_version"`
	ItemID           string             `json:"item_id"`
	WorkspaceID      string             `json:"workspace_id"`
	AttemptID        string             `json:"attempt_id"`
	ReviewedRevision int                `json:"reviewed_revision"`
	ContractSHA256   string             `json:"contract_sha256"`
	SignerKeyID      string             `json:"signer_key_id"`
	InstallationID   string             `json:"installation_id"`
	CustodyScope     string             `json:"custody_scope"`
	Decision         YodaReviewDecision `json:"decision"`
	Nonce            string             `json:"nonce"`
	IssuedAt         time.Time          `json:"issued_at"`
	Signature        string             `json:"signature"`
}

type yodaReviewSigningPayload struct {
	SchemaVersion    int                `json:"schema_version"`
	ItemID           string             `json:"item_id"`
	WorkspaceID      string             `json:"workspace_id"`
	AttemptID        string             `json:"attempt_id"`
	ReviewedRevision int                `json:"reviewed_revision"`
	ContractSHA256   string             `json:"contract_sha256"`
	SignerKeyID      string             `json:"signer_key_id"`
	InstallationID   string             `json:"installation_id"`
	CustodyScope     string             `json:"custody_scope"`
	Decision         YodaReviewDecision `json:"decision"`
	Nonce            string             `json:"nonce"`
	IssuedAt         time.Time          `json:"issued_at"`
}

// YodaReviewSigningPayload returns the canonical bytes signed by a Yoda
// adapter. The core verifies those bytes against the public key frozen in the
// immutable execution contract.
func YodaReviewSigningPayload(envelope YodaReviewEnvelope) ([]byte, error) {
	return json.Marshal(yodaReviewSigningPayload{
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

// YodaReviewReceipt is metadata-only. It records what ledger revision was
// reviewed and which immutable key authenticated the decision, not rationale.
type YodaReviewReceipt struct {
	SchemaVersion    int                `json:"schema_version"`
	ReviewID         string             `json:"review_id"`
	ItemID           string             `json:"item_id"`
	WorkspaceID      string             `json:"workspace_id"`
	AttemptID        string             `json:"attempt_id"`
	ReviewedRevision int                `json:"reviewed_revision"`
	RecordedRevision int                `json:"recorded_revision"`
	ContractSHA256   string             `json:"contract_sha256"`
	SignerKeyID      string             `json:"signer_key_id"`
	InstallationID   string             `json:"installation_id"`
	CustodyScope     string             `json:"custody_scope"`
	Decision         YodaReviewDecision `json:"decision"`
	SignerSHA256     string             `json:"signer_sha256"`
	EnvelopeSHA256   string             `json:"envelope_sha256"`
	ObservedAt       time.Time          `json:"observed_at"`
}

type YodaReviewInput struct {
	ExpectedRevision int
	AttemptID        string
	Envelope         YodaReviewEnvelope
}

type YodaReviewResult struct {
	Item    Item
	Receipt YodaReviewReceipt
}

func (store Store) RecordYodaReview(workspaceID, itemID string, input YodaReviewInput) (YodaReviewResult, error) {
	if err := store.validateItemInput(workspaceID, itemID); err != nil {
		return YodaReviewResult{}, err
	}
	if input.ExpectedRevision < 1 {
		return YodaReviewResult{}, errors.New("Yoda review expected revision must be positive")
	}
	if err := validateID("attempt", input.AttemptID); err != nil {
		return YodaReviewResult{}, err
	}
	unlock, err := store.lock(workspaceID, itemID)
	if err != nil {
		return YodaReviewResult{}, err
	}
	defer unlock()

	item, err := store.inspectUnlocked(workspaceID, itemID)
	if err != nil {
		return YodaReviewResult{}, err
	}
	if item.State.StateRevision != input.ExpectedRevision {
		return YodaReviewResult{}, ErrRevisionConflict
	}
	if item.State.State != StateRunning || item.Attempt == nil ||
		item.Attempt.State != AttemptActive ||
		item.State.ActiveAttemptID != input.AttemptID ||
		item.Attempt.AttemptID != input.AttemptID {
		return YodaReviewResult{}, ErrAttemptConflict
	}
	if !item.Contract.RequireYodaReview {
		return YodaReviewResult{}, errors.New("execution contract does not require Yoda review")
	}
	if err := verifyYodaReviewEnvelope(item, input.Envelope, store.now()); err != nil {
		return YodaReviewResult{}, err
	}

	envelopeBody, err := json.Marshal(input.Envelope)
	if err != nil {
		return YodaReviewResult{}, err
	}
	envelopeDigest := sha256.Sum256(envelopeBody)
	envelopeSHA256 := hex.EncodeToString(envelopeDigest[:])
	reviews, err := store.yodaReviews(workspaceID, itemID)
	if err != nil {
		return YodaReviewResult{}, err
	}
	for _, review := range reviews {
		if review.EnvelopeSHA256 == envelopeSHA256 {
			return YodaReviewResult{}, errors.New("Yoda review envelope was already recorded")
		}
	}

	reviewID, err := store.newID("review")
	if err != nil {
		return YodaReviewResult{}, err
	}
	if err := validateID("review", reviewID); err != nil {
		return YodaReviewResult{}, err
	}
	publicKey, err := decodeYodaPublicKey(item.Contract.YodaPublicKey)
	if err != nil {
		return YodaReviewResult{}, err
	}
	keyDigest := sha256.Sum256(publicKey)
	now := store.now()
	state := item.State
	state.StateRevision++
	state.UpdatedAt = now
	receipt := YodaReviewReceipt{
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
		Checkpoint: item.Checkpoint, YodaReview: &receipt, Transition: transition,
	}
	if err := validateRevision(revision); err != nil {
		return YodaReviewResult{}, err
	}
	if err := store.commitRevision(workspaceID, itemID, revision); err != nil {
		return YodaReviewResult{}, err
	}
	return YodaReviewResult{
		Item: Item{
			Contract: item.Contract, State: state, Attempt: item.Attempt,
			Checkpoint: item.Checkpoint,
		},
		Receipt: receipt,
	}, nil
}

func verifyYodaReviewEnvelope(item Item, envelope YodaReviewEnvelope, now time.Time) error {
	if envelope.SchemaVersion != 1 || envelope.IssuedAt.IsZero() {
		return errors.New("invalid Yoda review envelope header")
	}
	if envelope.ItemID != item.State.ItemID ||
		envelope.WorkspaceID != item.State.WorkspaceID ||
		envelope.AttemptID != item.State.ActiveAttemptID ||
		envelope.ReviewedRevision != item.State.StateRevision ||
		envelope.ContractSHA256 != item.State.ContractSHA256 {
		return errors.New("Yoda review envelope does not match the current ledger")
	}
	if envelope.SignerKeyID != item.Contract.YodaKeyID ||
		envelope.InstallationID != item.Contract.YodaInstallationID ||
		envelope.CustodyScope != reviewcustody.YodaReviewScope {
		return errors.New("Yoda review envelope is outside the installation custody scope")
	}
	if envelope.Decision != YodaReviewApproved && envelope.Decision != YodaReviewRejected {
		return errors.New("invalid Yoda review decision")
	}
	if err := validateID("nonce", envelope.Nonce); err != nil {
		return err
	}
	if envelope.IssuedAt.After(now.Add(5 * time.Minute)) {
		return errors.New("Yoda review envelope is issued in the future")
	}
	publicKey, err := decodeYodaPublicKey(item.Contract.YodaPublicKey)
	if err != nil {
		return err
	}
	signature, err := base64.RawStdEncoding.DecodeString(envelope.Signature)
	if err != nil || len(signature) != ed25519.SignatureSize {
		return errors.New("invalid Yoda review signature")
	}
	payload, err := YodaReviewSigningPayload(envelope)
	if err != nil {
		return err
	}
	if !ed25519.Verify(publicKey, payload, signature) {
		return errors.New("Yoda review signature verification failed")
	}
	return nil
}

func validateYodaReviewReceipt(receipt YodaReviewReceipt) error {
	if receipt.SchemaVersion != 1 || receipt.ObservedAt.IsZero() ||
		receipt.ReviewedRevision < 1 ||
		receipt.RecordedRevision != receipt.ReviewedRevision+1 {
		return errors.New("invalid Yoda review receipt header")
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
		receipt.CustodyScope != reviewcustody.YodaReviewScope {
		return errors.New("invalid Yoda review custody identity")
	}
	if receipt.Decision != YodaReviewApproved && receipt.Decision != YodaReviewRejected {
		return errors.New("invalid Yoda review receipt decision")
	}
	for label, digest := range map[string]string{
		"contract": receipt.ContractSHA256,
		"signer":   receipt.SignerSHA256,
		"envelope": receipt.EnvelopeSHA256,
	} {
		if len(digest) != 64 {
			return fmt.Errorf("invalid Yoda review %s digest", label)
		}
	}
	return nil
}

func decodeYodaPublicKey(encoded string) (ed25519.PublicKey, error) {
	key, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil || len(key) != ed25519.PublicKeySize {
		return nil, errors.New("invalid Yoda Ed25519 public key")
	}
	return ed25519.PublicKey(key), nil
}

func validateYodaContractSettings(required bool, encodedPublicKey, keyID, installationID string) error {
	if !required {
		if strings.TrimSpace(encodedPublicKey) != "" || strings.TrimSpace(keyID) != "" || strings.TrimSpace(installationID) != "" {
			return errors.New("Yoda public key requires the review gate")
		}
		return nil
	}
	if !safeCustodyIdentity(keyID) || !safeCustodyIdentity(installationID) {
		return errors.New("Yoda review custody identity is required")
	}
	_, err := decodeYodaPublicKey(encodedPublicKey)
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

func (store Store) yodaReviews(workspaceID, itemID string) ([]YodaReviewReceipt, error) {
	root := filepath.Join(store.itemRoot(workspaceID, itemID), "revisions")
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(left, right int) bool { return entries[left].Name() < entries[right].Name() })
	result := make([]YodaReviewReceipt, 0)
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
			return nil, errors.New("Yoda review belongs to an invalid revision")
		}
		if revision.YodaReview != nil {
			result = append(result, *revision.YodaReview)
		}
	}
	return result, nil
}

func (store Store) currentYodaApproval(workspaceID, itemID string, state State) error {
	reviews, err := store.yodaReviews(workspaceID, itemID)
	if err != nil {
		return err
	}
	if len(reviews) == 0 {
		return ErrYodaReviewUnsatisfied
	}
	review := reviews[len(reviews)-1]
	if review.Decision != YodaReviewApproved ||
		review.RecordedRevision != state.StateRevision ||
		review.ContractSHA256 != state.ContractSHA256 ||
		review.AttemptID != state.ActiveAttemptID {
		return ErrYodaReviewUnsatisfied
	}
	return nil
}
