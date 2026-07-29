package agentorchestration

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/agentcatalog"
)

func TestAdaptersEnforceEquivalentGovernedSequence(t *testing.T) {
	catalog := loadCatalog(t)
	var baseline []Decision
	for _, runtime := range []string{"claude", "codex"} {
		adapter, err := NewAdapter(runtime, catalog, testAuthorizations(), mustStore(t))
		if err != nil {
			t.Fatal(err)
		}
		var decisions []Decision
		for _, fixture := range governedSequence(t, runtime) {
			decisions = append(decisions, adapter.Handle(fixture))
		}
		if runtime == "claude" {
			baseline = decisions
			continue
		}
		if !reflect.DeepEqual(decisions, baseline) {
			t.Fatalf("runtime decisions differ\nclaude=%#v\ncodex=%#v", baseline, decisions)
		}
	}
	for _, decision := range baseline {
		if !decision.Allowed {
			t.Fatalf("governed sequence denied: %#v", decision)
		}
	}
}

func TestSharedAdversarialFixturesFailClosedInBothRuntimes(t *testing.T) {
	catalog := loadCatalog(t)
	body, err := os.ReadFile("../../adapters/conformance/agent-orchestration.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Cases []struct {
			Name            string `json:"name"`
			SemanticEvent   string `json:"semantic_event"`
			BranchID        string `json:"branch_id"`
			DispatchID      string `json:"dispatch_id"`
			ActorID         string `json:"actor_id"`
			ActorCapability string `json:"actor_capability"`
			TargetID        string `json:"target_id"`
			Scope           string `json:"scope"`
			ScopeKind       string `json:"scope_kind"`
			Tool            string `json:"tool"`
			Operation       string `json:"operation"`
			Resource        string `json:"resource"`
			ExpectedCode    string `json:"expected_code"`
		} `json:"adversarial_cases"`
	}
	if err := json.Unmarshal(body, &fixture); err != nil {
		t.Fatal(err)
	}
	if len(fixture.Cases) == 0 {
		t.Fatal("adversarial orchestration fixtures missing")
	}
	for _, runtime := range []string{"claude", "codex"} {
		for _, test := range fixture.Cases {
			t.Run(runtime+"/"+test.Name, func(t *testing.T) {
				adapter, err := NewAdapter(runtime, catalog, testAuthorizations(), mustStore(t))
				if err != nil {
					t.Fatal(err)
				}
				event := NativeEvent{
					Name:     nativeEventName(runtime, test.SemanticEvent),
					BranchID: test.BranchID, DispatchID: test.DispatchID,
					ActorID:         test.ActorID,
					ActorCapability: test.ActorCapability, TargetID: test.TargetID,
					Scope: test.Scope, ScopeKind: test.ScopeKind,
					Tool: test.Tool, Operation: test.Operation,
					Resource: test.Resource,
				}
				assertDenied(t, adapter.Handle(event), test.ExpectedCode)
			})
		}
	}
}

func TestAdaptersFailClosedOnCoreToolsParallelismAndRoleEscape(t *testing.T) {
	catalog := loadCatalog(t)
	for _, runtime := range []string{"claude", "codex"} {
		t.Run(runtime, func(t *testing.T) {
			adapter, err := NewAdapter(runtime, catalog, testAuthorizations(), mustStore(t))
			if err != nil {
				t.Fatal(err)
			}
			names := nativeNames(runtime)
			assertDenied(t, adapter.Handle(NativeEvent{Name: names.tool, ActorID: "maestro", ActorCapability: "maestro-cap", Scope: "anything"}), "tool_denied")
			assertAllowed(t, adapter.Handle(NativeEvent{Name: names.branchStart, BranchID: "run-alpha", Scope: "alpha", ScopeKind: "workspace", ActorID: "maestro", ActorCapability: "maestro-cap", TargetID: "workspace-alpha"}))
			assertDenied(t, adapter.Handle(NativeEvent{Name: names.branchStart, BranchID: "run-beta", Scope: "beta", ScopeKind: "practice", ActorID: "maestro", ActorCapability: "maestro-cap", TargetID: "practice-insurance"}), "branch_active")
			assertDenied(t, adapter.Handle(NativeEvent{Name: names.childStart, BranchID: "run-alpha", DispatchID: "child-1", Scope: "alpha", ScopeKind: "workspace", ActorID: "workspace-alpha", ActorCapability: "workspace-alpha-cap", TargetID: "subject-insurance"}), "edge_denied")
			assertAllowed(t, adapter.Handle(NativeEvent{Name: names.childStart, BranchID: "run-alpha", DispatchID: "child-1", Scope: "alpha", ScopeKind: "workspace", ActorID: "workspace-alpha", ActorCapability: "workspace-alpha-cap", TargetID: "capability-research"}))
			assertDenied(t, adapter.Handle(NativeEvent{Name: names.childStart, BranchID: "run-alpha", DispatchID: "child-2", Scope: "alpha", ScopeKind: "workspace", ActorID: "capability-research", ActorCapability: "capability-research-cap", TargetID: "capability-other"}), "child_active")
			assertDenied(t, adapter.Handle(NativeEvent{Name: names.tool, BranchID: "run-alpha", DispatchID: "child-1", ActorID: "capability-research", ActorCapability: "capability-research-cap", Scope: "beta", Tool: "workspace_reader", Operation: "read", Resource: "bcgos://workspace/alpha/file.md"}), "scope_denied")
			assertDenied(t, adapter.Handle(NativeEvent{Name: names.tool, BranchID: "run-alpha", DispatchID: "child-1", ActorID: "capability-other", ActorCapability: "capability-other-cap", Scope: "alpha", Tool: "workspace_reader", Operation: "read", Resource: "bcgos://workspace/alpha/file.md"}), "actor_denied")
			assertDenied(t, adapter.Handle(NativeEvent{Name: names.tool, BranchID: "run-alpha", DispatchID: "child-1", ActorID: "capability-research", ActorCapability: "wrong", Scope: "alpha"}), "actor_denied")
			assertDenied(t, adapter.Handle(NativeEvent{Name: names.tool, BranchID: "run-alpha", DispatchID: "child-1", ActorID: "capability-research", ActorCapability: "capability-research-cap", Scope: "alpha", Tool: "shell", Operation: "exec", Resource: "bcgos://workspace/alpha/file.md"}), "resource_denied")
			assertDenied(t, adapter.Handle(NativeEvent{Name: names.tool, BranchID: "run-alpha", DispatchID: "child-1", ActorID: "capability-research", ActorCapability: "capability-research-cap", Scope: "alpha", Tool: "workspace_reader", Operation: "read", Resource: "bcgos://workspace/alpha/%2e%2e/beta.md"}), "resource_denied")
		})
	}
}

func TestAdaptersRejectUnknownRuntimeEventAndMalformedState(t *testing.T) {
	catalog := loadCatalog(t)
	if _, err := NewAdapter("unknown", catalog, testAuthorizations(), mustStore(t)); err == nil {
		t.Fatal("unknown runtime must fail closed")
	}
	adapter, err := NewAdapter("claude", catalog, testAuthorizations(), mustStore(t))
	if err != nil {
		t.Fatal(err)
	}
	assertDenied(t, adapter.Handle(NativeEvent{Name: "invented"}), "event_unsupported")
	names := nativeNames("claude")
	assertDenied(t, adapter.Handle(NativeEvent{Name: names.childStart, BranchID: "run-alpha", DispatchID: "child-1", Scope: "alpha", ScopeKind: "workspace", ActorID: "workspace-alpha", ActorCapability: "workspace-alpha-cap", TargetID: "capability-research"}), "branch_missing")
	assertDenied(t, adapter.Handle(NativeEvent{Name: names.branchStop, BranchID: "alpha", ActorID: "workspace-alpha", ActorCapability: "workspace-alpha-cap"}), "branch_missing")
	assertDenied(t, adapter.Handle(NativeEvent{Name: nativeNames("codex").branchStart, BranchID: "alpha", ScopeKind: "workspace", ActorID: "maestro", ActorCapability: "maestro-cap", TargetID: "workspace-alpha"}), "event_unsupported")
}

func TestChildToolRequestsBindTheActiveDispatchID(t *testing.T) {
	catalog := loadCatalog(t)
	adapter, err := NewAdapter("claude", catalog, testAuthorizations(), mustStore(t))
	if err != nil {
		t.Fatal(err)
	}
	assertAllowed(t, adapter.StartBranch(
		"maestro", "maestro-cap", "workspace-alpha", "run-alpha", "alpha", "workspace",
	))
	assertAllowed(t, adapter.StartChild(
		"workspace-alpha", "workspace-alpha-cap", "capability-research",
		"run-alpha", "child-1", "alpha", "workspace",
	))
	toolEvent := NativeEvent{
		Name: nativeNames("claude").tool, BranchID: "run-alpha",
		DispatchID: "child-1", ActorID: "capability-research",
		ActorCapability: "capability-research-cap", Scope: "alpha",
		Tool: "workspace_reader", Operation: "read",
		Resource: "bcgos://workspace/alpha/file.md",
	}
	assertAllowed(t, adapter.Handle(toolEvent))
	assertAllowed(t, adapter.FinishChild(
		"capability-research", "capability-research-cap", "run-alpha", "child-1",
	))
	assertAllowed(t, adapter.StartChild(
		"workspace-alpha", "workspace-alpha-cap", "capability-research",
		"run-alpha", "child-2", "alpha", "workspace",
	))
	assertDenied(t, adapter.Handle(toolEvent), "dispatch_denied")
	toolEvent.DispatchID = "child-2"
	assertAllowed(t, adapter.Handle(toolEvent))
}

func TestDarwinScopedMaintenanceGrantIsEquivalentAndFailClosed(t *testing.T) {
	catalog := loadCatalog(t)
	for _, runtime := range []string{"claude", "codex"} {
		t.Run(runtime, func(t *testing.T) {
			adapter, err := NewAdapter(runtime, catalog, testAuthorizations(), mustStore(t))
			if err != nil {
				t.Fatal(err)
			}
			assertAllowed(t, adapter.StartBranch("maestro", "maestro-cap", "darwin", "darwin-run", "maestro-system", "health"))
			assertAllowed(t, adapter.GuardTool("darwin", "darwin-cap", "darwin-run", "", "maestro-system", "health", "filesystem", "write", "bcgos://health/maestro-system/derived/receipt.json"))
			assertDenied(t, adapter.GuardTool("darwin", "darwin-cap", "darwin-run", "", "maestro-system", "health", "shell", "exec", "bcgos://health/maestro-system/derived/receipt.json"), "resource_denied")
			assertDenied(t, adapter.GuardTool("darwin", "darwin-cap", "darwin-run", "", "maestro-system", "health", "filesystem", "write", "bcgos://workspace/secret/file.md"), "resource_denied")
			assertAllowed(t, adapter.FinishBranch("darwin", "darwin-cap", "darwin-run"))
		})
	}
}

func TestSharedStateSurvivesAdapterReplacementAndRejectsParallelControllers(t *testing.T) {
	catalog := loadCatalog(t)
	store := mustStore(t)
	claude, err := NewAdapter("claude", catalog, testAuthorizations(), store)
	if err != nil {
		t.Fatal(err)
	}
	codex, err := NewAdapter("codex", catalog, testAuthorizations(), store)
	if err != nil {
		t.Fatal(err)
	}
	assertAllowed(t, claude.Handle(NativeEvent{
		Name: nativeNames("claude").branchStart, BranchID: "run-alpha",
		Scope: "alpha", ScopeKind: "workspace", ActorID: "maestro", ActorCapability: "maestro-cap", TargetID: "workspace-alpha",
	}))
	assertDenied(t, codex.Handle(NativeEvent{
		Name: nativeNames("codex").branchStart, BranchID: "run-beta",
		Scope: "beta", ScopeKind: "practice", ActorID: "maestro", ActorCapability: "maestro-cap", TargetID: "practice-insurance",
	}), "branch_active")

	restored, err := RestoreStateStore(store.Snapshot(), "recovery-cap")
	if err != nil {
		t.Fatal(err)
	}
	restarted, err := NewAdapter("codex", catalog, testAuthorizations(), restored)
	if err != nil {
		t.Fatal(err)
	}
	assertDenied(t, restarted.Handle(NativeEvent{
		Name: nativeNames("codex").branchStart, BranchID: "run-beta",
		Scope: "beta", ScopeKind: "practice", ActorID: "maestro", ActorCapability: "maestro-cap", TargetID: "practice-insurance",
	}), "branch_active")
}

func TestLostStopRequiresExplicitAgeBoundedRecovery(t *testing.T) {
	catalog := loadCatalog(t)
	adapter, err := NewAdapter("claude", catalog, testAuthorizations(), mustStore(t))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	adapter.now = func() time.Time { return now }
	assertAllowed(t, adapter.Handle(NativeEvent{
		Name: nativeNames("claude").branchStart, BranchID: "run-alpha",
		Scope: "alpha", ScopeKind: "workspace", ActorID: "maestro", ActorCapability: "maestro-cap", TargetID: "workspace-alpha",
	}))
	now = now.Add(4 * time.Minute)
	if adapter.RecoverStale(5*time.Minute, "wrong") {
		t.Fatal("stale recovery accepted an invalid capability")
	}
	if adapter.RecoverStale(5*time.Minute, "recovery-cap") {
		t.Fatal("active branch recovered before stale threshold")
	}
	now = now.Add(2 * time.Minute)
	if !adapter.RecoverStale(5*time.Minute, "recovery-cap") {
		t.Fatal("stale branch was not explicitly recovered")
	}
	assertAllowed(t, adapter.Handle(NativeEvent{
		Name: nativeNames("claude").branchStart, BranchID: "run-beta",
		Scope: "beta", ScopeKind: "practice", ActorID: "maestro", ActorCapability: "maestro-cap", TargetID: "practice-insurance",
	}))
}

func TestAdapterRejectsUnsafeAuthorizationsAndRestoredState(t *testing.T) {
	catalog := loadCatalog(t)
	tests := []struct {
		name   string
		mutate func([]Authorization) []Authorization
	}{
		{"unsafe id", func(values []Authorization) []Authorization {
			values[1].AgentID = "../workspace"
			return values
		}},
		{"empty scope", func(values []Authorization) []Authorization {
			values[1].Scope = ""
			return values
		}},
		{"cross scope resource", func(values []Authorization) []Authorization {
			values[3].Tools[0].ResourcePrefix = "bcgos://workspace/beta/"
			return values
		}},
		{"practice reads workspace", func(values []Authorization) []Authorization {
			values[6].Tools = []ToolGrant{{Tool: "workspace_reader", Operation: "read", ResourcePrefix: "bcgos://workspace/beta/"}}
			return values
		}},
		{"encoded traversal resource", func(values []Authorization) []Authorization {
			values[3].Tools[0].ResourcePrefix = "bcgos://workspace/alpha/%2e%2e/"
			return values
		}},
		{"tool grant on maestro", func(values []Authorization) []Authorization {
			values[0].Tools = []ToolGrant{{Tool: "shell", Operation: "exec", ResourcePrefix: "bcgos://public/"}}
			return values
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			grants := testAuthorizations()
			grants = test.mutate(grants)
			if _, err := NewAdapter("claude", catalog, grants, mustStore(t)); err == nil {
				t.Fatal("unsafe authorization accepted")
			}
		})
	}

	restored, err := RestoreStateStore(StateSnapshot{
		BranchID: "run-alpha", ScopeID: "alpha", ScopeKind: "workspace",
		RootID: "workspace-unknown", Updated: time.Now().UTC(),
	}, "recovery-cap")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewAdapter("claude", catalog, testAuthorizations(), restored); err == nil {
		t.Fatal("unauthorized restored state accepted")
	}
}

func TestStateStoreBindsOneAuthorizationPolicy(t *testing.T) {
	catalog := loadCatalog(t)
	store := mustStore(t)
	if _, err := NewAdapter("claude", catalog, testAuthorizations(), store); err != nil {
		t.Fatal(err)
	}
	reordered := testAuthorizations()
	for left, right := 0, len(reordered)-1; left < right; left, right = left+1, right-1 {
		reordered[left], reordered[right] = reordered[right], reordered[left]
	}
	if _, err := NewAdapter("codex", catalog, reordered, store); err != nil {
		t.Fatalf("equivalent reordered policy rejected: %v", err)
	}
	changed := testAuthorizations()
	changed[0].Capability = "different-maestro-cap"
	if _, err := NewAdapter("codex", catalog, changed, store); err == nil {
		t.Fatal("state store accepted an inconsistent authorization policy")
	}
}

func mustStore(t *testing.T) *StateStore {
	t.Helper()
	store, err := NewStateStore("recovery-cap")
	if err != nil {
		t.Fatal(err)
	}
	return store
}

type eventNames struct {
	branchStart string
	childStart  string
	tool        string
	childStop   string
	branchStop  string
}

func nativeNames(runtime string) eventNames {
	if runtime == "claude" {
		return eventNames{"agent_branch_start", "agent_child_start", "pre_tool_use", "agent_child_stop", "agent_branch_stop"}
	}
	return eventNames{"collaboration_branch_start", "collaboration_child_start", "tool_call_guard", "collaboration_child_stop", "collaboration_branch_stop"}
}

func nativeEventName(runtime, semantic string) string {
	names := nativeNames(runtime)
	return map[string]string{
		"branch_start": names.branchStart, "child_start": names.childStart,
		"tool_request": names.tool, "child_finish": names.childStop,
		"branch_finish": names.branchStop,
	}[semantic]
}

func governedSequence(t *testing.T, runtime string) []NativeEvent {
	t.Helper()
	body, err := os.ReadFile("../../adapters/conformance/agent-orchestration.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		SchemaVersion int `json:"schema_version"`
		Events        []struct {
			SemanticEvent   string `json:"semantic_event"`
			BranchID        string `json:"branch_id"`
			DispatchID      string `json:"dispatch_id"`
			ActorID         string `json:"actor_id"`
			ActorCapability string `json:"actor_capability"`
			TargetID        string `json:"target_id"`
			Scope           string `json:"scope"`
			ScopeKind       string `json:"scope_kind"`
			Tool            string `json:"tool"`
			Operation       string `json:"operation"`
			Resource        string `json:"resource"`
		} `json:"events"`
	}
	if err := json.Unmarshal(body, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.SchemaVersion != 1 || len(fixture.Events) == 0 {
		t.Fatal("invalid orchestration conformance fixture")
	}
	names := nativeNames(runtime)
	nativeBySemantic := map[string]string{
		"branch_start":  names.branchStart,
		"child_start":   names.childStart,
		"tool_request":  names.tool,
		"child_finish":  names.childStop,
		"branch_finish": names.branchStop,
	}
	events := make([]NativeEvent, 0, len(fixture.Events))
	for _, event := range fixture.Events {
		nativeName, ok := nativeBySemantic[event.SemanticEvent]
		if !ok {
			t.Fatalf("unsupported fixture event %q", event.SemanticEvent)
		}
		events = append(events, NativeEvent{
			Name: nativeName, BranchID: event.BranchID,
			DispatchID: event.DispatchID, ActorID: event.ActorID,
			ActorCapability: event.ActorCapability, TargetID: event.TargetID,
			Scope: event.Scope, ScopeKind: event.ScopeKind,
			Tool: event.Tool, Operation: event.Operation,
			Resource: event.Resource,
		})
	}
	return events
}

func testAuthorizations() []Authorization {
	return []Authorization{
		{AgentID: "maestro", Role: "hub", ScopeKind: "control", Capability: "maestro-cap"},
		{AgentID: "workspace-alpha", Role: "workspace_agent", Scope: "alpha", ScopeKind: "workspace", Capability: "workspace-alpha-cap"},
		{AgentID: "workspace-alpha-project", Role: "workspace_agent", Scope: "client-alpha-project", ScopeKind: "workspace", Capability: "workspace-alpha-project-cap"},
		{AgentID: "capability-research", Role: "capability_specialist", Scope: "alpha", ScopeKind: "workspace", Capability: "capability-research-cap", Tools: []ToolGrant{{Tool: "workspace_reader", Operation: "read", ResourcePrefix: "bcgos://workspace/alpha/"}}},
		{AgentID: "capability-research-project", Role: "capability_specialist", Scope: "client-alpha-project", ScopeKind: "workspace", Capability: "capability-research-project-cap", Tools: []ToolGrant{{Tool: "workspace_reader", Operation: "read", ResourcePrefix: "bcgos://workspace/client-alpha-project/"}}},
		{AgentID: "capability-other", Role: "capability_specialist", Scope: "alpha", ScopeKind: "workspace", Capability: "capability-other-cap", Tools: []ToolGrant{{Tool: "workspace_reader", Operation: "read", ResourcePrefix: "bcgos://workspace/alpha/"}}},
		{AgentID: "practice-insurance", Role: "practice_agent", Scope: "beta", ScopeKind: "practice", Capability: "practice-insurance-cap"},
		{AgentID: "subject-insurance", Role: "subject_specialist", Scope: "alpha", ScopeKind: "practice", Capability: "subject-insurance-cap"},
		{AgentID: "darwin", Role: "governance_analyst", Scope: "maestro-system", ScopeKind: "health", Capability: "darwin-cap", Tools: []ToolGrant{
			{Tool: "filesystem", Operation: "read", ResourcePrefix: "bcgos://health/maestro-system/"},
			{Tool: "filesystem", Operation: "write", ResourcePrefix: "bcgos://health/maestro-system/"},
		}},
	}
}

func loadCatalog(t *testing.T) agentcatalog.Catalog {
	t.Helper()
	catalog, err := agentcatalog.ParseFile("../../bundles/base/agents/catalog.json")
	if err != nil {
		t.Fatal(err)
	}
	return catalog
}

func assertAllowed(t *testing.T, decision Decision) {
	t.Helper()
	if !decision.Allowed {
		t.Fatalf("decision denied: %#v", decision)
	}
}

func assertDenied(t *testing.T, decision Decision, code string) {
	t.Helper()
	if decision.Allowed || decision.Code != code {
		t.Fatalf("decision = %#v, want denied %s", decision, code)
	}
}
