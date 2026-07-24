package skillsindex_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/skillsindex"
)

func TestBuildCompilesSortedPointersWithoutSkillBodies(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "zeta", "Zeta", "Use for the last operation.", "Use $zeta for a final operation.")
	writeSkill(t, root, "alpha", "Alpha", "Use for the first operation.", "Use $alpha for a first operation.")

	catalog, err := skillsindex.Build(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Skills) != 2 || catalog.Skills[0].ID != "alpha" || catalog.Skills[1].ID != "zeta" {
		t.Fatalf("catalog = %#v", catalog)
	}
	entry := catalog.Skills[0]
	if entry.RelativePath != "skills/alpha/SKILL.md" || entry.Trigger != "summary" || entry.DefaultPrompt != "Use $alpha for a first operation." {
		t.Fatalf("entry = %#v", entry)
	}

	markdown, err := skillsindex.RenderMarkdown(catalog)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(markdown), "skills/alpha/SKILL.md") || strings.Contains(string(markdown), "# Skill body") {
		t.Fatalf("markdown = %s", markdown)
	}
}

func TestValidateRejectsStaleGeneratedCatalog(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "alpha", "Alpha", "Use for the first operation.", "Use $alpha for a first operation.")
	if err := os.WriteFile(filepath.Join(root, "catalog.json"), []byte(`{"schema_version":1,"skills":[]}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "INDEX.md"), []byte("stale\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := skillsindex.Validate(root); err == nil || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("Validate() error = %v, want stale generated artifact", err)
	}
}

func TestValidateAcceptsWindowsCRLFCheckoutOfGeneratedArtifacts(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "alpha", "Alpha", "Use for the first operation.", "Use $alpha for a first operation.")
	catalog, err := skillsindex.Build(root)
	if err != nil {
		t.Fatal(err)
	}
	jsonBody, err := skillsindex.RenderJSON(catalog)
	if err != nil {
		t.Fatal(err)
	}
	markdownBody, err := skillsindex.RenderMarkdown(catalog)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "catalog.json"), bytes.ReplaceAll(jsonBody, []byte("\n"), []byte("\r\n")), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "INDEX.md"), bytes.ReplaceAll(markdownBody, []byte("\n"), []byte("\r\n")), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := skillsindex.Validate(root); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func writeSkill(t *testing.T, root, id, displayName, description, prompt string) {
	t.Helper()
	directory := filepath.Join(root, id, "agents")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\nname: " + id + "\ndescription: " + description + "\n---\n\n# Skill body\n"
	if err := os.WriteFile(filepath.Join(root, id, "SKILL.md"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	metadata := "interface:\n  display_name: \"" + displayName + "\"\n  short_description: \"summary\"\n  default_prompt: \"" + prompt + "\"\n"
	if err := os.WriteFile(filepath.Join(directory, "openai.yaml"), []byte(metadata), 0o600); err != nil {
		t.Fatal(err)
	}
}
