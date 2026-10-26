package skillrouting

import (
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/skillpolicy"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/skillsindex"
)

func TestRouteExplicitOnlyMethodRequiresReference(t *testing.T) {
	catalog := skillsindex.Catalog{SchemaVersion: 1, Skills: []skillsindex.Skill{
		{ID: "pocock-implement", DisplayName: "Implement", Trigger: "Implement software with tests", DefaultPrompt: "Use $pocock-implement", RelativePath: "skills/pocock-implement/SKILL.md", UserInvokedOnly: true},
	}}
	request := Request{
		Role: "case_agent", Catalog: catalog,
		Policy:    skillpolicy.Policy{SchemaVersion: 1, Mode: "methods_not_authority", Direct: []skillpolicy.DirectRule{{Role: "case_agent", SkillIDs: []string{"pocock-implement"}}}},
		Installed: []InstalledSkill{{ID: "pocock-implement", Pointer: ".claude/skills/pocock-implement/SKILL.md"}},
	}
	for prompt, count := range map[string]int{
		"Implement software with tests":                          0,
		"Use $pocock-implement for this change":                  1,
		"/pocock-implement for this change":                      1,
		"Use `$pocock-implement` for this change":                1,
		"Read https://example.test/pocock-implement for context": 0,
		"Read docs/pocock-implement/SKILL.md":                    0,
		"Read /pocock-implement/SKILL.md":                        0,
		"Read ./pocock-implement":                                0,
		"Read https://pocock-implement.example":                  0,
	} {
		request.Prompt = prompt
		got, err := Route(request)
		if err != nil || len(got) != count {
			t.Fatalf("Route(%q)=%v, %v; want %d selections", prompt, got, err, count)
		}
		if len(got) > 0 && got[0].Reason != "explicit_skill_reference" {
			t.Fatalf("manual method auto-selected: %+v", got)
		}
	}
}
