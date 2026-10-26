package runtimeprojection

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	baseskills "github.com/agentic-os-brasil/bcg-brasil-agentic-os/bundles/base/skills"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/skillpolicy"
)

func TestInstallProjectsSkillResourcesAndPreservesCustomizedResources(t *testing.T) {
	workspace := t.TempDir()
	status, err := InstallForTracks("codex", workspace, []string{"software-engineering"})
	if err != nil {
		t.Fatal(err)
	}
	resource := filepath.Join(workspace, ".codex", "skills", "case-canon-ingest", "agents", "openai.yaml")
	body, err := os.ReadFile(resource)
	if err != nil || len(body) == 0 {
		t.Fatalf("installed resource = %q, %v", body, err)
	}
	catalog, err := baseskills.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	if status.SkillCount <= len(catalog.Skills) {
		t.Fatalf("selected track methods missing: %+v", status)
	}
	before, err := os.ReadFile(filepath.Join(workspace, ManifestRelativePath))
	if err != nil {
		t.Fatal(err)
	}
	custom := []byte("user resource\n")
	if err := os.WriteFile(resource, custom, 0o600); err != nil {
		t.Fatal(err)
	}
	if inspected, err := Inspect("codex", workspace); err != nil || inspected.State != "conflict" {
		t.Fatalf("modified resource = %+v, %v", inspected, err)
	}
	if _, _, _, err := RoutingInputs("codex", workspace); err == nil {
		t.Fatal("modified resource remained routable")
	}
	if _, err := InstallForTracks("codex", workspace, []string{"software-engineering"}); err == nil {
		t.Fatal("modified resource was overwritten")
	}
	if _, err := Uninstall("codex", workspace); err == nil {
		t.Fatal("modified resource was removed")
	}
	after, err := os.ReadFile(filepath.Join(workspace, ManifestRelativePath))
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("failed operations changed manifest")
	}
	got, err := os.ReadFile(resource)
	if err != nil || !bytes.Equal(got, custom) {
		t.Fatal("custom resource was not preserved")
	}
}

func TestPocockMethodResourcesReachNativeInstallWithoutInflatingMethodCount(t *testing.T) {
	workspace := t.TempDir()
	status, err := InstallForTracks("claude", workspace, []string{"software-engineering"})
	if err != nil {
		t.Fatal(err)
	}
	_, catalog, err := PolicyForTracks([]string{"software-engineering"})
	if err != nil {
		t.Fatal(err)
	}
	if status.SkillCount != len(catalog.Skills) {
		t.Fatalf("resources inflated method count: %+v", status)
	}
	planned, err := PlannedManagedPaths("claude", workspace, []string{"software-engineering"})
	if err != nil {
		t.Fatal(err)
	}
	paths := make(map[string]bool)
	for _, path := range planned {
		paths[path] = true
	}
	for _, relative := range []string{"SKILL.md", "tests.md", "mocking.md", "LICENSE"} {
		path := filepath.Join(workspace, ".claude", "skills", "pocock-tdd", relative)
		if body, err := os.ReadFile(path); err != nil || len(body) == 0 {
			t.Fatalf("Pocock resource %s missing: %v", relative, err)
		}
		if !paths[path] {
			t.Fatalf("transaction omitted %s", relative)
		}
	}
	if _, err := Uninstall("claude", workspace); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(workspace, ".claude", "skills", "pocock-tdd")); !os.IsNotExist(err) {
		t.Fatalf("Pocock resources survive uninstall: %v", err)
	}
}

func TestSkillResourceLifecycleAndLegacyUpgrade(t *testing.T) {
	workspace := t.TempDir()
	if _, err := InstallForTracks("codex", workspace, []string{"software-engineering"}); err != nil {
		t.Fatal(err)
	}
	resource := filepath.Join(workspace, ".codex", "skills", "pr-review", "agents", "openai.yaml")
	if _, err := os.Stat(resource); err != nil {
		t.Fatal(err)
	}
	planned, err := PlannedManagedPaths("codex", workspace, []string{"consulting"})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, path := range planned {
		if path == resource {
			found = true
		}
	}
	if !found {
		t.Fatal("transaction snapshots omit managed resource")
	}
	if _, err := InstallForTracks("codex", workspace, []string{"consulting"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(resource); err != nil {
		t.Fatalf("included Tech Core resource missing: %v", err)
	}

	manifestPath := filepath.Join(workspace, ManifestRelativePath)
	current, err := readManifest(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	for relative := range current.ResourceHashes {
		if err := os.Remove(filepath.Join(workspace, ".codex", "skills", filepath.FromSlash(relative))); err != nil {
			t.Fatal(err)
		}
	}
	current.ResourceHashes = nil
	if err := writeJSON(manifestPath, current); err != nil {
		t.Fatal(err)
	}
	if _, err := InstallForTracks("codex", workspace, []string{"consulting"}); err != nil {
		t.Fatalf("legacy v1 upgrade failed: %v", err)
	}
	if status, err := Inspect("codex", workspace); err != nil || status.State != "installed" {
		t.Fatalf("upgraded projection = %+v, %v", status, err)
	}
	if _, err := Uninstall("codex", workspace); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(workspace, ".codex", "skills")); !os.IsNotExist(err) {
		t.Fatalf("managed resources remain after uninstall: %v", err)
	}
}

func TestLegacyResourceUpgradePreservesUnownedFileEvenWhenCanonical(t *testing.T) {
	workspace := t.TempDir()
	if _, err := Install("codex", workspace); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(workspace, ManifestRelativePath)
	current, err := readManifest(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	current.ResourceHashes = nil
	if err := writeJSON(manifestPath, current); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Install("codex", workspace); err == nil {
		t.Fatal("legacy manifest took ownership of existing unowned resources")
	}
	after, err := os.ReadFile(manifestPath)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("blocked legacy upgrade changed manifest")
	}
}

func TestUnknownResourceManifestCannotAuthorizeMutation(t *testing.T) {
	for _, relative := range []string{"case-canon-ingest/user-notes.md", "../user-notes.md", "case-canon-ingest/../user-notes.md"} {
		t.Run(relative, func(t *testing.T) {
			workspace := t.TempDir()
			if _, err := Install("codex", workspace); err != nil {
				t.Fatal(err)
			}
			manifestPath := filepath.Join(workspace, ManifestRelativePath)
			current, err := readManifest(manifestPath)
			if err != nil {
				t.Fatal(err)
			}
			userPath := filepath.Join(workspace, ".codex", "skills", filepath.FromSlash(relative))
			if err := os.MkdirAll(filepath.Dir(userPath), 0o700); err != nil {
				t.Fatal(err)
			}
			userBody := []byte("owner data\n")
			if err := os.WriteFile(userPath, userBody, 0o600); err != nil {
				t.Fatal(err)
			}
			current.ResourceHashes[relative] = digest(userBody)
			if err := writeJSON(manifestPath, current); err != nil {
				t.Fatal(err)
			}
			if _, err := Install("codex", workspace); err == nil {
				t.Fatal("install accepted unknown resource authority")
			}
			if _, err := Uninstall("codex", workspace); err == nil {
				t.Fatal("uninstall accepted unknown resource authority")
			}
			got, err := os.ReadFile(userPath)
			if err != nil || !bytes.Equal(got, userBody) {
				t.Fatal("unknown resource manifest modified owner data")
			}
		})
	}
}

func TestResourceIntegrityRejectsCoTamperAndSymlinks(t *testing.T) {
	for _, mutation := range []string{"co-tamper", "symlink"} {
		t.Run(mutation, func(t *testing.T) {
			workspace := t.TempDir()
			if _, err := Install("codex", workspace); err != nil {
				t.Fatal(err)
			}
			relative := "case-canon-ingest/agents/openai.yaml"
			resource := filepath.Join(workspace, ".codex", "skills", filepath.FromSlash(relative))
			if mutation == "symlink" {
				target := filepath.Join(t.TempDir(), "owner.yaml")
				if err := os.WriteFile(target, []byte("owner\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Remove(resource); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, resource); err != nil {
					t.Skipf("symlink unavailable: %v", err)
				}
			} else {
				body := []byte("modified resource\n")
				if err := os.WriteFile(resource, body, 0o600); err != nil {
					t.Fatal(err)
				}
				current, err := readManifest(filepath.Join(workspace, ManifestRelativePath))
				if err != nil {
					t.Fatal(err)
				}
				current.ResourceHashes[relative] = digest(body)
				if err := writeJSON(filepath.Join(workspace, ManifestRelativePath), current); err != nil {
					t.Fatal(err)
				}
			}
			if status, err := Inspect("codex", workspace); err != nil || status.State != "conflict" {
				t.Fatalf("unsafe resource = %+v, %v", status, err)
			}
			if _, _, _, err := RoutingInputs("codex", workspace); err == nil {
				t.Fatal("unsafe resource remained routable")
			}
			if _, err := Uninstall("codex", workspace); err == nil {
				t.Fatal("unsafe resource was removed")
			}
		})
	}
}

func TestInstallRejectsResourceAndManifestCoTamperBeforeAnyWrites(t *testing.T) {
	workspace := t.TempDir()
	if _, err := InstallForTracks("codex", workspace, []string{"software-engineering"}); err != nil {
		t.Fatal(err)
	}
	resource := "pocock-tdd/tests.md"
	resourcePath := filepath.Join(workspace, ".codex", "skills", filepath.FromSlash(resource))
	mutated := []byte("owner-modified method resource\n")
	if err := os.WriteFile(resourcePath, mutated, 0o600); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(workspace, ManifestRelativePath)
	current, err := readManifest(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	current.ResourceHashes[resource] = digest(mutated)
	if err := writeJSON(manifestPath, current); err != nil {
		t.Fatal(err)
	}
	before := make(map[string][]byte)
	if err := filepath.WalkDir(workspace, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		body, err := os.ReadFile(path)
		before[path] = body
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := ValidateInstallForTracks("codex", workspace, []string{"software-engineering"}); err == nil {
		t.Error("preflight accepted a co-tampered resource identity")
	}
	if status, err := InstallForTracks("codex", workspace, []string{"software-engineering"}); err == nil || status.State != "conflict" {
		t.Errorf("co-tampered resource install = %+v, %v", status, err)
	}
	for path, expected := range before {
		if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, expected) {
			t.Fatalf("blocked installation changed %s: %v", path, err)
		}
	}
}

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
		"/maestro-onboarding",
		"/maestro-doctor",
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("orientation missing %q", expected)
		}
	}
	for _, hidden := range []string{"<maestro-cli> work next", "<maestro-cli> adapter status", "<maestro-cli> memory status"} {
		if strings.Contains(text, hidden) {
			t.Fatalf("orientation exposes compatibility command %q as normal workflow", hidden)
		}
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
	for _, skillID := range []string{"data-science-evaluation", "test-and-evidence", "bcg-deck"} {
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
	before, err := os.Stat(filepath.Join(workspace, "CLAUDE.md"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Install("claude", workspace); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(workspace, "CLAUDE.md"))
	if err != nil || info.Mode().Perm() != before.Mode().Perm() {
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
