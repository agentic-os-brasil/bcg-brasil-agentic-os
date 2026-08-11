package runtimeprojection

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/skillpolicy"
)

func TestInspectAndRoutingRejectSkillBodyAndManifestCoTamper(t *testing.T) {
	workspace := t.TempDir()
	if _, err := Install("codex", workspace); err != nil {
		t.Fatal(err)
	}
	skillPath := filepath.Join(workspace, ".codex", "skills", "dream-memory", "SKILL.md")
	mutated := []byte("# attacker-controlled method\n")
	if err := os.WriteFile(skillPath, mutated, 0o600); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(workspace, ManifestRelativePath)
	current, err := readManifest(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	current.SkillHashes["dream-memory"] = digest(mutated)
	if err := writeJSON(manifestPath, current); err != nil {
		t.Fatal(err)
	}

	status, err := Inspect("codex", workspace)
	if err != nil || status.State != "conflict" {
		t.Fatalf("co-tampered skill projection = %+v, %v", status, err)
	}
	if _, _, _, err := RoutingInputs("codex", workspace); err == nil {
		t.Fatal("routing accepted a skill body co-tampered with its manifest hash")
	}
}

func TestInspectAndRoutingRejectPolicyAndManifestCoTamper(t *testing.T) {
	workspace := t.TempDir()
	status, err := Install("codex", workspace)
	if err != nil {
		t.Fatal(err)
	}
	policy, err := skillpolicy.ParseFile(status.PolicyPath)
	if err != nil {
		t.Fatal(err)
	}
	caseRule := -1
	for i, rule := range policy.Direct {
		if rule.Role == "case_agent" {
			caseRule = i
			break
		}
	}
	if caseRule < 0 || len(policy.Direct[caseRule].SkillIDs) < 2 {
		t.Fatalf("unexpected base policy fixture: %#v", policy.Direct)
	}
	policy.Direct[caseRule].SkillIDs = append([]string(nil), policy.Direct[caseRule].SkillIDs[1:]...)
	mutated, err := json.MarshalIndent(policy, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	mutated = append(mutated, '\n')
	if err := os.WriteFile(status.PolicyPath, mutated, 0o600); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(workspace, ManifestRelativePath)
	current, err := readManifest(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	current.PolicyHash = digest(mutated)
	if err := writeJSON(manifestPath, current); err != nil {
		t.Fatal(err)
	}

	inspected, err := Inspect("codex", workspace)
	if err != nil || inspected.State != "conflict" {
		t.Fatalf("co-tampered policy projection = %+v, %v", inspected, err)
	}
	if _, _, _, err := RoutingInputs("codex", workspace); err == nil {
		t.Fatal("routing accepted a policy co-tampered with its manifest hash")
	}
}

func TestInstallProjectsRichOrientationAndSkills(t *testing.T) {
	workspace := t.TempDir()

	status, err := Install("claude", workspace)
	if err != nil {
		t.Fatal(err)
	}
	if status.State != "installed" || status.SkillCount < 10 {
		t.Fatalf("unexpected install status: %+v", status)
	}
	orientation, err := os.ReadFile(filepath.Join(workspace, "CLAUDE.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(orientation)
	for _, expected := range []string{
		"Sessão e hooks", "SELF do dono", "Memória e persistência",
		"Brain, wiki e navegação", "Agents e delegação", "Execução e continuidade",
		"execution-continuity", "dream-memory",
		"brain/tasks/", "receita conversacional",
		"<maestro-cli> adapter status --runtime claude",
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("orientation missing %q", expected)
		}
	}
	if strings.Contains(text, "<maestro-cli> work next --active --workspace <workspace>") {
		t.Fatalf("orientation still exposes the legacy ledger command: %q", text)
	}
	if !strings.Contains(text, "`/dream-memory`") || strings.Contains(text, "`$dream-memory`") {
		t.Fatalf("orientation skill references should use slash notation: %q", text)
	}
	entries, err := os.ReadDir(filepath.Join(workspace, ".claude", "skills"))
	if err != nil || len(entries) != status.SkillCount {
		t.Fatalf("projected skills = %d, err = %v", len(entries), err)
	}
	if _, err := os.Stat(filepath.Join(workspace, ManifestRelativePath)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(workspace, PolicyRelativePath)); err != nil {
		t.Fatal(err)
	}

	second, err := Install("claude", workspace)
	if err != nil || second.State != "installed" {
		t.Fatalf("idempotent install = %+v, %v", second, err)
	}
	updated, err := os.ReadFile(filepath.Join(workspace, "CLAUDE.md"))
	if err != nil || strings.Count(string(updated), OrientationBegin) != 1 || strings.Count(string(updated), OrientationEnd) != 1 {
		t.Fatalf("orientation markers after reinstall = %q, %v", updated, err)
	}
}

func TestInstallProjectsSelectionScopedPolicyAndPreservesModifiedPolicy(t *testing.T) {
	workspace := t.TempDir()
	status, err := InstallForTracks("codex", workspace, []string{"data-science"})
	if err != nil {
		t.Fatal(err)
	}
	policy, err := skillpolicy.ParseFile(status.PolicyPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, skillID := range []string{"data-science-evaluation", "test-and-evidence", "deck-storyline"} {
		if !policy.AllowsDirect("case_agent", skillID) {
			t.Fatalf("selected or dependency skill %q is not active", skillID)
		}
	}

	mutated := append([]byte("\n"), []byte("user-owned-policy\n")...)
	if err := os.WriteFile(status.PolicyPath, mutated, 0o600); err != nil {
		t.Fatal(err)
	}
	inspected, err := Inspect("codex", workspace)
	if err != nil || inspected.State != "conflict" || len(inspected.Conflicts) != 1 || inspected.Conflicts[0] != status.PolicyPath {
		t.Fatalf("modified policy was not detected: %+v, %v", inspected, err)
	}
	failed, err := InstallForTracks("codex", workspace, []string{"data-science"})
	if err == nil || failed.State != "conflict" {
		t.Fatalf("modified policy did not fail install closed: %+v, %v", failed, err)
	}
	if _, err := Uninstall("codex", workspace); err == nil {
		t.Fatal("modified policy did not fail uninstall closed")
	}
	body, err := os.ReadFile(status.PolicyPath)
	if err != nil || string(body) != string(mutated) {
		t.Fatalf("modified policy was not preserved: %q, %v", body, err)
	}
}

func TestInstallUpgradesLegacyManifestOnlyWhenPolicyPathIsFree(t *testing.T) {
	workspace := t.TempDir()
	if _, err := Install("codex", workspace); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(workspace, ManifestRelativePath)
	current, err := readManifest(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	current.PolicyPath, current.PolicyHash = "", ""
	if err := writeJSON(manifestPath, current); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(workspace, PolicyRelativePath)); err != nil {
		t.Fatal(err)
	}
	if _, err := Install("codex", workspace); err != nil {
		t.Fatalf("legacy projection was not upgraded: %v", err)
	}
	upgraded, err := readManifest(manifestPath)
	if err != nil || upgraded.PolicyPath != PolicyRelativePath || upgraded.PolicyHash == "" {
		t.Fatalf("policy ownership was not recorded: %+v, %v", upgraded, err)
	}
}

func TestInstallPreservesUserOrientationAndFailsClosedOnModifiedSkill(t *testing.T) {
	workspace := t.TempDir()
	userOrientation := "# Meu workspace\n\nNotas do usuário.\n"
	if err := os.WriteFile(filepath.Join(workspace, "CLAUDE.md"), []byte(userOrientation), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Install("claude", workspace); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(workspace, "CLAUDE.md"))
	if err != nil || info.Mode().Perm() != 0o644 {
		t.Fatalf("user orientation mode changed: %v, %v", info, err)
	}
	orientation, err := os.ReadFile(filepath.Join(workspace, "CLAUDE.md"))
	if err != nil || !strings.HasPrefix(string(orientation), userOrientation) {
		t.Fatalf("user orientation was not preserved: %q, %v", orientation, err)
	}

	skillPath := filepath.Join(workspace, ".claude", "skills", "dream-memory", "SKILL.md")
	if err := os.WriteFile(skillPath, []byte("# edited by user\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	status, err := Install("claude", workspace)
	if err == nil || status.State != "conflict" || len(status.Conflicts) != 1 {
		t.Fatalf("modified skill should fail closed: %+v, %v", status, err)
	}
	body, readErr := os.ReadFile(skillPath)
	if readErr != nil || string(body) != "# edited by user\n" {
		t.Fatalf("modified skill was overwritten: %q, %v", body, readErr)
	}
}

func TestModifiedManagedOrientationFailsClosed(t *testing.T) {
	workspace := t.TempDir()
	if _, err := Install("claude", workspace); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(workspace, "CLAUDE.md")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	mutated := strings.Replace(string(body), "O Maestro é o Agentic OS profissional", "O Maestro foi alterado", 1)
	if mutated == string(body) {
		t.Fatal("test fixture did not mutate managed orientation")
	}
	if err := os.WriteFile(path, []byte(mutated), 0o644); err != nil {
		t.Fatal(err)
	}
	status, err := Install("claude", workspace)
	if err == nil || status.State != "conflict" || len(status.Conflicts) != 1 || status.Conflicts[0] != path {
		t.Fatalf("modified orientation should fail closed: %+v, %v", status, err)
	}
	current, err := os.ReadFile(path)
	if err != nil || string(current) != mutated {
		t.Fatalf("modified orientation was overwritten: %q, %v", current, err)
	}
}

func TestInstallRemovesUnchangedRetiredSkill(t *testing.T) {
	workspace := t.TempDir()
	if _, err := Install("claude", workspace); err != nil {
		t.Fatal(err)
	}
	retiredPath := filepath.Join(workspace, ".claude", "skills", "retired-skill", "SKILL.md")
	retiredBody := []byte("retired\n")
	if err := os.MkdirAll(filepath.Dir(retiredPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(retiredPath, retiredBody, 0o600); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(workspace, ManifestRelativePath)
	current, err := readManifest(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	current.SkillHashes["retired-skill"] = digest(retiredBody)
	if err := writeJSON(manifestPath, current); err != nil {
		t.Fatal(err)
	}
	if _, err := Install("claude", workspace); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(retiredPath); !os.IsNotExist(err) {
		t.Fatalf("unchanged retired skill remains: %v", err)
	}
}

func TestCodexProjectionAndUninstallPreserveUserContent(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "AGENTS.md"), []byte("# Local rules\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	installed, err := InstallForTracks("codex", workspace, []string{"software-engineering"})
	if err != nil || installed.State != "installed" {
		t.Fatalf("codex install = %+v, %v", installed, err)
	}
	if _, err := os.Stat(filepath.Join(workspace, ".codex", "skills", "unit-test-wave", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	removed, err := Uninstall("codex", workspace)
	if err != nil || removed.State != "removed" {
		t.Fatalf("codex uninstall = %+v, %v", removed, err)
	}
	body, err := os.ReadFile(filepath.Join(workspace, "AGENTS.md"))
	if err != nil || string(body) != "# Local rules\n" {
		t.Fatalf("user AGENTS.md was not preserved: %q, %v", body, err)
	}
	if _, err := os.Stat(filepath.Join(workspace, ".codex", "skills")); !os.IsNotExist(err) {
		t.Fatalf("managed skills directory remains: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workspace, PolicyRelativePath)); !os.IsNotExist(err) {
		t.Fatalf("managed policy remains: %v", err)
	}
}
