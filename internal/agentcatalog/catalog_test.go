package agentcatalog

import "testing"

func TestCatalogAcceptsLeanMaestroCore(t *testing.T) {
	catalog := Catalog{
		SchemaVersion: 1,
		Hub:           "maestro",
		Delegation: DelegationPolicy{
			Mode: "role_gated_chains", RegisteredChains: "sequential_direct_spokes",
			MaxActiveBranches: 1, MaxDepth: 1, MaxChildrenPerAgent: 1,
			MaxErrandHelpers: 1, ErrandScope: "basic_reversible",
			AllowedEdges: []DelegationEdge{
				{FromRole: "hub", ToRoles: []string{"case_agent", "client_account_agent", "errand_helper", "governance_analyst", "pa_expert", "reviewer"}},
			},
		},
		Agents: []Agent{
			{ID: "darwin", Role: "governance_analyst", DirectUserAccess: false, ToolAccess: "scoped", MayDelegate: false, InputContract: "bounded_health_packet", RelativePath: "agents/darwin/AGENT.md"},
			{ID: "maestro", Role: "hub", DirectUserAccess: true, ToolAccess: "none", MayDelegate: true, InputContract: "session_context_packet", RelativePath: "agents/maestro/AGENT.md"},
			{ID: "walter", Role: "reviewer", DirectUserAccess: false, ToolAccess: "none", MayDelegate: false, InputContract: "sealed_review_packet", RelativePath: "agents/walter/AGENT.md", DefaultEmoji: "🦉"},
		},
	}
	if err := catalog.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, agent := range catalog.Agents {
		if agent.ID == "walter" && agent.DefaultEmoji != "🦉" {
			t.Fatalf("catalog Walter emoji = %q, want owl", agent.DefaultEmoji)
		}
	}
}

func TestCatalogAllowsOnlySequentialDirectSpokes(t *testing.T) {
	catalog := mustTestCatalog(t)
	tests := []struct {
		from, to string
		depth    int
		want     bool
	}{
		{"hub", "workspace_agent", 1, true},
		{"hub", "workspace_agent", 2, false},
		{"workspace_agent", "capability_specialist", 1, false},
		{"account_agent", "capability_specialist", 1, false},
		{"hub", "subject_specialist", 1, false},
		{"workspace_agent", "subject_specialist", 1, false},
		{"reviewer", "capability_specialist", 1, false},
		{"practice_agent", "subject_specialist", 1, false},
		{"hub", "client_account_agent", 1, true},
		{"hub", "case_agent", 1, true},
		{"client_account_agent", "case_agent", 1, false},
		{"hub", "pa_expert", 1, true},
		{"case_agent", "pa_expert", 1, false},
	}
	for _, test := range tests {
		if got := catalog.AllowsDelegation(test.from, test.to, test.depth); got != test.want {
			t.Errorf("AllowsDelegation(%q, %q, %d) = %v, want %v", test.from, test.to, test.depth, got, test.want)
		}
	}
}

func TestCatalogRejectsRetiredPracticeAuthority(t *testing.T) {
	catalog := mustTestCatalog(t)
	if err := catalog.RejectLegacyRegistration("practice-agent-insurance", "practice_agent"); err == nil {
		t.Fatal("legacy practice role and ID were accepted")
	}
	if catalog.AllowsDelegation("hub", "practice_agent", 1) || catalog.AllowsDelegation("practice_agent", "subject_specialist", 2) {
		t.Fatal("deprecated practice graph remains routable")
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
		{"excess depth", func(value *Catalog) { value.Delegation.MaxDepth = 2 }},
		{"multiple children", func(value *Catalog) { value.Delegation.MaxChildrenPerAgent = 2 }},
		{"multiple errands", func(value *Catalog) { value.Delegation.MaxErrandHelpers = 2 }},
		{"darwin delegates", func(value *Catalog) { value.Agents[0].MayDelegate = true }},
		{"reviewer edge", func(value *Catalog) {
			value.Delegation.AllowedEdges = append(value.Delegation.AllowedEdges, DelegationEdge{FromRole: "reviewer", ToRoles: []string{"case_agent"}})
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

func TestCatalogRejectsUnsafeIDsAndRoleContractDrift(t *testing.T) {
	base := mustTestCatalog(t)

	tests := []struct {
		name   string
		mutate func(*Catalog)
	}{
		{"path traversal id", func(value *Catalog) {
			value.Agents[0].ID = "../darwin"
			value.Agents[0].RelativePath = "agents/../darwin/AGENT.md"
		}},
		{"unknown role", func(value *Catalog) {
			value.Agents[0].Role = "general_assistant"
		}},
		{"practice reads raw workspace", func(value *Catalog) {
			value.Agents = append(value.Agents, Agent{})
			copy(value.Agents[3:], value.Agents[2:])
			value.Agents[2] = Agent{
				ID: "practice-insurance", Role: "practice_agent",
				ToolAccess: "none", MayDelegate: true,
				InputContract: "raw_workspace_context",
				RelativePath:  "agents/practice-insurance/AGENT.md",
			}
		}},
		{"second reviewer has tools", func(value *Catalog) {
			value.Agents = append(value.Agents, Agent{
				ID: "walter-shadow", Role: "reviewer",
				ToolAccess: "scoped", InputContract: "sealed_review_packet",
				RelativePath: "agents/walter-shadow/AGENT.md",
			})
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := base
			candidate.Agents = append([]Agent(nil), base.Agents...)
			candidate.Delegation.AllowedEdges = append([]DelegationEdge(nil), base.Delegation.AllowedEdges...)
			test.mutate(&candidate)
			if err := candidate.Validate(); err == nil {
				t.Fatal("expected role or path contract rejection")
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
