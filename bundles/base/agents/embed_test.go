package baseagents

import (
	"bytes"
	"testing"
)

func TestManagedScaffoldTemplatesAreEmbeddedAndDataFree(t *testing.T) {
	for _, role := range []string{"account_agent", "case_agent", "client_account_agent", "pa_expert", "workspace_agent"} {
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
	if _, err := Template("capability_specialist"); err == nil {
		t.Fatal("Capability Specialist scaffold template was exposed")
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
