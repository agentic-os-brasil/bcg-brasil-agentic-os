package agentcatalog

import "testing"

func TestCatalogAcceptsLeanMaestroCore(t *testing.T) {
	catalog := Catalog{
		SchemaVersion: 1,
		Hub:           "maestro",
		Delegation: DelegationPolicy{
			Mode: "role_gated_chains", RegisteredChains: "governed_unbounded",
			MaxActiveBranches: 1, MaxDepth: 2, MaxChildrenPerAgent: 1,
			MaxErrandHelpers: 1, ErrandScope: "basic_reversible",
			AllowedEdges: []DelegationEdge{
				{FromRole: "account_agent", ToRoles: []string{"capability_specialist"}},
				{FromRole: "hub", ToRoles: []string{"account_agent", "errand_helper", "governance_analyst", "practice_agent", "reviewer", "workspace_agent"}},
				{FromRole: "practice_agent", ToRoles: []string{"subject_specialist"}},
				{FromRole: "workspace_agent", ToRoles: []string{"capability_specialist"}},
			},
		},
		Agents: []Agent{
			{ID: "darwin", Role: "governance_analyst", DirectUserAccess: false, ToolAccess: "none", MayDelegate: false, InputContract: "bounded_health_packet", RelativePath: "agents/darwin/AGENT.md"},
			{ID: "maestro", Role: "hub", DirectUserAccess: true, ToolAccess: "none", MayDelegate: true, InputContract: "session_context_packet", RelativePath: "agents/maestro/AGENT.md"},
			{ID: "walter", Role: "reviewer", DirectUserAccess: false, ToolAccess: "none", MayDelegate: false, InputContract: "sealed_review_packet", RelativePath: "agents/walter/AGENT.md"},
		},
	}
	if err := catalog.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestCatalogAllowsOnlyTheGovernedDepthTwoChains(t *testing.T) {
	catalog := mustTestCatalog(t)
	tests := []struct {
		from, to string
		depth    int
		want     bool
	}{
		{"hub", "workspace_agent", 1, true},
		{"hub", "workspace_agent", 2, false},
		{"workspace_agent", "capability_specialist", 2, true},
		{"workspace_agent", "capability_specialist", 1, false},
		{"hub", "practice_agent", 1, true},
		{"practice_agent", "subject_specialist", 2, true},
		{"practice_agent", "subject_specialist", 1, false},
		{"account_agent", "capability_specialist", 1, false},
		{"hub", "subject_specialist", 1, false},
		{"workspace_agent", "subject_specialist", 2, false},
		{"reviewer", "capability_specialist", 2, false},
		{"practice_agent", "subject_specialist", 3, false},
	}
	for _, test := range tests {
		if got := catalog.AllowsDelegation(test.from, test.to, test.depth); got != test.want {
			t.Errorf("AllowsDelegation(%q, %q, %d) = %v, want %v", test.from, test.to, test.depth, got, test.want)
		}
	}
}

func TestCatalogAcceptsARegisteredPracticeAgentWithOneBoundedChild(t *testing.T) {
	catalog := mustTestCatalog(t)
	practice := Agent{
		ID: "practice-insurance", Role: "practice_agent", DirectUserAccess: false,
		ToolAccess: "none", MayDelegate: true, InputContract: "bounded_practice_packet",
		RelativePath: "agents/practice-insurance/AGENT.md",
	}
	catalog.Agents = append(catalog.Agents, Agent{})
	copy(catalog.Agents[3:], catalog.Agents[2:])
	catalog.Agents[2] = practice
	if err := catalog.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestCatalogRejectsToolsParallelismAndUngovernedChains(t *testing.T) {
	base := mustTestCatalog(t)

	tests := []struct {
		name   string
		mutate func(*Catalog)
	}{
		{"maestro tools", func(value *Catalog) { value.Agents[1].ToolAccess = "scoped" }},
		{"walter tools", func(value *Catalog) { value.Agents[2].ToolAccess = "read" }},
		{"parallel branches", func(value *Catalog) { value.Delegation.MaxActiveBranches = 2 }},
		{"excess depth", func(value *Catalog) { value.Delegation.MaxDepth = 3 }},
		{"multiple children", func(value *Catalog) { value.Delegation.MaxChildrenPerAgent = 2 }},
		{"multiple errands", func(value *Catalog) { value.Delegation.MaxErrandHelpers = 2 }},
		{"darwin delegates", func(value *Catalog) { value.Agents[0].MayDelegate = true }},
		{"reviewer edge", func(value *Catalog) {
			value.Delegation.AllowedEdges = append(value.Delegation.AllowedEdges, DelegationEdge{FromRole: "reviewer", ToRoles: []string{"capability_specialist"}})
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := base
			candidate.Agents = append([]Agent(nil), base.Agents...)
			candidate.Delegation.AllowedEdges = append([]DelegationEdge(nil), base.Delegation.AllowedEdges...)
			test.mutate(&candidate)
			if err := candidate.Validate(); err == nil {
				t.Fatal("expected invalid core contract")
			}
		})
	}
}

func mustTestCatalog(t *testing.T) Catalog {
	t.Helper()
	catalog, err := ParseFile("../../bundles/base/agents/catalog.json")
	if err != nil {
		t.Fatal(err)
	}
	return catalog
}
