// Package contextpromotion implements the explicit, auditable handoff from a
// workspace-owned source to curated account context.
package contextpromotion

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/agentcatalog"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/agentorchestration"
)

var (
	ErrRevoked      = errors.New("promoted account context is revoked")
	ErrExpired      = errors.New("promoted account context is expired")
	ErrUnauthorized = errors.New("context promotion authority is invalid or out of scope")
)

const maxPromotionLifetime = 366 * 24 * time.Hour

type AuthorityGrant struct {
	Action    string
	ScopeKind string
	ScopeID   string
}

type Authority struct {
	ID         string
	Capability string
	Grants     []AuthorityGrant
}

type authority struct {
	capabilitySHA256 [sha256.Size]byte
	grants           map[string]bool
}

// AnchorState is the trusted, monotonic state for one promotion. Native
// adapters must keep it outside the account/workspace evidence tree in a
// durable store with atomic Create and Transition operations.
type AnchorState struct {
	Phase               string    `json:"phase"`
	AccountID           string    `json:"account_id"`
	WorkspaceID         string    `json:"workspace_id"`
	PromotionID         string    `json:"promotion_id"`
	SourceReceiptID     string    `json:"source_receipt_id"`
	AccountRecordSHA256 string    `json:"account_record_sha256"`
	RevokedBy           string    `json:"revoked_by,omitempty"`
	RevokedAt           time.Time `json:"revoked_at,omitempty"`
	RevocationReason    string    `json:"revocation_reason,omitempty"`
}

type AnchorStore interface {
	Create(key string, state AnchorState) error
	Get(key string) (AnchorState, bool, error)
	Transition(key, expectedPhase string, next AnchorState) error
}

// MemoryAnchorStore is a conformance/test implementation. Runtime activation
// requires a durable implementation provisioned by the native adapter.
type MemoryAnchorStore struct {
	mu     sync.Mutex
	values map[string]AnchorState
}

func NewMemoryAnchorStore() *MemoryAnchorStore {
	return &MemoryAnchorStore{values: make(map[string]AnchorState)}
}

func (store *MemoryAnchorStore) Create(key string, state AnchorState) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	if _, exists := store.values[key]; exists {
		return errors.New("promotion ID already exists")
	}
	store.values[key] = state
	return nil
}

func (store *MemoryAnchorStore) Get(key string) (AnchorState, bool, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	state, exists := store.values[key]
	return state, exists, nil
}

func (store *MemoryAnchorStore) Transition(key, expectedPhase string, next AnchorState) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	current, exists := store.values[key]
	if !exists || current.Phase != expectedPhase ||
		current.AccountID != next.AccountID ||
		current.WorkspaceID != next.WorkspaceID ||
		current.PromotionID != next.PromotionID ||
		current.SourceReceiptID != next.SourceReceiptID ||
		current.AccountRecordSHA256 != next.AccountRecordSHA256 {
		return errors.New("promotion anchor transition rejected")
	}
	store.values[key] = next
	return nil
}

type Service struct {
	mu           sync.RWMutex
	authorities  map[string]authority
	anchor       AnchorStore
	integrityKey []byte
	now          func() time.Time
}

func NewService(integrityKey string, anchor AnchorStore, authorities []Authority) (*Service, error) {
	if len([]byte(integrityKey)) < 32 || anchor == nil {
		return nil, errors.New("context promotion requires a private integrity key and trusted anchor store")
	}
	values := make(map[string]authority, len(authorities))
	validActions := map[string]bool{"promote": true, "revoke": true, "read_account": true, "audit": true}
	for _, input := range authorities {
		if !agentcatalog.ValidAgentID(input.ID) || input.Capability == "" || len(input.Grants) == 0 {
			return nil, errors.New("context promotion authority is incomplete")
		}
		if _, exists := values[input.ID]; exists {
			return nil, errors.New("context promotion authority IDs must be unique")
		}
		grants := make(map[string]bool, len(input.Grants))
		for _, grant := range input.Grants {
			if !validActions[grant.Action] || (grant.ScopeKind != "workspace" && grant.ScopeKind != "account") ||
				!agentcatalog.ValidAgentID(grant.ScopeID) {
				return nil, errors.New("context promotion authority grant is invalid")
			}
			grants[grant.Action+"\x00"+grant.ScopeKind+"\x00"+grant.ScopeID] = true
		}
		values[input.ID] = authority{
			capabilitySHA256: sha256.Sum256([]byte(input.Capability)),
			grants:           grants,
		}
	}
	return &Service{
		authorities: values, anchor: anchor,
		integrityKey: []byte(integrityKey), now: time.Now,
	}, nil
}

func (service *Service) authorize(actor, capability, action, scopeKind, scopeID string) bool {
	value, ok := service.authorities[actor]
	if !ok || capability == "" || !value.grants[action+"\x00"+scopeKind+"\x00"+scopeID] {
		return false
	}
	digest := sha256.Sum256([]byte(capability))
	return subtle.ConstantTimeCompare(value.capabilitySHA256[:], digest[:]) == 1
}

type Promotion struct {
	PromotionID    string    `json:"promotion_id"`
	AccountID      string    `json:"account_id"`
	WorkspaceID    string    `json:"workspace_id"`
	Statement      string    `json:"statement"`
	SourceURI      string    `json:"source_uri"`
	SourceSHA256   string    `json:"source_sha256"`
	Author         string    `json:"author"`
	ApprovedBy     string    `json:"approved_by"`
	ApprovedAt     time.Time `json:"approved_at"`
	ValidUntil     time.Time `json:"valid_until"`
	Classification string    `json:"classification"`
	ReviewStatus   string    `json:"review_status"`
}

type Revocation struct {
	AccountID   string    `json:"account_id"`
	WorkspaceID string    `json:"workspace_id"`
	PromotionID string    `json:"promotion_id"`
	RevokedBy   string    `json:"revoked_by"`
	RevokedAt   time.Time `json:"revoked_at"`
	Reason      string    `json:"reason"`
}

type AccountContext struct {
	PromotionID     string    `json:"promotion_id"`
	Statement       string    `json:"statement"`
	SourceReceiptID string    `json:"source_receipt_id"`
	SourceSHA256    string    `json:"source_sha256"`
	Author          string    `json:"author"`
	ApprovedBy      string    `json:"approved_by"`
	ApprovedAt      time.Time `json:"approved_at"`
	ValidUntil      time.Time `json:"valid_until"`
	Classification  string    `json:"classification"`
	ReviewStatus    string    `json:"review_status"`
}

type accountRecord struct {
	SchemaVersion   int       `json:"schema_version"`
	AccountID       string    `json:"account_id"`
	WorkspaceID     string    `json:"workspace_id"`
	PromotionID     string    `json:"promotion_id"`
	Statement       string    `json:"statement"`
	SourceReceiptID string    `json:"source_receipt_id"`
	SourceSHA256    string    `json:"source_sha256"`
	Author          string    `json:"author"`
	ApprovedBy      string    `json:"approved_by"`
	ApprovedAt      time.Time `json:"approved_at"`
	ValidUntil      time.Time `json:"valid_until"`
	Classification  string    `json:"classification"`
	ReviewStatus    string    `json:"review_status"`
}

type sourceReceipt struct {
	SchemaVersion int    `json:"schema_version"`
	PromotionID   string `json:"promotion_id"`
	WorkspaceID   string `json:"workspace_id"`
	SourceURI     string `json:"source_uri"`
	SourceSHA256  string `json:"source_sha256"`
}

type AuditReceipt struct {
	SchemaVersion       int       `json:"schema_version"`
	Sequence            int       `json:"sequence"`
	Action              string    `json:"action"`
	AccountID           string    `json:"account_id"`
	WorkspaceID         string    `json:"workspace_id"`
	PromotionID         string    `json:"promotion_id"`
	Actor               string    `json:"actor"`
	At                  time.Time `json:"at"`
	Reason              string    `json:"reason,omitempty"`
	SourceReceiptID     string    `json:"source_receipt_id,omitempty"`
	AccountRecordSHA256 string    `json:"account_record_sha256,omitempty"`
}

type signedEnvelope[T any] struct {
	Payload    T      `json:"payload"`
	HMACSHA256 string `json:"hmac_sha256"`
}

func (service *Service) Promote(root string, input Promotion, actor, capability string) error {
	service.mu.Lock()
	defer service.mu.Unlock()

	now := service.now().UTC()
	if err := input.validate(now); err != nil {
		return err
	}
	if actor != input.ApprovedBy ||
		!service.authorize(actor, capability, "promote", "workspace", input.WorkspaceID) ||
		!service.authorize(actor, capability, "promote", "account", input.AccountID) {
		return ErrUnauthorized
	}
	normalizedSource, actualSourceSHA256, err := verifySourceArtifact(root, input.WorkspaceID, input.SourceURI)
	if err != nil {
		return err
	}
	if !hmac.Equal([]byte(actualSourceSHA256), []byte(strings.ToLower(input.SourceSHA256))) {
		return errors.New("promotion source hash does not match the workspace artifact")
	}

	source := sourceReceipt{
		SchemaVersion: 1, PromotionID: input.PromotionID,
		WorkspaceID: input.WorkspaceID, SourceURI: normalizedSource,
		SourceSHA256: actualSourceSHA256,
	}
	sourceID, err := hashJSON(source)
	if err != nil {
		return err
	}
	record := accountRecord{
		SchemaVersion: 1, AccountID: input.AccountID, WorkspaceID: input.WorkspaceID,
		PromotionID: input.PromotionID, Statement: strings.TrimSpace(input.Statement),
		SourceReceiptID: sourceID, SourceSHA256: actualSourceSHA256,
		Author: input.Author, ApprovedBy: input.ApprovedBy,
		ApprovedAt: input.ApprovedAt.UTC(), ValidUntil: input.ValidUntil.UTC(),
		Classification: input.Classification, ReviewStatus: input.ReviewStatus,
	}
	recordSHA256, err := hashJSON(record)
	if err != nil {
		return err
	}
	state := AnchorState{
		Phase: "preparing", AccountID: input.AccountID, WorkspaceID: input.WorkspaceID,
		PromotionID: input.PromotionID, SourceReceiptID: sourceID,
		AccountRecordSHA256: recordSHA256,
	}
	if err := service.anchor.Create(anchorKey(input.AccountID, input.PromotionID), state); err != nil {
		return err
	}

	evidenceRoot, err := os.OpenRoot(root)
	if err != nil {
		return err
	}
	defer evidenceRoot.Close()
	paths := promotionPaths(input.AccountID, input.WorkspaceID, input.PromotionID)
	prepared := AuditReceipt{
		SchemaVersion: 1, Sequence: 1, Action: "promotion_prepared",
		AccountID: input.AccountID, WorkspaceID: input.WorkspaceID,
		PromotionID: input.PromotionID, Actor: input.ApprovedBy,
		At: input.ApprovedAt.UTC(), SourceReceiptID: sourceID,
		AccountRecordSHA256: recordSHA256,
	}
	promoted := prepared
	promoted.Sequence = 2
	promoted.Action = "promoted"
	for _, item := range []struct {
		path  string
		value any
	}{
		{paths.prepared, prepared},
		{paths.source, source},
		{paths.account, record},
		{paths.promoted, promoted},
	} {
		if err := service.writeSignedImmutableJSON(evidenceRoot, item.path, item.value); err != nil {
			return err
		}
	}
	state.Phase = "active"
	return service.anchor.Transition(anchorKey(input.AccountID, input.PromotionID), "preparing", state)
}

func (input Promotion) validate(now time.Time) error {
	if !agentcatalog.ValidAgentID(input.PromotionID) ||
		!agentcatalog.ValidAgentID(input.AccountID) ||
		!agentcatalog.ValidAgentID(input.WorkspaceID) ||
		strings.TrimSpace(input.Statement) == "" || len([]byte(strings.TrimSpace(input.Statement))) > 1000 ||
		strings.TrimSpace(input.Author) == "" || strings.TrimSpace(input.ApprovedBy) == "" ||
		input.Classification != "account_safe" || input.ReviewStatus != "approved" ||
		input.ApprovedAt.IsZero() || !input.ValidUntil.After(input.ApprovedAt) ||
		input.ValidUntil.Sub(input.ApprovedAt) > maxPromotionLifetime ||
		input.ApprovedAt.After(now.Add(5*time.Minute)) || !input.ValidUntil.After(now) ||
		!validSHA256(input.SourceSHA256) {
		return errors.New("promotion violates the curated account-context contract")
	}
	normalized, valid := agentorchestration.NormalizeResource(input.SourceURI)
	if !valid || !agentorchestration.ResourceWithinScope(normalized, "workspace", input.WorkspaceID) {
		return errors.New("promotion source is not a specific artifact in the source workspace")
	}
	return nil
}

func (service *Service) Revoke(root string, input Revocation, actor, capability string) error {
	service.mu.Lock()
	defer service.mu.Unlock()

	if !agentcatalog.ValidAgentID(input.AccountID) ||
		!agentcatalog.ValidAgentID(input.WorkspaceID) ||
		!agentcatalog.ValidAgentID(input.PromotionID) ||
		strings.TrimSpace(input.RevokedBy) == "" || input.RevokedAt.IsZero() ||
		strings.TrimSpace(input.Reason) == "" || len([]byte(input.Reason)) > 500 {
		return errors.New("revocation is invalid")
	}
	// Authorization happens before any anchor or filesystem lookup, preventing
	// promotion-existence disclosure to an untrusted caller.
	if actor != input.RevokedBy ||
		!service.authorize(actor, capability, "revoke", "workspace", input.WorkspaceID) ||
		!service.authorize(actor, capability, "revoke", "account", input.AccountID) {
		return ErrUnauthorized
	}
	state, exists, err := service.anchor.Get(anchorKey(input.AccountID, input.PromotionID))
	if err != nil {
		return err
	}
	if !exists {
		return os.ErrNotExist
	}
	if state.AccountID != input.AccountID || state.WorkspaceID != input.WorkspaceID ||
		state.PromotionID != input.PromotionID {
		return errors.New("revocation scope does not match the trusted promotion anchor")
	}
	if state.Phase == "revoked" {
		return ErrRevoked
	}
	if state.Phase != "active" {
		return errors.New("promotion is not active")
	}
	evidenceRoot, err := os.OpenRoot(root)
	if err != nil {
		return err
	}
	defer evidenceRoot.Close()
	record, err := service.verifiedActiveRecord(evidenceRoot, state)
	if err != nil {
		return err
	}
	if input.RevokedAt.Before(record.ApprovedAt) || input.RevokedAt.After(service.now().UTC().Add(5*time.Minute)) {
		return errors.New("revocation timestamp is outside the allowed window")
	}

	next := state
	next.Phase = "revoked"
	next.RevokedBy = input.RevokedBy
	next.RevokedAt = input.RevokedAt.UTC()
	next.RevocationReason = strings.TrimSpace(input.Reason)
	// The trusted monotonic transition is the revocation barrier. Evidence is
	// written only after reads are already blocked.
	if err := service.anchor.Transition(anchorKey(input.AccountID, input.PromotionID), "active", next); err != nil {
		return err
	}

	paths := promotionPaths(input.AccountID, input.WorkspaceID, input.PromotionID)
	prepared := AuditReceipt{
		SchemaVersion: 1, Sequence: 3, Action: "revocation_prepared",
		AccountID: input.AccountID, WorkspaceID: input.WorkspaceID,
		PromotionID: input.PromotionID, Actor: input.RevokedBy,
		At: input.RevokedAt.UTC(), Reason: strings.TrimSpace(input.Reason),
		SourceReceiptID:     state.SourceReceiptID,
		AccountRecordSHA256: state.AccountRecordSHA256,
	}
	revoked := prepared
	revoked.Sequence = 4
	revoked.Action = "revoked"
	for _, item := range []struct {
		path  string
		value any
	}{
		{paths.revocationPrepared, prepared},
		{paths.revocation, input},
		{paths.revoked, revoked},
	} {
		if err := service.writeSignedImmutableJSON(evidenceRoot, item.path, item.value); err != nil {
			return err
		}
	}
	return nil
}

func (service *Service) GetActive(root, accountID, promotionID, actor, capability string) (AccountContext, error) {
	service.mu.RLock()
	defer service.mu.RUnlock()

	if !service.authorize(actor, capability, "read_account", "account", accountID) {
		return AccountContext{}, ErrUnauthorized
	}
	state, exists, err := service.anchor.Get(anchorKey(accountID, promotionID))
	if err != nil {
		return AccountContext{}, err
	}
	if !exists {
		return AccountContext{}, os.ErrNotExist
	}
	if state.Phase == "revoked" {
		return AccountContext{}, ErrRevoked
	}
	if state.Phase != "active" || state.AccountID != accountID || state.PromotionID != promotionID {
		return AccountContext{}, errors.New("promotion is not active in the trusted anchor")
	}
	evidenceRoot, err := os.OpenRoot(root)
	if err != nil {
		return AccountContext{}, err
	}
	defer evidenceRoot.Close()
	record, err := service.verifiedActiveRecord(evidenceRoot, state)
	if err != nil {
		return AccountContext{}, err
	}
	if !record.ValidUntil.After(service.now().UTC()) {
		return AccountContext{}, ErrExpired
	}
	return AccountContext{
		PromotionID: record.PromotionID, Statement: record.Statement,
		SourceReceiptID: record.SourceReceiptID, SourceSHA256: record.SourceSHA256,
		Author: record.Author, ApprovedBy: record.ApprovedBy,
		ApprovedAt: record.ApprovedAt, ValidUntil: record.ValidUntil,
		Classification: record.Classification, ReviewStatus: record.ReviewStatus,
	}, nil
}

func (service *Service) verifiedActiveRecord(root *os.Root, state AnchorState) (accountRecord, error) {
	paths := promotionPaths(state.AccountID, state.WorkspaceID, state.PromotionID)
	var record accountRecord
	if err := service.readSignedJSON(root, paths.account, &record); err != nil {
		return accountRecord{}, err
	}
	if err := validateAccountRecord(record, state.AccountID, state.PromotionID); err != nil ||
		record.WorkspaceID != state.WorkspaceID ||
		record.SourceReceiptID != state.SourceReceiptID {
		return accountRecord{}, errors.New("account promotion record does not match its trusted anchor")
	}
	recordSHA256, err := hashJSON(record)
	if err != nil || recordSHA256 != state.AccountRecordSHA256 {
		return accountRecord{}, errors.New("account promotion record does not match its trusted anchor")
	}
	var promoted AuditReceipt
	if err := service.readSignedJSON(root, paths.promoted, &promoted); err != nil ||
		!validAuditReceipt(promoted, 2, "promoted", state) {
		return accountRecord{}, errors.New("promotion has no valid completion receipt")
	}
	var source sourceReceipt
	if err := service.readSignedJSON(root, paths.source, &source); err != nil {
		return accountRecord{}, errors.New("workspace source receipt is unavailable")
	}
	sourceSHA256, err := hashJSON(source)
	if err != nil || sourceSHA256 != state.SourceReceiptID ||
		source.PromotionID != state.PromotionID || source.WorkspaceID != state.WorkspaceID ||
		source.SourceSHA256 != record.SourceSHA256 {
		return accountRecord{}, errors.New("workspace source receipt does not match promoted provenance")
	}
	return record, nil
}

func (service *Service) AuditReceipts(root, accountID, promotionID, actor, capability string) ([]AuditReceipt, error) {
	service.mu.RLock()
	defer service.mu.RUnlock()

	if !agentcatalog.ValidAgentID(accountID) || !agentcatalog.ValidAgentID(promotionID) ||
		!service.authorize(actor, capability, "audit", "account", accountID) {
		return nil, ErrUnauthorized
	}
	state, exists, err := service.anchor.Get(anchorKey(accountID, promotionID))
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, os.ErrNotExist
	}
	evidenceRoot, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	defer evidenceRoot.Close()
	paths := promotionPaths(state.AccountID, state.WorkspaceID, state.PromotionID)
	expected := []struct {
		path     string
		sequence int
		action   string
	}{
		{paths.prepared, 1, "promotion_prepared"},
		{paths.promoted, 2, "promoted"},
	}
	if state.Phase == "revoked" {
		expected = append(expected,
			struct {
				path     string
				sequence int
				action   string
			}{paths.revocationPrepared, 3, "revocation_prepared"},
			struct {
				path     string
				sequence int
				action   string
			}{paths.revoked, 4, "revoked"},
		)
	} else if state.Phase != "active" {
		return nil, errors.New("promotion audit is incomplete")
	}
	receipts := make([]AuditReceipt, 0, len(expected))
	for _, item := range expected {
		var receipt AuditReceipt
		if err := service.readSignedJSON(evidenceRoot, item.path, &receipt); err != nil ||
			!validAuditReceipt(receipt, item.sequence, item.action, state) {
			return nil, errors.New("promotion audit receipt failed integrity or sequence validation")
		}
		if item.sequence >= 3 &&
			(receipt.Actor != state.RevokedBy || receipt.At != state.RevokedAt ||
				receipt.Reason != state.RevocationReason) {
			return nil, errors.New("revocation audit receipt does not match its trusted anchor")
		}
		receipts = append(receipts, receipt)
	}
	return receipts, nil
}

func validAuditReceipt(receipt AuditReceipt, sequence int, action string, state AnchorState) bool {
	return receipt.SchemaVersion == 1 &&
		receipt.Sequence == sequence && receipt.Action == action &&
		receipt.AccountID == state.AccountID &&
		receipt.WorkspaceID == state.WorkspaceID &&
		receipt.PromotionID == state.PromotionID &&
		strings.TrimSpace(receipt.Actor) != "" && !receipt.At.IsZero() &&
		receipt.SourceReceiptID == state.SourceReceiptID &&
		receipt.AccountRecordSHA256 == state.AccountRecordSHA256
}

func validateAccountRecord(record accountRecord, accountID, promotionID string) error {
	if record.SchemaVersion != 1 || record.AccountID != accountID ||
		record.PromotionID != promotionID || !agentcatalog.ValidAgentID(record.WorkspaceID) ||
		strings.TrimSpace(record.Statement) == "" || len([]byte(record.Statement)) > 1000 ||
		!validSHA256(record.SourceSHA256) || !validSHA256(record.SourceReceiptID) ||
		strings.TrimSpace(record.Author) == "" || strings.TrimSpace(record.ApprovedBy) == "" ||
		record.Classification != "account_safe" || record.ReviewStatus != "approved" ||
		record.ApprovedAt.IsZero() || !record.ValidUntil.After(record.ApprovedAt) ||
		record.ValidUntil.Sub(record.ApprovedAt) > maxPromotionLifetime {
		return errors.New("account promotion record is invalid")
	}
	return nil
}

type paths struct {
	account, source, prepared, promoted     string
	revocationPrepared, revocation, revoked string
}

func promotionPaths(accountID, workspaceID, promotionID string) paths {
	audit := filepath.Join("governance", "promotion-audit", accountID, promotionID)
	return paths{
		account:            filepath.Join("accounts", accountID, "promotions", promotionID+".json"),
		source:             filepath.Join("workspaces", workspaceID, "agent", "promotions", promotionID+"-source.json"),
		prepared:           filepath.Join(audit, "01-promotion-prepared.json"),
		promoted:           filepath.Join(audit, "02-promoted.json"),
		revocationPrepared: filepath.Join(audit, "03-revocation-prepared.json"),
		revocation:         filepath.Join("accounts", accountID, "revocations", promotionID+".json"),
		revoked:            filepath.Join(audit, "04-revoked.json"),
	}
}

func verifySourceArtifact(root, workspaceID, sourceURI string) (string, string, error) {
	normalized, valid := agentorchestration.NormalizeResource(sourceURI)
	if !valid || !agentorchestration.ResourceWithinScope(normalized, "workspace", workspaceID) {
		return "", "", errors.New("promotion source is not a specific artifact in the source workspace")
	}
	parsed, _ := url.Parse(normalized)
	prefix := "/" + workspaceID + "/"
	relative := strings.TrimPrefix(parsed.Path, prefix)
	if relative == parsed.Path || relative == "" {
		return "", "", errors.New("promotion source URI cannot be mapped to a workspace artifact")
	}
	workspaceRoot := filepath.Join(root, "workspaces", workspaceID)
	scopedRoot, err := os.OpenRoot(workspaceRoot)
	if err != nil {
		return "", "", err
	}
	defer scopedRoot.Close()
	file, err := scopedRoot.Open(filepath.FromSlash(relative))
	if err != nil {
		return "", "", err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return "", "", errors.New("promotion source must be a regular workspace artifact")
	}
	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		return "", "", err
	}
	return normalized, hex.EncodeToString(digest.Sum(nil)), nil
}

func (service *Service) writeSignedImmutableJSON(root *os.Root, path string, value any) error {
	body, err := json.Marshal(value)
	if err != nil {
		return err
	}
	mac := hmac.New(sha256.New, service.integrityKey)
	if _, err := mac.Write(body); err != nil {
		return err
	}
	envelope := signedEnvelope[json.RawMessage]{
		Payload: body, HMACSHA256: hex.EncodeToString(mac.Sum(nil)),
	}
	encoded, err := json.Marshal(envelope)
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')
	return writeImmutable(root, path, encoded)
}

func (service *Service) readSignedJSON(root *os.Root, path string, target any) error {
	var envelope signedEnvelope[json.RawMessage]
	if err := readStrictJSON(root, path, &envelope); err != nil {
		return err
	}
	if !validSHA256(envelope.HMACSHA256) {
		return errors.New("signed record has an invalid authenticator")
	}
	mac := hmac.New(sha256.New, service.integrityKey)
	if _, err := mac.Write(envelope.Payload); err != nil {
		return err
	}
	expected, _ := hex.DecodeString(envelope.HMACSHA256)
	if !hmac.Equal(mac.Sum(nil), expected) {
		return errors.New("signed record failed integrity validation")
	}
	decoder := json.NewDecoder(strings.NewReader(string(envelope.Payload)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("signed payload contains multiple JSON values")
	}
	return nil
}

func writeImmutable(root *os.Root, path string, body []byte) error {
	directory := filepath.Dir(path)
	if err := root.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		return err
	}
	tempPath := filepath.Join(directory, ".bcgos-promotion-"+hex.EncodeToString(random))
	file, err := root.OpenFile(tempPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	defer root.Remove(tempPath)
	if err := file.Chmod(0o600); err != nil {
		file.Close()
		return err
	}
	if _, err := file.Write(body); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := root.Link(tempPath, path); err != nil {
		return err
	}
	return syncDirectory(root, directory)
}

func syncDirectory(root *os.Root, directory string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	scopedDirectory, err := root.OpenRoot(directory)
	if err != nil {
		return err
	}
	defer scopedDirectory.Close()
	file, err := scopedDirectory.Open(".")
	if err != nil {
		return err
	}
	defer file.Close()
	return file.Sync()
}

func readStrictJSON(root *os.Root, path string, target any) error {
	file, err := root.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("JSON record contains multiple values")
		}
		return err
	}
	return nil
}

func validSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size
}

func hashJSON(value any) (string, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:]), nil
}

func anchorKey(accountID, promotionID string) string {
	return accountID + "\x00" + promotionID
}
