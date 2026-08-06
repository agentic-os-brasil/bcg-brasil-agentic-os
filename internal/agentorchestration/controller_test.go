package agentorchestration

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/agentcatalog"
)

func TestAdaptersShareTheSameSingleActiveSpokeContract(t *testing.T) {
	catalog := loadCatalog(t)
	for _, runtime := range []string{"claude", "codex"} {
		t.Run(runtime, func(t *testing.T) {
			adapter, err := NewAdapter(runtime, catalog, testAuthorizations(), mustStore(t))
			if err != nil {
				t.Fatal(err)
			}
			if decision := adapter.StartBranch("maestro", "maestro-cap", "case-agent-alpha", "run-alpha", "alpha", "case"); !decision.Allowed {
				t.Fatalf("Case branch denied: %#v", decision)
			}
			if decision := adapter.StartBranch("maestro", "maestro-cap", "client-account-agent-alpha", "run-beta", "account-alpha", "account"); decision.Allowed || decision.Code != "branch_active" {
				t.Fatalf("parallel spoke was accepted: %#v", decision)
			}
			if decision := adapter.StartChild("case-agent-alpha", "case-cap", "walter", "run-alpha", "nested", "alpha", "case"); decision.Allowed || decision.Code != "depth_one_no_children" {
				t.Fatalf("nested delegation was accepted: %#v", decision)
			}
			if decision := adapter.FinishBranch("case-agent-alpha", "case-cap", "run-alpha"); !decision.Allowed {
				t.Fatalf("Case branch did not finish: %#v", decision)
			}
		})
	}
}

func TestAdaptersKeepToolAccessBoundToTheActiveRoot(t *testing.T) {
	catalog := loadCatalog(t)
	for _, runtime := range []string{"claude", "codex"} {
		t.Run(runtime, func(t *testing.T) {
			adapter, err := NewAdapter(runtime, catalog, testAuthorizations(), mustStore(t))
			if err != nil {
				t.Fatal(err)
			}
			if decision := adapter.StartBranch("maestro", "maestro-cap", "case-agent-alpha", "run-alpha", "alpha", "case"); !decision.Allowed {
				t.Fatal(decision)
			}
			if decision := adapter.GuardTool("case-agent-alpha", "case-cap", "run-alpha", "", "alpha", "case", "workspace_reader", "read", "bcgos://case/alpha/input.md"); !decision.Allowed {
				t.Fatalf("scoped Case tool denied: %#v", decision)
			}
			if decision := adapter.GuardTool("case-agent-alpha", "case-cap", "run-alpha", "", "other", "case", "workspace_reader", "read", "bcgos://case/alpha/input.md"); decision.Allowed || decision.Code != "scope_denied" {
				t.Fatalf("cross-scope tool was accepted: %#v", decision)
			}
			if decision := adapter.GuardTool("maestro", "maestro-cap", "run-alpha", "", "alpha", "case", "shell", "exec", "bcgos://case/alpha/input.md"); decision.Allowed || decision.Code != "tool_denied" {
				t.Fatalf("Maestro received tool access: %#v", decision)
			}
		})
	}
}

func TestAdaptersPersistMetadataOnlyBreadcrumbsAcrossRestart(t *testing.T) {
	catalog := loadCatalog(t)
	path := filepath.Join(t.TempDir(), ".bcgos", "maestro-orchestration-state.json")
	store, err := NewDurableStateStore(path, "recovery-cap")
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := NewAdapter("claude", catalog, testAuthorizations(), store)
	if err != nil {
		t.Fatal(err)
	}
	if decision := adapter.StartBranch("maestro", "maestro-cap", "case-agent-alpha", "run-breadcrumb", "alpha", "case"); !decision.Allowed {
		t.Fatal(decision)
	}
	if decision := adapter.GuardTool("case-agent-alpha", "case-cap", "run-breadcrumb", "", "alpha", "case", "workspace_reader", "read", "bcgos://case/alpha/input.md"); !decision.Allowed {
		t.Fatal(decision)
	}
	snapshot := adapter.Snapshot()
	if snapshot.BreadcrumbSeq != 2 || len(snapshot.BreadcrumbTail) != 2 || snapshot.BreadcrumbTail[1].Tool != "workspace_reader" || snapshot.BreadcrumbTail[1].ResourceSHA256 == "" {
		t.Fatalf("breadcrumb tail = %#v", snapshot.BreadcrumbTail)
	}
	encoded, _ := json.Marshal(snapshot.BreadcrumbTail)
	if strings.Contains(string(encoded), "input.md") {
		t.Fatalf("breadcrumb leaked resource path: %s", encoded)
	}
	restarted, err := NewDurableStateStore(path, "recovery-cap")
	if err != nil {
		t.Fatal(err)
	}
	if got := restarted.Snapshot(); got.BreadcrumbSeq != 2 || len(got.BreadcrumbTail) != 2 || got.BreadcrumbTail[0].Digest == "" {
		t.Fatalf("restarted breadcrumbs = %#v", got.BreadcrumbTail)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data = bytes.Replace(data, []byte(`"decision_code": "allowed"`), []byte(`"decision_code": "tampered"`), 1)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewDurableStateStore(path, "recovery-cap"); err == nil {
		t.Fatal("tampered breadcrumb chain was accepted")
	}
}

func TestDarwinRemainsScopedAndWalterRemainsToolless(t *testing.T) {
	catalog := loadCatalog(t)
	adapter, err := NewAdapter("claude", catalog, testAuthorizations(), mustStore(t))
	if err != nil {
		t.Fatal(err)
	}
	if decision := adapter.StartBranch("maestro", "maestro-cap", "darwin", "darwin-run", "maestro-system", "health"); !decision.Allowed {
		t.Fatal(decision)
	}
	if decision := adapter.GuardTool("darwin", "darwin-cap", "darwin-run", "", "maestro-system", "health", "filesystem", "write", "bcgos://health/maestro-system/derived/receipt.json"); !decision.Allowed {
		t.Fatal(decision)
	}
	if decision := adapter.FinishBranch("darwin", "darwin-cap", "darwin-run"); !decision.Allowed {
		t.Fatal(decision)
	}
	invalid := testAuthorizations()
	invalid[3].Tools = []ToolGrant{{Tool: "filesystem", Operation: "read", ResourcePrefix: "bcgos://review/review/"}}
	if _, err := NewAdapter("claude", catalog, invalid, mustStore(t)); err == nil {
		t.Fatal("Walter tool grant was accepted")
	}
}

func TestSharedStateReplacementAndRecoveryFenceTheBranch(t *testing.T) {
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
	if decision := claude.StartBranch("maestro", "maestro-cap", "case-agent-alpha", "run-alpha", "alpha", "case"); !decision.Allowed {
		t.Fatal(decision)
	}
	if decision := codex.StartBranch("maestro", "maestro-cap", "client-account-agent-alpha", "run-beta", "account-alpha", "account"); decision.Allowed || decision.Code != "branch_active" {
		t.Fatalf("replacement opened a parallel branch: %#v", decision)
	}
	restored, err := RestoreStateStore(store.Snapshot(), "recovery-cap")
	if err != nil {
		t.Fatal(err)
	}
	restarted, err := NewAdapter("codex", catalog, testAuthorizations(), restored)
	if err != nil {
		t.Fatal(err)
	}
	if decision := restarted.StartBranch("maestro", "maestro-cap", "client-account-agent-alpha", "run-beta", "account-alpha", "account"); decision.Allowed || decision.Code != "branch_active" {
		t.Fatalf("restarted adapter bypassed fence: %#v", decision)
	}
	if restarted.RecoverStale(time.Minute, "wrong-recovery") {
		t.Fatal("wrong recovery capability cleared active state")
	}
}

func TestAdaptersRejectUnknownEventsAndInvalidAuthorizations(t *testing.T) {
	catalog := loadCatalog(t)
	if _, err := NewAdapter("unknown", catalog, testAuthorizations(), mustStore(t)); err == nil {
		t.Fatal("unknown runtime was accepted")
	}
	adapter, err := NewAdapter("claude", catalog, testAuthorizations(), mustStore(t))
	if err != nil {
		t.Fatal(err)
	}
	if decision := adapter.Handle(NativeEvent{Name: "invented", ActorID: "maestro", ActorCapability: "maestro-cap"}); decision.Allowed || decision.Code != "event_unsupported" {
		t.Fatalf("unknown event decision = %#v", decision)
	}
	invalid := testAuthorizations()
	invalid[1].AgentID = "../case"
	if _, err := NewAdapter("claude", catalog, invalid, mustStore(t)); err == nil {
		t.Fatal("unsafe authorization was accepted")
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

func testAuthorizations() []Authorization {
	return []Authorization{
		{AgentID: "maestro", Role: "hub", ScopeKind: "control", Capability: "maestro-cap"},
		{AgentID: "case-agent-alpha", Role: "case_agent", Scope: "alpha", ScopeKind: "case", Capability: "case-cap", Tools: []ToolGrant{{Tool: "workspace_reader", Operation: "read", ResourcePrefix: "bcgos://case/alpha/"}}},
		{AgentID: "client-account-agent-alpha", Role: "client_account_agent", Scope: "account-alpha", ScopeKind: "account", Capability: "account-cap", Tools: []ToolGrant{{Tool: "workspace_reader", Operation: "read", ResourcePrefix: "bcgos://account/account-alpha/"}}},
		{AgentID: "walter", Role: "reviewer", Scope: "review", ScopeKind: "review", Capability: "walter-cap"},
		{AgentID: "pa-expert-fpa", Role: "pa_expert", Scope: "fpa", ScopeKind: "practice", Capability: "pa-cap"},
		{AgentID: "darwin", Role: "governance_analyst", Scope: "maestro-system", ScopeKind: "health", Capability: "darwin-cap", Tools: []ToolGrant{{Tool: "filesystem", Operation: "write", ResourcePrefix: "bcgos://health/maestro-system/"}}},
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
