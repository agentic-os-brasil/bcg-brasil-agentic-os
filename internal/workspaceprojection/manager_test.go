package workspaceprojection

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"

	baseskills "github.com/agentic-os-brasil/bcg-brasil-agentic-os/bundles/base/skills"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/runtimeprojection"
)

func TestEnrollClaudeProjectsIntoGitRepositoryWithoutReplacingUserOrientation(t *testing.T) {
	fixture := newFixture(t)
	userOrientation := "# Project instructions\n\nKeep this user-authored guidance.\n"
	if err := os.WriteFile(filepath.Join(fixture.worktree, "CLAUDE.md"), []byte(userOrientation), 0o600); err != nil {
		t.Fatal(err)
	}

	status, err := fixture.manager.Enroll(context.Background(), "claude", fixture.worktree)
	if err != nil {
		t.Fatalf("Enroll() error = %v", err)
	}
	if status.State != StateEnrolled || status.RepositoryID == "" || status.WorkspaceID == "" {
		t.Fatalf("unexpected status: %+v", status)
	}

	orientation := readFile(t, filepath.Join(fixture.worktree, "CLAUDE.md"))
	if !strings.HasPrefix(orientation, userOrientation) || !strings.Contains(orientation, ManagedBlockStart) || !strings.Contains(orientation, ManagedBlockEnd) {
		t.Fatalf("user orientation was not preserved around managed block:\n%s", orientation)
	}
	for _, private := range []string{fixture.managedRoot, fixture.dataRoot, fixture.worktree} {
		if strings.Contains(orientation, private) {
			t.Fatalf("orientation leaked machine path %q", private)
		}
	}

	settings := readFile(t, filepath.Join(fixture.worktree, ".claude", "settings.local.json"))
	for _, required := range []string{fixture.executable, "--managed-root", fixture.managedRoot, "--data-root", fixture.dataRoot, "--workspace-root", fixture.worktree, `"agent": "maestro-hub"`} {
		if !strings.Contains(settings, required) {
			t.Fatalf("settings missing %q:\n%s", required, settings)
		}
	}
	hub := readFile(t, filepath.Join(fixture.worktree, ".claude", "agents", "maestro-hub.md"))
	if !strings.Contains(hub, "skills:\n  - maestro-operator") || strings.Contains(strings.Split(hub, "---\n")[1], "tools:") {
		t.Fatalf("direct native Hub contract is incomplete: %s", hub)
	}
	if _, err := os.Stat(filepath.Join(fixture.worktree, ".claude", "skills", "maestro-operator", "SKILL.md")); err != nil {
		t.Fatalf("preloaded canonical operator is not projected: %v", err)
	}

	exclude := readFile(t, filepath.Join(fixture.gitCommonDir, "info", "exclude"))
	for _, required := range []string{"/.claude/settings.local.json", "/.claude/agents/maestro-hub.md", "/.bcgos/workspace-projections/claude.json"} {
		if !strings.Contains(exclude, required) {
			t.Fatalf("Git exclude missing %q:\n%s", required, exclude)
		}
	}
	if dirty := strings.TrimSpace(runGit(t, fixture.worktree, "status", "--porcelain")); dirty != "" {
		t.Fatalf("machine-local projection is visible to Git: %s", dirty)
	}
}

func TestStatusRequiresRepairWhenDirectNativeHubIsMissing(t *testing.T) {
	fixture := newFixture(t)
	if _, err := fixture.manager.Enroll(context.Background(), "claude", fixture.worktree); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(fixture.worktree, ".claude", "agents", "maestro-hub.md")); err != nil {
		t.Fatal(err)
	}
	status, err := fixture.manager.Status(context.Background(), "claude", fixture.worktree)
	if err != nil || status.State != StateRepairNeeded {
		t.Fatalf("missing direct native Hub status = %+v, %v", status, err)
	}
	repaired, err := fixture.manager.Repair(context.Background(), "claude", fixture.worktree)
	if err != nil || repaired.State != StateEnrolled {
		t.Fatalf("repair missing direct native Hub = %+v, %v", repaired, err)
	}
	if _, err := os.Stat(filepath.Join(fixture.worktree, ".claude", "agents", "maestro-hub.md")); err != nil {
		t.Fatalf("repair did not restore direct native Hub: %v", err)
	}
	oldManaged := "---\nname: maestro-hub\ndescription: old managed frontend\n---\n<!-- BCGOS:MANAGED-CLAUDE-AGENT -->\nold contract\n"
	if err := os.WriteFile(filepath.Join(fixture.worktree, ".claude", "agents", "maestro-hub.md"), []byte(oldManaged), 0o600); err != nil {
		t.Fatal(err)
	}
	status, err = fixture.manager.Status(context.Background(), "claude", fixture.worktree)
	if err != nil || status.State != StateRepairNeeded {
		t.Fatalf("outdated managed native Hub status = %+v, %v", status, err)
	}
	if repaired, err = fixture.manager.Repair(context.Background(), "claude", fixture.worktree); err != nil || repaired.State != StateEnrolled {
		t.Fatalf("repair outdated direct native Hub = %+v, %v", repaired, err)
	}
}

func TestEnrollPreservesTrackedOrientationByteForByteAndKeepsGitClean(t *testing.T) {
	fixture := newFixture(t)
	tracked := "# Team-owned Claude instructions\n\nNever rewrite this tracked file.\n"
	writeAndCommit(t, fixture.worktree, "CLAUDE.md", tracked)
	headBefore := strings.TrimSpace(runGit(t, fixture.worktree, "rev-parse", "HEAD"))
	if _, err := fixture.manager.Enroll(context.Background(), "claude", fixture.worktree); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(fixture.worktree, "CLAUDE.md")); got != tracked {
		t.Fatalf("tracked orientation changed:\n%s", got)
	}
	var projectionMap map[string]any
	if err := json.Unmarshal([]byte(readFile(t, scopedRuntimeManifestPath(t, fixture.worktree, "claude"))), &projectionMap); err != nil {
		t.Fatal(err)
	}
	if projectionMap["orientation_mode"] != runtimeprojection.OrientationModePreservedTracked {
		t.Fatalf("orientation mode = %v", projectionMap["orientation_mode"])
	}
	if exclude := readFile(t, filepath.Join(fixture.gitCommonDir, "info", "exclude")); strings.Contains(exclude, "/CLAUDE.md") {
		t.Fatalf("tracked user orientation was added to local excludes:\n%s", exclude)
	}
	if dirty := strings.TrimSpace(runGit(t, fixture.worktree, "status", "--porcelain")); dirty != "" {
		t.Fatalf("tracked orientation enrollment dirtied Git: %s", dirty)
	}
	if got := strings.TrimSpace(runGit(t, fixture.worktree, "rev-parse", "HEAD")); got != headBefore {
		t.Fatalf("HEAD changed: %s != %s", got, headBefore)
	}
	if _, err := fixture.manager.Remove(context.Background(), "claude", fixture.worktree); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(fixture.worktree, "CLAUDE.md")); got != tracked {
		t.Fatal("tracked orientation changed during removal")
	}
	if dirty := strings.TrimSpace(runGit(t, fixture.worktree, "status", "--porcelain")); dirty != "" {
		t.Fatalf("tracked orientation removal dirtied Git: %s", dirty)
	}
}

func TestEnrollCodexProjectsFiveEventsAndPreservesAgentsInstructions(t *testing.T) {
	fixture := newFixture(t)
	userOrientation := "# Existing Codex instructions\n"
	if err := os.WriteFile(filepath.Join(fixture.worktree, "AGENTS.md"), []byte(userOrientation), 0o600); err != nil {
		t.Fatal(err)
	}
	status, err := fixture.manager.Enroll(context.Background(), "codex", fixture.worktree)
	if err != nil {
		t.Fatal(err)
	}
	if status.State != StateEnrolled {
		t.Fatalf("unexpected status: %+v", status)
	}
	orientation := readFile(t, filepath.Join(fixture.worktree, "AGENTS.md"))
	if !strings.HasPrefix(orientation, userOrientation) || !strings.Contains(orientation, ManagedBlockStart) {
		t.Fatalf("AGENTS.md was not preserved:\n%s", orientation)
	}
	hooks := readHooks(t, filepath.Join(fixture.worktree, ".codex", "hooks.json"))
	wantEvents := []string{"PostToolUse", "PreToolUse", "SessionStart", "Stop", "UserPromptSubmit"}
	if got := sortedKeys(hooks); !reflect.DeepEqual(got, wantEvents) {
		t.Fatalf("Codex events = %v, want %v", got, wantEvents)
	}
}

func TestStatusIgnoresUserOwnedHooksWhenValidatingManagedAuthorities(t *testing.T) {
	for _, runtimeName := range []string{"claude", "codex"} {
		t.Run(runtimeName, func(t *testing.T) {
			fixture := newFixture(t)
			if _, err := fixture.manager.Enroll(context.Background(), runtimeName, fixture.worktree); err != nil {
				t.Fatal(err)
			}

			configPath := runtimeConfigPath(runtimeName, fixture.worktree)
			var config map[string]any
			if err := json.Unmarshal([]byte(readFile(t, configPath)), &config); err != nil {
				t.Fatal(err)
			}
			hooks := config["hooks"].(map[string]any)
			groups := hooks["PreToolUse"].([]any)
			hooks["PreToolUse"] = append(groups, map[string]any{
				"matcher": "Bash",
				"hooks": []any{map[string]any{
					"type":    "command",
					"command": "printf 'user-owned hook\\n'",
				}},
			})
			if err := writeJSONAtomic(configPath, config, 0o600); err != nil {
				t.Fatal(err)
			}

			status, err := fixture.manager.Status(context.Background(), runtimeName, fixture.worktree)
			if err != nil || status.State != StateEnrolled {
				t.Fatalf("Status() = %+v, %v; want enrolled with user-owned hook preserved", status, err)
			}
		})
	}
}

func TestEnrollRefusesTrackedLocalConfigurationBeforeAnyWrite(t *testing.T) {
	fixture := newFixture(t)
	configPath := filepath.Join(fixture.worktree, ".codex", "hooks.json")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o700); err != nil {
		t.Fatal(err)
	}
	original := "{\"hooks\":{}}\n"
	if err := os.WriteFile(configPath, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	runGit(t, fixture.worktree, "add", ".codex/hooks.json")
	runGit(t, fixture.worktree, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "tracked config")

	_, err := fixture.manager.Enroll(context.Background(), "codex", fixture.worktree)
	if err == nil || !strings.Contains(err.Error(), "tracked runtime configuration") || !strings.Contains(err.Error(), "no source code or work was lost") {
		t.Fatalf("Enroll() error = %v", err)
	}
	if got := readFile(t, configPath); got != original {
		t.Fatalf("tracked config changed: %q", got)
	}
	if _, statErr := os.Stat(filepath.Join(fixture.worktree, manifestRelativePath("codex"))); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("manifest was written despite preflight failure: %v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(fixture.gitCommonDir, "info", "maestro-workspace-projection.lock")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("transaction lock was written despite preflight failure: %v", statErr)
	}
}

func TestEnrollRefusesToAdoptIdenticalTrackedGeneratedSkill(t *testing.T) {
	fixture := newFixture(t)
	body, err := baseskills.Skill("dream-memory")
	if err != nil {
		t.Fatal(err)
	}
	relative := filepath.Join(".codex", "skills", "dream-memory", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(filepath.Join(fixture.worktree, relative)), 0o700); err != nil {
		t.Fatal(err)
	}
	writeAndCommit(t, fixture.worktree, relative, string(body))
	if _, err := fixture.manager.Enroll(context.Background(), "codex", fixture.worktree); err == nil || !strings.Contains(err.Error(), "tracked") {
		t.Fatalf("identical tracked skill enrollment error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(fixture.worktree, manifestRelativePath("codex"))); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("projection manifest was written after tracked-skill refusal: %v", err)
	}
	if _, err := os.Stat(filepath.Join(fixture.gitCommonDir, "info", "maestro-workspace-projection.lock")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("transaction lock was written after tracked-skill refusal: %v", err)
	}
}

func TestLifecycleIsIdempotent(t *testing.T) {
	fixture := newFixture(t)
	ctx := context.Background()
	first, err := fixture.manager.Enroll(ctx, "claude", fixture.worktree)
	if err != nil {
		t.Fatal(err)
	}
	before := captureProjectionBytes(t, fixture, "claude")
	second, err := fixture.manager.Enroll(ctx, "claude", fixture.worktree)
	if err != nil {
		t.Fatal(err)
	}
	if first.RepositoryID != second.RepositoryID || first.WorkspaceID != second.WorkspaceID {
		t.Fatalf("identity changed: first=%+v second=%+v", first, second)
	}
	after := captureProjectionBytes(t, fixture, "claude")
	if !reflect.DeepEqual(before, after) {
		t.Fatal("second enroll changed an intact projection")
	}
	for index := 0; index < 2; index++ {
		status, err := fixture.manager.Status(ctx, "claude", fixture.worktree)
		if err != nil || status.State != StateEnrolled {
			t.Fatalf("Status() = %+v, %v", status, err)
		}
		repaired, err := fixture.manager.Repair(ctx, "claude", fixture.worktree)
		if err != nil || repaired.State != StateEnrolled {
			t.Fatalf("Repair() = %+v, %v", repaired, err)
		}
	}
	removed, err := fixture.manager.Remove(ctx, "claude", fixture.worktree)
	if err != nil || removed.State != StateRemoved {
		t.Fatalf("Remove() = %+v, %v", removed, err)
	}
	removedAgain, err := fixture.manager.Remove(ctx, "claude", fixture.worktree)
	if err != nil || removedAgain.State != StateAbsent {
		t.Fatalf("second Remove() = %+v, %v", removedAgain, err)
	}
}

func TestConcurrentEnrollSerializesSharedGitAndProjectionState(t *testing.T) {
	fixture := newFixture(t)
	var wait sync.WaitGroup
	errorsSeen := make(chan error, 2)
	for index := 0; index < 2; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			status, err := fixture.manager.Enroll(context.Background(), "codex", fixture.worktree)
			if err == nil && status.State != StateEnrolled {
				err = errors.New("concurrent enrollment did not finish enrolled")
			}
			errorsSeen <- err
		}()
	}
	wait.Wait()
	close(errorsSeen)
	for err := range errorsSeen {
		if err != nil {
			t.Fatal(err)
		}
	}
	exclude := readFile(t, filepath.Join(fixture.gitCommonDir, "info", "exclude"))
	if strings.Count(exclude, "/.codex/hooks.json") != 1 {
		t.Fatalf("concurrent enrollment duplicated shared excludes:\n%s", exclude)
	}
}

func TestClaudeAndCodexCoexistAndRemoveIndependently(t *testing.T) {
	fixture := newFixture(t)
	ctx := context.Background()
	for _, runtimeName := range []string{"claude", "codex"} {
		status, err := fixture.manager.Enroll(ctx, runtimeName, fixture.worktree)
		if err != nil || status.State != StateEnrolled {
			t.Fatalf("Enroll(%s) = %+v, %v", runtimeName, status, err)
		}
	}
	for _, runtimeName := range []string{"claude", "codex"} {
		status, err := fixture.manager.Status(ctx, runtimeName, fixture.worktree)
		if err != nil || status.State != StateEnrolled {
			t.Fatalf("Status(%s) = %+v, %v", runtimeName, status, err)
		}
		if _, err := os.Stat(filepath.Join(fixture.worktree, manifestRelativePath(runtimeName))); err != nil {
			t.Fatalf("runtime-scoped %s manifest missing: %v", runtimeName, err)
		}
	}
	if dirty := strings.TrimSpace(runGit(t, fixture.worktree, "status", "--porcelain")); dirty != "" {
		t.Fatalf("co-resident projections dirtied Git: %s", dirty)
	}
	if removed, err := fixture.manager.Remove(ctx, "claude", fixture.worktree); err != nil || removed.State != StateRemoved {
		t.Fatalf("Remove(claude) = %+v, %v", removed, err)
	}
	if status, err := fixture.manager.Status(ctx, "codex", fixture.worktree); err != nil || status.State != StateEnrolled {
		t.Fatalf("Codex did not survive Claude removal: %+v, %v", status, err)
	}
	if _, err := os.Stat(filepath.Join(fixture.worktree, ".bcgos", "maestro-orchestration-state.json")); err != nil {
		t.Fatalf("shared orchestration state was removed with Claude: %v", err)
	}
	if removed, err := fixture.manager.Remove(ctx, "codex", fixture.worktree); err != nil || removed.State != StateRemoved {
		t.Fatalf("Remove(codex) = %+v, %v", removed, err)
	}
	if _, err := os.Stat(filepath.Join(fixture.worktree, ".bcgos", "maestro-orchestration-state.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("shared orchestration state remained after final runtime removal: %v", err)
	}
	if dirty := strings.TrimSpace(runGit(t, fixture.worktree, "status", "--porcelain")); dirty != "" {
		t.Fatalf("independent removal dirtied Git: %s", dirty)
	}
}

func TestLinkedWorktreesShareRepositoryIdentityAndKeepWorkspaceIdentityDistinct(t *testing.T) {
	t.Setenv("GIT_INDEX_FILE", ".git/index")
	fixture := newFixture(t)
	writeAndCommit(t, fixture.worktree, "README.md", "fixture\n")
	branchBefore := strings.TrimSpace(runGit(t, fixture.worktree, "symbolic-ref", "--short", "HEAD"))
	headBefore := strings.TrimSpace(runGit(t, fixture.worktree, "rev-parse", "HEAD"))
	second := filepath.Join(filepath.Dir(fixture.worktree), "second-worktree")
	runGit(t, fixture.worktree, "worktree", "add", "-b", "second-worktree", second)
	if info, err := os.Stat(filepath.Join(second, ".git")); err != nil || !info.Mode().IsRegular() {
		t.Fatalf("linked worktree .git is not a file: info=%v err=%v", info, err)
	}
	first, err := fixture.manager.Enroll(context.Background(), "codex", fixture.worktree)
	if err != nil {
		t.Fatal(err)
	}
	secondStatus, err := fixture.manager.Enroll(context.Background(), "codex", second)
	if err != nil {
		t.Fatal(err)
	}
	if first.RepositoryID != secondStatus.RepositoryID {
		t.Fatalf("repository IDs differ: %s != %s", first.RepositoryID, secondStatus.RepositoryID)
	}
	if first.WorkspaceID == secondStatus.WorkspaceID {
		t.Fatalf("workspace IDs are equal: %s", first.WorkspaceID)
	}
	var firstManifest, secondManifest projectionManifest
	if err := readJSONStrict(filepath.Join(fixture.worktree, manifestRelativePath("codex")), &firstManifest); err != nil {
		t.Fatal(err)
	}
	if err := readJSONStrict(filepath.Join(second, manifestRelativePath("codex")), &secondManifest); err != nil {
		t.Fatal(err)
	}
	if firstManifest.RepositoryRoot != firstManifest.WorktreeRoot || secondManifest.RepositoryRoot != firstManifest.WorktreeRoot || secondManifest.WorktreeRoot == firstManifest.WorktreeRoot {
		t.Fatalf("repository/worktree roots were conflated: first=%+v second=%+v", firstManifest, secondManifest)
	}
	common := strings.TrimSpace(runGit(t, second, "rev-parse", "--path-format=absolute", "--git-common-dir"))
	exclude := readFile(t, filepath.Join(common, "info", "exclude"))
	if strings.Count(exclude, "/.codex/hooks.json") != 1 {
		t.Fatalf("shared exclude is not idempotent:\n%s", exclude)
	}
	if _, err := fixture.manager.Remove(context.Background(), "codex", fixture.worktree); err != nil {
		t.Fatal(err)
	}
	afterFirstRemoval := readFile(t, filepath.Join(common, "info", "exclude"))
	if !strings.Contains(afterFirstRemoval, "/.codex/hooks.json") {
		t.Fatal("shared Git exclude was removed while another worktree still referenced it")
	}
	if _, err := fixture.manager.Remove(context.Background(), "codex", second); err != nil {
		t.Fatal(err)
	}
	afterSecondRemoval := readFile(t, filepath.Join(common, "info", "exclude"))
	if strings.Contains(afterSecondRemoval, "/.codex/hooks.json") {
		t.Fatal("last worktree removal left its exact Git exclude behind")
	}
	if got := strings.TrimSpace(runGit(t, fixture.worktree, "symbolic-ref", "--short", "HEAD")); got != branchBefore {
		t.Fatalf("branch changed: %s != %s", got, branchBefore)
	}
	if got := strings.TrimSpace(runGit(t, fixture.worktree, "rev-parse", "HEAD")); got != headBefore {
		t.Fatalf("HEAD changed: %s != %s", got, headBefore)
	}
	if dirty := strings.TrimSpace(runGit(t, fixture.worktree, "status", "--porcelain")); dirty != "" {
		t.Fatalf("worktree dirty after lifecycle: %s", dirty)
	}
}

func TestTraversalAndSymlinkTargetsAreRejected(t *testing.T) {
	fixture := newFixture(t)
	traversal := fixture.worktree + string(filepath.Separator) + ".." + string(filepath.Separator) + filepath.Base(fixture.worktree)
	if _, err := fixture.manager.Enroll(context.Background(), "claude", traversal); err == nil || !strings.Contains(err.Error(), "traversal") {
		t.Fatalf("traversal error = %v", err)
	}
	link := filepath.Join(filepath.Dir(fixture.worktree), "repository-link")
	if err := os.Symlink(fixture.worktree, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := fixture.manager.Enroll(context.Background(), "claude", link); err == nil || !strings.Contains(err.Error(), "real directory") {
		t.Fatalf("symlink error = %v", err)
	}
}

func TestEnrollmentRejectsSymlinkedGitInfoBeforeLock(t *testing.T) {
	fixture := newFixture(t)
	infoRoot := filepath.Join(fixture.gitCommonDir, "info")
	if err := os.Remove(filepath.Join(infoRoot, "exclude")); err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	if err := os.Remove(infoRoot); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside-info")
	if err := os.MkdirAll(outside, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, infoRoot); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	if _, err := fixture.manager.Enroll(context.Background(), "codex", fixture.worktree); err == nil || !strings.Contains(err.Error(), "Git info directory") {
		t.Fatalf("Enroll() error = %v, want Git info symlink refusal", err)
	}
	if _, err := os.Stat(filepath.Join(outside, "maestro-workspace-projection.lock")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("external transaction lock was written through Git info symlink: %v", err)
	}
	if _, err := os.Stat(filepath.Join(fixture.worktree, manifestRelativePath("codex"))); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("projection manifest was written after Git info refusal: %v", err)
	}
}

func TestPrivateBindingParentSymlinkIsRejectedBeforeProjectionWrites(t *testing.T) {
	fixture := newFixture(t)
	outside := filepath.Join(filepath.Dir(fixture.dataRoot), "outside-private")
	if err := os.MkdirAll(outside, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(fixture.dataRoot, "workspaces")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := fixture.manager.Enroll(context.Background(), "claude", fixture.worktree); err == nil || !strings.Contains(err.Error(), "parent") {
		t.Fatalf("private parent symlink error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(fixture.worktree, manifestRelativePath("claude"))); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("projection manifest was written through unsafe private state: %v", err)
	}
}

func TestRemoteURLNeverParticipatesInIdentityOrReceipts(t *testing.T) {
	fixture := newFixture(t)
	remoteOne := "https://client-name.example.invalid/secret-repository.git"
	runGit(t, fixture.worktree, "remote", "add", "origin", remoteOne)
	enrolled, err := fixture.manager.Enroll(context.Background(), "claude", fixture.worktree)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := fixture.manager.resolveGitIdentity(context.Background(), fixture.worktree)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		filepath.Join(fixture.worktree, manifestRelativePath("claude")),
		fixture.manager.bindingPath(fixture.dataRoot, identity.WorkspaceID, "claude"),
	} {
		if strings.Contains(readFile(t, path), remoteOne) {
			t.Fatalf("remote URL leaked into %s", path)
		}
	}
	receipt, err := json.Marshal(enrolled)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(receipt), remoteOne) {
		t.Fatal("remote URL leaked into enrollment receipt")
	}
	runGit(t, fixture.worktree, "remote", "set-url", "origin", "ssh://different.example.invalid/renamed.git")
	status, err := fixture.manager.Status(context.Background(), "claude", fixture.worktree)
	if err != nil || status.RepositoryID != enrolled.RepositoryID || status.WorkspaceID != enrolled.WorkspaceID {
		t.Fatalf("remote change affected local identity: enrolled=%+v status=%+v err=%v", enrolled, status, err)
	}
}

func TestMovedManagedRootRequiresAndAcceptsExplicitRepair(t *testing.T) {
	fixture := newFixture(t)
	if _, err := fixture.manager.Enroll(context.Background(), "claude", fixture.worktree); err != nil {
		t.Fatal(err)
	}
	movedRoot := filepath.Join(filepath.Dir(fixture.managedRoot), "Maestro-moved")
	movedExecutable := filepath.Join(movedRoot, "bin", executableName())
	if err := os.Rename(fixture.managedRoot, movedRoot); err != nil {
		t.Fatal(err)
	}
	moved := Manager{ManagedRoot: movedRoot, DataRoot: fixture.dataRoot, Executable: movedExecutable}
	status, err := moved.Status(context.Background(), "claude", fixture.worktree)
	if err != nil || status.State != StateRepairNeeded || status.Reason != "managed_root_moved" {
		t.Fatalf("moved Status() = %+v, %v", status, err)
	}
	if _, err := moved.Repair(context.Background(), "claude", fixture.worktree); err != nil {
		t.Fatal(err)
	}
	settings := readFile(t, filepath.Join(fixture.worktree, ".claude", "settings.local.json"))
	if !strings.Contains(settings, movedExecutable) || strings.Contains(settings, fixture.executable) {
		t.Fatalf("repair did not replace the intact absolute pointer:\n%s", settings)
	}
}

func TestIntactPriorProjectionCanBeExplicitlyRepairedForUpdatedCore(t *testing.T) {
	fixture := newFixture(t)
	if _, err := fixture.manager.Enroll(context.Background(), "codex", fixture.worktree); err != nil {
		t.Fatal(err)
	}
	runtimeManifestPath := scopedRuntimeManifestPath(t, fixture.worktree, "codex")
	var prior map[string]any
	if err := json.Unmarshal([]byte(readFile(t, runtimeManifestPath)), &prior); err != nil {
		t.Fatal(err)
	}
	priorBody := []byte("# intact body from the previously trusted core\n")
	priorSum := sha256.Sum256(priorBody)
	skillHashes, ok := prior["skill_hashes"].(map[string]any)
	if !ok {
		t.Fatal("runtime projection manifest has no skill_hashes")
	}
	skillHashes["dream-memory"] = hex.EncodeToString(priorSum[:])
	if err := writeJSONAtomic(runtimeManifestPath, prior, 0o600); err != nil {
		t.Fatal(err)
	}
	priorSkill := filepath.Join(fixture.worktree, ".codex", "skills", "dream-memory", "SKILL.md")
	if err := os.WriteFile(priorSkill, priorBody, 0o600); err != nil {
		t.Fatal(err)
	}
	identity, err := fixture.manager.resolveGitIdentity(context.Background(), fixture.worktree)
	if err != nil {
		t.Fatal(err)
	}
	bindingPath := fixture.manager.bindingPath(fixture.dataRoot, identity.WorkspaceID, "codex")
	var binding privateBinding
	if err := readJSONStrict(bindingPath, &binding); err != nil {
		t.Fatal(err)
	}
	binding.ProjectionDigest, err = digestRegularFile(runtimeManifestPath, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeJSONAtomic(bindingPath, binding, 0o600); err != nil {
		t.Fatal(err)
	}

	status, err := fixture.manager.Status(context.Background(), "codex", fixture.worktree)
	if err != nil || status.State != StateRepairNeeded || status.Reason != "managed_core_updated" {
		t.Fatalf("Status() = %+v, %v", status, err)
	}
	if _, err := fixture.manager.Repair(context.Background(), "codex", fixture.worktree); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, priorSkill); got == string(priorBody) {
		t.Fatal("repair did not replace the intact prior managed skill")
	}
	status, err = fixture.manager.Status(context.Background(), "codex", fixture.worktree)
	if err != nil || status.State != StateEnrolled {
		t.Fatalf("repaired Status() = %+v, %v", status, err)
	}
}

func TestPartialFailureRollsBackProjectionPrivateBindingAndGitExclude(t *testing.T) {
	fixture := newFixture(t)
	excludePath := filepath.Join(fixture.gitCommonDir, "info", "exclude")
	originalExclude := readFile(t, excludePath)
	fixture.manager.FailurePoint = func(point string) error {
		if point == "after_manifest" {
			return errors.New("injected manifest failure")
		}
		return nil
	}
	_, err := fixture.manager.Enroll(context.Background(), "claude", fixture.worktree)
	if err == nil || !strings.Contains(err.Error(), "injected manifest failure") {
		t.Fatalf("Enroll() error = %v", err)
	}
	for _, path := range []string{
		filepath.Join(fixture.worktree, "CLAUDE.md"),
		filepath.Join(fixture.worktree, ".claude", "settings.local.json"),
		filepath.Join(fixture.worktree, manifestRelativePath("claude")),
	} {
		if _, statErr := os.Stat(path); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("transaction path remains after rollback: %s (%v)", path, statErr)
		}
	}
	if got := readFile(t, excludePath); got != originalExclude {
		t.Fatalf("Git exclude was not restored:\n%s", got)
	}
	identity, identityErr := fixture.manager.resolveGitIdentity(context.Background(), fixture.worktree)
	if identityErr != nil {
		t.Fatal(identityErr)
	}
	if _, statErr := os.Stat(fixture.manager.bindingPath(fixture.dataRoot, identity.WorkspaceID, "claude")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("private binding remains after rollback: %v", statErr)
	}
}

func TestClaudeAndCodexPreserveEquivalentRootAndIdentityInvariants(t *testing.T) {
	claudeFixture := newFixture(t)
	codexFixture := newFixture(t)
	claudeStatus, err := claudeFixture.manager.Enroll(context.Background(), "claude", claudeFixture.worktree)
	if err != nil {
		t.Fatal(err)
	}
	codexStatus, err := codexFixture.manager.Enroll(context.Background(), "codex", codexFixture.worktree)
	if err != nil {
		t.Fatal(err)
	}
	for _, pair := range []struct {
		fixture testFixture
		status  Status
		path    string
	}{
		{claudeFixture, claudeStatus, filepath.Join(claudeFixture.worktree, ".claude", "settings.local.json")},
		{codexFixture, codexStatus, filepath.Join(codexFixture.worktree, ".codex", "hooks.json")},
	} {
		if pair.status.RepositoryID == "" || pair.status.WorkspaceID == "" {
			t.Fatalf("missing opaque identity: %+v", pair.status)
		}
		body := readFile(t, pair.path)
		for _, invariant := range []string{"--managed-root", pair.fixture.managedRoot, "--data-root", pair.fixture.dataRoot, "--workspace-root", pair.fixture.worktree} {
			if !strings.Contains(body, invariant) {
				t.Fatalf("%s missing %q", pair.path, invariant)
			}
		}
	}
}

func TestModifiedManagedContentBlocksRepairAndRemoveWithoutOverwrite(t *testing.T) {
	fixture := newFixture(t)
	if _, err := fixture.manager.Enroll(context.Background(), "codex", fixture.worktree); err != nil {
		t.Fatal(err)
	}
	orientationPath := filepath.Join(fixture.worktree, "AGENTS.md")
	modified := strings.Replace(readFile(t, orientationPath), ManagedBlockStart, ManagedBlockStart+"\nuser changed the managed block", 1)
	if err := os.WriteFile(orientationPath, []byte(modified), 0o600); err != nil {
		t.Fatal(err)
	}
	status, err := fixture.manager.Status(context.Background(), "codex", fixture.worktree)
	if err != nil || status.State != StateConflict {
		t.Fatalf("Status() = %+v, %v", status, err)
	}
	if _, err := fixture.manager.Repair(context.Background(), "codex", fixture.worktree); err == nil {
		t.Fatal("Repair() accepted modified managed content")
	}
	if _, err := fixture.manager.Remove(context.Background(), "codex", fixture.worktree); err == nil {
		t.Fatal("Remove() accepted modified managed content")
	}
	if got := readFile(t, orientationPath); got != modified {
		t.Fatal("modified managed content was overwritten")
	}
}

func TestResolveEnrolledWorkspaceRevalidatesPrivateBinding(t *testing.T) {
	fixture := newFixture(t)
	target := filepath.Join(filepath.Dir(fixture.worktree), "second-repository")
	if err := os.MkdirAll(target, 0o700); err != nil {
		t.Fatal(err)
	}
	runGit(t, target, "init")
	enrolled, err := fixture.manager.Enroll(context.Background(), "claude", target)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := fixture.manager.ResolveEnrolledWorkspace(context.Background(), "claude", enrolled.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.State != StateEnrolled || resolved.WorkspaceID != enrolled.WorkspaceID || resolved.RepositoryID != enrolled.RepositoryID {
		t.Fatalf("resolved enrollment = %#v", resolved)
	}
	if _, err := fixture.manager.Remove(context.Background(), "claude", target); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.manager.ResolveEnrolledWorkspace(context.Background(), "claude", enrolled.WorkspaceID); err == nil {
		t.Fatal("removed target enrollment was still resolvable")
	}
}

type testFixture struct {
	manager      Manager
	managedRoot  string
	dataRoot     string
	executable   string
	worktree     string
	gitCommonDir string
}

func newFixture(t *testing.T) testFixture {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	managedRoot := filepath.Join(root, "Maestro", "managed")
	dataRoot := filepath.Join(root, "Maestro", "data")
	executable := filepath.Join(managedRoot, "bin", executableName())
	worktree := filepath.Join(root, "client-repository")
	for _, directory := range []string{filepath.Dir(executable), dataRoot, worktree} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(executable, []byte("test executable"), 0o700); err != nil {
		t.Fatal(err)
	}
	runGit(t, worktree, "init")
	gitCommonDir := strings.TrimSpace(runGit(t, worktree, "rev-parse", "--path-format=absolute", "--git-common-dir"))
	return testFixture{
		manager:      Manager{ManagedRoot: managedRoot, DataRoot: dataRoot, Executable: executable},
		managedRoot:  managedRoot,
		dataRoot:     dataRoot,
		executable:   executable,
		worktree:     worktree,
		gitCommonDir: gitCommonDir,
	}
}

func executableName() string {
	if filepath.Separator == '\\' {
		return "bcgos.exe"
	}
	return "bcgos"
}

func runGit(t *testing.T, directory string, args ...string) string {
	t.Helper()
	command := isolatedGitCommand(context.Background(), "git", append([]string{"-C", directory}, args...)...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v: %s", strings.Join(args, " "), err, output)
	}
	return string(output)
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func readHooks(t *testing.T, path string) map[string]any {
	t.Helper()
	var config map[string]any
	if err := json.Unmarshal([]byte(readFile(t, path)), &config); err != nil {
		t.Fatal(err)
	}
	hooks, ok := config["hooks"].(map[string]any)
	if !ok {
		t.Fatalf("hooks missing from %s", path)
	}
	return hooks
}

func sortedKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func writeAndCommit(t *testing.T, worktree, relative, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(worktree, relative), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	runGit(t, worktree, "add", relative)
	runGit(t, worktree, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "fixture")
}

func captureProjectionBytes(t *testing.T, fixture testFixture, runtimeName string) map[string]string {
	t.Helper()
	paths, err := runtimeprojection.PlannedScopedManagedPaths(runtimeName, fixture.worktree, nil)
	if err != nil {
		t.Fatal(err)
	}
	paths = append(paths, filepath.Join(fixture.worktree, manifestRelativePath(runtimeName)))
	result := map[string]string{}
	for _, path := range paths {
		body, err := os.ReadFile(path)
		if err == nil {
			result[path] = string(body)
		}
	}
	return result
}

func scopedRuntimeManifestPath(t *testing.T, workspace, runtimeName string) string {
	t.Helper()
	relative, err := runtimeprojection.ScopedManifestRelativePath(runtimeName)
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(workspace, relative)
}
