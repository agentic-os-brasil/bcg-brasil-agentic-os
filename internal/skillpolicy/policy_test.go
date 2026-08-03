package skillpolicy_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/agentcatalog"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/skillpolicy"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/skillsindex"
)

func TestPolicyKeepsSkillsAsMethodsWithoutCreatingAuthority(t *testing.T) {
	policy, err := skillpolicy.ParseFile("../../bundles/base/skills/agent-skill-policy.json")
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := skillsindex.Build("../../bundles/base/skills")
	if err != nil {
		t.Fatal(err)
	}
	agents, err := agentcatalog.ParseFile("../../bundles/base/agents/catalog.json")
	if err != nil {
		t.Fatal(err)
	}
	registry, err := skillpolicy.Compile(policy, catalog, agents)
	if err != nil {
		t.Fatal(err)
	}
	if !registry.AllowsDirect("case_agent", "deck-storyline") {
		t.Fatal("Case Agent cannot select its direct deck skill")
	}
	if !registry.AllowsDirect("quality_guardian", "pr-quality-loop") || !registry.AllowsDirect("quality_guardian", "pr-review") {
		t.Fatal("Gamma Guardian cannot select its bounded quality methods")
	}
	if registry.AllowsDirect("quality_guardian", "deck-storyline") {
		t.Fatal("Gamma Guardian gained an unrelated case-deliverable method")
	}
	if registry.AllowsDirect("reviewer", "deck-storyline") {
		t.Fatal("Walter gained direct Case method selection")
	}
	if registry.AllowsDelegated("case_agent", "reviewer", "qualitative-analysis") {
		t.Fatal("agent-to-agent skill delegation was accepted")
	}
}

func TestPolicyRejectsSkillOutsideManagedCatalog(t *testing.T) {
	directory := t.TempDir()
	policyPath := filepath.Join(directory, "policy.json")
	if err := os.WriteFile(policyPath, []byte(`{
  "schema_version": 1,
  "mode": "methods_not_authority",
  "direct": [{"role": "workspace_agent", "skill_ids": ["missing"]}],
  "delegated": []
}`), 0o600); err != nil {
		t.Fatal(err)
	}
	policy, err := skillpolicy.ParseFile(policyPath)
	if err != nil {
		t.Fatal(err)
	}
	catalog := skillsindex.Catalog{SchemaVersion: 1, Skills: []skillsindex.Skill{{
		ID: "known", DisplayName: "Known", Trigger: "Known", DefaultPrompt: "Known", RelativePath: "skills/known/SKILL.md",
	}}}
	agents, err := agentcatalog.ParseFile("../../bundles/base/agents/catalog.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := skillpolicy.Compile(policy, catalog, agents); err == nil {
		t.Fatal("policy accepted an unknown managed skill")
	}
}

func TestPolicyCannotCreateANewAgentEdge(t *testing.T) {
	agents, err := agentcatalog.ParseFile("../../bundles/base/agents/catalog.json")
	if err != nil {
		t.Fatal(err)
	}
	skills := skillsindex.Catalog{SchemaVersion: 1, Skills: []skillsindex.Skill{{
		ID: "known", DisplayName: "Known", Trigger: "Known", DefaultPrompt: "Known", RelativePath: "skills/known/SKILL.md",
	}}}
	policy, err := skillpolicy.Parse(strings.NewReader(`{
  "schema_version": 1,
  "mode": "methods_not_authority",
  "direct": [],
  "delegated": [{"from_role":"case_agent","to_role":"reviewer","skill_ids":["known"]}]
}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := skillpolicy.Compile(policy, skills, agents); err == nil {
		t.Fatal("skill policy created an agent edge outside the canonical graph")
	}
}

func TestPolicyCannotGiveMaestroMethodSelection(t *testing.T) {
	agents, err := agentcatalog.ParseFile("../../bundles/base/agents/catalog.json")
	if err != nil {
		t.Fatal(err)
	}
	skills := skillsindex.Catalog{SchemaVersion: 1, Skills: []skillsindex.Skill{{
		ID: "known", DisplayName: "Known", Trigger: "Known", DefaultPrompt: "Known", RelativePath: "skills/known/SKILL.md",
	}}}
	policy, err := skillpolicy.Parse(strings.NewReader(`{
  "schema_version": 1,
  "mode": "methods_not_authority",
  "direct": [{"role":"hub","skill_ids":["known"]}],
  "delegated": []
}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := skillpolicy.Compile(policy, skills, agents); err == nil {
		t.Fatal("skill policy gave Maestro a direct method-selection role")
	}
}
