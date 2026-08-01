package agentcatalog

import "testing"

func TestCatalogAcceptsMaestroDepthOneQualityLoop(t *testing.T) {
	catalog := mustTestCatalog(t)
	if err := catalog.Validate(); err != nil {
		t.Fatal(err)
	}
	if catalog.Delegation.Mode != "maestro_planner" || catalog.Delegation.RegisteredChains != "bounded_sequential" || catalog.Delegation.MaxDepth != 1 || catalog.Delegation.MaxActiveBranches != 1 || catalog.Delegation.MaxChildrenPerAgent != 0 {
		t.Fatalf("unexpected topology: %#v", catalog.Delegation)
	}
}

func TestCatalogAllowsOnlyMaestroDirectSpokesAtDepthOne(t *testing.T) {
	catalog := mustTestCatalog(t)
	allowed := []string{"case_agent", "client_account_agent", "errand_helper", "governance_analyst", "pa_expert", "reviewer"}
	for _, role := range allowed {
		if !catalog.AllowsDelegation("hub", role, 1) {
			t.Errorf("Maestro cannot open %s", role)
		}
		if catalog.AllowsDelegation("hub", role, 2) || catalog.AllowsDelegation(role, "case_agent", 1) {
			t.Errorf("nested delegation was accepted for %s", role)
		}
	}
	for _, role := range []string{"retired_specialist_role", "practice_agent", "subject_specialist"} {
		if catalog.AllowsDelegation("hub", role, 1) {
			t.Errorf("retired role %q remains routable", role)
		}
	}
}

func TestCatalogRejectsParallelismAndRoleDrift(t *testing.T) {
	base := mustTestCatalog(t)
	tests := []struct {
		name   string
		mutate func(*Catalog)
	}{
		{"parallel branches", func(value *Catalog) { value.Delegation.MaxActiveBranches = 2 }},
		{"excess depth", func(value *Catalog) { value.Delegation.MaxDepth = 2 }},
		{"multiple children", func(value *Catalog) { value.Delegation.MaxChildrenPerAgent = 1 }},
		{"multiple errands", func(value *Catalog) { value.Delegation.MaxErrandHelpers = 2 }},
		{"Maestro tools", func(value *Catalog) { value.Agents[4].ToolAccess = "scoped" }},
		{"Walter tools", func(value *Catalog) { value.Agents[5].ToolAccess = "read" }},
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

func TestCatalogRejectsUnsafeIDsAndRetiredPracticeAuthority(t *testing.T) {
	catalog := mustTestCatalog(t)
	if err := catalog.RejectLegacyRegistration("practice-agent-insurance", "practice_agent"); err == nil {
		t.Fatal("legacy practice role and ID were accepted")
	}
	if err := catalog.RejectLegacyRegistration("subject-alpha", "subject_specialist"); err == nil {
		t.Fatal("retired specialist identity was accepted")
	}
	if ValidAgentID("../case") || !ValidAgentID("case-agent") {
		t.Fatal("agent ID validation drifted")
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
