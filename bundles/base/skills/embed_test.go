package baseskills

import (
	"strings"
	"testing"

	baseruntime "github.com/agentic-os-brasil/bcg-brasil-agentic-os/bundles/base/runtime"
)

func TestGuidedOwnerSkillsCarryExactResumableReviewCommands(t *testing.T) {
	for _, test := range []struct {
		id       string
		required []string
	}{
		{
			id: "maestro-onboarding",
			required: []string{
				"Maestro Onboarding",
				"owner onboarding status",
				"owner onboarding select --track",
				"owner onboarding confirm --digest",
				"interaction-profile",
			},
		},
		{
			id: "agent-identity-setup",
			required: []string{
				"interaction-profile",
				"data/profile/agents.json",
				"owner_id",
			},
		},
	} {
		t.Run(test.id, func(t *testing.T) {
			body, err := Skill(test.id)
			if err != nil {
				t.Fatal(err)
			}
			for _, required := range test.required {
				if !strings.Contains(string(body), required) {
					t.Fatalf("distributed skill %s is missing resumable command contract %q", test.id, required)
				}
			}
		})
	}
}

func TestEnvironmentSkillsCarryTheFriendlyConsolidationContract(t *testing.T) {
	for _, test := range []struct {
		id       string
		required []string
	}{
		{
			id: "maestro-environment-setup",
			required: []string{
				"setup apply",
				"Darwin's user-level maintenance state",
				"verified MarkItDown runtime pack",
				"Do not install MarkItDown from `pip`",
			},
		},
		{
			id: "maestro-runtime-checkup",
			required: []string{
				"LaunchAgent binding",
				"Until a versioned managed pack ships",
				"Pronto para trabalhar",
			},
		},
		{
			id: "account-case-setup",
			required: []string{
				"Client Account Agent → Case Agent",
				"partner-like steward",
				"PA Experts",
			},
		},
	} {
		t.Run(test.id, func(t *testing.T) {
			body, err := Skill(test.id)
			if err != nil {
				t.Fatal(err)
			}
			for _, required := range test.required {
				if !strings.Contains(string(body), required) {
					t.Fatalf("distributed skill %s is missing %q", test.id, required)
				}
			}
		})
	}
}

func TestEnvironmentSkillsKeepMarkItDownOptionalUntilTheManagedPackShips(t *testing.T) {
	manifest, err := baseruntime.Manifest()
	if err != nil {
		t.Fatal(err)
	}
	for _, capability := range manifest.Capabilities {
		if capability.ID != "local_ingestion_markitdown" {
			continue
		}
		for runtime, state := range capability.Runtimes {
			if state.State != "unavailable" {
				t.Fatalf("MarkItDown state for %s = %q, want unavailable until a managed pack ships", runtime, state.State)
			}
		}
		return
	}
	t.Fatal("MarkItDown capability is missing")
}
