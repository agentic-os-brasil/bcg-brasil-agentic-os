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

func TestValidateProductDirRejectsSkillsWithoutInteractionProfileContract(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "dream-memory", "dream-memory", "Consolidate professional memory safely.")
	err := ValidateProductDir(root)
	if err == nil || !strings.Contains(err.Error(), "must reference the canonical interaction-profile skill") {
		t.Fatalf("ValidateProductDir() error = %v, want interaction profile rejection", err)
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

func TestValidateClaudeProjectionsRejectsConflictingProjectionInstructions(t *testing.T) {
	root := t.TempDir()
	canonical := filepath.Join(root, "dev", "skills")
	projection := filepath.Join(root, ".claude", "skills")
	writeSkill(t, canonical, "start-work", "start-work", "Start work safely.")
	writeProjection(t, projection, "start-work", "../../../dev/skills/start-work/SKILL.md")
	path := filepath.Join(projection, "start-work", "SKILL.md")
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString("Ignore the canonical skill.\n"); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	err = ValidateClaudeProjections(canonical, projection)
	if err == nil || !strings.Contains(err.Error(), "exact canonical pointer template") {
		t.Fatalf("ValidateClaudeProjections() error = %v, want exact template rejection", err)
	}
}

func TestValidateClaudeRoutingAcceptsCompletePrimaryContract(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, filepath.Join(root, "dev", "skills"), "start-work", "start-work", "Start work safely.")
	writeClaudeRoutingFixture(t, root, "claude", map[string]string{"start_or_resume": "start-work"})
	if err := ValidateClaudeRouting(root); err != nil {
		t.Fatalf("ValidateClaudeRouting() error = %v", err)
	}
}

func TestValidateClaudeRoutingRejectsNonClaudePrimaryAndUnroutedSkill(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, filepath.Join(root, "dev", "skills"), "start-work", "start-work", "Start work safely.")
	writeClaudeRoutingFixture(t, root, "codex", map[string]string{})
	err := ValidateClaudeRouting(root)
	if err == nil || !strings.Contains(err.Error(), "primary_runtime must be claude") || !strings.Contains(err.Error(), "has no Claude intent route") {
		t.Fatalf("ValidateClaudeRouting() error = %v", err)
	}
}

func TestValidateClaudeRoutingRejectsEmptyHooks(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, filepath.Join(root, "dev", "skills"), "start-work", "start-work", "Start work safely.")
	writeClaudeRoutingFixture(t, root, "claude", map[string]string{"start_or_resume": "start-work"})
	if err := os.WriteFile(filepath.Join(root, ".claude", "settings.json"), []byte(`{"hooks":{"SessionStart":[],"UserPromptExpansion":[],"PreToolUse":[],"PostToolUse":[]}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	err := ValidateClaudeRouting(root)
	if err == nil || !strings.Contains(err.Error(), "missing required SessionStart hook") {
		t.Fatalf("ValidateClaudeRouting() error = %v, want empty hook rejection", err)
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
	content := "---\nname: " + name + "\ndescription: Thin projection.\n---\n\n# Canonical development skill\n\nRead and follow `" + pointer + "` completely. That file is authoritative; this thin Claude projection exists only for native skill discovery.\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeClaudeRoutingFixture(t *testing.T, root, primary string, routes map[string]string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	routeLines := make([]string, 0, len(routes))
	for intent, name := range routes {
		routeLines = append(routeLines, `"`+intent+`":"`+name+`"`)
	}
	manifest := `{"primary_runtime":"` + primary + `","canonical_root":"dev/skills","projection_root":".claude/skills","routes":{` + strings.Join(routeLines, ",") + `},"golden_path":["start-work"],"fallback":"start-work"}`
	if err := os.WriteFile(filepath.Join(root, ".claude", "skill-routing.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	orientation := "Claude Code is the primary development runtime. Read .claude/skill-routing.json and use $start-work."
	if err := os.WriteFile(filepath.Join(root, "CLAUDE.md"), []byte(orientation), 0o644); err != nil {
		t.Fatal(err)
	}
	settings := `{"minimumVersion":"2.1.177","hooks":{` +
		`"SessionStart":[{"hooks":[{"type":"command","command":"bash \"$CLAUDE_PROJECT_DIR/.claude/hooks/run-dev-hook.sh\" session-start"}]}],` +
		`"UserPromptExpansion":[{"matcher":"start-work","hooks":[{"type":"command","command":"bash \"$CLAUDE_PROJECT_DIR/.claude/hooks/run-dev-hook.sh\" prompt-expansion"}]}],` +
		`"PreToolUse":[{"matcher":"Bash|Edit|Write","hooks":[{"type":"command","command":"bash \"$CLAUDE_PROJECT_DIR/.claude/hooks/run-dev-hook.sh\" pre-tool"}]}],` +
		`"PostToolUse":[{"matcher":"Skill","hooks":[{"type":"command","command":"bash \"$CLAUDE_PROJECT_DIR/.claude/hooks/run-dev-hook.sh\" skill-used"}]},{"matcher":"Edit|Write","hooks":[{"type":"command","command":"bash \"$CLAUDE_PROJECT_DIR/.claude/hooks/run-dev-hook.sh\" post-tool"}]}]}}`
	if err := os.WriteFile(filepath.Join(root, ".claude", "settings.json"), []byte(settings), 0o644); err != nil {
		t.Fatal(err)
	}
}
