package skillrouting

import (
	"reflect"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/skillpolicy"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/skillsindex"
)

func TestRouteSelectsAtMostTwoInstalledPolicyAllowedSkills(t *testing.T) {
	catalog := testCatalog()
	policy := skillpolicy.Policy{SchemaVersion: 1, Mode: "methods_not_authority", Direct: []skillpolicy.DirectRule{{Role: "case_agent", SkillIDs: []string{"deck-review", "meeting-close", "pr-review"}}}}
	installed := []InstalledSkill{
		{ID: "deck-review", Pointer: ".codex/skills/deck-review/SKILL.md"},
		{ID: "meeting-close", Pointer: ".codex/skills/meeting-close/SKILL.md"},
		{ID: "pr-review", Pointer: ".codex/skills/pr-review/SKILL.md"},
	}

	got, err := Route(Request{
		Prompt:    "Use $pr-review and $deck-review; then $meeting-close.",
		Role:      "case_agent",
		Catalog:   catalog,
		Policy:    policy,
		Installed: installed,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []Selection{
		{ID: "pr-review", Reason: "explicit_skill_reference", Pointer: ".codex/skills/pr-review/SKILL.md"},
		{ID: "deck-review", Reason: "explicit_skill_reference", Pointer: ".codex/skills/deck-review/SKILL.md"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Route() = %#v, want %#v", got, want)
	}
}

func TestRouteUsesLexicalIntentWithoutLeakingPromptText(t *testing.T) {
	catalog := testCatalog()
	policy := skillpolicy.Policy{SchemaVersion: 1, Mode: "methods_not_authority", Direct: []skillpolicy.DirectRule{{Role: "case_agent", SkillIDs: []string{"deck-review", "meeting-close", "pr-review"}}}}
	got, err := Route(Request{
		Prompt:  "Please review this pull request for evidence and risk.",
		Role:    "case_agent",
		Catalog: catalog,
		Policy:  policy,
		Installed: []InstalledSkill{
			{ID: "deck-review", Pointer: ".claude/skills/deck-review/SKILL.md"},
			{ID: "pr-review", Pointer: ".claude/skills/pr-review/SKILL.md"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "pr-review" || got[0].Reason != "lexical_intent" || got[0].Pointer != ".claude/skills/pr-review/SKILL.md" {
		t.Fatalf("Route() = %#v", got)
	}
}

func TestRouteUnknownOrDisallowedIntentSelectsNothing(t *testing.T) {
	catalog := testCatalog()
	policy := skillpolicy.Policy{SchemaVersion: 1, Mode: "methods_not_authority", Direct: []skillpolicy.DirectRule{{Role: "case_agent", SkillIDs: []string{"meeting-close"}}}}
	installed := []InstalledSkill{{ID: "pr-review", Pointer: ".codex/skills/pr-review/SKILL.md"}}
	for _, prompt := range []string{"What is the weather?", "Use $pr-review for this change."} {
		got, err := Route(Request{Prompt: prompt, Role: "case_agent", Catalog: catalog, Policy: policy, Installed: installed})
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 0 {
			t.Fatalf("Route(%q) = %#v, want no selection", prompt, got)
		}
	}
}

func testCatalog() skillsindex.Catalog {
	return skillsindex.Catalog{SchemaVersion: 1, Skills: []skillsindex.Skill{
		{ID: "deck-review", DisplayName: "Deck Review", Trigger: "Review supplied slide text for storyline and evidence risks", DefaultPrompt: "Use $deck-review to review the supplied slide text.", RelativePath: "skills/deck-review/SKILL.md"},
		{ID: "meeting-close", DisplayName: "Meeting Close", Trigger: "Turn meeting notes into a reviewable closure packet", DefaultPrompt: "Use $meeting-close to close the meeting.", RelativePath: "skills/meeting-close/SKILL.md"},
		{ID: "pr-review", DisplayName: "Pull Request Review", Trigger: "Review a pull request for risk gates and evidence", DefaultPrompt: "Use $pr-review to review this pull request.", RelativePath: "skills/pr-review/SKILL.md"},
	}}
}
