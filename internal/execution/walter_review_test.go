package execution

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

func walterReviewStore(t *testing.T) Store {
	t.Helper()
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	counts := make(map[string]int)
	return Store{
		Root: t.TempDir(),
		Now:  func() time.Time { return now },
		NewID: func(kind string) (string, error) {
			counts[kind]++
			return kind + "-" + string(rune('a'+counts[kind]-1)), nil
		},
	}
}

func walterKeypair(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	seed := bytes.Repeat([]byte{0x29}, ed25519.SeedSize)
	privateKey := ed25519.NewKeyFromSeed(seed)
	return privateKey.Public().(ed25519.PublicKey), privateKey
}

func walterExecutionInput(publicKey ed25519.PublicKey) CreateInput {
	return CreateInput{
		WorkspaceID:          testWorkspaceID,
		Objective:            "Complete one durable Maestro goal.",
		InitialNextStep:      "Produce the governed artifact.",
		Criteria:             []Criterion{{ID: "artifact", Type: CriterionArtifactSnapshot, TargetRef: "bcgos://workspace/result.txt"}},
		AllowedRefs:          []string{"bcgos://workspace/result.txt"},
		RequireWalterReview:  true,
		WalterPublicKey:      base64.RawURLEncoding.EncodeToString(publicKey),
		WalterKeyID:          "walter-review-key",
		WalterInstallationID: "install-alpha",
	}
}

func signedWalterEnvelope(t *testing.T, item Item, decision WalterReviewDecision, privateKey ed25519.PrivateKey) WalterReviewEnvelope {
	t.Helper()
	envelope := WalterReviewEnvelope{
		SchemaVersion:    1,
		ItemID:           item.State.ItemID,
		WorkspaceID:      item.State.WorkspaceID,
		AttemptID:        item.State.ActiveAttemptID,
		ReviewedRevision: item.State.StateRevision,
		ContractSHA256:   item.State.ContractSHA256,
		SignerKeyID:      item.Contract.WalterKeyID,
		InstallationID:   item.Contract.WalterInstallationID,
		CustodyScope:     "maestro/walter-review",
		Decision:         decision,
		Nonce:            "review-nonce",
		IssuedAt:         time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC),
	}
	payload, err := WalterReviewSigningPayload(envelope)
	if err != nil {
		t.Fatal(err)
	}
	envelope.Signature = base64.RawStdEncoding.EncodeToString(ed25519.Sign(privateKey, payload))
	return envelope
}

func prepareWalterExecution(t *testing.T, store Store, publicKey ed25519.PublicKey) (string, Item) {
	t.Helper()
	workspaceRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspaceRoot, "result.txt"), []byte("governed result\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	created, err := store.Create(walterExecutionInput(publicKey))
	if err != nil {
		t.Fatal(err)
	}
	started, err := store.Start(testWorkspaceID, created.State.ItemID, created.State.StateRevision)
	if err != nil {
		t.Fatal(err)
	}
	evidenced, err := store.CollectEvidence(testWorkspaceID, created.State.ItemID, EvidenceInput{
		WorkspaceRoot: workspaceRoot, ExpectedRevision: started.State.StateRevision,
		AttemptID: started.State.ActiveAttemptID, CriterionID: "artifact",
	})
	if err != nil {
		t.Fatal(err)
	}
	return workspaceRoot, evidenced.Item
}

func TestWalterReviewAuthenticatesExactLedgerRevisionAndCompletes(t *testing.T) {
	store := walterReviewStore(t)
	publicKey, privateKey := walterKeypair(t)
	workspaceRoot, evidenced := prepareWalterExecution(t, store, publicKey)
	envelope := signedWalterEnvelope(t, evidenced, WalterReviewApproved, privateKey)

	reviewed, err := store.RecordWalterReview(testWorkspaceID, evidenced.State.ItemID, WalterReviewInput{
		ExpectedRevision: evidenced.State.StateRevision,
		AttemptID:        evidenced.State.ActiveAttemptID,
		Envelope:         envelope,
	})
	if err != nil {
		t.Fatal(err)
	}
	if reviewed.Receipt.ReviewedRevision != evidenced.State.StateRevision ||
		reviewed.Receipt.RecordedRevision != reviewed.Item.State.StateRevision {
		t.Fatalf("reviewed item = %#v", reviewed)
	}
	completed, err := store.Complete(testWorkspaceID, evidenced.State.ItemID, CompletionInput{
		WorkspaceRoot: workspaceRoot, ExpectedRevision: reviewed.Item.State.StateRevision,
		AttemptID: reviewed.Item.State.ActiveAttemptID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if completed.State.State != StateCompleted {
		t.Fatalf("completed state = %s", completed.State.State)
	}
	exported, err := store.Export(testWorkspaceID, evidenced.State.ItemID)
	if err != nil {
		t.Fatal(err)
	}
	if len(exported.WalterReviews) != 1 || exported.WalterReviews[0].Decision != WalterReviewApproved {
		t.Fatalf("Walter reviews = %#v", exported.WalterReviews)
	}
	inspected, err := store.Inspect(testWorkspaceID, evidenced.State.ItemID)
	if err != nil {
		t.Fatal(err)
	}
	inspectedBody, err := json.Marshal(inspected)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(inspectedBody, []byte(`"walter_review":`)) ||
		bytes.Contains(inspectedBody, []byte(reviewed.Receipt.EnvelopeSHA256)) {
		t.Fatalf("inspect exposed explicit-export review data: %s", inspectedBody)
	}
}

func TestWalterReviewRejectsForgeryAndRejectedDecisionCannotComplete(t *testing.T) {
	store := walterReviewStore(t)
	publicKey, privateKey := walterKeypair(t)
	workspaceRoot, evidenced := prepareWalterExecution(t, store, publicKey)

	forged := signedWalterEnvelope(t, evidenced, WalterReviewApproved, privateKey)
	forged.Signature = base64.RawStdEncoding.EncodeToString(bytes.Repeat([]byte{0x42}, ed25519.SignatureSize))
	if _, err := store.RecordWalterReview(testWorkspaceID, evidenced.State.ItemID, WalterReviewInput{
		ExpectedRevision: evidenced.State.StateRevision,
		AttemptID:        evidenced.State.ActiveAttemptID, Envelope: forged,
	}); err == nil || !strings.Contains(err.Error(), "signature verification failed") {
		t.Fatalf("forged signature error = %v", err)
	}

	rejected := signedWalterEnvelope(t, evidenced, WalterReviewRejected, privateKey)
	reviewed, err := store.RecordWalterReview(testWorkspaceID, evidenced.State.ItemID, WalterReviewInput{
		ExpectedRevision: evidenced.State.StateRevision,
		AttemptID:        evidenced.State.ActiveAttemptID, Envelope: rejected,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Complete(testWorkspaceID, evidenced.State.ItemID, CompletionInput{
		WorkspaceRoot: workspaceRoot, ExpectedRevision: reviewed.Item.State.StateRevision,
		AttemptID: reviewed.Item.State.ActiveAttemptID,
	}); !errors.Is(err, ErrWalterReviewUnsatisfied) {
		t.Fatalf("completion after rejection error = %v", err)
	}
}

func TestWalterApprovalIsInvalidatedByAnyLaterMutation(t *testing.T) {
	store := walterReviewStore(t)
	publicKey, privateKey := walterKeypair(t)
	workspaceRoot, evidenced := prepareWalterExecution(t, store, publicKey)
	reviewed, err := store.RecordWalterReview(testWorkspaceID, evidenced.State.ItemID, WalterReviewInput{
		ExpectedRevision: evidenced.State.StateRevision,
		AttemptID:        evidenced.State.ActiveAttemptID,
		Envelope:         signedWalterEnvelope(t, evidenced, WalterReviewApproved, privateKey),
	})
	if err != nil {
		t.Fatal(err)
	}
	checkpointed, err := store.Checkpoint(testWorkspaceID, evidenced.State.ItemID, CheckpointInput{
		ExpectedRevision: reviewed.Item.State.StateRevision,
		AttemptID:        reviewed.Item.State.ActiveAttemptID,
		Summary:          "Changed after approval.", NextStep: "Request a fresh review.",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Complete(testWorkspaceID, evidenced.State.ItemID, CompletionInput{
		WorkspaceRoot: workspaceRoot, ExpectedRevision: checkpointed.State.StateRevision,
		AttemptID: checkpointed.State.ActiveAttemptID,
	}); !errors.Is(err, ErrWalterReviewUnsatisfied) {
		t.Fatalf("completion after later mutation error = %v", err)
	}
}

func TestWalterReviewRecoversFromProjectionCrash(t *testing.T) {
	store := walterReviewStore(t)
	publicKey, privateKey := walterKeypair(t)
	_, evidenced := prepareWalterExecution(t, store, publicKey)
	triggered := false
	store.FaultPoint = func(point string) error {
		if point == "after_revision_commit" && !triggered {
			triggered = true
			return errors.New("injected projection crash")
		}
		return nil
	}
	if _, err := store.RecordWalterReview(testWorkspaceID, evidenced.State.ItemID, WalterReviewInput{
		ExpectedRevision: evidenced.State.StateRevision,
		AttemptID:        evidenced.State.ActiveAttemptID,
		Envelope:         signedWalterEnvelope(t, evidenced, WalterReviewApproved, privateKey),
	}); err == nil {
		t.Fatal("projection crash was not injected")
	}
	recovered, err := (Store{Root: store.Root}).Inspect(testWorkspaceID, evidenced.State.ItemID)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.State.StateRevision != evidenced.State.StateRevision+1 {
		t.Fatalf("recovered item = %#v", recovered)
	}
	exported, err := (Store{Root: store.Root}).Export(testWorkspaceID, evidenced.State.ItemID)
	if err != nil {
		t.Fatal(err)
	}
	if len(exported.WalterReviews) != 1 ||
		exported.WalterReviews[0].Decision != WalterReviewApproved {
		t.Fatalf("recovered reviews = %#v", exported.WalterReviews)
	}
}

func TestWalterContractRequiresOneValidBoundPublicKey(t *testing.T) {
	input := testCreateInput()
	input.RequireWalterReview = true
	input.WalterKeyID = "walter-review-key"
	input.WalterInstallationID = "install-alpha"
	if _, err := walterReviewStore(t).Create(input); err == nil {
		t.Fatal("review-gated contract without a public key was accepted")
	}
	input.RequireWalterReview = false
	input.WalterPublicKey = base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{0x11}, ed25519.PublicKeySize))
	if _, err := walterReviewStore(t).Create(input); err == nil {
		t.Fatal("unused Walter public key was accepted")
	}
}

func TestExecutionSchemaClosesWalterReviewContracts(t *testing.T) {
	path := filepath.Join("..", "..", "schemas", "execution-state.schema.json")
	schemaBody, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var document any
	if err := json.Unmarshal(schemaBody, &document); err != nil {
		t.Fatal(err)
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource(filepath.Base(path), document); err != nil {
		t.Fatal(err)
	}
	schema, err := compiler.Compile(filepath.Base(path))
	if err != nil {
		t.Fatal(err)
	}
	publicKey, _ := walterKeypair(t)
	store := walterReviewStore(t)
	created, err := store.Create(walterExecutionInput(publicKey))
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(created.Contract)
	if err != nil {
		t.Fatal(err)
	}
	var contract map[string]any
	if err := json.Unmarshal(body, &contract); err != nil {
		t.Fatal(err)
	}
	if err := schema.Validate(contract); err != nil {
		t.Fatalf("valid review-gated contract rejected by schema: %v", err)
	}
	contract["professional_payload"] = "must not enter the contract"
	if err := schema.Validate(contract); err == nil {
		t.Fatal("execution schema accepted an arbitrary contract field")
	}
}
