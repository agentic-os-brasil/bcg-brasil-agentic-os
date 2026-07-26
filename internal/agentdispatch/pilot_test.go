package agentdispatch

import (
	"strings"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/agentcatalog"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/agentorchestration"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/agentscaffold"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspaceagent"
)

func TestPilotDelegatesWorkspaceIntentAndReturnsEvidenceForBothRuntimes(t *testing.T) {
	for _, runtimeName := range []string{"claude", "codex"} {
		t.Run(runtimeName, func(t *testing.T) {
			pilot := newTestPilot(t, runtimeName)
			now := time.Date(2026, 7, 25, 18, 0, 0, 0, time.UTC)
			pilot.dispatcher.now = func() time.Time { return now }

			receipt, err := pilot.Delegate(Intent{
				WorkspaceID: "alpha",
				Objective:   "Assess the approved research before the steering discussion.",
				Pointers:    []string{"bcgos://workspace/alpha/dossier/research.md"},
				Constraints: []string{"Return evidence and uncertainty."},
				TTL:         time.Hour,
			})
			if err != nil {
				t.Fatal(err)
			}
			if receipt.State != StateDelegated || receipt.OwnerAgentID != "maestro" || receipt.TargetAgentID != "workspace-agent-alpha" || receipt.Packet.PacketID == "" {
				t.Fatalf("unexpected delegation receipt: %#v", receipt)
			}

			result, err := pilot.Return(receipt.Packet, Return{
				Summary:      "Two sources support the current hypothesis; the market-size input remains stale.",
				EvidenceRefs: []string{"bcgos://workspace/alpha/dossier/evidence/source-a.json"},
				Uncertainty:  "The source refresh date is outside the requested period.",
			})
			if err != nil {
				t.Fatal(err)
			}
			if result.State != StateCompleted || result.Result.Summary == "" || len(result.Result.EvidenceRefs) != 1 || result.Result.EvidenceRefs[0] != "bcgos://workspace/alpha/dossier/evidence/source-a.json" {
				t.Fatalf("unexpected completed receipt: %#v", result)
			}
			if decision := pilot.dispatcher.FinishRoot(receipt.Packet); decision.Allowed || decision.Code != "branch_missing" {
				t.Fatalf("completed pilot delegation can be replayed: %#v", decision)
			}
		})
	}
}

func TestPilotDelegatesOnlyOneBasicReversibleErrandForBothRuntimes(t *testing.T) {
	for _, runtimeName := range []string{"claude", "codex"} {
		t.Run(runtimeName, func(t *testing.T) {
			pilot := newTestPilot(t, runtimeName)
			receipt, err := pilot.DelegateErrand(ErrandIntent{
				ErrandID: "pilot", Objective: "Collect the approved meeting link.", Reversible: true,
				Pointers: []string{"bcgos://errand/pilot/meeting-link.txt"}, TTL: time.Hour,
			})
			if err != nil || receipt.State != StateDelegated || receipt.TargetAgentID != "errand-helper" || receipt.OwnerAgentID != "maestro" {
				t.Fatalf("errand delegation = %#v, %v", receipt, err)
			}
			completed, err := pilot.Return(receipt.Packet, Return{
				Summary:      "The approved meeting link was collected.",
				EvidenceRefs: []string{"bcgos://errand/pilot/meeting-link.txt"},
			})
			if err != nil || completed.State != StateCompleted {
				t.Fatalf("errand return = %#v, %v", completed, err)
			}
		})
	}
}

func TestPilotRejectsMultipleErrandHelpers(t *testing.T) {
	dispatcher := newTestDispatcherForRuntime(t, "claude")
	if _, err := NewPilot(dispatcher, []Instance{
		{AgentID: "errand-helper", Role: "errand_helper", ScopeKind: "errand", ScopeID: "pilot", ParentAgentID: "maestro", Available: true},
		{AgentID: "errand-helper-2", Role: "errand_helper", ScopeKind: "errand", ScopeID: "pilot", ParentAgentID: "maestro", Available: true},
	}); err == nil || !strings.Contains(err.Error(), "one errand helper") {
		t.Fatalf("multiple helpers accepted: %v", err)
	}
}

func TestPilotRejectsNonReversibleErrandBeforeDispatch(t *testing.T) {
	pilot := newTestPilot(t, "claude")
	receipt, err := pilot.DelegateErrand(ErrandIntent{
		ErrandID: "pilot", Objective: "Change the production configuration.", Reversible: false, TTL: time.Hour,
	})
	if err == nil || receipt.State != StateFailed || receipt.Failure.Code != "intent_invalid" {
		t.Fatalf("non-reversible errand = %#v, %v", receipt, err)
	}
	if snapshot := pilot.dispatcher.gate.Snapshot(); snapshot.BranchID != "" {
		t.Fatalf("non-reversible errand opened a branch: %#v", snapshot)
	}
}

func TestPilotReportsUnavailableTargetWithoutOpeningBranch(t *testing.T) {
	pilot := newTestPilot(t, "claude")
	instance := pilot.instances["workspace-agent-alpha"]
	instance.Available = false
	pilot.instances["workspace-agent-alpha"] = instance

	receipt, err := pilot.Delegate(Intent{WorkspaceID: "alpha", Objective: "Assess one bounded source.", TTL: time.Hour})
	if err == nil || receipt.State != StateUnavailable || receipt.Failure.Code != "target_unavailable" || !strings.Contains(receipt.Failure.Reason, "not available") {
		t.Fatalf("unavailable delegation = %#v, %v", receipt, err)
	}
	if snapshot := pilot.dispatcher.gate.Snapshot(); snapshot.BranchID != "" {
		t.Fatalf("unavailable target opened a branch: %#v", snapshot)
	}
}

func TestPilotSelectsTheRegisteredWorkspaceAgentInstance(t *testing.T) {
	dataRoot := t.TempDir()
	if _, err := workspaceagent.Initialize(dataRoot, "alpha"); err != nil {
		t.Fatal(err)
	}
	status, err := agentscaffold.Scaffold(dataRoot, agentscaffold.WorkspaceRequest("alpha"))
	if err != nil {
		t.Fatal(err)
	}
	instance := InstanceFromScaffold(status)
	if !instance.Available || instance.AgentID != "workspace-agent-alpha" || instance.ParentAgentID != "maestro" {
		t.Fatalf("scaffold projection is not a permitted workspace root: %#v", instance)
	}
	pilot, err := NewPilot(newTestDispatcherForRuntime(t, "claude"), []Instance{instance})
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := pilot.Delegate(Intent{
		WorkspaceID: "alpha", Objective: "Assess one bounded source.",
		Pointers: []string{"bcgos://workspace/alpha/dossier/brief.json"}, TTL: time.Hour,
	})
	if err != nil || receipt.TargetAgentID != instance.AgentID || receipt.State != StateDelegated {
		t.Fatalf("registered workspace delegation = %#v, %v", receipt, err)
	}
	completed, err := pilot.Return(receipt.Packet, Return{
		Summary:      "The registered workspace agent returned a bounded finding.",
		EvidenceRefs: []string{"bcgos://workspace/alpha/dossier/evidence/brief-source.json"},
	})
	if err != nil || completed.State != StateCompleted {
		t.Fatalf("registered workspace return = %#v, %v", completed, err)
	}
}

func TestPilotRecordsDispatchAndReturnFailuresExplicitly(t *testing.T) {
	pilot := newTestPilot(t, "claude")
	receipt, err := pilot.Delegate(Intent{WorkspaceID: "alpha", Objective: "Assess one bounded source.", TTL: time.Hour})
	if err != nil {
		t.Fatal(err)
	}

	wrong := receipt.Packet
	wrong.TargetAgentID = "practice-insurance"
	failed, err := pilot.Return(wrong, Return{Summary: "No evidence."})
	if err == nil || failed.State != StateFailed || failed.Failure.Code != "packet_invalid" {
		t.Fatalf("invalid return = %#v, %v", failed, err)
	}
	current, ok := pilot.Inspect(receipt.DelegationID)
	if !ok || current.State != StateDelegated {
		t.Fatalf("invalid return overwrote active delegation: %#v, present=%t", current, ok)
	}

	failed, err = pilot.Fail(receipt.Packet, Failure{Code: "runtime_disconnected", Reason: "The delegated runtime stopped before returning a result."})
	if err != nil || failed.State != StateFailed || failed.Failure.Code != "runtime_disconnected" {
		t.Fatalf("explicit failure = %#v, %v", failed, err)
	}
	if snapshot := pilot.dispatcher.gate.Snapshot(); snapshot.BranchID != "" {
		t.Fatalf("failed delegation left an active branch: %#v", snapshot)
	}
}

func newTestPilot(t *testing.T, runtimeName string) *Pilot {
	t.Helper()
	dispatcher := newTestDispatcherForRuntime(t, runtimeName)
	pilot, err := NewPilot(dispatcher, []Instance{
		{AgentID: "workspace-agent-alpha", Role: "workspace_agent", ScopeKind: "workspace", ScopeID: "alpha", ParentAgentID: "maestro", Available: true},
		{AgentID: "errand-helper", Role: "errand_helper", ScopeKind: "errand", ScopeID: "pilot", ParentAgentID: "maestro", Available: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	return pilot
}

func newTestDispatcherForRuntime(t *testing.T, runtimeName string) *Dispatcher {
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
		{AgentID: "workspace-agent-alpha", Role: "workspace_agent", Scope: "alpha", ScopeKind: "workspace", Capability: "workspace-alpha-cap"},
		{AgentID: "errand-helper", Role: "errand_helper", Scope: "pilot", ScopeKind: "errand", Capability: "errand-helper-cap"},
		{AgentID: "capability-research", Role: "capability_specialist", Scope: "alpha", ScopeKind: "workspace", Capability: "capability-research-cap"},
		{AgentID: "practice-insurance", Role: "practice_agent", Scope: "insurance", ScopeKind: "practice", Capability: "practice-insurance-cap"},
	}
	adapter, err := agentorchestration.NewAdapter(runtimeName, catalog, grants, store)
	if err != nil {
		t.Fatal(err)
	}
	dispatcher, err := New(adapter, "packet-signing-capability", map[string]string{
		"maestro": "maestro-cap", "workspace-agent-alpha": "workspace-alpha-cap",
		"errand-helper":       "errand-helper-cap",
		"capability-research": "capability-research-cap", "practice-insurance": "practice-insurance-cap",
	})
	if err != nil {
		t.Fatal(err)
	}
	return dispatcher
}
