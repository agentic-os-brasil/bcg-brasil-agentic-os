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
	if !registry.AllowsDirect("workspace_agent", "deck-storyline") {
		t.Fatal("workspace agent cannot select its direct deck skill")
	}
	if registry.AllowsDirect("capability_specialist", "deck-storyline") {
		t.Fatal("capability specialist gained direct method selection")
	}
	if !registry.AllowsDelegated("workspace_agent", "capability_specialist", "qualitative-analysis") {
		t.Fatal("workspace agent cannot assign the bounded qualitative skill")
	}
	if registry.AllowsDelegated("workspace_agent", "capability_specialist", "unknown-method") {
		t.Fatal("unknown skill was delegable")
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
  "delegated": [{"from_role":"workspace_agent","to_role":"subject_specialist","skill_ids":["known"]}]
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
