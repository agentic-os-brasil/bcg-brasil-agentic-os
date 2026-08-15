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
	if plan.CaseEntry != CaseEntryAccountFirst || !plan.RequiresAccountFraming || !plan.RequiresAccountValidation || !plan.RequiresYoda || plan.SkipYoda || len(plan.Bindings) != 3 {
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
	advance(Event{AgentID: "yoda", Decision: "approve", ContentDigest: digest})
	if state.Stage != StageFinal || state.YodaApprovalDigest != digest {
		t.Fatalf("material account path did not finish: %#v", state)
	}
}

func TestPlannerBuildsActionSpecificChainsForEveryNonCaseRoute(t *testing.T) {
	tests := []struct {
		name       string
		input      Input
		stage      Stage
		activeRole string
		direct     bool
	}{
		{name: "direct answer", input: Input{SchemaVersion: 1, IntentClass: IntentDirectAnswer, ScopeKind: "workspace", ScopeID: "workspace-a", Sensitivity: SensitivityInternal, Materiality: MaterialityNone, HealthIntent: HealthNone}, stage: StageFinal, direct: true},
		{name: "account advisory", input: routeInput(IntentAccount, "account", "client-alpha", RegisteredAgent{ID: "account-agent", Role: "client_account_agent", ScopeKind: "account", ScopeID: "client-alpha"}), stage: StageAccountAdvisory, activeRole: "client_account_agent"},
		{name: "PA advisory", input: routeInput(IntentAdvisory, "practice", "fpa", RegisteredAgent{ID: "pa-expert", Role: "pa_expert", ScopeKind: "practice", ScopeID: "fpa"}), stage: StagePAExpert, activeRole: "pa_expert"},
		{name: "Yoda review", input: routeInput(IntentReview, "review", "review", RegisteredAgent{ID: "yoda", Role: "reviewer", ScopeKind: "review", ScopeID: "review"}), stage: StageYodaReview, activeRole: "reviewer"},
		{name: "Darwin health", input: func() Input {
			input := routeInput(IntentHealth, "health", "system", RegisteredAgent{ID: "darwin", Role: "governance_analyst", ScopeKind: "health", ScopeID: "system"})
			input.HealthIntent = HealthSystem
			return input
		}(), stage: StageDarwinHealth, activeRole: "governance_analyst"},
		{name: "bounded errand", input: routeInput(IntentErrand, "errand", "errand-a", RegisteredAgent{ID: "errand", Role: "errand_helper", ScopeKind: "errand", ScopeID: "errand-a"}), stage: StageErrandExecution, activeRole: "errand_helper"},
		{name: "longitudinal Gamma quality", input: routeInput(IntentQuality, "workspace", "repo-a", RegisteredAgent{ID: "gamma-guardian", Role: "quality_guardian", ScopeKind: "workspace", ScopeID: "repo-a"}), stage: StageGammaQuality, activeRole: "quality_guardian"},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			plan, err := PlanFor(testCase.input)
			if err != nil {
				t.Fatal(err)
			}
			state, err := NewChain(plan, DefaultLoopPolicy)
			if err != nil {
				t.Fatal(err)
			}
			if state.Stage != testCase.stage {
				t.Fatalf("initial stage = %s, want %s", state.Stage, testCase.stage)
			}
			if testCase.direct {
				if len(state.Receipts) != 1 || state.Receipts[0].Decision != "direct_no_branch" {
					t.Fatalf("direct route receipt = %#v", state.Receipts)
				}
				return
			}
			if state.ActiveAgentID != bindingID(plan, testCase.activeRole) {
				t.Fatalf("active spoke = %q, want %q", state.ActiveAgentID, bindingID(plan, testCase.activeRole))
			}
			state, _, err = state.Advance(plan, DefaultLoopPolicy, "maestro", Event{AgentID: state.ActiveAgentID, Decision: "return", ContentDigest: digestFor(testCase.name)})
			if err != nil || state.Stage != StageFinal {
				t.Fatalf("route did not finish: state=%#v err=%v", state, err)
			}
		})
	}
}

func routeInput(intent IntentClass, scopeKind, scopeID string, agent RegisteredAgent) Input {
	agent.Available = true
	agent.AuthorizationDigest = digestFor(agent.ID + "-authorization")
	agent.CapabilityDigest = digestFor(agent.ID + "-capability")
	agent.StateSnapshotDigest = digestFor(agent.ID + "-state")
	input := Input{SchemaVersion: 1, IntentClass: intent, ScopeKind: scopeKind, ScopeID: scopeID, Sensitivity: SensitivityInternal, Materiality: MaterialityNone, HealthIntent: HealthNone, SimpleReversible: intent == IntentErrand, ExecutionOnly: intent == IntentErrand, AvailableAgents: []RegisteredAgent{agent}}
	if intent == IntentQuality {
		input.SourceHead = strings.Repeat("a", 40)
	}
	return input
}

func TestGammaQualityRejectsCaseScopeContext(t *testing.T) {
	input := routeInput(IntentQuality, "case", "case-alpha", RegisteredAgent{ID: "gamma-guardian", Role: "quality_guardian", ScopeKind: "case", ScopeID: "case-alpha"})
	if _, err := PlanFor(input); err == nil || !strings.Contains(err.Error(), "workspace scope") {
		t.Fatalf("Gamma accepted case-scoped quality context: %v", err)
	}
}

func TestGammaQualityPinsCanonicalSourceHeadIntoPlanDigest(t *testing.T) {
	input := routeInput(IntentQuality, "workspace", "repo-a", RegisteredAgent{ID: "gamma-guardian", Role: "quality_guardian", ScopeKind: "workspace", ScopeID: "repo-a"})
	plan, err := PlanFor(input)
	if err != nil || plan.SourceHead != input.SourceHead || plan.PlanDigest != digestPlan(plan) {
		t.Fatalf("Gamma source head was not plan-bound: %#v %v", plan, err)
	}
	mutated := plan
	mutated.SourceHead = strings.Repeat("b", 40)
	if err := mutated.Validate(); err == nil {
		t.Fatal("Gamma source-head mutation bypassed the plan/dispatch binding digest")
	}
	for _, malformed := range []string{"", "main", strings.Repeat("A", 40), strings.Repeat("a", 39)} {
		input.SourceHead = malformed
		if _, err := PlanFor(input); err == nil {
			t.Fatalf("Gamma accepted mutable or malformed source head %q", malformed)
		}
	}
	input = caseInput(true)
	input.SourceHead = strings.Repeat("a", 40)
	if _, err := PlanFor(input); err == nil {
		t.Fatal("non-Gamma plan accepted source head")
	}
}

func TestPlanValidateAcceptsAllFourCaseRoutes(t *testing.T) {
	cases := []struct {
		name    string
		input   Input
		entry   CaseEntry
		account bool
		yoda    bool
	}{
		{name: "account-assisted-yoda", input: func() Input { input := caseInput(false); input.Materiality = MaterialityReview; return input }(), entry: CaseEntryAccountFirst, account: true, yoda: true},
		{name: "account-assisted-no-yoda", input: caseInput(false), entry: CaseEntryAccountFirst, account: true, yoda: false},
		{name: "direct-yoda", input: func() Input { input := caseInput(true); input.Materiality = MaterialityReview; return input }(), entry: CaseEntryDirect, account: false, yoda: true},
		{name: "direct-no-yoda", input: caseInput(true), entry: CaseEntryDirect, account: false, yoda: false},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			plan, err := PlanFor(testCase.input)
			if err != nil {
				t.Fatal(err)
			}
			if plan.CaseEntry != testCase.entry || plan.RequiresAccountValidation != testCase.account || plan.RequiresYoda != testCase.yoda {
				t.Fatalf("unexpected route: %#v", plan)
			}
			if err := plan.Validate(); err != nil {
				t.Fatalf("valid route rejected: %v", err)
			}
		})
	}
}

func TestPlanValidateRejectsExtraBindingsForCaseRouteSemantics(t *testing.T) {
	tests := []struct {
		name  string
		input Input
		extra AgentBinding
	}{
		{name: "direct-case-account-extra", input: caseInput(true), extra: AgentBinding{ID: "account-agent-client-alpha", Role: "client_account_agent", ScopeKind: "account", ScopeID: "client-alpha", AuthorizationDigest: digestFor("extra-account-auth"), CapabilityDigest: digestFor("extra-account-capability"), StateSnapshotDigest: digestFor("extra-account-state")}},
		{name: "account-first-yoda-extra", input: caseInput(false), extra: AgentBinding{ID: "yoda", Role: "reviewer", ScopeKind: "review", ScopeID: "review", AuthorizationDigest: digestFor("extra-yoda-auth"), CapabilityDigest: digestFor("extra-yoda-capability"), StateSnapshotDigest: digestFor("extra-yoda-state")}},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			plan, err := PlanFor(testCase.input)
			if err != nil {
				t.Fatal(err)
			}
			plan.Bindings = append(plan.Bindings, testCase.extra)
			plan.PlanDigest = digestPlan(plan)
			if err := plan.Validate(); err == nil {
				t.Fatal("extra route binding was accepted")
			}
		})
	}
}

func TestPlannerRequiresExplicitCaseAccountBindingAndDoesNotUseRegistryOrder(t *testing.T) {
	input := caseInput(false)
	input.AccountScopeID = "client-alpha"
	input.AvailableAgents = append(input.AvailableAgents, RegisteredAgent{ID: "account-agent-client-beta", Role: "client_account_agent", ScopeKind: "account", ScopeID: "client-beta", AuthorizationDigest: digestFor("account-beta"), CapabilityDigest: digestFor("capability-beta"), StateSnapshotDigest: digestFor("state-beta"), Available: true})
	input.AvailableAgents[0], input.AvailableAgents[3] = input.AvailableAgents[3], input.AvailableAgents[0]
	plan, err := PlanFor(input)
	if err != nil || len(plan.Bindings) < 2 || plan.Bindings[0].ScopeID != "client-alpha" {
		t.Fatalf("explicit account binding was not selected: %#v %v", plan, err)
	}
	input.AccountScopeID = "client-beta"
	if _, err := PlanFor(input); err == nil {
		t.Fatal("Case crossed into an account without a matching parent binding")
	}
	input.AccountScopeID = ""
	if _, err := PlanFor(input); err == nil {
		t.Fatal("missing Case-to-Account binding was accepted")
	}
}

func TestPlannerAccountAssistedLowMaterialitySkipsOnlyYoda(t *testing.T) {
	plan, err := PlanFor(caseInput(false))
	if err != nil {
		t.Fatal(err)
	}
	if !plan.RequiresAccountFraming || !plan.RequiresAccountValidation || plan.RequiresYoda || !plan.SkipYoda || plan.YodaSkipReasonCode == "" || plan.YodaSkipEvidence == "" {
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
	if err != nil || plan.CaseEntry != CaseEntryDirect || !plan.RequiresYoda || plan.RequiresAccountValidation {
		t.Fatalf("execution-only quality risk collapsed into Account consultation: %#v %v", plan, err)
	}
	strategicLowRisk := caseInput(true)
	strategicLowRisk.StrategicImplication = true
	plan, err = PlanFor(strategicLowRisk)
	if err != nil || plan.CaseEntry != CaseEntryAccountFirst || plan.RequiresYoda {
		t.Fatalf("strategic lens and Yoda decisions were coupled: %#v %v", plan, err)
	}
}

func TestPlannerUsesYodaForHighLeverageSignalsAndSkipsOrdinaryWorkCalmly(t *testing.T) {
	highLeverage := caseInput(false)
	highLeverage.ExecutionOnly = true
	highLeverage.ConsequentialDecision = true
	plan, err := PlanFor(highLeverage)
	if err != nil || plan.CaseEntry != CaseEntryDirect || !plan.RequiresYoda || plan.YodaReasonCode != "yoda_required_high_leverage" {
		t.Fatalf("high-leverage execution did not select Yoda: %#v %v", plan, err)
	}
	ordinary := caseInput(true)
	plan, err = PlanFor(ordinary)
	if err != nil || plan.RequiresYoda || !plan.SkipYoda || plan.YodaReasonCode != "yoda_skipped_low_leverage" {
		t.Fatalf("ordinary work inflated Yoda loop: %#v %v", plan, err)
	}
}

func TestPlannerDirectCaseConvergesToYodaWhenMaterial(t *testing.T) {
	input := caseInput(true)
	input.Materiality = MaterialityReview
	plan, err := PlanFor(input)
	if err != nil {
		t.Fatal(err)
	}
	if plan.CaseEntry != CaseEntryDirect || !plan.SkipPreAccount || plan.RequiresAccountValidation || !plan.RequiresYoda || len(plan.Bindings) != 2 {
		t.Fatalf("direct material plan = %#v", plan)
	}
	state, err := NewChain(plan, DefaultLoopPolicy)
	if err != nil {
		t.Fatal(err)
	}
	digest := digestFor("direct-material-v1")
	state, _, err = state.Advance(plan, DefaultLoopPolicy, "maestro", Event{AgentID: "case-agent-transformation", Decision: "return", ContentDigest: digest})
	if err != nil || state.Stage != StageYodaReview {
		t.Fatalf("direct material Case did not converge to Yoda: %#v %v", state, err)
	}
	state, _, err = state.Advance(plan, DefaultLoopPolicy, "maestro", Event{AgentID: "yoda", Decision: "approve", ContentDigest: digest})
	if err != nil || state.Stage != StageFinal {
		t.Fatalf("direct material Yoda gate = %#v %v", state, err)
	}
}

func TestPlannerDirectCaseLowMaterialitySkipsAccountAndYoda(t *testing.T) {
	plan, err := PlanFor(caseInput(true))
	if err != nil {
		t.Fatal(err)
	}
	if plan.CaseEntry != CaseEntryDirect || !plan.SkipPreAccount || plan.RequiresAccountValidation || plan.RequiresYoda || !plan.SkipYoda || plan.YodaSkipReasonCode == "" || len(plan.Bindings) != 1 {
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
	if receipt.AccountConsultationRequired || receipt.YodaRequired || !receipt.YodaSkipped || state.Stage != StageFinal {
		t.Fatalf("receipt did not preserve direct low-leverage decisions: %+v", receipt)
	}
}

func TestPlannerRevalidatesAfterAccountAndYodaRefinement(t *testing.T) {
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
	advance(Event{AgentID: "yoda", Decision: "refine", ContentDigest: second})
	if state.AccountApprovalDigest != "" || state.YodaApprovalDigest != "" || state.Stage != StageCaseExecution {
		t.Fatalf("refinement did not invalidate approvals: %#v", state)
	}
	advance(Event{AgentID: "case-agent-transformation", Decision: "return", ContentDigest: third})
	advance(Event{AgentID: "account-agent-client-alpha", Decision: "approve", ContentDigest: third})
	advance(Event{AgentID: "yoda", Decision: "approve", ContentDigest: third})
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
	if _, _, err := state.Advance(plan, DefaultLoopPolicy, "maestro", Event{AgentID: "yoda", Decision: "approve", ContentDigest: digestFor("mutated")}); err == nil {
		t.Fatal("Yoda accepted a stale Case digest")
	}
}

func TestPlannerRejectsUnknownReviewTrigger(t *testing.T) {
	input := caseInput(true)
	input.ReviewTrigger = "caller_invented_trigger"
	if _, err := PlanFor(input); err == nil {
		t.Fatal("unknown review trigger was accepted")
	}
}

func TestPlannerBudgetsFailClosedAndMaterialityCannotSkipYoda(t *testing.T) {
	input := caseInput(false)
	input.Materiality = MaterialityReview
	plan, err := PlanFor(input)
	if err != nil {
		t.Fatal(err)
	}
	state, err := NewChain(plan, LoopPolicy{MaxAccountCycles: 1, MaxYodaCycles: 1, MaxCaseAttempts: 2})
	if err != nil {
		t.Fatal(err)
	}
	policy := LoopPolicy{MaxAccountCycles: 1, MaxYodaCycles: 1, MaxCaseAttempts: 2}
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
	bad.RequiresYoda = false
	bad.SkipYoda = false
	bad.YodaSkipReasonCode = ""
	bad.YodaSkipEvidence = ""
	bad.PlanDigest = digestPlan(bad)
	if _, err := NewChain(bad, DefaultLoopPolicy); err == nil {
		t.Fatal("material Case plan without Yoda or valid skip was accepted")
	}
}

func caseInput(simple bool) Input {
	return Input{SchemaVersion: 1, IntentClass: IntentCase, ScopeKind: "case", ScopeID: "transformation", AccountScopeID: "client-alpha", Sensitivity: SensitivityInternal, Materiality: MaterialityNone, HealthIntent: HealthNone, SimpleReversible: simple, ExecutionOnly: simple, AvailableAgents: []RegisteredAgent{
		{ID: "account-agent-client-alpha", Role: "client_account_agent", ScopeKind: "account", ScopeID: "client-alpha", AuthorizationDigest: digestFor("account-auth"), CapabilityDigest: digestFor("account-capability"), StateSnapshotDigest: digestFor("account-state"), Available: true},
		{ID: "case-agent-transformation", Role: "case_agent", ScopeKind: "case", ScopeID: "transformation", ParentScopeKind: "account", ParentScopeID: "client-alpha", AuthorizationDigest: digestFor("case-auth"), CapabilityDigest: digestFor("case-capability"), StateSnapshotDigest: digestFor("case-state"), Available: true},
		{ID: "yoda", Role: "reviewer", ScopeKind: "review", ScopeID: "review", AuthorizationDigest: digestFor("yoda-auth"), CapabilityDigest: digestFor("yoda-capability"), StateSnapshotDigest: digestFor("yoda-state"), Available: true},
	}}
}

func digestFor(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
