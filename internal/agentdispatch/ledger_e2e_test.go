package agentdispatch

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/execution"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/reviewcustody"
)

func TestCaseDirectWalterApprovedCompletesExecutionLedger(t *testing.T) {
	workspaceID := "0123456789abcdef0123456789abcdef"
	store := execution.Store{Root: t.TempDir()}
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	created, err := store.Create(execution.CreateInput{
		WorkspaceID: workspaceID, Objective: "Complete one reviewed artifact.",
		InitialNextStep: "Produce the artifact.",
		Criteria:        []execution.Criterion{{ID: "artifact", Type: execution.CriterionArtifactSnapshot, TargetRef: "bcgos://workspace/result.txt"}},
		AllowedRefs:     []string{"bcgos://workspace/result.txt"}, RequireWalterReview: true,
		WalterPublicKey: base64.RawURLEncoding.EncodeToString(publicKey),
		WalterKeyID:     "walter-review-key", WalterInstallationID: "install-alpha",
	})
	if err != nil {
		t.Fatal(err)
	}
	workspaceRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspaceRoot, "result.txt"), []byte("reviewed artifact\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	started, err := store.Start(workspaceID, created.State.ItemID, created.State.StateRevision)
	if err != nil {
		t.Fatal(err)
	}
	evidenced, err := store.CollectEvidence(workspaceID, created.State.ItemID, execution.EvidenceInput{
		WorkspaceRoot: workspaceRoot, ExpectedRevision: started.State.StateRevision,
		AttemptID: started.State.ActiveAttemptID, CriterionID: "artifact",
	})
	if err != nil {
		t.Fatal(err)
	}

	pilot := newTestPilot(t, "claude")
	dispatch, source, err := pilot.Delegate(Intent{
		WorkspaceID: "alpha", Objective: "Prepare the reviewed recommendation.",
		ReviewTrigger: ReviewMaterialRecommendation, TTL: time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	producer := newTestExecutor(t, "claude", "workspace-agent-alpha", "workspace-alpha-cap", time.Now())
	returnBody := ReturnBody{Summary: "The direct Case output is ready for review."}
	producerEnvelope, err := producer.SealReturn(dispatch, returnBody)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pilot.Return(producerEnvelope, returnBody); err != nil {
		t.Fatal(err)
	}
	reviewDispatch, _, err := pilot.RequireWalterReview(source.DelegationID, WalterReviewRequest{
		Trigger: ReviewMaterialRecommendation, ReviewObjective: "Improve the recommendation where a load-bearing gap exists.",
		Audience: "sponsor", Recommendation: "Preserve the central thesis.",
		DefinitionOfDone:     "The sponsor can act on the recommendation.",
		ExecutionWorkspaceID: workspaceID, ExecutionItemID: created.State.ItemID,
		Chain: directCaseReviewChain(source), TTL: time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	signer, err := reviewcustody.NewEd25519Signer(privateKey, "walter-review-key", "install-alpha")
	if err != nil {
		t.Fatal(err)
	}
	custody, err := reviewcustody.NewProvider(func(scope string) (reviewcustody.Signer, error) {
		if scope != reviewcustody.WalterReviewScope {
			return nil, reviewcustody.ErrUnavailable
		}
		return signer, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	wrongPublicKey, wrongPrivateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	wrongSigner, err := reviewcustody.NewEd25519Signer(wrongPrivateKey, "walter-review-key", "install-alpha")
	if err != nil {
		t.Fatal(err)
	}
	if string(wrongPublicKey) == string(publicKey) {
		t.Fatal("test signer unexpectedly reused the contract key")
	}
	wrongCustody, err := reviewcustody.NewProvider(func(string) (reviewcustody.Signer, error) { return wrongSigner, nil })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pilot.RecordApprovedWalterReview(store, wrongCustody, source.DelegationID, WalterReviewBody{Verdict: WalterApproved}, LedgerApprovalInput{
		WorkspaceID: workspaceID, ItemID: created.State.ItemID,
		ExpectedRevision: evidenced.Item.State.StateRevision, AttemptID: evidenced.Item.State.ActiveAttemptID,
	}); err == nil {
		t.Fatal("forged Walter custody signer was accepted")
	}
	if _, err := pilot.RecordApprovedWalterReview(store, custody, source.DelegationID, WalterReviewBody{Verdict: WalterApproved}, LedgerApprovalInput{
		WorkspaceID: workspaceID, ItemID: "different-item",
		ExpectedRevision: evidenced.Item.State.StateRevision, AttemptID: evidenced.Item.State.ActiveAttemptID,
	}); err == nil {
		t.Fatal("cross-item Walter approval was accepted")
	}
	if _, err := HandleWalterReview(pilot, reviewDispatch, WalterReviewBody{Verdict: WalterApproved}, custody); err != nil {
		t.Fatal(err)
	}
	reviewed, err := pilot.RecordApprovedWalterReview(store, custody, source.DelegationID, WalterReviewBody{Verdict: WalterApproved}, LedgerApprovalInput{
		WorkspaceID: workspaceID, ItemID: created.State.ItemID,
		ExpectedRevision: evidenced.Item.State.StateRevision, AttemptID: evidenced.Item.State.ActiveAttemptID,
	})
	if err != nil {
		t.Fatal(err)
	}
	completed, err := store.Complete(workspaceID, created.State.ItemID, execution.CompletionInput{
		WorkspaceRoot: workspaceRoot, ExpectedRevision: reviewed.Item.State.StateRevision,
		AttemptID: reviewed.Item.State.ActiveAttemptID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if completed.State.State != execution.StateCompleted {
		t.Fatalf("execution state = %s", completed.State.State)
	}
}
