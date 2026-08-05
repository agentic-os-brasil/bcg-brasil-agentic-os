package workspaceagent

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestInitializeCreatesCompactAgentControlPlane(t *testing.T) {
	root := t.TempDir()
	status, err := Initialize(root, "ws-123")
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	if status.AgentID != "workspace-agent-ws-123" || !status.Initialized || !status.State.Available || !status.Dossier.Available {
		t.Fatalf("Initialize() = %#v", status)
	}
	for _, relative := range []string{
		"workspaces/ws-123/agent/agent.json",
		"workspaces/ws-123/agent/state.json",
		"workspaces/ws-123/dossier/README.md",
	} {
		if _, err := os.Stat(filepath.Join(root, relative)); err != nil {
			t.Fatalf("expected %s: %v", relative, err)
		}
	}
}

func TestInspectRequiresAllProtectedDependencies(t *testing.T) {
	root := t.TempDir()
	if _, err := Initialize(root, "ws-123"); err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(root, "workspaces", "ws-123", "agent", "state.json")
	if err := os.Remove(statePath); err != nil {
		t.Fatal(err)
	}
	status, err := Inspect(root, "ws-123")
	if err != nil {
		t.Fatal(err)
	}
	if status.Initialized || status.State.Available {
		t.Fatalf("incomplete workspace agent reported initialized: %#v", status)
	}
}

func TestRegistryRejectsPermissiveAndTrailingJSON(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits are not enforced on Windows")
	}
	root := t.TempDir()
	if _, err := Initialize(root, "ws-123"); err != nil {
		t.Fatal(err)
	}
	registryPath := filepath.Join(root, "workspaces", "ws-123", "agent", "agent.json")
	if err := os.Chmod(registryPath, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Inspect(root, "ws-123"); err == nil {
		t.Fatal("permissive workspace agent registry was accepted")
	}
	if err := os.Chmod(registryPath, 0o600); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(registryPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(registryPath, append(body, []byte("{}\n")...), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Inspect(root, "ws-123"); err == nil {
		t.Fatal("trailing workspace agent JSON was accepted")
	}
}

func TestInterviewAndResearchApprovalPreserveDisclosureGuardrail(t *testing.T) {
	interview := ColdStartInterview()
	if interview.Kind != "case_agent_setup" || len(interview.Steps) < 5 {
		t.Fatalf("ColdStartInterview() = %#v", interview)
	}

	plan := ResearchPlan{
		PlanID:      "plan-123",
		WorkspaceID: "ws-123",
		State:       "approved",
		CreatedAt:   time.Now().UTC(),
		ValidUntil:  time.Now().UTC().Add(time.Hour),
		MaxQueries:  1,
		Purpose:     "understand public market conditions",
		QueryThemes: []string{"public market size"},
		Sources:     []string{"official statistics"},
	}
	if !errors.Is(plan.Validate(), ErrResearchApprovalRequired) {
		t.Fatalf("Validate() = %v, want ErrResearchApprovalRequired", plan.Validate())
	}
	plan.Approval = Approval{ApprovedAt: time.Now().UTC(), ApprovedBy: "owner", DisclosureLevel: "public_only"}
	if err := plan.Validate(); err != nil {
		t.Fatalf("approved plan Validate() error = %v", err)
	}
}
