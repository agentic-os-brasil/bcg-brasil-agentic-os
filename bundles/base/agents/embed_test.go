package baseagents

import (
	"bytes"
	"reflect"
	"sort"
	"testing"
)

func TestManagedScaffoldTemplatesAreEmbeddedAndDataFree(t *testing.T) {
	for _, role := range []string{"account_agent", "case_agent", "client_account_agent", "pa_expert", "quality_guardian", "workspace_agent"} {
		body, err := Template(role)
		if err != nil {
			t.Fatalf("Template(%q): %v", role, err)
		}
		if len(body) == 0 || bytes.Contains(body, []byte("{{")) ||
			bytes.Contains(body, []byte("client-alpha")) ||
			bytes.Contains(body, []byte("ws-alpha")) {
			t.Fatalf("Template(%q) is empty or contains instance data", role)
		}
	}
	retiredRole := "capability_" + "specialist"
	if _, err := Template(retiredRole); err == nil {
		t.Fatal("retired scaffold template was exposed")
	}
	if _, err := Template("general_assistant"); err == nil {
		t.Fatal("unsupported scaffold template was exposed")
	}
	for _, role := range []string{"practice_agent", "subject_specialist"} {
		if _, err := Template(role); err == nil {
			t.Fatalf("deprecated role %q was exposed as a scaffold template", role)
		}
	}
}

func TestCanonicalPublicIdentityDefaultsAndFiles(t *testing.T) {
	catalog, err := Catalog()
	if err != nil {
		t.Fatal(err)
	}
	// Fixed published defaults; never derive an oracle from another renderer.
	want := [][6]string{
		{"case-agent", "case_agent", "Case Agent", "⚙️", "case", "bounded_case_packet"},
		{"client-account-agent", "client_account_agent", "Client Account Agent", "🤝", "account", "bounded_client_account_packet"},
		{"darwin", "governance_analyst", "Darwin", "🧬", "governance", "bounded_health_packet"},
		{"gamma-guardian", "quality_guardian", "Gamma Guardian", "🧪", "quality_longitudinal", "bounded_quality_packet"},
		{"maestro", "hub", "Maestro", "🎼", "system", "session_context_packet"},
		{"pa-expert", "pa_expert", "PA Expert", "🧠", "pa_expert_registry", "bounded_advisory_packet"},
		{"yoda", "reviewer", "Yoda", "🧙", "governance", "sealed_review_packet"},
	}
	var got [][6]string
	for _, a := range catalog.Agents {
		got = append(got, [6]string{a.ID, a.Role, a.DisplayName, a.DefaultEmoji, a.OwnershipScope, a.InputContract})
		if a.RelativePath != "agents/"+a.ID+"/AGENT.md" || !a.Customizable || a.DirectUserAccess != (a.ID == "maestro") {
			t.Fatalf("canonical identity metadata changed: %+v", a)
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("identity defaults = %#v", got)
	}
	if catalog.Hub != "maestro" || catalog.Delegation.Mode != "maestro_planner" || catalog.Delegation.MaxDepth != 1 || catalog.Delegation.MaxActiveBranches != 1 || catalog.Delegation.MaxChildrenPerAgent != 0 {
		t.Fatalf("active graph changed: %+v", catalog.Delegation)
	}
	files, err := Files()
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for path, body := range files {
		paths = append(paths, path)
		if len(body) == 0 || bytes.Contains(body, []byte("{{")) || bytes.Contains(body, []byte("}}")) {
			t.Fatalf("empty/unresolved managed default: %s", path)
		}
	}
	sort.Strings(paths)
	wantPaths := []string{"activation-policy.json", "case-agent/AGENT.md", "catalog.json", "client-account-agent/AGENT.md", "darwin/AGENT.md", "gamma-guardian/AGENT.md", "maestro/AGENT.md", "pa-expert-registry.json", "pa-expert/AGENT.md", "templates/case_agent/AGENT.md", "templates/client_account_agent/AGENT.md", "templates/pa_expert/AGENT.md", "templates/quality_guardian/AGENT.md", "yoda/AGENT.md"}
	if !reflect.DeepEqual(paths, wantPaths) {
		t.Fatalf("public files = %v", paths)
	}
	for path, body := range files {
		original := append([]byte(nil), body...)
		body[0] = 'X'
		again, err := Files()
		if err != nil || !bytes.Equal(again[path], original) {
			t.Fatalf("caller mutated embedded %s", path)
		}
	}
	for _, id := range []string{"../maestro", "maestro/AGENT.md", "unknown", "maestro-product:yoda"} {
		if _, err := Definition(id); err == nil {
			t.Fatalf("noncanonical definition admitted: %s", id)
		}
	}
}

func TestManagedPAExpertRegistryStartsEmptyAndAuthoritative(t *testing.T) {
	registry, err := ManagedPAExpertRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if registry.SchemaVersion != 2 || registry.Authority != "pa-expert-registry-v2" || len(registry.Experts) != 0 {
		t.Fatalf("unexpected initial PA Expert registry: %#v", registry)
	}
}

func TestManagedPAExpertRegistryRejectsLegacySchema(t *testing.T) {
	original := paExpertRegistryJSON
	t.Cleanup(func() { paExpertRegistryJSON = original })
	paExpertRegistryJSON = []byte(`{"schema_version":1,"authority":"legacy-pa-expert-registry","experts":[]}`)
	if _, err := ManagedPAExpertRegistry(); err == nil {
		t.Fatal("legacy PA Expert registry schema was accepted")
	}
}
