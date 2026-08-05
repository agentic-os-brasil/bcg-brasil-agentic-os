package agentdispatch

import (
	"strings"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/agentcatalog"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/agentorchestration"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/runtimeprojection"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/skillpolicy"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/skillsindex"
)

func TestDispatcherIssuesOneRootPacketAndRejectsNestedDelegation(t *testing.T) {
	dispatcher := newTestDispatcher(t)
	root, decision, err := dispatcher.StartRoot(PacketRequest{
		TargetAgentID: "case-agent-alpha", ScopeKind: "case", ScopeID: "alpha",
		Objective: "Execute the bounded case task.", Pointers: []string{"bcgos://case/alpha/input.md"}, TTL: time.Hour,
	})
	if err != nil || !decision.Allowed {
		t.Fatalf("root dispatch failed: %#v %#v %v", root, decision, err)
	}
	if root.IssuerAgentID != "maestro" || root.ParentPacketID != "" {
		t.Fatalf("unexpected root packet: %#v", root)
	}
	if _, decision, err := dispatcher.StartChild(root, PacketRequest{TargetAgentID: "walter", ScopeKind: "case", ScopeID: "alpha", Objective: "Attempt a direct handoff.", TTL: time.Hour}); err == nil || decision.Allowed {
		t.Fatalf("nested packet was accepted: %#v %v", decision, err)
	}
	if decision := dispatcher.FinishRoot(root); !decision.Allowed {
		t.Fatalf("root did not finish: %#v", decision)
	}
}

func TestDispatcherKeepsDirectSkillsInsideCaseAgent(t *testing.T) {
	dispatcher := newTestDispatcher(t)
	if err := dispatcher.SelectDirectSkill(WorkPacket{}, "case-agent-alpha", "case-cap", "deck-storyline"); err == nil {
		t.Fatal("inactive Case selected a skill")
	}
	root, decision, err := dispatcher.StartRoot(PacketRequest{TargetAgentID: "case-agent-alpha", ScopeKind: "case", ScopeID: "alpha", Objective: "Prepare the bounded case analysis.", TTL: time.Hour})
	if err != nil || !decision.Allowed {
		t.Fatalf("root dispatch failed: %#v %v", decision, err)
	}
	if err := dispatcher.SelectDirectSkill(root, "case-agent-alpha", "case-cap", "deck-storyline"); err != nil {
		t.Fatal(err)
	}
	if err := dispatcher.SelectDirectSkill(root, "walter", "walter-cap", "deck-storyline"); err == nil {
		t.Fatal("Walter selected a Case skill")
	}
	if err := dispatcher.SelectDirectSkill(root, "case-agent-alpha", "forged", "deck-storyline"); err == nil {
		t.Fatal("forged capability selected a skill")
	}
	if decision := dispatcher.FinishRoot(root); !decision.Allowed {
		t.Fatalf("root did not finish: %#v", decision)
	}
}

func TestDispatcherEnforcesConfirmedTrackSelection(t *testing.T) {
	dataDispatcher := newTrackDispatcher(t, []string{"data-science"})
	root, decision, err := dataDispatcher.StartRoot(PacketRequest{TargetAgentID: "case-agent-alpha", ScopeKind: "case", ScopeID: "alpha", Objective: "Evaluate the bounded data-science result.", TTL: time.Hour})
	if err != nil || !decision.Allowed {
		t.Fatalf("data root dispatch failed: %#v %v", decision, err)
	}
	for _, skillID := range []string{"data-science-evaluation", "test-and-evidence"} {
		if err := dataDispatcher.SelectDirectSkill(root, "case-agent-alpha", "case-cap", skillID); err != nil {
			t.Fatalf("selected or dependency skill %q was denied: %v", skillID, err)
		}
	}

	engineeringDispatcher := newTrackDispatcher(t, []string{"software-engineering"})
	root, decision, err = engineeringDispatcher.StartRoot(PacketRequest{TargetAgentID: "case-agent-alpha", ScopeKind: "case", ScopeID: "alpha", Objective: "Implement the bounded software change.", TTL: time.Hour})
	if err != nil || !decision.Allowed {
		t.Fatalf("engineering root dispatch failed: %#v %v", decision, err)
	}
	if err := engineeringDispatcher.SelectDirectSkill(root, "case-agent-alpha", "case-cap", "data-science-evaluation"); err != nil {
		t.Fatalf("unified Tech Core data skill was denied for a technical track: %v", err)
	}
}

func TestDispatcherRejectsTamperingExpiryAndUnboundedPointers(t *testing.T) {
	dispatcher := newTestDispatcher(t)
	now := time.Date(2026, 7, 25, 15, 0, 0, 0, time.UTC)
	dispatcher.now = func() time.Time { return now }
	if _, _, err := dispatcher.StartRoot(PacketRequest{TargetAgentID: "case-agent-alpha", ScopeKind: "case", ScopeID: "alpha", Objective: "Read outside scope.", Pointers: []string{"bcgos://account/account-alpha/secret.md"}, TTL: time.Hour}); err == nil {
		t.Fatal("cross-scope packet accepted")
	}
	packet, decision, err := dispatcher.StartRoot(PacketRequest{TargetAgentID: "case-agent-alpha", ScopeKind: "case", ScopeID: "alpha", Objective: "Summarize one artifact.", Pointers: []string{"bcgos://case/alpha/input.md"}, TTL: time.Hour})
	if err != nil || !decision.Allowed {
		t.Fatalf("root dispatch failed: %#v %v", decision, err)
	}
	tampered := packet
	tampered.Objective = "Read every case."
	if err := dispatcher.Verify(tampered); err == nil {
		t.Fatal("tampered packet verified")
	}
	now = now.Add(2 * time.Hour)
	if err := dispatcher.Verify(packet); err == nil {
		t.Fatal("expired packet verified")
	}
}

func TestDispatcherBoundsPacketAndSkillFields(t *testing.T) {
	dispatcher := newTestDispatcher(t)
	large := strings.Repeat("x", 1100)
	if _, _, err := dispatcher.StartRoot(PacketRequest{TargetAgentID: "case-agent-alpha", ScopeKind: "case", ScopeID: "alpha", Objective: large, TTL: time.Hour}); err == nil {
		t.Fatal("oversized objective accepted")
	}
	if _, _, err := dispatcher.StartRoot(PacketRequest{TargetAgentID: "case-agent-alpha", ScopeKind: "case", ScopeID: "alpha", Objective: "invalid delegated skill", SkillID: "deck-storyline", TTL: time.Hour}); err == nil {
		t.Fatal("skill selection crossed packet boundary")
	}
}

func newTestDispatcher(t *testing.T) *Dispatcher {
	t.Helper()
	catalog, err := agentcatalog.ParseFile("../../bundles/base/agents/catalog.json")
	if err != nil {
		t.Fatal(err)
	}
	policy, err := skillpolicy.ParseFile("../../bundles/base/skills/agent-skill-policy.json")
	if err != nil {
		t.Fatal(err)
	}
	skills, err := skillsindex.Build("../../bundles/base/skills")
	if err != nil {
		t.Fatal(err)
	}
	registry, err := skillpolicy.Compile(policy, skills, catalog)
	if err != nil {
		t.Fatal(err)
	}
	store, err := agentorchestration.NewStateStore("recovery-cap")
	if err != nil {
		t.Fatal(err)
	}
	grants := []agentorchestration.Authorization{
		{AgentID: "maestro", Role: "hub", ScopeKind: "control", Capability: "maestro-cap"},
		{AgentID: "case-agent-alpha", Role: "case_agent", Scope: "alpha", ScopeKind: "case", Capability: "case-cap"},
		{AgentID: "walter", Role: "reviewer", Scope: "review", ScopeKind: "review", Capability: "walter-cap"},
	}
	adapter, err := agentorchestration.NewAdapter("claude", catalog, grants, store)
	if err != nil {
		t.Fatal(err)
	}
	return mustNew(adapter, registry, t)
}

func newTrackDispatcher(t *testing.T, tracks []string) *Dispatcher {
	t.Helper()
	catalog, err := agentcatalog.ParseFile("../../bundles/base/agents/catalog.json")
	if err != nil {
		t.Fatal(err)
	}
	policy, skills, err := runtimeprojection.PolicyForTracks(tracks)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := skillpolicy.Compile(policy, skills, catalog)
	if err != nil {
		t.Fatal(err)
	}
	store, err := agentorchestration.NewStateStore("recovery-cap")
	if err != nil {
		t.Fatal(err)
	}
	grants := []agentorchestration.Authorization{
		{AgentID: "maestro", Role: "hub", ScopeKind: "control", Capability: "maestro-cap"},
		{AgentID: "case-agent-alpha", Role: "case_agent", Scope: "alpha", ScopeKind: "case", Capability: "case-cap"},
	}
	adapter, err := agentorchestration.NewAdapter("claude", catalog, grants, store)
	if err != nil {
		t.Fatal(err)
	}
	return mustNew(adapter, registry, t)
}

func mustNew(adapter *agentorchestration.Adapter, registry skillpolicy.Registry, t *testing.T) *Dispatcher {
	t.Helper()
	dispatcher, err := New(adapter, "packet-signing-capability", map[string]string{
		"maestro": "maestro-cap", "case-agent-alpha": "case-cap", "walter": "walter-cap",
	}, registry)
	if err != nil {
		t.Fatal(err)
	}
	return dispatcher
}
