package gitguard

import (
	"bufio"
	"bytes"
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
	command := exec.Command("git", args...)
	command.Dir = root
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, output)
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
