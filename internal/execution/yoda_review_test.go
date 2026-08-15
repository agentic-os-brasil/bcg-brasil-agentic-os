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

func yodaReviewStore(t *testing.T) Store {
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

func yodaKeypair(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	seed := bytes.Repeat([]byte{0x29}, ed25519.SeedSize)
	privateKey := ed25519.NewKeyFromSeed(seed)
	return privateKey.Public().(ed25519.PublicKey), privateKey
}

func yodaExecutionInput(publicKey ed25519.PublicKey) CreateInput {
	return CreateInput{
		WorkspaceID:          testWorkspaceID,
		Objective:            "Complete one durable Maestro goal.",
		InitialNextStep:      "Produce the governed artifact.",
		Criteria:             []Criterion{{ID: "artifact", Type: CriterionArtifactSnapshot, TargetRef: "bcgos://workspace/result.txt"}},
		AllowedRefs:          []string{"bcgos://workspace/result.txt"},
		RequireYodaReview:  true,
		YodaPublicKey:      base64.RawURLEncoding.EncodeToString(publicKey),
		YodaKeyID:          "yoda-review-key",
		YodaInstallationID: "install-alpha",
	}
}

func signedYodaEnvelope(t *testing.T, item Item, decision YodaReviewDecision, privateKey ed25519.PrivateKey) YodaReviewEnvelope {
	t.Helper()
	envelope := YodaReviewEnvelope{
		SchemaVersion:    1,
		ItemID:           item.State.ItemID,
		WorkspaceID:      item.State.WorkspaceID,
		AttemptID:        item.State.ActiveAttemptID,
		ReviewedRevision: item.State.StateRevision,
		ContractSHA256:   item.State.ContractSHA256,
		SignerKeyID:      item.Contract.YodaKeyID,
		InstallationID:   item.Contract.YodaInstallationID,
		CustodyScope:     "maestro/yoda-review",
		Decision:         decision,
		Nonce:            "review-nonce",
		IssuedAt:         time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC),
	}
	payload, err := YodaReviewSigningPayload(envelope)
	if err != nil {
		t.Fatal(err)
	}
	envelope.Signature = base64.RawStdEncoding.EncodeToString(ed25519.Sign(privateKey, payload))
	return envelope
}

func prepareYodaExecution(t *testing.T, store Store, publicKey ed25519.PublicKey) (string, Item) {
	t.Helper()
	workspaceRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspaceRoot, "result.txt"), []byte("governed result\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	created, err := store.Create(yodaExecutionInput(publicKey))
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

func TestYodaReviewAuthenticatesExactLedgerRevisionAndCompletes(t *testing.T) {
	store := yodaReviewStore(t)
	publicKey, privateKey := yodaKeypair(t)
	workspaceRoot, evidenced := prepareYodaExecution(t, store, publicKey)
	envelope := signedYodaEnvelope(t, evidenced, YodaReviewApproved, privateKey)

	reviewed, err := store.RecordYodaReview(testWorkspaceID, evidenced.State.ItemID, YodaReviewInput{
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
	if len(exported.YodaReviews) != 1 || exported.YodaReviews[0].Decision != YodaReviewApproved {
		t.Fatalf("Yoda reviews = %#v", exported.YodaReviews)
	}
	inspected, err := store.Inspect(testWorkspaceID, evidenced.State.ItemID)
	if err != nil {
		t.Fatal(err)
	}
	inspectedBody, err := json.Marshal(inspected)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(inspectedBody, []byte(`"yoda_review":`)) ||
		bytes.Contains(inspectedBody, []byte(reviewed.Receipt.EnvelopeSHA256)) {
		t.Fatalf("inspect exposed explicit-export review data: %s", inspectedBody)
	}
}

func TestYodaReviewRejectsForgeryAndRejectedDecisionCannotComplete(t *testing.T) {
	store := yodaReviewStore(t)
	publicKey, privateKey := yodaKeypair(t)
	workspaceRoot, evidenced := prepareYodaExecution(t, store, publicKey)

	forged := signedYodaEnvelope(t, evidenced, YodaReviewApproved, privateKey)
	forged.Signature = base64.RawStdEncoding.EncodeToString(bytes.Repeat([]byte{0x42}, ed25519.SignatureSize))
	if _, err := store.RecordYodaReview(testWorkspaceID, evidenced.State.ItemID, YodaReviewInput{
		ExpectedRevision: evidenced.State.StateRevision,
		AttemptID:        evidenced.State.ActiveAttemptID, Envelope: forged,
	}); err == nil || !strings.Contains(err.Error(), "signature verification failed") {
		t.Fatalf("forged signature error = %v", err)
	}

	rejected := signedYodaEnvelope(t, evidenced, YodaReviewRejected, privateKey)
	reviewed, err := store.RecordYodaReview(testWorkspaceID, evidenced.State.ItemID, YodaReviewInput{
		ExpectedRevision: evidenced.State.StateRevision,
		AttemptID:        evidenced.State.ActiveAttemptID, Envelope: rejected,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Complete(testWorkspaceID, evidenced.State.ItemID, CompletionInput{
		WorkspaceRoot: workspaceRoot, ExpectedRevision: reviewed.Item.State.StateRevision,
		AttemptID: reviewed.Item.State.ActiveAttemptID,
	}); !errors.Is(err, ErrYodaReviewUnsatisfied) {
		t.Fatalf("completion after rejection error = %v", err)
	}
}

func TestYodaApprovalIsInvalidatedByAnyLaterMutation(t *testing.T) {
	store := yodaReviewStore(t)
	publicKey, privateKey := yodaKeypair(t)
	workspaceRoot, evidenced := prepareYodaExecution(t, store, publicKey)
	reviewed, err := store.RecordYodaReview(testWorkspaceID, evidenced.State.ItemID, YodaReviewInput{
		ExpectedRevision: evidenced.State.StateRevision,
		AttemptID:        evidenced.State.ActiveAttemptID,
		Envelope:         signedYodaEnvelope(t, evidenced, YodaReviewApproved, privateKey),
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
	}); !errors.Is(err, ErrYodaReviewUnsatisfied) {
		t.Fatalf("completion after later mutation error = %v", err)
	}
}

func TestYodaReviewRecoversFromProjectionCrash(t *testing.T) {
	store := yodaReviewStore(t)
	publicKey, privateKey := yodaKeypair(t)
	_, evidenced := prepareYodaExecution(t, store, publicKey)
	triggered := false
	store.FaultPoint = func(point string) error {
		if point == "after_revision_commit" && !triggered {
			triggered = true
			return errors.New("injected projection crash")
		}
		return nil
	}
	if _, err := store.RecordYodaReview(testWorkspaceID, evidenced.State.ItemID, YodaReviewInput{
		ExpectedRevision: evidenced.State.StateRevision,
		AttemptID:        evidenced.State.ActiveAttemptID,
		Envelope:         signedYodaEnvelope(t, evidenced, YodaReviewApproved, privateKey),
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
	if len(exported.YodaReviews) != 1 ||
		exported.YodaReviews[0].Decision != YodaReviewApproved {
		t.Fatalf("recovered reviews = %#v", exported.YodaReviews)
	}
}

func TestYodaContractRequiresOneValidBoundPublicKey(t *testing.T) {
	input := testCreateInput()
	input.RequireYodaReview = true
	input.YodaKeyID = "yoda-review-key"
	input.YodaInstallationID = "install-alpha"
	if _, err := yodaReviewStore(t).Create(input); err == nil {
		t.Fatal("review-gated contract without a public key was accepted")
	}
	input.RequireYodaReview = false
	input.YodaPublicKey = base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{0x11}, ed25519.PublicKeySize))
	if _, err := yodaReviewStore(t).Create(input); err == nil {
		t.Fatal("unused Yoda public key was accepted")
	}
}

func TestExecutionSchemaClosesYodaReviewContracts(t *testing.T) {
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
	publicKey, _ := yodaKeypair(t)
	store := yodaReviewStore(t)
	created, err := store.Create(yodaExecutionInput(publicKey))
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
