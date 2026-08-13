package gitguard

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestBlockedCommandRejectsDangerousGitOperations(t *testing.T) {
	commands := []string{
		"git push --force origin feature",
		"git push origin main",
		"git reset --hard HEAD~1",
		"git clean -fd",
		"git branch -D old-work",
		"git push origin --delete old-work",
		"gh pr merge 12 --squash",
	}
	for _, command := range commands {
		if _, recovery, blocked := BlockedCommand(command); !blocked || recovery == "" {
			t.Errorf("BlockedCommand(%q) = blocked %v, recovery %q", command, blocked, recovery)
		}
	}
}

func TestDoctorHandlesBareRepositoryWithoutTreatingItAsAWorktree(t *testing.T) {
	// Doctor spawns `git` subprocesses; when this test runs inside a git hook
	// (e.g. pre-commit) the inherited GIT_DIR/GIT_INDEX_FILE/GIT_WORK_TREE
	// point at the outer repository, so git ignores `command.Dir` and reports
	// on the parent worktree instead of the bare tempdir. Clear the env at the
	// test scope so Doctor's own exec calls also see a clean slate.
	clearGitHookEnvironment(t)
	root := t.TempDir()
	runGit(t, root, "init", "--bare")
	var output bytes.Buffer
	if err := Doctor(root, &output); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	if !strings.Contains(text, "repositorio bare") || !strings.Contains(text, "git worktree list") || !strings.Contains(text, "nenhum arquivo foi alterado") {
		t.Fatalf("Doctor() output = %s", text)
	}
}

func TestScanStagedFindsSecretInFilenameWithSpaces(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init")
	path := filepath.Join(root, "client notes.txt")
	fakeSecret := "ghp_" + strings.Repeat("1", 25)
	if err := os.WriteFile(path, []byte("token="+fakeSecret+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "add", "client notes.txt")
	err := scanStaged(root)
	if err == nil || !strings.Contains(err.Error(), "possivel segredo") {
		t.Fatalf("scanStaged() error = %v, want secret block", err)
	}
}

func runGit(t *testing.T, root string, args ...string) {
	t.Helper()
	clearGitHookEnvironment(t)
	command := exec.Command("git", args...)
	command.Dir = root
	for _, variable := range os.Environ() {
		if strings.HasPrefix(variable, "GIT_INDEX_FILE=") || strings.HasPrefix(variable, "GIT_DIR=") || strings.HasPrefix(variable, "GIT_WORK_TREE=") {
			continue
		}
		command.Env = append(command.Env, variable)
	}
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, output)
	}
}

func clearGitHookEnvironment(t *testing.T) {
	t.Helper()
	for _, name := range []string{"GIT_INDEX_FILE", "GIT_DIR", "GIT_WORK_TREE", "GIT_PREFIX", "GIT_CONFIG_PARAMETERS"} {
		value, exists := os.LookupEnv(name)
		if err := os.Unsetenv(name); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if exists {
				_ = os.Setenv(name, value)
				return
			}
			_ = os.Unsetenv(name)
		})
	}
}

func TestPrePushRejectsMainWithoutTouchingRepository(t *testing.T) {
	input := bufio.NewScanner(strings.NewReader("refs/heads/feature abc refs/heads/main def\n"))
	var output bytes.Buffer
	err := PrePush(t.TempDir(), input, &output)
	if err == nil || !strings.Contains(err.Error(), "Nada foi apagado") {
		t.Fatalf("PrePush() error = %v", err)
	}
}

func TestBlockedCommandAllowsNormalWorkflow(t *testing.T) {
	for _, command := range []string{"git status", "git switch -c feature/readme", "git push -u origin HEAD", "gh pr create --draft"} {
		if reason, _, blocked := BlockedCommand(command); blocked {
			t.Errorf("BlockedCommand(%q) blocked: %s", command, reason)
		}
	}
}

func TestClaudePreToolBlockExplainsRecovery(t *testing.T) {
	input := strings.NewReader(`{"tool_name":"Bash","tool_input":{"command":"git reset --hard HEAD"}}`)
	var output bytes.Buffer
	code, err := ClaudeHook(t.TempDir(), "pre-tool", input, &output)
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 || !strings.Contains(output.String(), `"permissionDecision":"deny"`) || !strings.Contains(output.String(), "Nada foi apagado") || !strings.Contains(output.String(), "recover") {
		t.Fatalf("code = %d, output = %s", code, output.String())
	}
}

func TestClaudeSessionStartInjectsPrimarySkillRouting(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init")
	writeClaudeManifest(t, root)
	var output bytes.Buffer
	code, err := ClaudeHook(root, "session-start", strings.NewReader(`{"session_id":"session-1"}`), &output)
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 || !strings.Contains(output.String(), "runtime principal") || !strings.Contains(output.String(), "$start-work") || !strings.Contains(output.String(), "$recover-work") {
		t.Fatalf("code = %d, output = %s", code, output.String())
	}
}

func TestClaudeMutationRequiresAndRecordsNativeSkill(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init")
	writeClaudeManifest(t, root)

	edit := `{"session_id":"session-1","tool_name":"Edit","tool_input":{"file_path":"README.md"}}`
	var blocked bytes.Buffer
	if _, err := ClaudeHook(root, "pre-tool", strings.NewReader(edit), &blocked); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(blocked.String(), `"permissionDecision":"deny"`) || !strings.Contains(blocked.String(), "$develop-change") {
		t.Fatalf("unrouted edit output = %s", blocked.String())
	}

	skill := `{"session_id":"session-1","tool_name":"Skill","tool_input":{"skill":"develop-change"}}`
	if _, err := ClaudeHook(root, "skill-used", strings.NewReader(skill), &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	var allowed bytes.Buffer
	if _, err := ClaudeHook(root, "pre-tool", strings.NewReader(edit), &allowed); err != nil {
		t.Fatal(err)
	}
	if allowed.Len() != 0 {
		t.Fatalf("active skill should allow edit, output = %s", allowed.String())
	}
}

func TestClaudeDecisionEditRequiresRecordDecision(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init")
	writeClaudeManifest(t, root)
	skill := `{"session_id":"session-1","tool_name":"Skill","tool_input":{"skill":"develop-change"}}`
	if _, err := ClaudeHook(root, "skill-used", strings.NewReader(skill), &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	edit := `{"session_id":"session-1","tool_name":"Edit","tool_input":{"file_path":"docs/decisions/decision-log.md"}}`
	var output bytes.Buffer
	if _, err := ClaudeHook(root, "pre-tool", strings.NewReader(edit), &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "$record-decision") {
		t.Fatalf("decision edit output = %s", output.String())
	}
}

func TestClaudeMemoryEditRequiresEvolveMemory(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init")
	writeClaudeManifest(t, root)
	develop := `{"session_id":"session-1","tool_name":"Skill","tool_input":{"skill":"develop-change"}}`
	if _, err := ClaudeHook(root, "skill-used", strings.NewReader(develop), &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	edit := `{"session_id":"session-1","tool_name":"Edit","tool_input":{"file_path":"internal/memory/policy.go"}}`
	var output bytes.Buffer
	if _, err := ClaudeHook(root, "pre-tool", strings.NewReader(edit), &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "$evolve-memory") {
		t.Fatalf("memory edit output = %s", output.String())
	}
}

func TestClaudeDirectSkillExpansionRecordsActivation(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init")
	writeClaudeManifest(t, root)
	input := `{"session_id":"session-1","command_name":"develop-change"}`
	var output bytes.Buffer
	if _, err := ClaudeHook(root, "prompt-expansion", strings.NewReader(input), &output); err != nil {
		t.Fatal(err)
	}
	if !claudeSkillActive(root, "session-1", "develop-change") || !strings.Contains(output.String(), "registrada como ativa") {
		t.Fatalf("direct skill expansion was not recorded: %s", output.String())
	}
}

func TestClaudeBashMutationCannotBypassDevelopChange(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init")
	writeClaudeManifest(t, root)
	start := `{"session_id":"session-1","tool_name":"Skill","tool_input":{"skill":"start-work"}}`
	if _, err := ClaudeHook(root, "skill-used", strings.NewReader(start), &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	mutation := `{"session_id":"session-1","tool_name":"Bash","tool_input":{"command":"printf x >> README.md"}}`
	var output bytes.Buffer
	if _, err := ClaudeHook(root, "pre-tool", strings.NewReader(mutation), &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "$develop-change") {
		t.Fatalf("Bash mutation bypassed develop-change: %s", output.String())
	}
}

func TestClaudeBashDecisionMutationRequiresRecordDecision(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init")
	writeClaudeManifest(t, root)
	develop := `{"session_id":"session-1","tool_name":"Skill","tool_input":{"skill":"develop-change"}}`
	if _, err := ClaudeHook(root, "skill-used", strings.NewReader(develop), &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	mutation := `{"session_id":"session-1","tool_name":"Bash","tool_input":{"command":"python update.py docs/decisions/decision-log.md"}}`
	var output bytes.Buffer
	if _, err := ClaudeHook(root, "pre-tool", strings.NewReader(mutation), &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "$record-decision") {
		t.Fatalf("Bash decision mutation bypassed record-decision: %s", output.String())
	}
}

func TestClaudeSkillStateWorksInGitWorktree(t *testing.T) {
	base := t.TempDir()
	main := filepath.Join(base, "main")
	if err := os.MkdirAll(main, 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, main, "init")
	runGit(t, main, "config", "user.name", "Test User")
	runGit(t, main, "config", "user.email", "test@example.com")
	writeClaudeManifest(t, main)
	if err := os.WriteFile(filepath.Join(main, "README.md"), []byte("test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, main, "add", ".")
	runGit(t, main, "commit", "-m", "initial")
	worktree := filepath.Join(base, "worktree")
	runGit(t, main, "worktree", "add", "-b", "feature/test", worktree)

	skill := `{"session_id":"session-1","tool_name":"Skill","tool_input":{"skill":"develop-change"}}`
	if _, err := ClaudeHook(worktree, "skill-used", strings.NewReader(skill), &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if !claudeSkillActive(worktree, "session-1", "develop-change") {
		t.Fatal("skill state was not resolved through the worktree git path")
	}
}

func TestNoGoWrapperFailsClosedWithoutRecommendingRepositorySkill(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(root, ".claude", "hooks", "run-dev-hook.sh"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(content)
	if !strings.Contains(text, "ESTADO NAO SUPORTADO") || strings.Contains(text, "Go ainda nao esta instalado: use $start-contributing") {
		t.Fatalf("no-Go recovery must fail closed and point outside the repository")
	}
}

func TestCanonicalSkillCommandsRequireAndAcceptOwningSkill(t *testing.T) {
	cases := []struct {
		skill   string
		command string
	}{
		{"start-contributing", "go run ./dev/harness setup"},
		{"start-work", "git pull --ff-only origin main"},
		{"develop-change", "go test ./..."},
		{"evolve-memory", "go test ./internal/memory"},
		{"record-decision", "go run ./dev/harness decision available ABCD"},
		{"prepare-pr", "git add README.md"},
		{"recover-work", "go run ./dev/harness recover"},
	}
	for index, test := range cases {
		t.Run(test.skill, func(t *testing.T) {
			root := t.TempDir()
			runGit(t, root, "init")
			writeClaudeManifest(t, root)
			sessionID := fmt.Sprintf("session-%d", index)
			input := fmt.Sprintf(`{"session_id":%q,"tool_name":"Bash","tool_input":{"command":%q}}`, sessionID, test.command)

			var blocked bytes.Buffer
			if _, err := ClaudeHook(root, "pre-tool", strings.NewReader(input), &blocked); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(blocked.String(), "$"+test.skill) {
				t.Fatalf("command %q should require $%s: %s", test.command, test.skill, blocked.String())
			}

			skillInput := fmt.Sprintf(`{"session_id":%q,"tool_name":"Skill","tool_input":{"skill_name":%q}}`, sessionID, test.skill)
			if _, err := ClaudeHook(root, "skill-used", strings.NewReader(skillInput), &bytes.Buffer{}); err != nil {
				t.Fatal(err)
			}
			var allowed bytes.Buffer
			if _, err := ClaudeHook(root, "pre-tool", strings.NewReader(input), &allowed); err != nil {
				t.Fatal(err)
			}
			if allowed.Len() != 0 {
				t.Fatalf("command %q should be allowed after $%s: %s", test.command, test.skill, allowed.String())
			}
		})
	}
}

func TestCanonicalDiagnosticCommandsRemainAvailableInOwningWorkflow(t *testing.T) {
	cases := []struct {
		skill   string
		command string
	}{
		{"start-work", "git status --short --branch"},
		{"recover-work", "git diff --stat"},
	}
	for index, test := range cases {
		t.Run(test.command, func(t *testing.T) {
			root := t.TempDir()
			runGit(t, root, "init")
			writeClaudeManifest(t, root)
			sessionID := fmt.Sprintf("diagnostic-%d", index)
			skillInput := fmt.Sprintf(`{"session_id":%q,"tool_name":"Skill","tool_input":{"skill_name":%q}}`, sessionID, test.skill)
			if _, err := ClaudeHook(root, "skill-used", strings.NewReader(skillInput), &bytes.Buffer{}); err != nil {
				t.Fatal(err)
			}
			input := fmt.Sprintf(`{"session_id":%q,"tool_name":"Bash","tool_input":{"command":%q}}`, sessionID, test.command)
			var output bytes.Buffer
			if _, err := ClaudeHook(root, "pre-tool", strings.NewReader(input), &output); err != nil {
				t.Fatal(err)
			}
			if output.Len() != 0 {
				t.Fatalf("diagnostic %q blocked in $%s: %s", test.command, test.skill, output.String())
			}
		})
	}
}

func writeClaudeManifest(t *testing.T, root string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `{"primary_runtime":"claude","canonical_root":"dev/skills","projection_root":".claude/skills","routes":{"onboard":"start-contributing","start":"start-work","develop":"develop-change","memory":"evolve-memory","decision":"record-decision","deliver":"prepare-pr","recover":"recover-work"},"golden_path":["start-contributing","start-work","develop-change","prepare-pr"],"fallback":"recover-work"}`
	if err := os.WriteFile(filepath.Join(root, ".claude", "skill-routing.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
}
