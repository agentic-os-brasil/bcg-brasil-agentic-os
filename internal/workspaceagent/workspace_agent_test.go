package workspaceagent

import (
	"errors"
	"os"
	"path/filepath"
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
