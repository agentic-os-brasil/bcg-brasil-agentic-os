package baseagents

import (
	"bytes"
	"testing"
)

func TestManagedScaffoldTemplatesAreEmbeddedAndDataFree(t *testing.T) {
	for _, role := range []string{"account_agent", "practice_agent", "workspace_agent", "capability_specialist", "subject_specialist"} {
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
