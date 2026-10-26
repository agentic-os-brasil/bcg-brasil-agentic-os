package skillsindex_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/skillsindex"
)

func TestCatalogPreservesExplicitOnlyInvocation(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "manual", "Manual", "A deliberate method", "Use $manual.")
	path := filepath.Join(root, "manual", "SKILL.md")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	body = []byte(strings.Replace(string(body), "\n---\n", "\ndisable-model-invocation: true\n---\n", 1))
	if err := os.WriteFile(path, body, 0600); err != nil {
		t.Fatal(err)
	}
	catalog, err := skillsindex.Build(root)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := skillsindex.RenderJSON(catalog)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(encoded, []byte(`"user_invoked_only": true`)) {
		t.Fatalf("catalog lost invocation policy: %s", encoded)
	}
}
