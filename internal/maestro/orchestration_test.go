package maestro

import (
	"strings"
	"testing"
)

func orchestrationTestAgent(id, role, scopeKind, scopeID, parentKind, parentID string) RegisteredAgent {
	return RegisteredAgent{
		ID: id, Role: role, ScopeKind: scopeKind, ScopeID: scopeID,
		ParentScopeKind: parentKind, ParentScopeID: parentID,
		AuthorizationDigest: SHA256Hex(id + ":authorization"),
		CapabilityDigest:    SHA256Hex(id + ":capability"),
		StateSnapshotDigest: SHA256Hex(id + ":state"), Available: true,
	}
}

func TestExecuteAgentEventsAccountCaseAccountWalter(t *testing.T) {
	plan, err := PlanFor(Input{
		SchemaVersion: 1, IntentClass: IntentCase, ScopeKind: "case", ScopeID: "case-alpha",
		AccountScopeID: "account-alpha", Sensitivity: SensitivityInternal, Materiality: MaterialityReview,
		StrategicImplication: true, ClientImplication: true, HealthIntent: HealthNone,
		AvailableAgents: []RegisteredAgent{
			orchestrationTestAgent("account-agent-alpha", "client_account_agent", "account", "account-alpha", "", ""),
			orchestrationTestAgent("case-agent-alpha", "case_agent", "case", "case-alpha", "account", "account-alpha"),
			orchestrationTestAgent("walter", "reviewer", "review", "review", "", ""),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	digest := SHA256Hex("case-output")
	state, err := ExecuteAgentEvents(plan, []AgentEvent{
		{AgentID: "account-agent-alpha", Decision: "approve"},
		{AgentID: "case-agent-alpha", Decision: "return", ContentDigest: digest},
		{AgentID: "account-agent-alpha", Decision: "approve", ContentDigest: digest},
		{AgentID: "walter", Decision: "approve", ContentDigest: digest},
	}, DefaultLoopPolicy)
	if err != nil {
		t.Fatal(err)
	}
	if state.Stage != StageFinal || state.ActiveAgentID != "" || len(state.Receipts) != 4 {
		t.Fatalf("orchestration state = %#v", state)
	}
	if got := []Stage{state.Receipts[0].Stage, state.Receipts[1].Stage, state.Receipts[2].Stage, state.Receipts[3].Stage}; strings.Join([]string{string(got[0]), string(got[1]), string(got[2]), string(got[3])}, ",") !=
		"account_framing,case_execution,account_validation,walter_review" {
		t.Fatalf("stage order = %v", got)
	}
}

func TestExecuteAgentEventsRejectsWrongActorAndUnboundedEvents(t *testing.T) {
	plan, err := PlanFor(Input{SchemaVersion: 1, IntentClass: IntentDirectAnswer, ScopeKind: "workspace", ScopeID: "workspace-a", Sensitivity: SensitivityInternal, Materiality: MaterialityNone, HealthIntent: HealthNone})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ExecuteAgentEvents(plan, []AgentEvent{{AgentID: "forged", Decision: "return", ContentDigest: SHA256Hex("x")}}, DefaultLoopPolicy); err == nil {
		t.Fatal("forged event was accepted")
	}
	events := make([]AgentEvent, MaximumAgentEvents+1)
	if _, err := ExecuteAgentEvents(plan, events, DefaultLoopPolicy); err == nil {
		t.Fatal("unbounded event sequence was accepted")
	}
}
