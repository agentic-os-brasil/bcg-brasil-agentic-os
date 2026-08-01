package agentdispatch

import (
	"testing"
	"time"
)

func materialWalterDispatch(t *testing.T) (*Pilot, Dispatch) {
	t.Helper()
	pilot := newTestPilot(t, "claude")
	dispatch, source, err := pilot.Delegate(Intent{WorkspaceID: "alpha", Objective: "Prepare a consequential output.", ReviewTrigger: ReviewConsequentialTradeoff, TTL: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	producer := newTestExecutor(t, "claude", "workspace-agent-alpha", "workspace-alpha-cap", time.Now())
	body := ReturnBody{Summary: "The output is ready for Walter."}
	envelope, err := producer.SealReturn(dispatch, body)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pilot.Return(envelope, body); err != nil {
		t.Fatal(err)
	}
	reviewDispatch, _, err := pilot.RequireWalterReview(source.DelegationID, WalterReviewRequest{
		Trigger: ReviewConsequentialTradeoff, ReviewObjective: "Check the consequential trade-off.",
		Audience: "sponsor", Recommendation: "Keep the defensible option.", DefinitionOfDone: "The trade-off is explicit.",
		Chain: directCaseReviewChain(source), TTL: time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	return pilot, reviewDispatch
}

func TestWalterAdapterFailsClosedWhenRuntimeCustodyIsUnavailable(t *testing.T) {
	pilot, reviewDispatch := materialWalterDispatch(t)
	receipt, err := HandleWalterReview(pilot, reviewDispatch, WalterReviewBody{Verdict: WalterApproved}, nil)
	if err == nil || receipt.State != StateUnavailable || receipt.FailureCode != "review_custody_unavailable" {
		t.Fatalf("unavailable Walter custody = %#v err=%v", receipt, err)
	}
}
