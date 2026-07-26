package baseagents

import (
	"bytes"
	"testing"
)

func TestManagedScaffoldTemplatesAreEmbeddedAndDataFree(t *testing.T) {
	for _, role := range []string{"account_agent", "case_agent", "client_account_agent", "pa_expert", "practice_agent", "workspace_agent", "capability_specialist", "subject_specialist"} {
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
	if _, err := Template("general_assistant"); err == nil {
		t.Fatal("unsupported scaffold template was exposed")
	}
}

func TestManagedHelixRegistryStartsEmptyAndAuthoritative(t *testing.T) {
	registry, err := ManagedHelixRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if registry.Authority != "helix-brasil" || len(registry.Experts) != 0 {
		t.Fatalf("unexpected initial Helix registry: %#v", registry)
	}
}
