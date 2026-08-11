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
