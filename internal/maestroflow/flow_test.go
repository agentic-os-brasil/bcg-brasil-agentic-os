package maestroflow

import (
	"strings"
	"testing"
	"time"
)

func TestAccountAssistedFlowRequiresValidationAndWalter(t *testing.T) {
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	request, err := NewRequest("attempt-account", "framed-case-material", AccountFirst, "", true, "", "", now)
	if err != nil {
		t.Fatal(err)
	}
	state, receipt, err := Start(request)
	if err != nil || receipt.Event != "started" || !state.PreAccountUsed || !state.PostAccountValidationRequired {
		t.Fatalf("start = %#v %#v %v", state, receipt, err)
	}
	state, _, err = state.CompleteAccountBriefing(now)
	if err != nil {
		t.Fatal(err)
	}
	state, receipt, err = state.CompleteCase(now)
	if err != nil || !receipt.Invalidated || state.Phase != PhaseAccountValidation {
		t.Fatalf("case completion = %#v %#v %v", state, receipt, err)
	}
	state, receipt, err = state.ValidateAccount(AccountApproved, now)
	if err != nil || receipt.AccountVerdict != AccountApproved || !state.PostAccountValidated {
		t.Fatalf("account validation = %#v %#v %v", state, receipt, err)
	}
	state, _, err = state.OpenWalter(now)
	if err != nil {
		t.Fatal(err)
	}
	state, _, err = state.ApplyWalter(WalterApproved, now)
	if err != nil {
		t.Fatal(err)
	}
	state, receipt, err = state.Deliver(now)
	if err != nil || receipt.Event != "material_delivered" || state.Phase != PhaseDelivered {
		t.Fatalf("delivery = %#v %#v %v", state, receipt, err)
	}
	if err := receipt.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestDirectCaseSkipsClientAccountButRequiresWalter(t *testing.T) {
	now := time.Date(2026, 8, 1, 13, 0, 0, 0, time.UTC)
	request, err := NewRequest("attempt-direct", "simple-case-material", CaseDirect, DirectCaseReason, true, "", "", now)
	if err != nil {
		t.Fatal(err)
	}
	state, _, err := Start(request)
	if err != nil {
		t.Fatal(err)
	}
	if state.PreAccountUsed || state.PostAccountValidationRequired || state.Phase != PhaseCaseExecution {
		t.Fatalf("direct path flags = %#v", state)
	}
	state, _, err = state.CompleteCase(now)
	if err != nil || state.Phase != PhaseMaestroReturn {
		t.Fatalf("direct case return = %#v %v", state, err)
	}
	if _, _, err := state.ValidateAccount(AccountApproved, now); err == nil {
		t.Fatal("direct Case accepted Client Account validation")
	}
	if _, _, err := state.Deliver(now); err == nil {
		t.Fatal("direct Case delivered without Walter")
	}
	state, _, err = state.OpenWalter(now)
	if err != nil {
		t.Fatal(err)
	}
	state, _, err = state.ApplyWalter(WalterApproved, now)
	if err != nil {
		t.Fatal(err)
	}
	state, receipt, err := state.Deliver(now)
	if err != nil || !receipt.WalterGate || receipt.PostAccountValidationRequired || state.Phase != PhaseDelivered {
		t.Fatalf("direct delivery = %#v %#v %v", state, receipt, err)
	}
}

func TestAccountAssistedAndDirectLowMaterialityCanSkipWalter(t *testing.T) {
	now := time.Date(2026, 8, 1, 13, 30, 0, 0, time.UTC)
	evidence := strings.Repeat("a", 64)
	for _, path := range []EntryPath{AccountFirst, CaseDirect} {
		reason := ""
		if path == CaseDirect {
			reason = DirectCaseReason
		}
		request, err := NewRequest("skip-"+string(path), "low-materiality", path, reason, false, WalterSkipReason, evidence, now)
		if err != nil {
			t.Fatal(err)
		}
		state, _, _ := Start(request)
		if path == AccountFirst {
			state, _, _ = state.CompleteAccountBriefing(now)
		}
		state, _, _ = state.CompleteCase(now)
		if path == AccountFirst {
			state, _, err = state.ValidateAccount(AccountApproved, now)
			if err != nil {
				t.Fatal(err)
			}
		}
		state, receipt, err := state.SkipWalter(now)
		if err != nil || !receipt.WalterSkipped {
			t.Fatalf("skip %s = %#v %#v %v", path, state, receipt, err)
		}
		state, receipt, err = state.Deliver(now)
		if err != nil || receipt.Event != "material_delivered" {
			t.Fatalf("delivery %s = %#v %#v %v", path, state, receipt, err)
		}
	}
}

func TestMaterialityEscalationInvalidatesWalterSkip(t *testing.T) {
	now := time.Date(2026, 8, 1, 13, 45, 0, 0, time.UTC)
	evidence := strings.Repeat("b", 64)
	request, err := NewRequest("escalated", "low-materiality", CaseDirect, DirectCaseReason, false, WalterSkipReason, evidence, now)
	if err != nil {
		t.Fatal(err)
	}
	state, _, _ := Start(request)
	state, _, _ = state.CompleteCase(now)
	state, _, _ = state.SkipWalter(now)
	state, receipt, err := state.EscalateMateriality(now)
	if err != nil || !receipt.Invalidated || !state.WalterRequired || state.WalterSkipped {
		t.Fatalf("escalation = %#v %#v %v", state, receipt, err)
	}
	state, _, err = state.OpenWalter(now)
	if err != nil {
		t.Fatal(err)
	}
	state, _, err = state.ApplyWalter(WalterApproved, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := state.Deliver(now); err != nil {
		t.Fatal(err)
	}
}

func TestAccountConsultationUsesStrategicLensNotTaskSize(t *testing.T) {
	now := time.Now().UTC()
	evidence := strings.Repeat("f", 64)
	request, err := NewDecisionRequest("small-sensitive", "small", CaseDirect, true, []AccountSignal{SignalStakeholderPressure}, "", false, WalterSkipReason, evidence, now)
	if err == nil {
		t.Fatal("account consultation was allowed on the direct path")
	}
	request, err = NewDecisionRequest("small-sensitive", "small", AccountFirst, true, []AccountSignal{SignalStakeholderPressure}, "", false, WalterSkipReason, evidence, now)
	if err != nil {
		t.Fatal(err)
	}
	if !request.AccountConsultationRequired {
		t.Fatal("strategic signal did not require Account")
	}
	request, err = NewDecisionRequest("complex-execution", "complex", CaseDirect, false, []AccountSignal{SignalExecutionOnly}, DirectCaseReason, true, "", "", now)
	if err != nil {
		t.Fatal(err)
	}
	if request.AccountConsultationRequired {
		t.Fatal("execution-only complexity routed to Account")
	}
	consult, err := ResolveAccountConsultation(nil)
	if err != nil || !consult {
		t.Fatalf("absence of signals was not fail-safe: %v %v", consult, err)
	}
}

func TestWalterIsConstructiveSeniorAdvisorNotNaysayer(t *testing.T) {
	now := time.Now().UTC()
	request, err := NewRequest("advisor", "material", CaseDirect, DirectCaseReason, true, "", "", now)
	if err != nil {
		t.Fatal(err)
	}
	state, _, _ := Start(request)
	state, _, _ = state.CompleteCase(now)
	state, _, _ = state.OpenWalter(now)
	if _, _, err := state.ApplyWalter(WalterRefine, now); err == nil {
		t.Fatal("Walter accepted a non-actionable refinement")
	}
	state, err = state.WithWalterRefinement(RefinementClarity)
	if err != nil {
		t.Fatal(err)
	}
	state, receipt, err := state.ApplyWalter(WalterRefine, now)
	if err != nil || !receipt.RefinementLoadBearing || !receipt.RefinementActionable || receipt.RefinementKind != RefinementClarity || state.Phase != PhaseRefinement {
		t.Fatalf("constructive Walter refinement = %#v %#v %v", state, receipt, err)
	}
	if _, err := ResolveWalterRequirement([]LeverageSignal{SignalOrdinaryReversible}); err != nil {
		t.Fatal(err)
	}
	if required, _ := ResolveWalterRequirement([]LeverageSignal{SignalImportantTradeoff}); !required {
		t.Fatal("high-leverage tradeoff skipped Walter")
	}
}

func TestRefinementInvalidatesGatesAndReentersCanonicalPath(t *testing.T) {
	now := time.Date(2026, 8, 1, 14, 0, 0, 0, time.UTC)
	request, err := NewRequest("attempt-refine", "refinable-material", AccountFirst, "", true, "", "", now)
	if err != nil {
		t.Fatal(err)
	}
	state, _, _ := Start(request)
	state, _, _ = state.CompleteAccountBriefing(now)
	state, _, _ = state.CompleteCase(now)
	state, receipt, err := state.ValidateAccount(AccountRefine, now)
	if err != nil || !receipt.Invalidated || state.PostAccountValidated || state.Phase != PhaseRefinement {
		t.Fatalf("account refine = %#v %#v %v", state, receipt, err)
	}
	state, _, err = state.Reenter(now)
	if err != nil || state.Cycle != 1 || state.Phase != PhaseCaseExecution {
		t.Fatalf("reentry = %#v %v", state, err)
	}
	state, _, _ = state.CompleteCase(now)
	state, _, _ = state.ValidateAccount(AccountApproved, now)
	state, _, _ = state.OpenWalter(now)
	state, err = state.WithWalterRefinement(RefinementTradeoff)
	if err != nil {
		t.Fatal(err)
	}
	state, receipt, err = state.ApplyWalter(WalterRefine, now)
	if err != nil || !receipt.Invalidated || state.WalterGate || state.Phase != PhaseRefinement {
		t.Fatalf("Walter refine = %#v %#v %v", state, receipt, err)
	}
	state, _, err = state.Reenter(now)
	if err != nil || state.Cycle != 2 {
		t.Fatalf("second reentry = %#v %v", state, err)
	}
}

func TestFlowRejectsAsymmetryAndMetadataOnlyReceipt(t *testing.T) {
	now := time.Date(2026, 8, 1, 15, 0, 0, 0, time.UTC)
	if _, err := NewRequest("direct-without-reason", "material", CaseDirect, "", true, "", "", now); err == nil {
		t.Fatal("direct path without reason code was accepted")
	}
	evidence := strings.Repeat("e", 64)
	request, err := NewRequest("attempt-metadata", "material", CaseDirect, DirectCaseReason, false, WalterSkipReason, evidence, now)
	if err != nil {
		t.Fatal(err)
	}
	state, receipt, err := Start(request)
	if err != nil {
		t.Fatal(err)
	}
	receipt.PostAccountValidationRequired = true
	if err := receipt.Validate(); err == nil {
		t.Fatal("asymmetric receipt was accepted")
	}
	state, receipt, _ = state.MarkBudgetExhausted(now)
	if !receipt.BudgetExhausted || state.AttemptID != "attempt-metadata" {
		t.Fatalf("budget receipt = %#v %#v", state, receipt)
	}
	if strings.Contains(strings.ToLower(receipt.Event), "material") && strings.Contains(receipt.Event, "prompt") {
		t.Fatal("receipt contains prompt content")
	}
}
