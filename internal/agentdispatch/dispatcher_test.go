package agentdispatch

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/agentcatalog"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/agentorchestration"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/skillpolicy"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/skillsindex"
)

func TestDispatcherIssuesSealedSequentialWorkspacePackets(t *testing.T) {
	dispatcher := newTestDispatcher(t)
	now := time.Date(2026, 7, 25, 15, 0, 0, 0, time.UTC)
	dispatcher.now = func() time.Time { return now }

	root, decision, err := dispatcher.StartRoot(PacketRequest{
		TargetAgentID: "workspace-agent-alpha", ScopeKind: "workspace", ScopeID: "alpha",
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
	if root.IssuerAgentID != "maestro" || root.TargetAgentID != "workspace-agent-alpha" || root.ParentPacketID != "" {
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
		SkillID:   "qualitative-analysis",
		TTL:       time.Hour,
	})
	if err != nil || !childDecision.Allowed {
		t.Fatalf("child dispatch failed: packet=%#v decision=%#v err=%v", child, childDecision, err)
	}
	if child.ParentPacketID != root.PacketID || child.IssuerAgentID != "workspace-agent-alpha" {
		t.Fatalf("unexpected child packet: %#v", child)
	}
	if decision := dispatcher.FinishChild(child); !decision.Allowed {
		t.Fatalf("finish child = %#v", decision)
	}
	secondChild, secondDecision, err := dispatcher.StartChild(root, PacketRequest{
		TargetAgentID: "capability-research", ScopeKind: "workspace", ScopeID: "alpha",
		Objective: "Check a second bounded question.",
		Pointers:  []string{"bcgos://workspace/alpha/dossier/research.md"},
		SkillID:   "quantitative-analysis",
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
		TargetAgentID: "workspace-agent-alpha", ScopeKind: "workspace", ScopeID: "alpha",
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
		TargetAgentID: "workspace-agent-alpha", ScopeKind: "workspace", ScopeID: "alpha",
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

func TestDispatcherKeepsSkillSelectionWithTheVerticalOwner(t *testing.T) {
	dispatcher := newTestDispatcher(t)
	if err := dispatcher.SelectDirectSkill("workspace-agent-alpha", "deck-storyline"); err != nil {
		t.Fatal(err)
	}
	if err := dispatcher.SelectDirectSkill("capability-research", "deck-storyline"); err == nil {
		t.Fatal("capability specialist selected a direct skill")
	}

	root, decision, err := dispatcher.StartRoot(PacketRequest{
		TargetAgentID: "workspace-agent-alpha", ScopeKind: "workspace", ScopeID: "alpha",
		Objective: "Prepare the approved case analysis.", TTL: time.Hour,
	})
	if err != nil || !decision.Allowed {
		t.Fatalf("root dispatch failed: %#v %v", decision, err)
	}
	if _, _, err := dispatcher.StartChild(root, PacketRequest{
		TargetAgentID: "capability-research", ScopeKind: "workspace", ScopeID: "alpha",
		Objective: "Synthesize qualitative evidence.", TTL: time.Hour,
	}); err == nil {
		t.Fatal("child delegation without an atomic skill was accepted")
	}
	child, childDecision, err := dispatcher.StartChild(root, PacketRequest{
		TargetAgentID: "capability-research", ScopeKind: "workspace", ScopeID: "alpha",
		Objective: "Synthesize qualitative evidence.", SkillID: "qualitative-analysis", TTL: time.Hour,
	})
	if err != nil || !childDecision.Allowed || child.SkillID != "qualitative-analysis" {
		t.Fatalf("bounded skill delegation failed: %#v %#v %v", child, childDecision, err)
	}
	tampered := child
	tampered.SkillID = "quantitative-analysis"
	if err := dispatcher.Verify(tampered); err == nil {
		t.Fatal("tampered skill selection verified")
	}
}

func TestDispatcherPreservesGovernedSubjectDelegationWithoutTransversalSkill(t *testing.T) {
	dispatcher := newTestDispatcher(t)
	root, decision, err := dispatcher.StartRoot(PacketRequest{
		TargetAgentID: "practice-insurance", ScopeKind: "practice", ScopeID: "insurance",
		Objective: "Review the approved insurance subject canon.", TTL: time.Hour,
	})
	if err != nil || !decision.Allowed {
		t.Fatalf("practice root dispatch failed: %#v %v", decision, err)
	}
	child, childDecision, err := dispatcher.StartChild(root, PacketRequest{
		TargetAgentID: "subject-insurance", ScopeKind: "practice", ScopeID: "insurance",
		Objective: "Pressure-test the subject canon.", TTL: time.Hour,
	})
	if err != nil || !childDecision.Allowed || child.SkillID != "" {
		t.Fatalf("subject delegation failed: packet=%#v decision=%#v err=%v", child, childDecision, err)
	}
	if err := dispatcher.Verify(child); err != nil {
		t.Fatal(err)
	}
	if decision := dispatcher.FinishChild(child); !decision.Allowed {
		t.Fatalf("finish subject child = %#v", decision)
	}
	if decision := dispatcher.FinishRoot(root); !decision.Allowed {
		t.Fatalf("finish practice root = %#v", decision)
	}
}

func TestDispatcherPreservesAccountCapabilityDelegationWithManagedSkill(t *testing.T) {
	dispatcher := newTestDispatcher(t)
	root, decision, err := dispatcher.StartRoot(PacketRequest{
		TargetAgentID: "account-agent-alpha", ScopeKind: "account", ScopeID: "account-alpha",
		Objective: "Prepare the approved account analysis.", TTL: time.Hour,
	})
	if err != nil || !decision.Allowed {
		t.Fatalf("account root dispatch failed: %#v %v", decision, err)
	}
	child, childDecision, err := dispatcher.StartChild(root, PacketRequest{
		TargetAgentID: "capability-account", ScopeKind: "account", ScopeID: "account-alpha",
		Objective: "Synthesize bounded account evidence.", SkillID: "qualitative-analysis", TTL: time.Hour,
	})
	if err != nil || !childDecision.Allowed || child.SkillID != "qualitative-analysis" {
		t.Fatalf("account capability delegation failed: packet=%#v decision=%#v err=%v", child, childDecision, err)
	}
	if decision := dispatcher.FinishChild(child); !decision.Allowed {
		t.Fatalf("finish account child = %#v", decision)
	}
	if decision := dispatcher.FinishRoot(root); !decision.Allowed {
		t.Fatalf("finish account root = %#v", decision)
	}
}

func TestVerticalSkillDelegationConformanceAcrossRuntimes(t *testing.T) {
	body, err := os.ReadFile("../../adapters/conformance/vertical-skill-delegation.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Direct struct {
			AgentID string `json:"agent_id"`
			SkillID string `json:"skill_id"`
			Allowed bool   `json:"allowed"`
		} `json:"direct_selection"`
		Delegated struct {
			Parent  string `json:"parent_agent_id"`
			Target  string `json:"target_agent_id"`
			Kind    string `json:"scope_kind"`
			Scope   string `json:"scope_id"`
			SkillID string `json:"skill_id"`
			Allowed bool   `json:"allowed"`
		} `json:"delegated_selection"`
		Denials []struct {
			Target  string `json:"target_agent_id"`
			Kind    string `json:"scope_kind"`
			Scope   string `json:"scope_id"`
			SkillID string `json:"skill_id"`
		} `json:"denials"`
	}
	if err := json.Unmarshal(body, &fixture); err != nil {
		t.Fatal(err)
	}
	for _, runtime := range []string{"claude", "codex"} {
		t.Run(runtime, func(t *testing.T) {
			dispatcher := newSkillTestDispatcherForRuntime(t, runtime)
			if err := dispatcher.SelectDirectSkill(fixture.Direct.AgentID, fixture.Direct.SkillID); (err == nil) != fixture.Direct.Allowed {
				t.Fatalf("direct selection error = %v", err)
			}
			root, decision, err := dispatcher.StartRoot(PacketRequest{
				TargetAgentID: fixture.Delegated.Parent, ScopeKind: fixture.Delegated.Kind,
				ScopeID: fixture.Delegated.Scope, Objective: "Run bounded conformance.", TTL: time.Hour,
			})
			if err != nil || !decision.Allowed {
				t.Fatalf("root = %#v %v", decision, err)
			}
			_, decision, err = dispatcher.StartChild(root, PacketRequest{
				TargetAgentID: fixture.Delegated.Target, ScopeKind: fixture.Delegated.Kind,
				ScopeID: fixture.Delegated.Scope, Objective: "Run named method.",
				SkillID: fixture.Delegated.SkillID, TTL: time.Hour,
			})
			if err != nil || decision.Allowed != fixture.Delegated.Allowed {
				t.Fatalf("delegated selection = %#v %v", decision, err)
			}
			for _, denial := range fixture.Denials {
				_, decision, err := dispatcher.StartChild(root, PacketRequest{
					TargetAgentID: denial.Target, ScopeKind: denial.Kind, ScopeID: denial.Scope,
					Objective: "Exercise denied method selection.", SkillID: denial.SkillID, TTL: time.Hour,
				})
				if err == nil || decision.Allowed {
					t.Fatalf("denial %q was allowed: %#v %v", denial.SkillID, decision, err)
				}
			}
		})
	}
}

func TestPacketBudgetsRejectBlobContext(t *testing.T) {
	dispatcher := newTestDispatcher(t)
	large := make([]byte, 1100)
	for index := range large {
		large[index] = 'x'
	}
	if _, _, err := dispatcher.StartRoot(PacketRequest{
		TargetAgentID: "workspace-agent-alpha", ScopeKind: "workspace", ScopeID: "alpha",
		Objective: string(large), TTL: time.Hour,
	}); err == nil {
		t.Fatal("oversized objective accepted")
	}
	if _, _, err := dispatcher.StartRoot(PacketRequest{
		TargetAgentID: "workspace-agent-alpha", ScopeKind: "workspace", ScopeID: "alpha",
		Objective: "Read one bounded source.",
		Pointers:  []string{"bcgos://workspace/alpha/"}, TTL: time.Hour,
	}); err == nil {
		t.Fatal("broad workspace-root pointer accepted")
	}
	if _, _, err := dispatcher.StartRoot(PacketRequest{
		TargetAgentID: "workspace-agent-alpha", ScopeKind: "workspace", ScopeID: "alpha",
		Objective: "Read one bounded source.",
		Pointers:  []string{"bcgos://workspace/alpha/dossier/"}, TTL: time.Hour,
	}); err == nil {
		t.Fatal("collection pointer accepted as a specific artifact")
	}
}

func newTestDispatcher(t *testing.T) *Dispatcher {
	return newSkillTestDispatcherForRuntime(t, "claude")
}

func newSkillTestDispatcherForRuntime(t *testing.T, runtime string) *Dispatcher {
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
		{AgentID: "account-agent-alpha", Role: "account_agent", Scope: "account-alpha", ScopeKind: "account", Capability: "account-agent-alpha-cap"},
		{AgentID: "capability-account", Role: "capability_specialist", Scope: "account-alpha", ScopeKind: "account", Capability: "capability-account-cap"},
		{AgentID: "workspace-agent-alpha", Role: "workspace_agent", Scope: "alpha", ScopeKind: "workspace", Capability: "workspace-agent-alpha-cap"},
		{AgentID: "capability-research", Role: "capability_specialist", Scope: "alpha", ScopeKind: "workspace", Capability: "capability-research-cap"},
		{AgentID: "practice-insurance", Role: "practice_agent", Scope: "insurance", ScopeKind: "practice", Capability: "practice-insurance-cap"},
		{AgentID: "subject-insurance", Role: "subject_specialist", Scope: "insurance", ScopeKind: "practice", Capability: "subject-insurance-cap"},
	}
	adapter, err := agentorchestration.NewAdapter(runtime, catalog, grants, store)
	if err != nil {
		t.Fatal(err)
	}
	dispatcher, err := New(adapter, "packet-signing-capability", map[string]string{
		"maestro": "maestro-cap", "account-agent-alpha": "account-agent-alpha-cap", "capability-account": "capability-account-cap",
		"workspace-agent-alpha": "workspace-agent-alpha-cap", "capability-research": "capability-research-cap",
		"practice-insurance": "practice-insurance-cap", "subject-insurance": "subject-insurance-cap",
	}, registry)
	if err != nil {
		t.Fatal(err)
	}
	return dispatcher
}
