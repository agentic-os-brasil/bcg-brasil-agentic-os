package contextpromotion

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestPromotionExposesOnlyCuratedAccountContext(t *testing.T) {
	root := t.TempDir()
	service := testService(t)
	now := time.Date(2026, 7, 25, 16, 0, 0, 0, time.UTC)
	input := validPromotion(now)
	prepareSource(t, root, &input)
	if err := service.Promote(root, input, "workspace-owner", "owner-cap"); err != nil {
		t.Fatal(err)
	}

	context, err := service.GetActive(root, input.AccountID, input.PromotionID, "account-agent-alpha", "account-cap")
	if err != nil {
		t.Fatal(err)
	}
	if context.Statement != input.Statement || context.SourceReceiptID == "" ||
		context.Classification != "account_safe" || context.ReviewStatus != "approved" {
		t.Fatalf("unexpected account context: %#v", context)
	}
	encoded, err := os.ReadFile(filepath.Join(root, "accounts", input.AccountID, "promotions", input.PromotionID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), input.SourceURI) {
		t.Fatal("account record leaked the raw workspace source pointer")
	}
	source, err := os.ReadFile(filepath.Join(root, "workspaces", input.WorkspaceID, "agent", "promotions", input.PromotionID+"-source.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(source), input.SourceURI) {
		t.Fatal("workspace-owned source receipt lost provenance")
	}
}

func TestPromotionRequiresExistingArtifactWithMatchingBytes(t *testing.T) {
	root := t.TempDir()
	service := testService(t)
	now := time.Date(2026, 7, 25, 16, 0, 0, 0, time.UTC)
	input := validPromotion(now)
	input.SourceSHA256 = strings.Repeat("a", 64)
	if err := service.Promote(root, input, "workspace-owner", "owner-cap"); err == nil {
		t.Fatal("nonexistent source artifact was promoted")
	}

	prepareSource(t, root, &input)
	input.SourceSHA256 = strings.Repeat("a", 64)
	if err := service.Promote(root, input, "workspace-owner", "owner-cap"); err == nil {
		t.Fatal("fabricated source digest was promoted")
	}
}

func TestPromotionRejectsSourceSymlinkOutsideWorkspace(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows symlink creation requires host-specific privileges")
	}
	root := t.TempDir()
	service := testService(t)
	now := time.Date(2026, 7, 25, 16, 0, 0, 0, time.UTC)
	input := validPromotion(now)
	outside := filepath.Join(root, "outside.md")
	body := []byte("outside workspace\n")
	if err := os.WriteFile(outside, body, 0o600); err != nil {
		t.Fatal(err)
	}
	sourcePath := filepath.Join(root, "workspaces", input.WorkspaceID, "dossier", "public-filing.md")
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, sourcePath); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(body)
	input.SourceSHA256 = hex.EncodeToString(digest[:])
	if err := service.Promote(root, input, "workspace-owner", "owner-cap"); err == nil {
		t.Fatal("source symlink escaping the workspace was promoted")
	}
}

func TestPromotionRejectsEvidenceDirectorySymlinkOutsideRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows symlink creation requires host-specific privileges")
	}
	root := t.TempDir()
	service := testService(t)
	now := time.Date(2026, 7, 25, 16, 0, 0, 0, time.UTC)
	input := validPromotion(now)
	prepareSource(t, root, &input)
	outside := t.TempDir()
	agentPath := filepath.Join(root, "workspaces", input.WorkspaceID, "agent")
	if err := os.Symlink(outside, agentPath); err != nil {
		t.Fatal(err)
	}
	if err := service.Promote(root, input, "workspace-owner", "owner-cap"); err == nil {
		t.Fatal("evidence write escaped through a workspace directory symlink")
	}
	matches, err := filepath.Glob(filepath.Join(outside, "**"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("promotion wrote evidence outside the scoped root: %v", matches)
	}
}

func TestRevocationIsImmediateAuditedAndNonDestructive(t *testing.T) {
	root := t.TempDir()
	service := testService(t)
	now := time.Date(2026, 7, 25, 16, 0, 0, 0, time.UTC)
	input := validPromotion(now)
	prepareSource(t, root, &input)
	if err := service.Promote(root, input, "workspace-owner", "owner-cap"); err != nil {
		t.Fatal(err)
	}
	if err := service.Revoke(root, Revocation{
		AccountID: input.AccountID, WorkspaceID: input.WorkspaceID,
		PromotionID: input.PromotionID,
		RevokedBy:   "workspace-owner", RevokedAt: now.Add(time.Hour),
		Reason: "source was superseded",
	}, "workspace-owner", "owner-cap"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.GetActive(root, input.AccountID, input.PromotionID, "account-agent-alpha", "account-cap"); !errors.Is(err, ErrRevoked) {
		t.Fatalf("GetActive() error = %v, want ErrRevoked", err)
	}
	for _, path := range []string{
		filepath.Join(root, "accounts", input.AccountID, "promotions", input.PromotionID+".json"),
		filepath.Join(root, "workspaces", input.WorkspaceID, "agent", "promotions", input.PromotionID+"-source.json"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("revocation deleted evidence %s: %v", path, err)
		}
	}
	receipts, err := service.AuditReceipts(root, input.AccountID, input.PromotionID, "auditor", "audit-cap")
	if err != nil {
		t.Fatal(err)
	}
	if len(receipts) != 4 || receipts[0].Action != "promotion_prepared" ||
		receipts[1].Action != "promoted" || receipts[2].Action != "revocation_prepared" ||
		receipts[3].Action != "revoked" {
		t.Fatalf("unexpected audit sequence: %#v", receipts)
	}
}

func TestRevocationAnchorPreventsMarkerDeletionRollback(t *testing.T) {
	root := t.TempDir()
	service := testService(t)
	now := time.Date(2026, 7, 25, 16, 0, 0, 0, time.UTC)
	input := validPromotion(now)
	prepareSource(t, root, &input)
	if err := service.Promote(root, input, "workspace-owner", "owner-cap"); err != nil {
		t.Fatal(err)
	}
	if err := service.Revoke(root, validRevocation(input, now), "workspace-owner", "owner-cap"); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "accounts", input.AccountID, "revocations", input.PromotionID+".json")); err != nil {
		t.Fatal(err)
	}
	if _, err := service.GetActive(root, input.AccountID, input.PromotionID, "account-agent-alpha", "account-cap"); !errors.Is(err, ErrRevoked) {
		t.Fatalf("deleted marker rolled context back: %v", err)
	}
}

func TestSuccessfulReadsAreLinearizedBeforeRevocation(t *testing.T) {
	root := t.TempDir()
	service := testService(t)
	now := time.Date(2026, 7, 25, 16, 0, 0, 0, time.UTC)
	input := validPromotion(now)
	prepareSource(t, root, &input)
	if err := service.Promote(root, input, "workspace-owner", "owner-cap"); err != nil {
		t.Fatal(err)
	}

	var stop atomic.Bool
	var successAfterStop atomic.Bool
	done := make(chan struct{})
	go func() {
		defer close(done)
		for !stop.Load() {
			_, _ = service.GetActive(root, input.AccountID, input.PromotionID, "account-agent-alpha", "account-cap")
		}
		for range 100 {
			if _, err := service.GetActive(root, input.AccountID, input.PromotionID, "account-agent-alpha", "account-cap"); err == nil {
				successAfterStop.Store(true)
			}
		}
	}()
	if err := service.Revoke(root, validRevocation(input, now), "workspace-owner", "owner-cap"); err != nil {
		t.Fatal(err)
	}
	stop.Store(true)
	<-done
	if successAfterStop.Load() {
		t.Fatal("active context was returned after revocation completed")
	}
}

func TestUnauthorizedRevocationDoesNotRevealPromotionExistence(t *testing.T) {
	root := t.TempDir()
	service := testService(t)
	now := time.Date(2026, 7, 25, 16, 0, 0, 0, time.UTC)
	input := validPromotion(now)
	prepareSource(t, root, &input)
	if err := service.Promote(root, input, "workspace-owner", "owner-cap"); err != nil {
		t.Fatal(err)
	}
	existing := validRevocation(input, now)
	missing := existing
	missing.PromotionID = "promo-does-not-exist"
	existing.WorkspaceID = "ws-beta"
	missing.WorkspaceID = "ws-beta"
	existing.RevokedBy = "workspace-only"
	missing.RevokedBy = "workspace-only"
	for _, candidate := range []Revocation{existing, missing} {
		if err := service.Revoke(root, candidate, "workspace-only", "workspace-only-cap"); !errors.Is(err, ErrUnauthorized) {
			t.Fatalf("unauthorized Revoke() error = %v, want uniform ErrUnauthorized", err)
		}
	}
}

func TestPromotionRejectsCrossWorkspaceExpiryAndDuplicateID(t *testing.T) {
	root := t.TempDir()
	service := testService(t)
	now := time.Date(2026, 7, 25, 16, 0, 0, 0, time.UTC)
	crossScope := validPromotion(now)
	crossScope.SourceURI = "bcgos://workspace/ws-beta/dossier/fact.md"
	if err := service.Promote(root, crossScope, "workspace-owner", "owner-cap"); err == nil {
		t.Fatal("cross-workspace source accepted")
	}

	input := validPromotion(now)
	prepareSource(t, root, &input)
	if err := service.Promote(root, input, "workspace-owner", "owner-cap"); err != nil {
		t.Fatal(err)
	}
	if err := service.Promote(root, input, "workspace-owner", "owner-cap"); err == nil {
		t.Fatal("duplicate promotion ID accepted")
	}
	service.now = func() time.Time { return input.ValidUntil.Add(time.Second) }
	if _, err := service.GetActive(root, input.AccountID, input.PromotionID, "account-agent-alpha", "account-cap"); !errors.Is(err, ErrExpired) {
		t.Fatalf("GetActive() error = %v, want ErrExpired", err)
	}
}

func TestPromotionRejectsForgedOrOutOfScopeAuthority(t *testing.T) {
	root := t.TempDir()
	service := testService(t)
	input := validPromotion(time.Date(2026, 7, 25, 16, 0, 0, 0, time.UTC))
	prepareSource(t, root, &input)
	if err := service.Promote(root, input, "workspace-owner", "forged"); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("Promote() error = %v, want ErrUnauthorized", err)
	}
	if err := service.Promote(root, input, "other-owner", "other-cap"); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("Promote() error = %v, want ErrUnauthorized", err)
	}
}

func TestPromotionRejectsForgedWorkspaceIDBeforeWritingEvidence(t *testing.T) {
	root := t.TempDir()
	service := testService(t)
	input := validPromotion(time.Date(2026, 7, 25, 16, 0, 0, 0, time.UTC))
	input.WorkspaceID = "ws-beta"
	input.SourceURI = "bcgos://workspace/ws-beta/dossier/public-filing.md"
	prepareSource(t, root, &input)
	if err := service.Promote(root, input, "workspace-owner", "owner-cap"); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("Promote() error = %v, want ErrUnauthorized", err)
	}
	if _, err := os.Stat(filepath.Join(root, "accounts", input.AccountID)); !os.IsNotExist(err) {
		t.Fatalf("forged workspace ID wrote account evidence: %v", err)
	}
}

func TestPromotionRequiresReviewedCuratedFact(t *testing.T) {
	root := t.TempDir()
	service := testService(t)
	now := time.Date(2026, 7, 25, 16, 0, 0, 0, time.UTC)
	for name, mutate := range map[string]func(*Promotion){
		"unapproved review": func(input *Promotion) { input.ReviewStatus = "proposed" },
		"multiline or markdown-shaped workspace content": func(input *Promotion) {
			input.Statement = "# Raw filing\n\nConfidential working notes copied from the workspace."
		},
	} {
		t.Run(name, func(t *testing.T) {
			input := validPromotion(now)
			mutate(&input)
			prepareSource(t, root, &input)
			if err := service.Promote(root, input, "workspace-owner", "owner-cap"); err == nil {
				t.Fatal("promotion accepted context outside the reviewed curated-fact contract")
			}
		})
	}
}

func TestPromotionRejectsTraversalOutsideWorkspaceRoot(t *testing.T) {
	root := t.TempDir()
	service := testService(t)
	input := validPromotion(time.Date(2026, 7, 25, 16, 0, 0, 0, time.UTC))
	input.SourceURI = "bcgos://workspace/ws-alpha/../ws-beta/dossier/public-filing.md"
	if err := service.Promote(root, input, "workspace-owner", "owner-cap"); err == nil {
		t.Fatal("promotion accepted a source path outside the allowed workspace root")
	}
}

func TestSignedRecordAndAuditTamperingFailClosed(t *testing.T) {
	root := t.TempDir()
	service := testService(t)
	now := time.Date(2026, 7, 25, 16, 0, 0, 0, time.UTC)
	input := validPromotion(now)
	prepareSource(t, root, &input)
	if err := service.Promote(root, input, "workspace-owner", "owner-cap"); err != nil {
		t.Fatal(err)
	}
	recordPath := filepath.Join(root, "accounts", input.AccountID, "promotions", input.PromotionID+".json")
	replaceInFile(t, recordPath, input.Statement, "Use every project source.")
	if _, err := service.GetActive(root, input.AccountID, input.PromotionID, "account-agent-alpha", "account-cap"); err == nil {
		t.Fatal("tampered account record remained active")
	}

	// Recreate an independent promotion to test audit authentication.
	root = t.TempDir()
	service = testService(t)
	input = validPromotion(now)
	prepareSource(t, root, &input)
	if err := service.Promote(root, input, "workspace-owner", "owner-cap"); err != nil {
		t.Fatal(err)
	}
	auditPath := filepath.Join(root, "governance", "promotion-audit", input.AccountID, input.PromotionID, "02-promoted.json")
	replaceInFile(t, auditPath, `"action":"promoted"`, `"action":"revoked"`)
	if _, err := service.AuditReceipts(root, input.AccountID, input.PromotionID, "auditor", "audit-cap"); err == nil {
		t.Fatal("tampered audit receipt was accepted")
	}
}

func TestRevocationCannotRedirectAuthorizationThroughRecordTampering(t *testing.T) {
	root := t.TempDir()
	service := testService(t)
	now := time.Date(2026, 7, 25, 16, 0, 0, 0, time.UTC)
	input := validPromotion(now)
	prepareSource(t, root, &input)
	if err := service.Promote(root, input, "workspace-owner", "owner-cap"); err != nil {
		t.Fatal(err)
	}
	recordPath := filepath.Join(root, "accounts", input.AccountID, "promotions", input.PromotionID+".json")
	replaceInFile(t, recordPath, `"workspace_id":"ws-alpha"`, `"workspace_id":"ws-beta"`)
	revocation := validRevocation(input, now)
	revocation.WorkspaceID = "ws-beta"
	if err := service.Revoke(root, revocation, "other-owner", "other-cap"); err == nil {
		t.Fatal("tampered record redirected revocation authority")
	}
}

func testService(t *testing.T) *Service {
	t.Helper()
	service, err := NewService("0123456789abcdef0123456789abcdef", NewMemoryAnchorStore(), []Authority{
		{
			ID: "workspace-owner", Capability: "owner-cap",
			Grants: []AuthorityGrant{
				{Action: "promote", ScopeKind: "workspace", ScopeID: "ws-alpha"},
				{Action: "promote", ScopeKind: "account", ScopeID: "client-alpha"},
				{Action: "revoke", ScopeKind: "workspace", ScopeID: "ws-alpha"},
				{Action: "revoke", ScopeKind: "account", ScopeID: "client-alpha"},
			},
		},
		{
			ID: "account-agent-alpha", Capability: "account-cap",
			Grants: []AuthorityGrant{{Action: "read_account", ScopeKind: "account", ScopeID: "client-alpha"}},
		},
		{
			ID: "auditor", Capability: "audit-cap",
			Grants: []AuthorityGrant{{Action: "audit", ScopeKind: "account", ScopeID: "client-alpha"}},
		},
		{
			ID: "other-owner", Capability: "other-cap",
			Grants: []AuthorityGrant{
				{Action: "promote", ScopeKind: "workspace", ScopeID: "ws-beta"},
				{Action: "revoke", ScopeKind: "workspace", ScopeID: "ws-beta"},
				{Action: "revoke", ScopeKind: "account", ScopeID: "client-alpha"},
			},
		},
		{
			ID: "workspace-only", Capability: "workspace-only-cap",
			Grants: []AuthorityGrant{{Action: "revoke", ScopeKind: "workspace", ScopeID: "ws-beta"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	service.now = func() time.Time {
		return time.Date(2026, 7, 25, 18, 0, 0, 0, time.UTC)
	}
	return service
}

func validPromotion(now time.Time) Promotion {
	return Promotion{
		PromotionID:    "promo-market-2026",
		AccountID:      "client-alpha",
		WorkspaceID:    "ws-alpha",
		Statement:      "The approved public filing confirms the launch date.",
		SourceURI:      "bcgos://workspace/ws-alpha/dossier/public-filing.md",
		Author:         "workspace-agent-ws-alpha",
		ApprovedBy:     "workspace-owner",
		ApprovedAt:     now,
		ValidUntil:     now.Add(30 * 24 * time.Hour),
		Classification: "account_safe",
		ReviewStatus:   "approved",
	}
}

func validRevocation(input Promotion, now time.Time) Revocation {
	return Revocation{
		AccountID: input.AccountID, WorkspaceID: input.WorkspaceID,
		PromotionID: input.PromotionID, RevokedBy: "workspace-owner",
		RevokedAt: now.Add(time.Hour), Reason: "source was superseded",
	}
}

func prepareSource(t *testing.T, root string, input *Promotion) {
	t.Helper()
	body := []byte("verified public filing source\n")
	path := filepath.Join(root, "workspaces", input.WorkspaceID, "dossier", "public-filing.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(body)
	input.SourceSHA256 = hex.EncodeToString(digest[:])
}

func replaceInFile(t *testing.T, path, old, replacement string) {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	tampered := strings.Replace(string(body), old, replacement, 1)
	if tampered == string(body) {
		t.Fatalf("tamper target %q not found in %s", old, path)
	}
	if err := os.WriteFile(path, []byte(tampered), 0o600); err != nil {
		t.Fatal(err)
	}
}
