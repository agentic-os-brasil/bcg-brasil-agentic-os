package agentdispatch

import (
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/agentcatalog"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/agentorchestration"
)

func TestDispatcherIssuesSealedSequentialWorkspacePackets(t *testing.T) {
	dispatcher := newTestDispatcher(t)
	now := time.Date(2026, 7, 25, 15, 0, 0, 0, time.UTC)
	dispatcher.now = func() time.Time { return now }

	root, decision, err := dispatcher.StartRoot(PacketRequest{
		TargetAgentID: "workspace-alpha", ScopeKind: "workspace", ScopeID: "alpha",
		Objective:   "Assess the approved public research against the project hypothesis.",
		Pointers:    []string{"bcgos://workspace/alpha/dossier/research.md", "bcgos://public/economic/snapshot-2026-07-25.json"},
		Constraints: []string{"Do not read another workspace.", "Return evidence and uncertainty."},
		TTL:         2 * time.Hour,
	})
	if err != nil || !decision.Allowed {
		t.Fatalf("root dispatch failed: packet=%#v decision=%#v err=%v", root, decision, err)
	}
	if err := dispatcher.Verify(root); err != nil {
		t.Fatal(err)
	}
	if root.IssuerAgentID != "maestro" || root.TargetAgentID != "workspace-alpha" || root.ParentPacketID != "" {
		t.Fatalf("unexpected root packet: %#v", root)
	}

	_, parallel, err := dispatcher.StartRoot(PacketRequest{
		TargetAgentID: "practice-insurance", ScopeKind: "practice", ScopeID: "insurance",
		Objective: "Review insurance concepts.", TTL: time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	if parallel.Allowed || parallel.Code != "branch_active" {
		t.Fatalf("parallel dispatch = %#v", parallel)
	}

	child, childDecision, err := dispatcher.StartChild(root, PacketRequest{
		TargetAgentID: "capability-research", ScopeKind: "workspace", ScopeID: "alpha",
		Objective: "Pressure-test source freshness and provenance.",
		Pointers:  []string{"bcgos://workspace/alpha/dossier/research.md"},
		TTL:       time.Hour,
	})
	if err != nil || !childDecision.Allowed {
		t.Fatalf("child dispatch failed: packet=%#v decision=%#v err=%v", child, childDecision, err)
	}
	if child.ParentPacketID != root.PacketID || child.IssuerAgentID != "workspace-alpha" {
		t.Fatalf("unexpected child packet: %#v", child)
	}
	if decision := dispatcher.FinishChild(child); !decision.Allowed {
		t.Fatalf("finish child = %#v", decision)
	}
	secondChild, secondDecision, err := dispatcher.StartChild(root, PacketRequest{
		TargetAgentID: "capability-research", ScopeKind: "workspace", ScopeID: "alpha",
		Objective: "Check a second bounded question.",
		Pointers:  []string{"bcgos://workspace/alpha/dossier/research.md"},
		TTL:       time.Hour,
	})
	if err != nil || !secondDecision.Allowed {
		t.Fatalf("second child dispatch failed: %#v %v", secondDecision, err)
	}
	if replay := dispatcher.FinishChild(child); replay.Allowed {
		t.Fatalf("old child packet replayed against a new dispatch: %#v", replay)
	}
	if decision := dispatcher.FinishChild(secondChild); !decision.Allowed {
		t.Fatalf("finish second child = %#v", decision)
	}
	if decision := dispatcher.FinishRoot(root); !decision.Allowed {
		t.Fatalf("finish root = %#v", decision)
	}
	replacement, replacementDecision, err := dispatcher.StartRoot(PacketRequest{
		TargetAgentID: "workspace-alpha", ScopeKind: "workspace", ScopeID: "alpha",
		Objective: "Start a later independent branch in the same workspace.",
		TTL:       time.Hour,
	})
	if err != nil || !replacementDecision.Allowed {
		t.Fatalf("replacement root failed: %#v %v", replacementDecision, err)
	}
	if replay := dispatcher.FinishRoot(root); replay.Allowed {
		t.Fatalf("old root packet replayed against a new branch: %#v", replay)
	}
	if decision := dispatcher.FinishRoot(replacement); !decision.Allowed {
		t.Fatalf("finish replacement root = %#v", decision)
	}
}

func TestDispatcherRejectsTamperingExpiryAndCrossScopePointers(t *testing.T) {
	dispatcher := newTestDispatcher(t)
	now := time.Date(2026, 7, 25, 15, 0, 0, 0, time.UTC)
	dispatcher.now = func() time.Time { return now }

	if _, _, err := dispatcher.StartRoot(PacketRequest{
		TargetAgentID: "practice-insurance", ScopeKind: "practice", ScopeID: "insurance",
		Objective: "Review the subject canon.",
		Pointers:  []string{"bcgos://workspace/insurance/raw-client.md"},
		TTL:       time.Hour,
	}); err == nil {
		t.Fatal("practice packet accepted a workspace pointer")
	}

	packet, decision, err := dispatcher.StartRoot(PacketRequest{
		TargetAgentID: "workspace-alpha", ScopeKind: "workspace", ScopeID: "alpha",
		Objective: "Summarize bounded evidence.",
		Pointers:  []string{"bcgos://workspace/alpha/dossier/index.md"},
		TTL:       time.Hour,
	})
	if err != nil || !decision.Allowed {
		t.Fatal(err)
	}
	tampered := packet
	tampered.Objective = "Read every client workspace."
	if err := dispatcher.Verify(tampered); err == nil {
		t.Fatal("tampered packet verified")
	}
	if decision := dispatcher.FinishRoot(tampered); decision.Allowed || decision.Code != "packet_denied" {
		t.Fatalf("tampered finish = %#v", decision)
	}

	now = now.Add(2 * time.Hour)
	if err := dispatcher.Verify(packet); err == nil {
		t.Fatal("expired packet verified")
	}
}

func TestPacketBudgetsRejectBlobContext(t *testing.T) {
	dispatcher := newTestDispatcher(t)
	large := make([]byte, 1100)
	for index := range large {
		large[index] = 'x'
	}
	if _, _, err := dispatcher.StartRoot(PacketRequest{
		TargetAgentID: "workspace-alpha", ScopeKind: "workspace", ScopeID: "alpha",
		Objective: string(large), TTL: time.Hour,
	}); err == nil {
		t.Fatal("oversized objective accepted")
	}
	if _, _, err := dispatcher.StartRoot(PacketRequest{
		TargetAgentID: "workspace-alpha", ScopeKind: "workspace", ScopeID: "alpha",
		Objective: "Read one bounded source.",
		Pointers:  []string{"bcgos://workspace/alpha/"}, TTL: time.Hour,
	}); err == nil {
		t.Fatal("broad workspace-root pointer accepted")
	}
	if _, _, err := dispatcher.StartRoot(PacketRequest{
		TargetAgentID: "workspace-alpha", ScopeKind: "workspace", ScopeID: "alpha",
		Objective: "Read one bounded source.",
		Pointers:  []string{"bcgos://workspace/alpha/dossier/"}, TTL: time.Hour,
	}); err == nil {
		t.Fatal("collection pointer accepted as a specific artifact")
	}
}

func newTestDispatcher(t *testing.T) *Dispatcher {
	t.Helper()
	catalog, err := agentcatalog.ParseFile("../../bundles/base/agents/catalog.json")
	if err != nil {
		t.Fatal(err)
	}
	store, err := agentorchestration.NewStateStore("recovery-cap")
	if err != nil {
		t.Fatal(err)
	}
	grants := []agentorchestration.Authorization{
		{AgentID: "maestro", Role: "hub", ScopeKind: "control", Capability: "maestro-cap"},
		{AgentID: "workspace-alpha", Role: "workspace_agent", Scope: "alpha", ScopeKind: "workspace", Capability: "workspace-alpha-cap"},
		{AgentID: "capability-research", Role: "capability_specialist", Scope: "alpha", ScopeKind: "workspace", Capability: "capability-research-cap"},
		{AgentID: "practice-insurance", Role: "practice_agent", Scope: "insurance", ScopeKind: "practice", Capability: "practice-insurance-cap"},
	}
	adapter, err := agentorchestration.NewAdapter("claude", catalog, grants, store)
	if err != nil {
		t.Fatal(err)
	}
	dispatcher, err := New(adapter, "packet-signing-capability", map[string]string{
		"maestro": "maestro-cap", "workspace-alpha": "workspace-alpha-cap",
		"capability-research": "capability-research-cap", "practice-insurance": "practice-insurance-cap",
	})
	if err != nil {
		t.Fatal(err)
	}
	return dispatcher
}
