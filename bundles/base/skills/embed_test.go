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
				"owner expand status",
				"owner expand next",
				"owner expand draft --question-token",
				"owner expand review --id",
				"owner expand confirm --id",
				"The `question_token` already binds the facet",
				"A SELF expansion draft uses `owner expand confirm`",
			},
		},
		{
			id: "agent-identity-setup",
			required: []string{
				"open_draft_id",
				"bcgos agent personalize review --id",
				"bcgos agent personalize confirm --id",
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

func TestBCGOSOperatorCarriesTheInstalledOperationalLoop(t *testing.T) {
	body, err := Skill("bcgos-operator")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"Resolve the exact installed CLI",
		"Runtime owns normal work",
		"Inspect before acting",
		"Control-plane intent map",
		"Verify the outcome",
		"Recover without guessing",
		"status <workspace>",
		"doctor <workspace>",
		"setup status --workspace <workspace>",
		"update --check",
		"brain/tasks/",
	} {
		if !strings.Contains(string(body), required) {
			t.Fatalf("bcgos operator is missing %q", required)
		}
	}
	for _, internal := range []string{"work next --active", "prior-work source status", "agent status --id", "adapter status --runtime"} {
		if strings.Contains(string(body), internal) {
			t.Fatalf("bcgos operator exposes compatibility command %q as normal workflow", internal)
		}
	}
}
