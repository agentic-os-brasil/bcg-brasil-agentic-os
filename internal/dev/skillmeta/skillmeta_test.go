package skillmeta

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateDirAcceptsValidSkill(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "develop-change", "develop-change", "Implement changes through the development harness.")
	if err := ValidateDir(root); err != nil {
		t.Fatalf("ValidateDir() error = %v", err)
	}
}

func TestValidateDirRejectsUnfinishedDescription(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "develop-change", "develop-change", "TODO")
	err := ValidateDir(root)
	if err == nil || !strings.Contains(err.Error(), "description is empty or unfinished") {
		t.Fatalf("ValidateDir() error = %v, want unfinished description", err)
	}
}

func TestValidateDirRejectsNameDrift(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "develop-change", "wrong-name", "Implement changes safely.")
	err := ValidateDir(root)
	if err == nil || !strings.Contains(err.Error(), `frontmatter name is "wrong-name"`) {
		t.Fatalf("ValidateDir() error = %v, want name drift", err)
	}
}

func writeSkill(t *testing.T, root, folder, name, description string) {
	t.Helper()
	dir := filepath.Join(root, folder)
	if err := os.MkdirAll(filepath.Join(dir, "agents"), 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: " + name + "\ndescription: " + description + "\n---\n\n# Skill\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "agents", "openai.yaml"), []byte("interface: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}
