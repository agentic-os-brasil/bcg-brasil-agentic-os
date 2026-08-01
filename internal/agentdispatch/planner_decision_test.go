package agentdispatch

import (
	"strings"
	"testing"
	"time"
)

func TestAccountConsultationUsesClientLensSignalsNotTechnicalDepth(t *testing.T) {
	tests := []struct {
		name     string
		signals  AccountConsultationSignals
		required bool
		reason   string
	}{
		{
			name:     "small but stakeholder sensitive",
			signals:  AccountConsultationSignals{SignalsSufficient: true, StakeholderPressureTest: true},
			required: true, reason: "client_strategic_lens_required",
		},
		{
			name:     "technically complex but execution only",
			signals:  AccountConsultationSignals{SignalsSufficient: true},
			required: false, reason: "execution_only_no_client_lens",
		},
		{
			name:     "strategic recommendation",
			signals:  AccountConsultationSignals{SignalsSufficient: true, ClientNarrativeOrDecision: true},
			required: true, reason: "client_strategic_lens_required",
		},
		{
			name:     "insufficient signals fail safe",
			signals:  AccountConsultationSignals{},
			required: true, reason: "insufficient_client_lens_signals",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			decision := ResolveAccountConsultation(test.signals, []string{"bcgos://workspace/alpha/dossier/planner.json"})
			if decision.Required != test.required || decision.ReasonCode != test.reason || decision.ResolvedBy != "maestro" {
				t.Fatalf("decision=%#v, want required=%v reason=%q", decision, test.required, test.reason)
			}
		})
	}
}

func TestPilotRequiresAnAuditedWalterDecisionForNonMaterialWork(t *testing.T) {
	pilot := newTestPilot(t, "claude")
	_, receipt, err := pilot.Delegate(Intent{WorkspaceID: "alpha", Objective: "Perform an ordinary operation.", TTL: time.Hour})
	if err == nil || receipt.FailureCode != "walter_decision_missing" {
		t.Fatalf("missing Walter planner decision = %#v err=%v", receipt, err)
	}
}

func TestWalterRequirementIsIndependentHighLeverageAxis(t *testing.T) {
	if ResolveWalterRequired(WalterLeverageSignals{LowLeverage: true}) {
		t.Fatal("ordinary low-leverage work unexpectedly requires Walter")
	}
	if !ResolveWalterRequired(WalterLeverageSignals{LowLeverage: true, ReputationalRisk: true}) {
		t.Fatal("reputational risk did not require Walter")
	}
	if !ResolveWalterRequired(WalterLeverageSignals{LowLeverage: true, MaterialityUnclear: true}) {
		t.Fatal("materiality uncertainty did not fail closed to Walter")
	}
	if !ResolveWalterRequired(WalterLeverageSignals{ExecutiveRecommendation: true}) {
		t.Fatal("executive recommendation did not require Walter")
	}
}

func TestWalterIsConstructiveAndLoadBearingOnly(t *testing.T) {
	review := ReviewPacket{
		SourcePacketID: strings.Repeat("a", 64), SourcePacketSHA256: strings.Repeat("b", 64),
		SourceScopeKind: "workspace", SourceScopeID: "alpha", Trigger: ReviewMaterialRecommendation,
		Audience: "sponsor", Recommendation: "Preserve the central thesis.", DefinitionOfDone: "The decision is usable.",
	}
	if err := validateWalterReviewBody(WalterReviewBody{
		Verdict:    WalterApproved,
		Objections: []WalterObjection{{Code: "polish", Fix: "Tighten one sentence if useful.", ExitCondition: "Meaning remains unchanged."}},
	}, review); err != nil {
		t.Fatalf("non-blocking polish should accompany approval: %v", err)
	}
	if err := validateWalterReviewBody(WalterReviewBody{
		Verdict:    WalterRefineAndReturn,
		Objections: []WalterObjection{{Code: "nitpick", Fix: "Change a preferred word.", ExitCondition: "The word changes."}},
	}, review); err == nil {
		t.Fatal("cosmetic critique was allowed to block")
	}
	if err := validateWalterReviewBody(WalterReviewBody{
		Verdict:    WalterRefineAndReturn,
		Objections: []WalterObjection{{Code: "missing-evidence", Fix: "Add the evidence supporting the consequential claim.", ExitCondition: "The evidence is linked and supports the claim.", Blocking: true}},
	}, review); err != nil {
		t.Fatalf("load-bearing constructive refinement was rejected: %v", err)
	}
	if err := validateWalterReviewBody(WalterReviewBody{Verdict: WalterHold}, review); err == nil {
		t.Fatal("empty exceptional hold was accepted")
	}
}
