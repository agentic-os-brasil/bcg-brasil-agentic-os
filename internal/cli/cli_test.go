package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	basememory "github.com/agentic-os-brasil/bcg-brasil-agentic-os/bundles/base/memory"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/agentidentity"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/lifecycle"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/memory"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ownerctx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/portableactivation"
)

func TestWorkspacePublicLifecycleEndToEnd(t *testing.T) {
	for _, runtimeName := range []string{"claude", "codex"} {
		t.Run(runtimeName, func(t *testing.T) {
			fixture := newCLIFixture(t)
			for _, operation := range []string{"enroll", "status", "repair"} {
				var out, errOut bytes.Buffer
				code := Run(workspaceArgs(fixture, operation, runtimeName), strings.NewReader(""), &out, &errOut)
				if code != ExitOK {
					t.Fatalf("%s exit=%d stderr=%s", operation, code, errOut.String())
				}
				if !strings.Contains(out.String(), `"state": "enrolled"`) {
					t.Fatalf("%s output=%s", operation, out.String())
				}
			}
			for index := 0; index < 2; index++ {
				var out, errOut bytes.Buffer
				code := Run(workspaceArgs(fixture, "remove", runtimeName), strings.NewReader(""), &out, &errOut)
				if code != ExitOK {
					t.Fatalf("remove %d exit=%d stderr=%s", index, code, errOut.String())
				}
				want := `"state": "removed"`
				if index == 1 {
					want = `"state": "absent"`
				}
				if !strings.Contains(out.String(), want) {
					t.Fatalf("remove %d output=%s", index, out.String())
				}
			}
		})
	}
}

func TestWorkspaceLifecycleRefusesMissingActivationBeforeProjectionWrite(t *testing.T) {
	fixture := newCLIFixture(t)
	if err := os.Remove(filepath.Join(fixture.dataRoot, portableactivation.StateRelativePath)); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	code := Run(workspaceArgs(fixture, "enroll", "claude"), strings.NewReader(""), &out, &errOut)
	if code != ExitFailure || !strings.Contains(errOut.String(), "activation is not verified") {
		t.Fatalf("enroll exit=%d output=%s stderr=%s", code, out.String(), errOut.String())
	}
	if _, err := os.Stat(filepath.Join(fixture.worktree, ".bcgos")); !os.IsNotExist(err) {
		t.Fatalf("projection was written without activation: %v", err)
	}
}

func TestHookSessionStartUsesScopedEnrollmentWithoutLeakingRoots(t *testing.T) {
	fixture := newCLIFixture(t)
	var setupOut, setupErr bytes.Buffer
	if code := Run(workspaceArgs(fixture, "enroll", "codex"), strings.NewReader(""), &setupOut, &setupErr); code != ExitOK {
		t.Fatalf("enroll exit=%d stderr=%s", code, setupErr.String())
	}
	var out, errOut bytes.Buffer
	args := []string{
		"hook", "session-start", "--runtime", "codex",
		"--adapter-source", "maestro",
		"--orchestration-state", ".bcgos/maestro-orchestration-state.json",
		"--managed-root", fixture.managedRoot,
		"--data-root", fixture.dataRoot,
		"--workspace-root", fixture.worktree,
		"--executable", fixture.executable,
	}
	if code := Run(args, strings.NewReader("{}"), &out, &errOut); code != ExitOK {
		t.Fatalf("hook exit=%d stderr=%s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "Maestro direct workspace is active") || !strings.Contains(out.String(), "workspace_id") {
		t.Fatalf("unexpected hook output: %s", out.String())
	}
	for _, private := range []string{fixture.managedRoot, fixture.dataRoot, fixture.worktree} {
		if strings.Contains(out.String(), private) {
			t.Fatalf("hook output leaked root %q: %s", private, out.String())
		}
	}
}

func TestDirectSessionStartUsesNativeClaudeHubAndReviewedOwnerContextForBothRuntimes(t *testing.T) {
	fixture := newCLIFixture(t)
	prepareReviewedOwnerContext(t, fixture.dataRoot)
	otherContext := filepath.Join(fixture.dataRoot, "workspaces", strings.Repeat("f", 32), "context")
	if err := os.MkdirAll(otherContext, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(otherContext, "session-context.md"), []byte("other-workspace-secret-sentinel"), 0o600); err != nil {
		t.Fatal(err)
	}

	startup := map[string]string{}
	for _, runtimeName := range []string{"claude", "codex"} {
		var out, errOut bytes.Buffer
		if code := Run(workspaceArgs(fixture, "enroll", runtimeName), strings.NewReader(""), &out, &errOut); code != ExitOK {
			t.Fatalf("%s enroll exit=%d stderr=%s", runtimeName, code, errOut.String())
		}
		out.Reset()
		errOut.Reset()
		if code := Run(hookArgs(fixture, runtimeName, "session-start"), strings.NewReader(`{"session_id":"owner-session"}`), &out, &errOut); code != ExitOK {
			t.Fatalf("%s SessionStart exit=%d stderr=%s", runtimeName, code, errOut.String())
		}
		startup[runtimeName] = out.String()
		for _, wanted := range []string{"Synthetic Owner", "Synthetic engineering role"} {
			if !strings.Contains(out.String(), wanted) {
				t.Fatalf("%s SessionStart omitted %q: %s", runtimeName, wanted, out.String())
			}
		}
		hostRuntime := map[string]string{"claude": "Claude Code", "codex": "Codex"}[runtimeName]
		if runtimeName == "claude" {
			for _, forbidden := range []string{
				"Maestro is the configured professional operating layer",
				"Use the installed CLI silently",
				"Both facts remain visible",
				"ONBOARDING AVAILABLE",
			} {
				if strings.Contains(out.String(), forbidden) {
					t.Fatalf("direct Claude SessionStart retained imperative policy %q: %s", forbidden, out.String())
				}
			}
			if !strings.Contains(out.String(), "Host runtime: "+hostRuntime) || !strings.Contains(out.String(), "Native frontend: maestro-hub") {
				t.Fatalf("direct Claude SessionStart omitted factual native Hub state: %s", out.String())
			}
		} else if !strings.Contains(out.String(), "Host runtime: "+hostRuntime) || !strings.Contains(out.String(), "Both facts remain visible") {
			t.Fatalf("%s SessionStart omitted transparent host identity: %s", runtimeName, out.String())
		}
		for _, forbidden := range []string{
			"personal-context-secret-sentinel",
			"other-workspace-secret-sentinel",
			fixture.dataRoot,
			fixture.managedRoot,
		} {
			if strings.Contains(out.String(), forbidden) {
				t.Fatalf("%s SessionStart exposed %q: %s", runtimeName, forbidden, out.String())
			}
		}

		out.Reset()
		errOut.Reset()
		prompt := `{"session_id":"owner-session","prompt":"continue"}`
		if code := Run(hookArgs(fixture, runtimeName, "context-injection"), strings.NewReader(prompt), &out, &errOut); code != ExitOK {
			t.Fatalf("%s UserPromptSubmit exit=%d stderr=%s", runtimeName, code, errOut.String())
		}
		if !strings.Contains(out.String(), "MAESTRO CONTEXT UPDATE") || strings.Contains(out.String(), "Synthetic Owner") || strings.Contains(out.String(), "Synthetic engineering role") {
			t.Fatalf("%s UserPromptSubmit repeated or lost bounded context: %s", runtimeName, out.String())
		}
	}
	if err := validateDirectStartupParity(startup, []string{"Synthetic Owner", "Synthetic engineering role"}); err != nil {
		t.Fatal(err)
	}
}

func TestDirectSessionStartInjectsGeneratedMemoryForBothRuntimes(t *testing.T) {
	fixture := newCLIFixture(t)
	var enrollment bytes.Buffer
	var errOut bytes.Buffer
	if code := Run(workspaceArgs(fixture, "enroll", "claude"), strings.NewReader(""), &enrollment, &errOut); code != ExitOK {
		t.Fatalf("Claude enroll exit=%d stderr=%s", code, errOut.String())
	}
	var enrolled struct {
		WorkspaceID string `json:"workspace_id"`
	}
	if err := json.Unmarshal(enrollment.Bytes(), &enrolled); err != nil {
		t.Fatal(err)
	}
	policy, err := basememory.Policy()
	if err != nil {
		t.Fatal(err)
	}
	runtimeConfig, err := basememory.Runtime()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	engine := memory.Engine{
		Root: fixture.dataRoot, Policy: policy, Budgets: runtimeConfig.ContextBudgets(),
		Synthesizer: fixedMemorySynthesizer("generated-memory-parity-sentinel"), SynthesizerID: "cli-test-synth-v1", Now: func() time.Time { return now },
	}
	if _, err := engine.Capture(memory.Capture{WorkspaceID: enrolled.WorkspaceID, RecordedAt: now, Kind: "test", Text: "bounded source", Sanitized: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.DreamDaily(context.Background(), enrolled.WorkspaceID, now); err != nil {
		t.Fatal(err)
	}

	startup := map[string]string{}
	for _, runtimeName := range []string{"claude", "codex"} {
		if runtimeName == "codex" {
			enrollment.Reset()
			errOut.Reset()
			if code := Run(workspaceArgs(fixture, "enroll", runtimeName), strings.NewReader(""), &enrollment, &errOut); code != ExitOK {
				t.Fatalf("Codex enroll exit=%d stderr=%s", code, errOut.String())
			}
		}
		var out bytes.Buffer
		errOut.Reset()
		if code := Run(hookArgs(fixture, runtimeName, "session-start"), strings.NewReader(`{"session_id":"memory-parity"}`), &out, &errOut); code != ExitOK {
			t.Fatalf("%s SessionStart exit=%d stderr=%s", runtimeName, code, errOut.String())
		}
		startup[runtimeName] = out.String()
		for _, wanted := range []string{"MAESTRO LOCAL MEMORY", "generated-memory-parity-sentinel"} {
			if !strings.Contains(out.String(), wanted) {
				t.Fatalf("%s SessionStart omitted %q: %s", runtimeName, wanted, out.String())
			}
		}

		out.Reset()
		errOut.Reset()
		if code := Run(hookArgs(fixture, runtimeName, "context-injection"), strings.NewReader(`{"session_id":"memory-parity","prompt":"continue"}`), &out, &errOut); code != ExitOK {
			t.Fatalf("%s UserPromptSubmit exit=%d stderr=%s", runtimeName, code, errOut.String())
		}
		if strings.Contains(out.String(), "MAESTRO LOCAL MEMORY") || strings.Contains(out.String(), "generated-memory-parity-sentinel") {
			t.Fatalf("%s UserPromptSubmit repeated generated memory: %s", runtimeName, out.String())
		}
	}
	if err := validateDirectStartupParity(startup, []string{"MAESTRO LOCAL MEMORY", "generated-memory-parity-sentinel"}); err != nil {
		t.Fatal(err)
	}
}

func TestDirectStartupParityRejectsOneSidedOwnerContext(t *testing.T) {
	contexts := map[string]string{
		"claude": "Maestro is the configured professional operating layer\nSynthetic Owner",
		"codex":  "Maestro is the configured professional operating layer",
	}
	if err := validateDirectStartupParity(contexts, []string{"Maestro is the configured professional operating layer", "Synthetic Owner"}); err == nil {
		t.Fatal("parity gate accepted owner context only for Claude")
	}
}

func TestDirectSessionStartSupportsReviewedPortableLegacyOwnerContext(t *testing.T) {
	fixture := newCLIFixture(t)
	prepareLegacyReviewedOwnerContext(t, fixture.dataRoot)
	for _, runtimeName := range []string{"claude", "codex"} {
		var out, errOut bytes.Buffer
		if code := Run(workspaceArgs(fixture, "enroll", runtimeName), strings.NewReader(""), &out, &errOut); code != ExitOK {
			t.Fatalf("%s enroll exit=%d stderr=%s", runtimeName, code, errOut.String())
		}
		out.Reset()
		errOut.Reset()
		if code := Run(hookArgs(fixture, runtimeName, "session-start"), strings.NewReader(`{"session_id":"legacy-owner"}`), &out, &errOut); code != ExitOK {
			t.Fatalf("%s legacy SessionStart exit=%d stderr=%s", runtimeName, code, errOut.String())
		}
		for _, wanted := range []string{"Portable Synthetic Owner", "Portable synthetic role"} {
			if !strings.Contains(out.String(), wanted) {
				t.Fatalf("%s legacy SessionStart omitted %q: %s", runtimeName, wanted, out.String())
			}
		}
		if runtimeName == "claude" {
			if !strings.Contains(out.String(), "Native frontend: maestro-hub") || strings.Contains(out.String(), "Maestro is the configured professional operating layer") {
				t.Fatalf("Claude legacy owner context used the wrong direct trust channel: %s", out.String())
			}
		} else if !strings.Contains(out.String(), "Maestro is the configured professional operating layer") {
			t.Fatalf("Codex legacy owner context omitted its operating orientation: %s", out.String())
		}
		if strings.Contains(out.String(), "portable-personal-secret") {
			t.Fatalf("%s legacy SessionStart leaked personal context: %s", runtimeName, out.String())
		}
	}
}

func TestDirectSessionStartKeepsFreshPortableScaffoldAvailableForBothRuntimes(t *testing.T) {
	fixture := newCLIFixture(t)
	prepareLegacyReviewedOwnerContext(t, fixture.dataRoot)
	registryPath := filepath.Join(fixture.dataRoot, "owner", "registry.json")
	registry, err := os.ReadFile(registryPath)
	if err != nil {
		t.Fatal(err)
	}
	registry = bytes.Replace(registry, []byte(`"initialized": true`), []byte(`"initialized": false`), 1)
	if err := os.WriteFile(registryPath, registry, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, runtimeName := range []string{"claude", "codex"} {
		var out, errOut bytes.Buffer
		if code := Run(workspaceArgs(fixture, "enroll", runtimeName), strings.NewReader(""), &out, &errOut); code != ExitOK {
			t.Fatalf("%s enroll exit=%d stderr=%s", runtimeName, code, errOut.String())
		}
		out.Reset()
		errOut.Reset()
		if code := Run(hookArgs(fixture, runtimeName, "session-start"), strings.NewReader(`{"session_id":"fresh-portable"}`), &out, &errOut); code != ExitOK {
			t.Fatalf("%s fresh portable SessionStart exit=%d output=%s stderr=%s", runtimeName, code, out.String(), errOut.String())
		}
		if runtimeName == "claude" {
			if !strings.Contains(out.String(), "Native frontend: maestro-hub") || strings.Contains(out.String(), "ONBOARDING AVAILABLE") {
				t.Fatalf("Claude fresh portable SessionStart used an imperative hook channel: %s", out.String())
			}
		} else if !strings.Contains(out.String(), "Maestro is the configured professional operating layer") {
			t.Fatalf("Codex fresh portable SessionStart omitted its operating orientation: %s", out.String())
		}
		if strings.Contains(out.String(), "Portable Synthetic Owner") {
			t.Fatalf("%s fresh portable SessionStart injected unconfirmed identity: %s", runtimeName, out.String())
		}
	}
}

func TestHookSessionStartInjectsManagedOrientationWhenCodexAgentsIsTracked(t *testing.T) {
	fixture := newCLIFixture(t)
	prepareReviewedOwnerContext(t, fixture.dataRoot)
	tracked := "# Team-owned Codex instructions\n\nKeep this file byte-for-byte.\n"
	if err := os.WriteFile(filepath.Join(fixture.worktree, "AGENTS.md"), []byte(tracked), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"-C", fixture.worktree, "config", "user.email", "test@example.invalid"},
		{"-C", fixture.worktree, "config", "user.name", "Test"},
		{"-C", fixture.worktree, "add", "AGENTS.md"},
		{"-C", fixture.worktree, "commit", "-m", "tracked orientation"},
	} {
		if output, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, output)
		}
	}
	var setupOut, setupErr bytes.Buffer
	if code := Run(workspaceArgs(fixture, "enroll", "codex"), strings.NewReader(""), &setupOut, &setupErr); code != ExitOK {
		t.Fatalf("enroll exit=%d stderr=%s", code, setupErr.String())
	}
	if body, err := os.ReadFile(filepath.Join(fixture.worktree, "AGENTS.md")); err != nil || string(body) != tracked {
		t.Fatalf("tracked AGENTS.md changed: body=%q err=%v", body, err)
	}

	var out, errOut bytes.Buffer
	if code := Run(hookArgs(fixture, "codex", "session-start"), strings.NewReader(`{"session_id":"tracked-session"}`), &out, &errOut); code != ExitOK {
		t.Fatalf("session start exit=%d stderr=%s", code, errOut.String())
	}
	for _, required := range []string{"MAESTRO WORKSPACE CONTEXT", "Configured layer: Maestro", "Host runtime: Codex"} {
		if !strings.Contains(out.String(), required) {
			t.Errorf("tracked orientation context is missing %q: %s", required, out.String())
		}
	}
	if strings.Contains(out.String(), "# Maestro — orientação operacional") {
		t.Fatalf("tracked Codex SessionStart repeated the complete orientation document: %s", out.String())
	}
	for _, private := range []string{fixture.managedRoot, fixture.dataRoot, fixture.worktree} {
		if strings.Contains(out.String(), private) {
			t.Fatalf("tracked orientation context leaked root %q", private)
		}
	}
	additional := hookAdditionalContext(t, out.Bytes())
	if len(additional) > 8<<10 {
		t.Fatalf("tracked Codex SessionStart context = %d bytes, want at most 8 KiB", len(additional))
	}
	ownerAt := strings.Index(additional, "Synthetic Owner")
	packetAt := strings.Index(additional, "Maestro bounded session context")
	if ownerAt < 0 || packetAt < 0 || ownerAt > packetAt {
		t.Fatalf("reviewed owner context must precede the pointer packet: %s", additional)
	}

	out.Reset()
	errOut.Reset()
	if code := Run(hookArgs(fixture, "codex", "context-injection"), strings.NewReader(`{"session_id":"tracked-session","prompt":"continue"}`), &out, &errOut); code != ExitOK {
		t.Fatalf("prompt hook exit=%d stderr=%s", code, errOut.String())
	}
	promptContext := hookAdditionalContext(t, out.Bytes())
	for _, repeated := range []string{"# Maestro — orientação operacional", "Synthetic Owner", "MAESTRO REVIEWED OWNER CONTEXT"} {
		if strings.Contains(promptContext, repeated) {
			t.Fatalf("tracked Codex UserPromptSubmit repeated %q: %s", repeated, promptContext)
		}
	}
}

func TestHookSessionStartDoesNotInjectTrackedClaudeOrientationIntoNativeHub(t *testing.T) {
	fixture := newCLIFixture(t)
	tracked := "# Team-owned Claude instructions\n\ntracked-orientation-policy-sentinel\n"
	if err := os.WriteFile(filepath.Join(fixture.worktree, "CLAUDE.md"), []byte(tracked), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"-C", fixture.worktree, "config", "user.email", "test@example.invalid"},
		{"-C", fixture.worktree, "config", "user.name", "Test"},
		{"-C", fixture.worktree, "add", "CLAUDE.md"},
		{"-C", fixture.worktree, "commit", "-m", "tracked Claude orientation"},
	} {
		if output, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, output)
		}
	}
	var setupOut, setupErr bytes.Buffer
	if code := Run(workspaceArgs(fixture, "enroll", "claude"), strings.NewReader(""), &setupOut, &setupErr); code != ExitOK {
		t.Fatalf("enroll exit=%d stderr=%s", code, setupErr.String())
	}
	if body, err := os.ReadFile(filepath.Join(fixture.worktree, "CLAUDE.md")); err != nil || string(body) != tracked {
		t.Fatalf("tracked CLAUDE.md changed: body=%q err=%v", body, err)
	}
	var out, errOut bytes.Buffer
	if code := Run(hookArgs(fixture, "claude", "session-start"), strings.NewReader(`{"session_id":"tracked-claude"}`), &out, &errOut); code != ExitOK {
		t.Fatalf("session start exit=%d stderr=%s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "Native frontend: maestro-hub") ||
		strings.Contains(out.String(), "tracked-orientation-policy-sentinel") ||
		strings.Contains(out.String(), "# Maestro — orientação operacional") ||
		strings.Contains(out.String(), "Use the installed CLI silently") {
		t.Fatalf("tracked Claude orientation leaked into the factual hook channel: %s", out.String())
	}
}

func TestHookInjectsOnlyWorkspaceScopedMemoryAndRejectsTraversal(t *testing.T) {
	fixture := newCLIFixture(t)
	var setupOut, setupErr bytes.Buffer
	if code := Run(workspaceArgs(fixture, "enroll", "claude"), strings.NewReader(""), &setupOut, &setupErr); code != ExitOK {
		t.Fatalf("enroll exit=%d stderr=%s", code, setupErr.String())
	}
	var enrolled struct {
		WorkspaceID string `json:"workspace_id"`
	}
	if err := json.Unmarshal(setupOut.Bytes(), &enrolled); err != nil {
		t.Fatal(err)
	}
	memoryPath := filepath.Join(fixture.dataRoot, "workspaces", enrolled.WorkspaceID, "memory", "session-context.md")
	if err := os.MkdirAll(filepath.Dir(memoryPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(memoryPath, []byte("workspace-scoped-memory-sentinel"), 0o600); err != nil {
		t.Fatal(err)
	}
	baseArgs := []string{
		"hook", "claude", "session-start",
		"--adapter-source", "maestro",
		"--orchestration-state", ".bcgos/maestro-orchestration-state.json",
		"--managed-root", fixture.managedRoot,
		"--data-root", fixture.dataRoot,
		"--workspace-root", fixture.worktree,
		"--executable", fixture.executable,
	}
	var out, errOut bytes.Buffer
	if code := Run(baseArgs, strings.NewReader("{}"), &out, &errOut); code != ExitOK || !strings.Contains(out.String(), "workspace-scoped-memory-sentinel") {
		t.Fatalf("SessionStart exit=%d output=%s stderr=%s", code, out.String(), errOut.String())
	}

	guardArgs := append([]string{}, baseArgs...)
	guardArgs[2] = "pre-action-guard"
	out.Reset()
	errOut.Reset()
	payload := `{"tool_name":"Read","tool_input":{"file_path":"../outside.txt"}}`
	if code := Run(guardArgs, strings.NewReader(payload), &out, &errOut); code != ExitOK || !strings.Contains(out.String(), `"permissionDecision": "deny"`) || !strings.Contains(out.String(), "traversal") {
		t.Fatalf("PreToolUse exit=%d output=%s stderr=%s", code, out.String(), errOut.String())
	}
	outside := filepath.Join(filepath.Dir(fixture.worktree), "outside")
	if err := os.MkdirAll(outside, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(fixture.worktree, "linked-outside")
	if err := os.Symlink(outside, link); err == nil {
		out.Reset()
		errOut.Reset()
		payload = `{"tool_name":"Read","tool_input":{"file_path":"linked-outside/secret.txt"}}`
		if code := Run(guardArgs, strings.NewReader(payload), &out, &errOut); code != ExitOK || !strings.Contains(out.String(), `"permissionDecision": "deny"`) || !strings.Contains(out.String(), "symlink") {
			t.Fatalf("symlink PreToolUse exit=%d output=%s stderr=%s", code, out.String(), errOut.String())
		}
	}
	out.Reset()
	errOut.Reset()
	for _, command := range []string{
		"git reset --hard HEAD",
		"git -C . reset --hard HEAD",
		"git --work-tree=. reset --hard HEAD",
		"eval 'git reset --hard HEAD'",
		"git -c alias.wipe='reset --hard' wipe HEAD",
		"sh -c \"eval 'git reset --hard HEAD'\"",
		"git -c alias.first=wipe -c alias.wipe='reset --hard' first HEAD",
		"git -c alias.wipe='!git reset --hard HEAD' wipe",
		"git --config-env=alias.wipe=MAESTRO_TEST_ALIAS wipe HEAD",
		"printf 'reset --hard HEAD' | xargs git",
		"git wipe HEAD",
		"git restore .",
		"git checkout -- .",
		"git checkout README.md",
		"git checkout src/file.go",
		"git checkout -fB feature HEAD",
		"git stash drop",
		"git stash clear",
		"git stash pop",
		"git reflog delete HEAD@{0}",
		"git tag -d old-tag",
		"git branch -df old-branch",
		"git branch -M old-branch existing-branch",
		"git branch -C old-branch existing-branch",
		"git rm tracked.txt",
		"git switch --discard-changes feature",
		"git switch -C feature",
	} {
		t.Run(command, func(t *testing.T) {
			out.Reset()
			errOut.Reset()
			payload = `{"tool_name":"Bash","tool_input":{"command":` + strconv.Quote(command) + `}}`
			if code := Run(guardArgs, strings.NewReader(payload), &out, &errOut); code != ExitOK || !strings.Contains(out.String(), `"permissionDecision": "deny"`) {
				t.Fatalf("dangerous Git %q PreToolUse exit=%d output=%s stderr=%s", command, code, out.String(), errOut.String())
			}
		})
	}
	for _, command := range []string{
		`rm --recursive --force "$HOME"`,
		`python3 -c 'import subprocess; subprocess.run(["git","reset","--hard","HEAD"])'`,
		`source ./wipe.sh`,
		`cp file "$HOME/out"`,
	} {
		t.Run("fail-closed "+command, func(t *testing.T) {
			out.Reset()
			errOut.Reset()
			payload = `{"session_id":"session-a","tool_name":"Bash","tool_input":{"command":` + strconv.Quote(command) + `}}`
			if code := Run(guardArgs, strings.NewReader(payload), &out, &errOut); code != ExitOK || !strings.Contains(out.String(), `"permissionDecision": "deny"`) {
				t.Fatalf("opaque or expanded command %q was not denied: exit=%d output=%s stderr=%s", command, code, out.String(), errOut.String())
			}
		})
	}
	out.Reset()
	errOut.Reset()
	for _, command := range []string{
		"git -c alias.overview='status --short' overview",
		"git switch feature-branch",
		"git switch -c new-feature",
		"git stash push -m checkpoint",
		"git tag new-tag",
		"git branch -m old-branch new-branch",
		"git branch -c old-branch copied-branch",
		"git reflog show",
	} {
		out.Reset()
		errOut.Reset()
		payload = `{"tool_name":"Bash","tool_input":{"command":` + strconv.Quote(command) + `}}`
		if code := Run(guardArgs, strings.NewReader(payload), &out, &errOut); code != ExitOK || strings.Contains(out.String(), `"permissionDecision": "deny"`) {
			t.Fatalf("safe Git command %q was denied: exit=%d output=%s stderr=%s", command, code, out.String(), errOut.String())
		}
	}
	out.Reset()
	errOut.Reset()
	payload = `{"tool_name":"Bash","tool_input":{"command":"sed -n 1p ../outside.txt"}}`
	if code := Run(guardArgs, strings.NewReader(payload), &out, &errOut); code != ExitOK || !strings.Contains(out.String(), `"permissionDecision": "deny"`) || !strings.Contains(out.String(), "traversal") {
		t.Fatalf("command traversal PreToolUse exit=%d output=%s stderr=%s", code, out.String(), errOut.String())
	}
	privateRoot := filepath.Join(fixture.dataRoot, "workspaces", enrolled.WorkspaceID)
	outsideContext := filepath.Join(filepath.Dir(fixture.worktree), "outside-context")
	if err := os.MkdirAll(outsideContext, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outsideContext, "active.md"), []byte("must-not-leak"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideContext, filepath.Join(privateRoot, "continuity")); err == nil {
		out.Reset()
		errOut.Reset()
		if code := Run(baseArgs, strings.NewReader("{}"), &out, &errOut); code != ExitFailure || !strings.Contains(errOut.String(), "symlink") || strings.Contains(out.String(), "must-not-leak") {
			t.Fatalf("private symlink SessionStart exit=%d output=%s stderr=%s", code, out.String(), errOut.String())
		}
	}
}

func TestDirectPreToolUseRequiresBoundConfirmationForExternalMutation(t *testing.T) {
	for _, runtimeName := range []string{"claude", "codex"} {
		t.Run(runtimeName, func(t *testing.T) {
			fixture := newCLIFixture(t)
			profile := agentidentity.Profile{
				SchemaVersion: agentidentity.SchemaVersion,
				OwnerID:       "owner-test",
				Confirmed:     true,
				UpdatedAt:     time.Now().UTC(),
				Selections: []agentidentity.Selection{{
					Role: "maestro", DisplayName: "Maestro", Emoji: "🎼", OwnerID: "owner-test", OwnershipScope: "system",
				}},
			}
			if err := agentidentity.Save(fixture.dataRoot, profile); err != nil {
				t.Fatal(err)
			}
			var out, errOut bytes.Buffer
			if code := Run(workspaceArgs(fixture, "enroll", runtimeName), strings.NewReader(""), &out, &errOut); code != ExitOK {
				t.Fatalf("enroll exit=%d stderr=%s", code, errOut.String())
			}
			run := func(event, payload string) (int, string, string) {
				out.Reset()
				errOut.Reset()
				code := Run(hookArgs(fixture, runtimeName, event), strings.NewReader(payload), &out, &errOut)
				return code, out.String(), errOut.String()
			}
			push := `{"session_id":"session-a","tool_name":"Bash","tool_input":{"command":"git push --force origin HEAD:main"}}`
			code, output, stderr := run("pre-action-guard", push)
			if code != ExitOK || !strings.Contains(output, `"permissionDecision": "deny"`) || !strings.Contains(output, "CONFIRM MAESTRO ") {
				t.Fatalf("unconfirmed push exit=%d output=%s stderr=%s", code, output, stderr)
			}
			marker := "CONFIRM MAESTRO "
			start := strings.Index(output, marker)
			challenge := ""
			if start >= 0 {
				challenge = strings.Fields(output[start:])[2]
				challenge = strings.TrimSuffix(challenge, ".")
			}
			if len(challenge) != 32 {
				t.Fatalf("challenge not found in denial: %q", output)
			}
			confirm := `{"session_id":"session-a","prompt":` + strconv.Quote(marker+challenge) + `}`
			if code, _, stderr := run("context-injection", confirm); code != ExitOK {
				t.Fatalf("confirmation exit=%d stderr=%s", code, stderr)
			}
			if code, output, stderr := run("pre-action-guard", push); code != ExitOK || strings.Contains(output, `"permissionDecision": "deny"`) {
				t.Fatalf("confirmed push exit=%d output=%s stderr=%s", code, output, stderr)
			}
		})
	}
}

func TestClaudeAndCodexSharedDirectLifecycleStaysInParity(t *testing.T) {
	requiredEvents := []string{
		lifecycle.SessionStart,
		lifecycle.ContextInject,
		lifecycle.PreActionGuard,
		lifecycle.PostActionObserve,
		lifecycle.StopFinalize,
	}
	observed := map[string]map[string]bool{}
	for _, runtimeName := range []string{"claude", "codex"} {
		t.Run(runtimeName, func(t *testing.T) {
			fixture := newCLIFixture(t)
			var out, errOut bytes.Buffer
			if code := Run(workspaceArgs(fixture, "enroll", runtimeName), strings.NewReader(""), &out, &errOut); code != ExitOK {
				t.Fatalf("enroll exit=%d stderr=%s", code, errOut.String())
			}
			var enrolled struct {
				WorkspaceID string `json:"workspace_id"`
			}
			if err := json.Unmarshal(out.Bytes(), &enrolled); err != nil {
				t.Fatal(err)
			}
			invocations := []struct {
				event    string
				semantic string
				payload  string
				assert   func(string) bool
			}{
				{event: "session-start", semantic: lifecycle.SessionStart, payload: `{"session_id":"parity-session"}`, assert: func(output string) bool { return strings.Contains(output, "Maestro direct workspace is active") }},
				{event: "context-injection", semantic: lifecycle.ContextInject, payload: `{"session_id":"parity-session","prompt":"synthetic parity prompt"}`, assert: func(output string) bool { return strings.Contains(output, "Maestro direct workspace is active") }},
				{event: "pre-action-guard", semantic: lifecycle.PreActionGuard, payload: `{"session_id":"parity-session","tool_use_id":"parity-tool","tool_name":"Bash","tool_input":{"command":"git reset --hard HEAD"}}`, assert: func(output string) bool { return strings.Contains(output, `"permissionDecision": "deny"`) }},
				{event: "post-action-receipt", semantic: lifecycle.PostActionObserve, payload: `{"session_id":"parity-session","tool_use_id":"parity-post","tool_name":"Read"}`, assert: func(output string) bool { return strings.Contains(output, `"continue": true`) }},
				{event: "stop-finalization", semantic: lifecycle.StopFinalize, payload: `{"session_id":"parity-session"}`, assert: func(output string) bool { return strings.Contains(output, `"continue": true`) }},
			}
			observed[runtimeName] = map[string]bool{}
			for _, invocation := range invocations {
				out.Reset()
				errOut.Reset()
				code := Run(hookArgs(fixture, runtimeName, invocation.event), strings.NewReader(invocation.payload), &out, &errOut)
				if code != ExitOK || !invocation.assert(out.String()) {
					t.Fatalf("%s exit=%d output=%s stderr=%s", invocation.event, code, out.String(), errOut.String())
				}
				observed[runtimeName][invocation.semantic] = true
			}
			summary, err := lifecycle.DiagnoseRuntime(fixture.dataRoot, enrolled.WorkspaceID, runtimeName)
			if err != nil {
				t.Fatal(err)
			}
			for _, event := range []string{lifecycle.PostActionObserve, lifecycle.StopFinalize} {
				if !containsString(summary.Events, event) {
					t.Fatalf("shared durable receipt %s missing: %+v", event, summary)
				}
			}
		})
	}
	if err := validateSharedLifecycleParity(observed, requiredEvents); err != nil {
		t.Fatal(err)
	}
}

func TestSharedLifecycleParityGateRejectsOneSidedEvent(t *testing.T) {
	required := []string{lifecycle.SessionStart, lifecycle.ContextInject}
	observed := map[string]map[string]bool{
		"claude": {lifecycle.SessionStart: true, lifecycle.ContextInject: true},
		"codex":  {lifecycle.SessionStart: true},
	}
	if err := validateSharedLifecycleParity(observed, required); err == nil {
		t.Fatal("parity gate accepted context injection only for Claude")
	}
}

func validateSharedLifecycleParity(observed map[string]map[string]bool, required []string) error {
	for _, event := range required {
		claude := observed["claude"][event]
		codex := observed["codex"][event]
		if !claude || !codex {
			return fmt.Errorf("shared lifecycle event %s diverged: claude=%t codex=%t", event, claude, codex)
		}
	}
	return nil
}

func validateDirectStartupParity(contexts map[string]string, required []string) error {
	for _, marker := range required {
		claude := strings.Contains(contexts["claude"], marker)
		codex := strings.Contains(contexts["codex"], marker)
		if !claude || !codex {
			return fmt.Errorf("direct SessionStart marker %q diverged: claude=%t codex=%t", marker, claude, codex)
		}
	}
	return nil
}

func TestClaudeDirectHooksEnforceManagedAgentFlowAndRecordLifecycle(t *testing.T) {
	fixture := newCLIFixture(t)
	var out, errOut bytes.Buffer
	if code := Run(workspaceArgs(fixture, "enroll", "claude"), strings.NewReader(""), &out, &errOut); code != ExitOK {
		t.Fatalf("enroll exit=%d stderr=%s", code, errOut.String())
	}
	var enrolled struct {
		WorkspaceID string `json:"workspace_id"`
	}
	if err := json.Unmarshal(out.Bytes(), &enrolled); err != nil {
		t.Fatal(err)
	}

	run := func(event, payload string) (int, string, string) {
		out.Reset()
		errOut.Reset()
		code := Run(hookArgs(fixture, "claude", event), strings.NewReader(payload), &out, &errOut)
		return code, out.String(), errOut.String()
	}
	if code, output, stderr := run("context-injection", `{"session_id":"session-a","prompt":"start"}`); code != ExitOK || !strings.Contains(output, "Maestro direct workspace is active") {
		t.Fatalf("context exit=%d output=%s stderr=%s", code, output, stderr)
	}
	if code, output, stderr := run("subagent-start", `{"session_id":"session-a","agent_id":"case-1","agent_type":"case-agent"}`); code != ExitOK || !strings.Contains(output, "managed Maestro specialist case-agent") {
		t.Fatalf("subagent start exit=%d output=%s stderr=%s", code, output, stderr)
	}
	guardPayload := `{"session_id":"session-a","cwd":` + strconv.Quote(fixture.worktree) + `,"tool_name":"Bash","agent_type":"case-agent","tool_input":{"command":"pwd"}}`
	if code, output, stderr := run("pre-action-guard", guardPayload); code != ExitOK || !strings.Contains(output, `"permissionDecision": "deny"`) || !strings.Contains(output, "Case Agent") {
		t.Fatalf("Case guard exit=%d output=%s stderr=%s", code, output, stderr)
	}
	if code, output, stderr := run("stop-finalization", `{"session_id":"session-a"}`); code != ExitOK || !strings.Contains(output, `"decision": "block"`) {
		t.Fatalf("active-agent stop exit=%d output=%s stderr=%s", code, output, stderr)
	}
	for _, invalid := range []string{`{}`, `{not-json`} {
		if code, output, stderr := run("stop-finalization", invalid); code != ExitOK || !strings.Contains(output, `"decision": "block"`) {
			t.Fatalf("invalid stop %q exit=%d output=%s stderr=%s", invalid, code, output, stderr)
		}
	}
	if code, output, stderr := run("subagent-stop", `{"session_id":"session-a","agent_id":"case-1","agent_type":"case-agent"}`); code != ExitOK || !strings.Contains(output, `"continue": true`) {
		t.Fatalf("subagent stop exit=%d output=%s stderr=%s", code, output, stderr)
	}
	if code, output, stderr := run("post-action-receipt", `{"session_id":"session-a","tool_use_id":"tool-1","tool_name":"Read"}`); code != ExitOK || !strings.Contains(output, `"continue": true`) {
		t.Fatalf("post action exit=%d output=%s stderr=%s", code, output, stderr)
	}
	if code, output, stderr := run("stop-finalization", `{"session_id":"session-a"}`); code != ExitOK || !strings.Contains(output, `"continue": true`) {
		t.Fatalf("final stop exit=%d output=%s stderr=%s", code, output, stderr)
	}

	summary, err := lifecycle.DiagnoseRuntime(fixture.dataRoot, enrolled.WorkspaceID, "claude")
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range []string{lifecycle.SubagentStart, lifecycle.SubagentStop, lifecycle.PostActionObserve, lifecycle.StopFinalize} {
		if !containsString(summary.Events, event) {
			t.Fatalf("lifecycle summary missing %s: %+v", event, summary)
		}
	}
}

func TestCodexDirectPostAndStopHooksRecordLifecycle(t *testing.T) {
	fixture := newCLIFixture(t)
	var out, errOut bytes.Buffer
	if code := Run(workspaceArgs(fixture, "enroll", "codex"), strings.NewReader(""), &out, &errOut); code != ExitOK {
		t.Fatalf("enroll exit=%d stderr=%s", code, errOut.String())
	}
	var enrolled struct {
		WorkspaceID string `json:"workspace_id"`
	}
	if err := json.Unmarshal(out.Bytes(), &enrolled); err != nil {
		t.Fatal(err)
	}
	for _, event := range []struct {
		name    string
		payload string
	}{
		{name: "post-action-receipt", payload: `{"session_id":"session-a","tool_use_id":"tool-1","tool_name":"Read"}`},
		{name: "stop-finalization", payload: `{"session_id":"session-a"}`},
	} {
		out.Reset()
		errOut.Reset()
		if code := Run(hookArgs(fixture, "codex", event.name), strings.NewReader(event.payload), &out, &errOut); code != ExitOK || !strings.Contains(out.String(), `"continue": true`) {
			t.Fatalf("%s exit=%d output=%s stderr=%s", event.name, code, out.String(), errOut.String())
		}
	}
	summary, err := lifecycle.DiagnoseRuntime(fixture.dataRoot, enrolled.WorkspaceID, "codex")
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range []string{lifecycle.PostActionObserve, lifecycle.StopFinalize} {
		if !containsString(summary.Events, event) {
			t.Fatalf("lifecycle summary missing %s: %+v", event, summary)
		}
	}
}

func TestCodexContextAndDeniedGuardRecordMetadataOnlyLifecycle(t *testing.T) {
	fixture := newCLIFixture(t)
	var out, errOut bytes.Buffer
	if code := Run(workspaceArgs(fixture, "enroll", "codex"), strings.NewReader(""), &out, &errOut); code != ExitOK {
		t.Fatalf("enroll exit=%d stderr=%s", code, errOut.String())
	}
	var enrolled struct {
		WorkspaceID string `json:"workspace_id"`
	}
	if err := json.Unmarshal(out.Bytes(), &enrolled); err != nil {
		t.Fatal(err)
	}
	invocations := []struct {
		event   string
		payload string
		denied  bool
	}{
		{event: "session-start", payload: `{"session_id":"session-a"}`},
		{event: "context-injection", payload: `{"session_id":"session-a","prompt":"private prompt"}`},
		{event: "pre-action-guard", payload: `{"session_id":"session-a","tool_use_id":"tool-a","tool_name":"Bash","tool_input":{"command":"git reset --hard HEAD"}}`, denied: true},
	}
	for _, invocation := range invocations {
		out.Reset()
		errOut.Reset()
		if code := Run(hookArgs(fixture, "codex", invocation.event), strings.NewReader(invocation.payload), &out, &errOut); code != ExitOK {
			t.Fatalf("%s exit=%d stderr=%s", invocation.event, code, errOut.String())
		}
		if invocation.denied && !strings.Contains(out.String(), `"permissionDecision": "deny"`) {
			t.Fatalf("%s was not denied: %s", invocation.event, out.String())
		}
	}
	summary, err := lifecycle.DiagnoseRuntime(fixture.dataRoot, enrolled.WorkspaceID, "codex")
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range []string{lifecycle.SessionStart, lifecycle.ContextInject, lifecycle.PreActionGuard} {
		if !containsString(summary.Events, event) {
			t.Fatalf("lifecycle summary missing %s: %+v", event, summary)
		}
	}
	entries, err := os.ReadDir(filepath.Join(fixture.dataRoot, "runtime", "receipts", enrolled.WorkspaceID))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		body, readErr := os.ReadFile(filepath.Join(fixture.dataRoot, "runtime", "receipts", enrolled.WorkspaceID, entry.Name()))
		if readErr != nil {
			t.Fatal(readErr)
		}
		if strings.Contains(string(body), "private prompt") || strings.Contains(string(body), "reset --hard") {
			t.Fatalf("receipt leaked native payload: %s", body)
		}
	}
}

func TestCodexNativeHookOutputsContainOnlySupportedTopLevelFields(t *testing.T) {
	for name, run := range map[string]func(*bytes.Buffer, *bytes.Buffer) int{
		"context": func(out, errOut *bytes.Buffer) int {
			return writeHookContext(out, "codex", "session_start", "bounded context", errOut)
		},
		"denial": func(out, errOut *bytes.Buffer) int {
			return writeHookDenial(out, "codex", "blocked", errOut)
		},
	} {
		t.Run(name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			if code := run(&out, &errOut); code != ExitOK {
				t.Fatalf("hook output exit=%d stderr=%s", code, errOut.String())
			}
			var payload map[string]any
			if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
				t.Fatal(err)
			}
			if len(payload) != 1 || payload["hookSpecificOutput"] == nil {
				t.Fatalf("Codex hook output contains unsupported top-level fields: %s", out.String())
			}
		})
	}
}

func TestClaudeStopBlocksWhenReceiptCannotBePersisted(t *testing.T) {
	fixture := newCLIFixture(t)
	var out, errOut bytes.Buffer
	if code := Run(workspaceArgs(fixture, "enroll", "claude"), strings.NewReader(""), &out, &errOut); code != ExitOK {
		t.Fatalf("enroll exit=%d stderr=%s", code, errOut.String())
	}
	var enrolled struct {
		WorkspaceID string `json:"workspace_id"`
	}
	if err := json.Unmarshal(out.Bytes(), &enrolled); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	errOut.Reset()
	if code := Run(hookArgs(fixture, "claude", "context-injection"), strings.NewReader(`{"session_id":"session-persist","prompt":"start"}`), &out, &errOut); code != ExitOK {
		t.Fatalf("context exit=%d stderr=%s", code, errOut.String())
	}
	receiptParent := filepath.Join(fixture.dataRoot, "runtime", "receipts")
	if err := os.MkdirAll(receiptParent, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(receiptParent, enrolled.WorkspaceID), []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	errOut.Reset()
	if code := Run(hookArgs(fixture, "claude", "stop-finalization"), strings.NewReader(`{"session_id":"session-persist"}`), &out, &errOut); code != ExitOK || !strings.Contains(out.String(), `"decision": "block"`) || !strings.Contains(out.String(), "persist") {
		t.Fatalf("unpersisted stop exit=%d output=%s stderr=%s", code, out.String(), errOut.String())
	}
}

type cliFixture struct {
	managedRoot string
	dataRoot    string
	executable  string
	worktree    string
}

type fixedMemorySynthesizer string

func (value fixedMemorySynthesizer) Synthesize(context.Context, memory.SynthesisRequest) (string, error) {
	return string(value), nil
}

func hookAdditionalContext(t *testing.T, body []byte) string {
	t.Helper()
	var output struct {
		HookSpecificOutput struct {
			AdditionalContext string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal(body, &output); err != nil {
		t.Fatalf("decode hook output: %v: %s", err, body)
	}
	return output.HookSpecificOutput.AdditionalContext
}

func newCLIFixture(t *testing.T) cliFixture {
	t.Helper()
	root := t.TempDir()
	fixture := cliFixture{
		managedRoot: filepath.Join(root, "managed"),
		dataRoot:    filepath.Join(root, "data"),
		worktree:    filepath.Join(root, "repository"),
	}
	fixture.executable = filepath.Join(fixture.managedRoot, "bin", executableName())
	for _, directory := range []string{filepath.Dir(fixture.executable), fixture.dataRoot, fixture.worktree} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(fixture.executable, []byte("test CLI"), 0o700); err != nil {
		t.Fatal(err)
	}
	cliBody, err := os.ReadFile(fixture.executable)
	if err != nil {
		t.Fatal(err)
	}
	cliSum := sha256.Sum256(cliBody)
	manifest := portableactivation.Manifest{
		SchemaVersion: 1, Version: "0.1.12", TargetOS: runtime.GOOS,
		TargetArch: runtime.GOARCH, CLIPath: filepath.ToSlash(filepath.Join("bin", executableName())),
		CLISHA256: hex.EncodeToString(cliSum[:]),
	}
	manifestBody, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fixture.managedRoot, portableactivation.ManifestFileName), append(manifestBody, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := portableactivation.Activate(portableactivation.Options{ManagedRoot: fixture.managedRoot, DataRoot: fixture.dataRoot}); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("git", "-C", fixture.worktree, "init")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, output)
	}
	return fixture
}

func prepareReviewedOwnerContext(t *testing.T, dataRoot string) {
	t.Helper()
	if _, err := ownerctx.Initialize(dataRoot); err != nil {
		t.Fatal(err)
	}
	if _, err := ownerctx.SelectOnboardingTrack(dataRoot, ownerctx.OnboardingTrackQuick); err != nil {
		t.Fatal(err)
	}
	bodies := map[string]string{
		"owner-identity":      "# Owner identity\n\n## Current\n\nSynthetic Owner\n",
		"personal-context":    "# Authorized personal context\n\n## Current\n\npersonal-context-secret-sentinel\n",
		"professional-role":   "# Professional role\n\n## Current\n\nSynthetic engineering role\n",
		"communication-style": "# Communication style\n\n## Current\n\nConclusion first.\n",
		"preferences":         "# Preferences\n\n## Current\n\nUse deterministic tests.\n",
		"quality-bar":         "# Quality bar\n\n## Current\n\nRequire evidence.\n",
	}
	for id, body := range bodies {
		path := filepath.Join(dataRoot, "owner", "self", id+".md")
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	status, err := ownerctx.Inspect(dataRoot)
	if err != nil || status.Onboarding.State != "review_required" {
		t.Fatalf("owner context review state=%#v err=%v", status.Onboarding, err)
	}
	status, err = ownerctx.ConfirmOnboarding(dataRoot, status.Onboarding.ReviewDigest)
	if err != nil || status.Onboarding.State != "complete" {
		t.Fatalf("owner context confirmation state=%#v err=%v", status.Onboarding, err)
	}
}

func prepareLegacyReviewedOwnerContext(t *testing.T, dataRoot string) {
	t.Helper()
	for _, directory := range []string{filepath.Join(dataRoot, "owner", "self"), filepath.Join(dataRoot, "profile")} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{
		filepath.Join(dataRoot, "owner", "registry.json"):                "{\n  \"schema_version\": 1,\n  \"trees\": {\"self\":\"owner/self/\",\"operating\":\"owner/operating/\",\"observations\":\"owner/observations/\",\"interview\":\"owner/interview/\"},\n  \"initialized\": true,\n  \"owner_type\": \"distro-adopter\",\n  \"personal_context\": {\"state\":\"authorized\",\"state_timestamp\":\"2026-08-28T00:00:00Z\",\"source_file\":\"owner/self/personal-context.md\"},\n  \"onboarding_mode\": \"quick\"\n}\n",
		filepath.Join(dataRoot, "profile", "onboarding.json"):            "{\"status\":\"complete\",\"track\":\"quick\",\"completed_at\":\"2026-08-28T00:00:00Z\",\"version\":\"0.1.12\"}\n",
		filepath.Join(dataRoot, "profile", "identity.json"):              "{\"schema_version\":1,\"name\":\"Portable Synthetic Owner\",\"role\":\"Portable synthetic role\",\"initialized\":true}\n",
		filepath.Join(dataRoot, "owner", "self", "professional-role.md"): "# Professional role\n\n## Current\n\nPortable synthetic role\n",
		filepath.Join(dataRoot, "owner", "self", "personal-context.md"):  "# Personal context\n\n## Current\n\nportable-personal-secret\n",
	}
	for path, body := range files {
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func workspaceArgs(fixture cliFixture, operation, runtimeName string) []string {
	return []string{
		"workspace", operation,
		"--runtime", runtimeName,
		"--managed-root", fixture.managedRoot,
		"--data-root", fixture.dataRoot,
		"--executable", fixture.executable,
		fixture.worktree,
	}
}

func hookArgs(fixture cliFixture, runtimeName, event string) []string {
	return []string{
		"hook", runtimeName, event,
		"--adapter-source", "maestro",
		"--orchestration-state", ".bcgos/maestro-orchestration-state.json",
		"--managed-root", fixture.managedRoot,
		"--data-root", fixture.dataRoot,
		"--workspace-root", fixture.worktree,
		"--executable", fixture.executable,
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func executableName() string {
	if filepath.Separator == '\\' {
		return "bcgos.exe"
	}
	return "bcgos"
}
