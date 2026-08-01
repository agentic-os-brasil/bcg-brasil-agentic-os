package maestro

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

func TestPlannerAccountAssistedMaterialPath(t *testing.T) {
	input := caseInput(false)
	input.Materiality = MaterialityReview
	plan, err := PlanFor(input)
	if err != nil {
		t.Fatal(err)
	}
	if plan.CaseEntry != CaseEntryAccountFirst || !plan.RequiresAccountFraming || !plan.RequiresAccountValidation || !plan.RequiresWalter || plan.SkipWalter || len(plan.Bindings) != 3 {
		t.Fatalf("account-assisted material plan = %#v", plan)
	}
	state, err := NewChain(plan, DefaultLoopPolicy)
	if err != nil {
		t.Fatal(err)
	}
	advance := func(event Event) {
		t.Helper()
		var advanceErr error
		state, _, advanceErr = state.Advance(plan, DefaultLoopPolicy, "maestro", event)
		if advanceErr != nil {
			t.Fatal(advanceErr)
		}
	}
	digest := digestFor("material-v1")
	advance(Event{AgentID: "account-agent-client-alpha", Decision: "approve"})
	advance(Event{AgentID: "case-agent-transformation", Decision: "return", ContentDigest: digest})
	advance(Event{AgentID: "account-agent-client-alpha", Decision: "approve", ContentDigest: digest})
	advance(Event{AgentID: "walter", Decision: "approve", ContentDigest: digest})
	if state.Stage != StageFinal || state.WalterApprovalDigest != digest {
		t.Fatalf("material account path did not finish: %#v", state)
	}
}

func TestPlannerAccountAssistedLowMaterialitySkipsOnlyWalter(t *testing.T) {
	plan, err := PlanFor(caseInput(false))
	if err != nil {
		t.Fatal(err)
	}
	if !plan.RequiresAccountFraming || !plan.RequiresAccountValidation || plan.RequiresWalter || !plan.SkipWalter || plan.WalterSkipReasonCode == "" || plan.WalterSkipEvidence == "" {
		t.Fatalf("account low-materiality plan = %#v", plan)
	}
	state, err := NewChain(plan, DefaultLoopPolicy)
	if err != nil {
		t.Fatal(err)
	}
	var advanceErr error
	state, _, advanceErr = state.Advance(plan, DefaultLoopPolicy, "maestro", Event{AgentID: "account-agent-client-alpha", Decision: "approve"})
	if advanceErr != nil {
		t.Fatal(advanceErr)
	}
	digest := digestFor("low-material-v1")
	state, _, advanceErr = state.Advance(plan, DefaultLoopPolicy, "maestro", Event{AgentID: "case-agent-transformation", Decision: "return", ContentDigest: digest})
	if advanceErr != nil {
		t.Fatal(advanceErr)
	}
	state, _, advanceErr = state.Advance(plan, DefaultLoopPolicy, "maestro", Event{AgentID: "account-agent-client-alpha", Decision: "approve", ContentDigest: digest})
	if advanceErr != nil || state.Stage != StageFinal {
		t.Fatalf("low-materiality account path = %#v %v", state, advanceErr)
	}
}

func TestPlannerUsesClientStrategicLensSignalsInsteadOfTaskSize(t *testing.T) {
	smallStakeholder := caseInput(true)
	smallStakeholder.StakeholderImplication = true
	plan, err := PlanFor(smallStakeholder)
	if err != nil || plan.CaseEntry != CaseEntryAccountFirst || plan.ReasonCode != "client_strategic_lens_required" {
		t.Fatalf("small stakeholder-sensitive task was not account-assisted: %#v %v", plan, err)
	}
	complexExecution := caseInput(false)
	complexExecution.ExecutionOnly = true
	plan, err = PlanFor(complexExecution)
	if err != nil || plan.CaseEntry != CaseEntryDirect || plan.AccountConsultationRequired {
		t.Fatalf("complex execution-only task was not direct Case: %#v %v", plan, err)
	}
	noSignals := caseInput(true)
	noSignals.ExecutionOnly = false
	plan, err = PlanFor(noSignals)
	if err != nil || plan.CaseEntry != CaseEntryAccountFirst {
		t.Fatalf("absence of strategic signals did not fail safe to Account: %#v %v", plan, err)
	}
}

func TestPlannerKeepsQualityRiskIndependentFromClientLens(t *testing.T) {
	directQuality := caseInput(false)
	directQuality.ExecutionOnly = true
	directQuality.Materiality = MaterialityReview
	plan, err := PlanFor(directQuality)
	if err != nil || plan.CaseEntry != CaseEntryDirect || !plan.RequiresWalter || plan.RequiresAccountValidation {
		t.Fatalf("execution-only quality risk collapsed into Account consultation: %#v %v", plan, err)
	}
	strategicLowRisk := caseInput(true)
	strategicLowRisk.StrategicImplication = true
	plan, err = PlanFor(strategicLowRisk)
	if err != nil || plan.CaseEntry != CaseEntryAccountFirst || plan.RequiresWalter {
		t.Fatalf("strategic lens and Walter decisions were coupled: %#v %v", plan, err)
	}
}

func TestPlannerUsesWalterForHighLeverageSignalsAndSkipsOrdinaryWorkCalmly(t *testing.T) {
	highLeverage := caseInput(false)
	highLeverage.ExecutionOnly = true
	highLeverage.ConsequentialDecision = true
	plan, err := PlanFor(highLeverage)
	if err != nil || plan.CaseEntry != CaseEntryDirect || !plan.RequiresWalter || plan.WalterReasonCode != "walter_required_high_leverage" {
		t.Fatalf("high-leverage execution did not select Walter: %#v %v", plan, err)
	}
	ordinary := caseInput(true)
	plan, err = PlanFor(ordinary)
	if err != nil || plan.RequiresWalter || !plan.SkipWalter || plan.WalterReasonCode != "walter_skipped_low_leverage" {
		t.Fatalf("ordinary work inflated Walter loop: %#v %v", plan, err)
	}
}

func TestPlannerDirectCaseConvergesToWalterWhenMaterial(t *testing.T) {
	input := caseInput(true)
	input.Materiality = MaterialityReview
	plan, err := PlanFor(input)
	if err != nil {
		t.Fatal(err)
	}
	if plan.CaseEntry != CaseEntryDirect || !plan.SkipPreAccount || plan.RequiresAccountValidation || !plan.RequiresWalter || len(plan.Bindings) != 2 {
		t.Fatalf("direct material plan = %#v", plan)
	}
	state, err := NewChain(plan, DefaultLoopPolicy)
	if err != nil {
		t.Fatal(err)
	}
	digest := digestFor("direct-material-v1")
	state, _, err = state.Advance(plan, DefaultLoopPolicy, "maestro", Event{AgentID: "case-agent-transformation", Decision: "return", ContentDigest: digest})
	if err != nil || state.Stage != StageWalterReview {
		t.Fatalf("direct material Case did not converge to Walter: %#v %v", state, err)
	}
	state, _, err = state.Advance(plan, DefaultLoopPolicy, "maestro", Event{AgentID: "walter", Decision: "approve", ContentDigest: digest})
	if err != nil || state.Stage != StageFinal {
		t.Fatalf("direct material Walter gate = %#v %v", state, err)
	}
}

func TestPlannerDirectCaseLowMaterialitySkipsAccountAndWalter(t *testing.T) {
	plan, err := PlanFor(caseInput(true))
	if err != nil {
		t.Fatal(err)
	}
	if plan.CaseEntry != CaseEntryDirect || !plan.SkipPreAccount || plan.RequiresAccountValidation || plan.RequiresWalter || !plan.SkipWalter || plan.WalterSkipReasonCode == "" || len(plan.Bindings) != 1 {
		t.Fatalf("direct low-materiality plan = %#v", plan)
	}
	state, err := NewChain(plan, DefaultLoopPolicy)
	if err != nil {
		t.Fatal(err)
	}
	state, _, err = state.Advance(plan, DefaultLoopPolicy, "maestro", Event{AgentID: "case-agent-transformation", Decision: "return", ContentDigest: digestFor("direct-low-v1")})
	if err != nil || state.Stage != StageFinal {
		t.Fatalf("direct low-materiality path = %#v %v", state, err)
	}
}

func TestPlannerReceiptsExposeIndependentRoutingDecisions(t *testing.T) {
	plan, err := PlanFor(caseInput(true))
	if err != nil {
		t.Fatal(err)
	}
	state, err := NewChain(plan, DefaultLoopPolicy)
	if err != nil {
		t.Fatal(err)
	}
	digest := digestFor("ordinary")
	state, receipt, err := state.Advance(plan, DefaultLoopPolicy, "maestro", Event{AgentID: "case-agent-transformation", Decision: "return", ContentDigest: digest})
	if err != nil {
		t.Fatal(err)
	}
	if receipt.AccountConsultationRequired || receipt.WalterRequired || !receipt.WalterSkipped || state.Stage != StageFinal {
		t.Fatalf("receipt did not preserve direct low-leverage decisions: %+v", receipt)
	}
}

func TestPlannerRevalidatesAfterAccountAndWalterRefinement(t *testing.T) {
	input := caseInput(false)
	input.Materiality = MaterialityReview
	plan, err := PlanFor(input)
	if err != nil {
		t.Fatal(err)
	}
	state, err := NewChain(plan, DefaultLoopPolicy)
	if err != nil {
		t.Fatal(err)
	}
	advance := func(event Event) {
		t.Helper()
		var advanceErr error
		state, _, advanceErr = state.Advance(plan, DefaultLoopPolicy, "maestro", event)
		if advanceErr != nil {
			t.Fatal(advanceErr)
		}
	}
	first := digestFor("v1")
	second := digestFor("v2")
	third := digestFor("v3")
	advance(Event{AgentID: "account-agent-client-alpha", Decision: "approve"})
	advance(Event{AgentID: "case-agent-transformation", Decision: "return", ContentDigest: first})
	advance(Event{AgentID: "account-agent-client-alpha", Decision: "refine", ContentDigest: first})
	advance(Event{AgentID: "case-agent-transformation", Decision: "return", ContentDigest: second})
	advance(Event{AgentID: "account-agent-client-alpha", Decision: "approve", ContentDigest: second})
	advance(Event{AgentID: "walter", Decision: "refine", ContentDigest: second})
	if state.AccountApprovalDigest != "" || state.WalterApprovalDigest != "" || state.Stage != StageCaseExecution {
		t.Fatalf("refinement did not invalidate approvals: %#v", state)
	}
	advance(Event{AgentID: "case-agent-transformation", Decision: "return", ContentDigest: third})
	advance(Event{AgentID: "account-agent-client-alpha", Decision: "approve", ContentDigest: third})
	advance(Event{AgentID: "walter", Decision: "approve", ContentDigest: third})
	if state.Stage != StageFinal || len(state.Receipts) != 9 {
		t.Fatalf("bounded revalidation sequence = %#v", state)
	}
}

func TestPlannerIgnoresCallerRoleAndRejectsDirectHandoffsOrStaleApproval(t *testing.T) {
	input := caseInput(true)
	input.Materiality = MaterialityReview
	input.CallerRole = "reviewer"
	plan, err := PlanFor(input)
	if err != nil {
		t.Fatal(err)
	}
	input.CallerRole = "hub"
	other, err := PlanFor(input)
	if err != nil || plan.PlanDigest != other.PlanDigest {
		t.Fatalf("caller role changed plan authority: %#v %#v %v", plan, other, err)
	}
	state, err := NewChain(plan, DefaultLoopPolicy)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := state.Advance(plan, DefaultLoopPolicy, "case-agent-transformation", Event{AgentID: "case-agent-transformation", Decision: "return", ContentDigest: digestFor("bad")}); err == nil || !strings.Contains(err.Error(), "only Maestro") {
		t.Fatalf("direct agent handoff was accepted: %v", err)
	}
	state, _, err = state.Advance(plan, DefaultLoopPolicy, "maestro", Event{AgentID: "case-agent-transformation", Decision: "return", ContentDigest: digestFor("v1")})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := state.Advance(plan, DefaultLoopPolicy, "maestro", Event{AgentID: "walter", Decision: "approve", ContentDigest: digestFor("mutated")}); err == nil {
		t.Fatal("Walter accepted a stale Case digest")
	}
}

func TestPlannerRejectsUnknownReviewTrigger(t *testing.T) {
	input := caseInput(true)
	input.ReviewTrigger = "caller_invented_trigger"
	if _, err := PlanFor(input); err == nil {
		t.Fatal("unknown review trigger was accepted")
	}
}

func TestPlannerBudgetsFailClosedAndMaterialityCannotSkipWalter(t *testing.T) {
	input := caseInput(false)
	input.Materiality = MaterialityReview
	plan, err := PlanFor(input)
	if err != nil {
		t.Fatal(err)
	}
	state, err := NewChain(plan, LoopPolicy{MaxAccountCycles: 1, MaxWalterCycles: 1, MaxCaseAttempts: 2})
	if err != nil {
		t.Fatal(err)
	}
	policy := LoopPolicy{MaxAccountCycles: 1, MaxWalterCycles: 1, MaxCaseAttempts: 2}
	advance := func(event Event) error {
		var advanceErr error
		state, _, advanceErr = state.Advance(plan, policy, "maestro", event)
		return advanceErr
	}
	if err := advance(Event{AgentID: "account-agent-client-alpha", Decision: "approve"}); err != nil {
		t.Fatal(err)
	}
	if err := advance(Event{AgentID: "case-agent-transformation", Decision: "return", ContentDigest: digestFor("v1")}); err != nil {
		t.Fatal(err)
	}
	if err := advance(Event{AgentID: "account-agent-client-alpha", Decision: "refine", ContentDigest: digestFor("v1")}); err != nil {
		t.Fatal(err)
	}
	if err := advance(Event{AgentID: "case-agent-transformation", Decision: "return", ContentDigest: digestFor("v2")}); err != nil {
		t.Fatal(err)
	}
	if err := advance(Event{AgentID: "account-agent-client-alpha", Decision: "refine", ContentDigest: digestFor("v2")}); err == nil || state.Stage != StageFailed {
		t.Fatalf("account budget did not fail closed: %#v %v", state, err)
	}
	bad := plan
	bad.RequiresWalter = false
	bad.SkipWalter = false
	bad.WalterSkipReasonCode = ""
	bad.WalterSkipEvidence = ""
	bad.PlanDigest = digestPlan(bad)
	if _, err := NewChain(bad, DefaultLoopPolicy); err == nil {
		t.Fatal("material Case plan without Walter or valid skip was accepted")
	}
}

func caseInput(simple bool) Input {
	return Input{SchemaVersion: 1, IntentClass: IntentCase, ScopeKind: "case", ScopeID: "transformation", Sensitivity: SensitivityInternal, Materiality: MaterialityNone, HealthIntent: HealthNone, SimpleReversible: simple, ExecutionOnly: simple, AvailableAgents: []RegisteredAgent{
		{ID: "account-agent-client-alpha", Role: "client_account_agent", ScopeKind: "account", ScopeID: "client-alpha", AuthorizationDigest: digestFor("account-auth"), StateSnapshotDigest: digestFor("account-state"), Available: true},
		{ID: "case-agent-transformation", Role: "case_agent", ScopeKind: "case", ScopeID: "transformation", AuthorizationDigest: digestFor("case-auth"), StateSnapshotDigest: digestFor("case-state"), Available: true},
		{ID: "walter", Role: "reviewer", ScopeKind: "review", ScopeID: "review", AuthorizationDigest: digestFor("walter-auth"), StateSnapshotDigest: digestFor("walter-state"), Available: true},
	}}
}

func digestFor(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
