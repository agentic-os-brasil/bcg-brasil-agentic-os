package baseskills

import (
	"strings"
	"testing"
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
				"data/profile/onboarding.json",
				"data/profile/identity.json",
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
