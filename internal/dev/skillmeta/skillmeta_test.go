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

func TestValidateClaudeProjectionsAcceptsThinMatchingProjection(t *testing.T) {
	root := t.TempDir()
	canonical := filepath.Join(root, "dev", "skills")
	projection := filepath.Join(root, ".claude", "skills")
	writeSkill(t, canonical, "start-work", "start-work", "Start work safely.")
	writeProjection(t, projection, "start-work", "../../../dev/skills/start-work/SKILL.md")
	if err := ValidateClaudeProjections(canonical, projection); err != nil {
		t.Fatalf("ValidateClaudeProjections() error = %v", err)
	}
}

func TestValidateClaudeProjectionsRejectsMissingAndOrphanedSkills(t *testing.T) {
	root := t.TempDir()
	canonical := filepath.Join(root, "dev", "skills")
	projection := filepath.Join(root, ".claude", "skills")
	writeSkill(t, canonical, "start-work", "start-work", "Start work safely.")
	writeProjection(t, projection, "orphan", "../../../dev/skills/orphan/SKILL.md")
	err := ValidateClaudeProjections(canonical, projection)
	if err == nil || !strings.Contains(err.Error(), "missing") || !strings.Contains(err.Error(), "no canonical") {
		t.Fatalf("ValidateClaudeProjections() error = %v", err)
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

func writeProjection(t *testing.T, root, name, pointer string) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: " + name + "\ndescription: Thin projection.\n---\n\nRead `" + pointer + "`.\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
