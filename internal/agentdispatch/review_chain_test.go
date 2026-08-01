package agentdispatch

import (
	"strings"
	"testing"
	"time"
)

func accountAssistedReviewChain(source Receipt) ReviewChain {
	return ReviewChain{
		Mode: ReviewChainAccountCaseAccount, AccountConsultationRequired: true, PostAccountValidationRequired: true,
		AccountConsultationReasonCode:   "client_strategic_lens_required",
		AccountConsultationEvidenceRefs: []string{"bcgos://workspace/alpha/dossier/client-lens.json"},
		Steps: []ReviewChainStep{
			{Sequence: 1, AgentID: "client-account-alpha", Role: "client_account_agent", PacketID: strings.Repeat("c", 64), PacketSHA256: strings.Repeat("d", 64), IssuerAgentID: "maestro"},
			{Sequence: 2, AgentID: source.TargetAgentID, Role: "case_agent", PacketID: source.DelegationID, PacketSHA256: source.PacketSHA256, IssuerAgentID: "maestro"},
			{Sequence: 3, AgentID: "client-account-alpha", Role: "client_account_agent", PacketID: strings.Repeat("e", 64), PacketSHA256: strings.Repeat("f", 64), IssuerAgentID: "maestro"},
		},
		ValidatedPacketID: source.DelegationID, ValidatedPacketSHA256: source.PacketSHA256,
	}
}

func TestWalterReviewChainAcceptsOnlyTheTwoMaestroMediatedPaths(t *testing.T) {
	pilot := newTestPilot(t, "claude")
	_, source, err := pilot.Delegate(Intent{WorkspaceID: "alpha", Objective: "Prepare one material result.", ReviewTrigger: ReviewMaterialRecommendation, TTL: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	for name, chain := range map[string]ReviewChain{
		"account-assisted": accountAssistedReviewChain(source),
		"direct-case":      directCaseReviewChain(source),
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateReviewChainForSource(chain, WorkPacket{
				PacketID: source.DelegationID, TargetAgentID: source.TargetAgentID,
				ScopeKind: "workspace", ScopeID: "alpha", Signature: "test",
			}, "workspace_agent"); err == nil {
				// The synthetic packet above has no signed-body digest matching the
				// receipt. The structural path is checked separately below.
				t.Fatal("unsealed synthetic packet unexpectedly passed source binding")
			}
			if err := validateReviewChain(chain); err != nil {
				t.Fatalf("valid %s chain rejected: %v", name, err)
			}
		})
	}
}

func TestAccountConsultationPathRequiresFramingAndPostValidation(t *testing.T) {
	pilot := newTestPilot(t, "claude")
	dispatch, source, err := pilot.Delegate(Intent{WorkspaceID: "alpha", Objective: "Prepare a client-sensitive recommendation.", ReviewTrigger: ReviewMaterialRecommendation, TTL: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	producer := newTestExecutor(t, "claude", "workspace-agent-alpha", "workspace-alpha-cap", time.Now())
	body := ReturnBody{Summary: "The recommendation is ready for account validation."}
	envelope, err := producer.SealReturn(dispatch, body)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pilot.Return(envelope, body); err != nil {
		t.Fatal(err)
	}
	reviewDispatch, _, err := pilot.RequireWalterReview(source.DelegationID, WalterReviewRequest{
		Trigger: ReviewMaterialRecommendation, ReviewObjective: "Review the account-validated recommendation.",
		Audience: "sponsor", Recommendation: "Preserve the client-relevant thesis.",
		DefinitionOfDone: "The sponsor can decide with the account lens applied.",
		Chain:            accountAssistedReviewChain(source), TTL: time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reviewDispatch.Packet.Review.Chain.AccountConsultationRequired ||
		!reviewDispatch.Packet.Review.Chain.PostAccountValidationRequired ||
		reviewDispatch.Packet.Review.Chain.ValidatedPacketID != source.DelegationID {
		t.Fatalf("Account consultation chain was not preserved: %#v", reviewDispatch.Packet.Review.Chain)
	}
}

func TestWalterReviewChainRejectsMismatchedAccountAndNestedCapability(t *testing.T) {
	chain := directCaseReviewChain(Receipt{DelegationID: strings.Repeat("a", 64), TargetAgentID: "case-alpha", PacketSHA256: strings.Repeat("b", 64)})
	chain.AccountConsultationRequired = true
	if err := validateReviewChain(chain); err == nil {
		t.Fatal("direct Case chain with pre-account framing did not fail closed")
	}
	chain = directCaseReviewChain(Receipt{DelegationID: strings.Repeat("a", 64), TargetAgentID: "case-alpha", PacketSHA256: strings.Repeat("b", 64)})
	chain.Steps[0].ParentPacketID = strings.Repeat("c", 64)
	if err := validateReviewChain(chain); err == nil {
		t.Fatal("nested Walter chain did not fail closed")
	}
	chain = directCaseReviewChain(Receipt{DelegationID: strings.Repeat("a", 64), TargetAgentID: "case-alpha", PacketSHA256: strings.Repeat("b", 64)})
	chain.Steps[0].Role = "capability_specialist"
	if err := validateReviewChain(chain); err == nil {
		t.Fatal("Capability Specialist Walter chain did not fail closed")
	}
}

func TestWalterSkipIsExplicitAndMaterialityEscalationInvalidatesIt(t *testing.T) {
	pilot := newTestPilot(t, "claude")
	dispatch, source, err := pilot.Delegate(Intent{
		WorkspaceID: "alpha", Objective: "Read one mechanical source.", TTL: time.Hour,
		WalterSkip: &WalterSkipDecision{ReasonCode: "mechanical_nonmaterial", EvidenceRefs: []string{"bcgos://workspace/alpha/dossier/brief.json"}, LowMateriality: true, ResolvedBy: "maestro"},
	})
	if err != nil || source.Review == nil || source.Review.State != ReviewSkipped {
		t.Fatalf("explicit skip was not recorded: %#v err=%v", source, err)
	}
	producer := newTestExecutor(t, "claude", "workspace-agent-alpha", "workspace-alpha-cap", time.Now())
	result := ReturnBody{Summary: "Mechanical finding."}
	envelope, err := producer.SealReturn(dispatch, result)
	if err != nil {
		t.Fatal(err)
	}
	completed, err := pilot.Return(envelope, result)
	if err != nil || completed.State != StateCompleted || completed.Review == nil || completed.Review.State != ReviewSkipped {
		t.Fatalf("audited skip did not complete: %#v err=%v", completed, err)
	}
	if _, err := pilot.EscalateMateriality(source.DelegationID, ReviewMaterialRecommendation); err == nil {
		t.Fatal("completed Walter skip was mutated after return")
	}

	pilot = newTestPilot(t, "claude")
	dispatch, source, err = pilot.Delegate(Intent{
		WorkspaceID: "alpha", Objective: "Read one source whose risk may change.", TTL: time.Hour,
		WalterSkip: &WalterSkipDecision{ReasonCode: "low_materiality", EvidenceRefs: []string{"bcgos://workspace/alpha/dossier/brief.json"}, LowMateriality: true, ResolvedBy: "maestro"},
	})
	if err != nil {
		t.Fatal(err)
	}
	dispatch, err = pilot.EscalateMateriality(source.DelegationID, ReviewConsequentialTradeoff)
	if err != nil {
		t.Fatal(err)
	}
	producer = newTestExecutor(t, "claude", "workspace-agent-alpha", "workspace-alpha-cap", time.Now())
	envelope, err = producer.SealReturn(dispatch, ReturnBody{Summary: "Trade-off discovered."})
	if err != nil {
		t.Fatal(err)
	}
	pending, err := pilot.Return(envelope, ReturnBody{Summary: "Trade-off discovered."})
	if err != nil || pending.State != StatePendingReview {
		t.Fatalf("materiality escalation did not require Walter: %#v err=%v", pending, err)
	}
}

func TestWalterReviewBudgetExhaustionStopsInfiniteRefinement(t *testing.T) {
	pilot := newTestPilot(t, "claude")
	dispatch, source, err := pilot.Delegate(Intent{WorkspaceID: "alpha", Objective: "Prepare a material recommendation.", ReviewTrigger: ReviewMaterialRecommendation, TTL: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	producer := newTestExecutor(t, "claude", "workspace-agent-alpha", "workspace-alpha-cap", time.Now())
	result := ReturnBody{Summary: "Recommendation."}
	envelope, err := producer.SealReturn(dispatch, result)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pilot.Return(envelope, result); err != nil {
		t.Fatal(err)
	}
	request := WalterReviewRequest{Trigger: ReviewMaterialRecommendation, ReviewObjective: "Pressure-test.", Audience: "sponsor", Recommendation: "Use it.", DefinitionOfDone: "Sponsor can decide.", Chain: directCaseReviewChain(source), TTL: time.Hour}
	for round := 1; round <= maxWalterReviewRounds; round++ {
		reviewDispatch, _, err := pilot.RequireWalterReview(source.DelegationID, request)
		if err != nil {
			t.Fatalf("round %d opening review: %v", round, err)
		}
		walter := newTestExecutor(t, "claude", "walter", "walter-cap", time.Now())
		body := WalterReviewBody{Verdict: WalterRefineAndReturn, Objections: []WalterObjection{{Code: "fix", Fix: "Apply the correction.", ExitCondition: "The correction is evidenced.", Blocking: true}}}
		reviewEnvelope, err := walter.SealWalterReview(reviewDispatch, body)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pilot.ReturnWalterReview(reviewEnvelope, body); err != nil {
			t.Fatal(err)
		}
		if round < maxWalterReviewRounds {
			dispatch, source, err = pilot.Rework(source.DelegationID, Intent{WorkspaceID: "alpha", Objective: "Revised recommendation.", ReviewTrigger: ReviewMaterialRecommendation, TTL: time.Hour})
			if err != nil {
				t.Fatal(err)
			}
			producer = newTestExecutor(t, "claude", "workspace-agent-alpha", "workspace-alpha-cap", time.Now())
			envelope, err = producer.SealReturn(dispatch, ReturnBody{Summary: "Revised recommendation."})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := pilot.Return(envelope, ReturnBody{Summary: "Revised recommendation."}); err != nil {
				t.Fatal(err)
			}
			request.Chain = directCaseReviewChain(source)
		}
	}
	_, receipt, err := pilot.RequireWalterReview(source.DelegationID, request)
	if err == nil || receipt.FailureCode != "review_budget_exhausted" {
		t.Fatalf("review budget was not exhausted: receipt=%#v err=%v", receipt, err)
	}
}
