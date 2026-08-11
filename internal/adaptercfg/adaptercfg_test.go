package adaptercfg

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/claudeagents"
)

func ownedCommands(t *testing.T, path, runtimeName string) []string {
	t.Helper()
	config, err := read(path)
	if err != nil {
		t.Fatal(err)
	}
	hooks, err := hooksMap(config)
	if err != nil {
		t.Fatal(err)
	}
	groups, err := sessionGroups(hooks)
	if err != nil {
		t.Fatal(err)
	}
	var commands []string
	for _, group := range groups {
		entry := group.(map[string]any)
		for _, raw := range entry["hooks"].([]any) {
			hook := raw.(map[string]any)
			if command, ok := hook["command"].(string); ok && isOwnedCommand(runtimeName, command) {
				commands = append(commands, command)
			}
		}
	}
	return commands
}

func TestInstallStatusAndUninstallPreserveOtherHooks(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, ".codex", "hooks.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"other"}]}]}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	installed, err := Install("codex", workspace, "/opt/maestro/bcgos")
	if err != nil || installed.State != "installed" {
		t.Fatalf("install = %#v, %v", installed, err)
	}
	expected, err := filepath.Abs("/opt/maestro/bcgos")
	if err != nil {
		t.Fatal(err)
	}
	commands := ownedCommands(t, path, "codex")
	data, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(data), "other") || len(commands) != 1 || !strings.Contains(commands[0], expected) || !strings.Contains(commands[0], "--adapter-source maestro") {
		t.Fatalf("config = %s, %v", data, err)
	}
	status, err := Inspect("codex", workspace)
	if err != nil || status.State != "installed" {
		t.Fatalf("status = %#v, %v", status, err)
	}
	removed, err := Uninstall("codex", workspace)
	if err != nil || removed.State != "removed" {
		t.Fatalf("remove = %#v, %v", removed, err)
	}
	data, err = os.ReadFile(path)
	if err != nil || !strings.Contains(string(data), "other") || strings.Contains(string(data), "--adapter-source maestro") {
		t.Fatalf("config after remove = %s, %v", data, err)
	}
}

func TestUninstallRemovesOnlyOwnedHookInsideSharedGroup(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, ".claude", "settings.local.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	content := `{"hooks":{"SessionStart":[{"matcher":"startup","hooks":[{"type":"command","command":"other"},{"type":"command","command":"bcgos hook session-start --runtime claude"}]}]}}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Uninstall("claude", workspace); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(data), "other") || strings.Contains(string(data), "bcgos hook session-start --runtime claude") || !strings.Contains(string(data), "startup") {
		t.Fatalf("config = %s, %v", data, err)
	}
}

func TestInstallUpdatesOwnedExecutableAndKeepsConfigOutOfGit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is required to exercise workspace exclusion")
	}
	clearGitIndexFile(t)
	workspace := t.TempDir()
	initCommand := exec.Command("git", "-C", workspace, "init", "-q")
	initCommand.Env = withoutGitIndexFile(os.Environ())
	if output, err := initCommand.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v (%s)", err, output)
	}
	gitInfo := filepath.Join(workspace, ".git", "info")
	if err := os.MkdirAll(gitInfo, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gitInfo, "exclude"), []byte("# local configuration\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Install("claude", workspace, "/opt/maestro/v1/bcgos"); err != nil {
		t.Fatal(err)
	}
	if _, err := Install("claude", workspace, "/opt/maestro/v2/bcgos"); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(workspace, ".claude", "settings.local.json")
	config, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	v1, err := filepath.Abs("/opt/maestro/v1/bcgos")
	if err != nil {
		t.Fatal(err)
	}
	v2, err := filepath.Abs("/opt/maestro/v2/bcgos")
	if err != nil {
		t.Fatal(err)
	}
	commands := ownedCommands(t, configPath, "claude")
	if len(commands) != 1 || strings.Contains(commands[0], v1) || !strings.Contains(commands[0], v2) || !strings.Contains(commands[0], "--adapter-source maestro") {
		t.Fatalf("config = %s", config)
	}
	exclude, err := os.ReadFile(filepath.Join(gitInfo, "exclude"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(exclude), "/.claude/settings.local.json") != 1 {
		t.Fatalf("exclude = %s", exclude)
	}
}

func TestInstallMigratesLegacyPathLookupWithoutDuplicatingTheHook(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, ".codex", "hooks.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	legacy := `{"hooks":{"SessionStart":[{"matcher":"startup","hooks":[{"type":"command","command":"other"},{"type":"command","command":"bcgos hook session-start --runtime codex"}]}]}}`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Install("codex", workspace, "/opt/maestro/bcgos"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	expected, err := filepath.Abs("/opt/maestro/bcgos")
	if err != nil {
		t.Fatal(err)
	}
	commands := ownedCommands(t, path, "codex")
	if strings.Contains(string(data), `"command": "bcgos hook session-start --runtime codex"`) || len(commands) != 1 || !strings.Contains(commands[0], expected) || !strings.Contains(commands[0], "--adapter-source maestro") || !strings.Contains(string(data), "startup") || !strings.Contains(string(data), "other") {
		t.Fatalf("config = %s", data)
	}
}

func TestInstallRefusesTrackedRuntimeConfiguration(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is required to exercise tracked-configuration protection")
	}
	clearGitIndexFile(t)
	workspace := t.TempDir()
	initCommand := exec.Command("git", "-C", workspace, "init", "-q")
	initCommand.Env = withoutGitIndexFile(os.Environ())
	if output, err := initCommand.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v (%s)", err, output)
	}
	path := filepath.Join(workspace, ".codex", "hooks.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	original := []byte(`{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"other"}]}]}}`)
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("git", "-C", workspace, "add", ".codex/hooks.json")
	command.Env = withoutGitIndexFile(os.Environ())
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git add: %v (%s)", err, output)
	}
	excludePath := filepath.Join(workspace, ".git", "info", "exclude")
	excludeBefore, err := os.ReadFile(excludePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Install("codex", workspace, "/opt/maestro/bcgos"); err == nil || !strings.Contains(err.Error(), "tracked runtime configuration") {
		t.Fatalf("Install tracked config error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(original) {
		t.Fatalf("tracked config changed: %s", data)
	}
	excludeAfter, err := os.ReadFile(excludePath)
	if err != nil || string(excludeAfter) != string(excludeBefore) {
		t.Fatalf("exclude changed for rejected install: %s, %v", excludeAfter, err)
	}
}

func withoutGitIndexFile(environment []string) []string {
	filtered := make([]string, 0, len(environment))
	for _, variable := range environment {
		if !strings.HasPrefix(variable, "GIT_INDEX_FILE=") &&
			!strings.HasPrefix(variable, "GIT_DIR=") &&
			!strings.HasPrefix(variable, "GIT_WORK_TREE=") {
			filtered = append(filtered, variable)
		}
	}
	return filtered
}

func clearGitIndexFile(t *testing.T) {
	t.Helper()
	values := make(map[string]string, 3)
	present := make(map[string]bool, 3)
	for _, name := range []string{"GIT_INDEX_FILE", "GIT_DIR", "GIT_WORK_TREE"} {
		value, exists := os.LookupEnv(name)
		if exists {
			values[name] = value
			present[name] = true
		}
		if err := os.Unsetenv(name); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		for _, name := range []string{"GIT_INDEX_FILE", "GIT_DIR", "GIT_WORK_TREE"} {
			if present[name] {
				_ = os.Setenv(name, values[name])
			} else {
				_ = os.Unsetenv(name)
			}
		}
	})
}

func TestInstalledCommandPassesTheOwnedMarkerToItsExecutable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell parsing is exercised on Unix; Windows quoting is covered by command construction")
	}
	workspace := t.TempDir()
	arguments := filepath.Join(workspace, "arguments.txt")
	executable := filepath.Join(workspace, "fake bcgos")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$OUTPUT_FILE\"\n"
	if err := os.WriteFile(executable, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := Install("codex", workspace, executable); err != nil {
		t.Fatal(err)
	}
	command, err := commandFor("codex", executable)
	if err != nil {
		t.Fatal(err)
	}
	run := exec.Command("sh", "-c", command)
	run.Env = append(os.Environ(), "OUTPUT_FILE="+arguments)
	if output, err := run.CombinedOutput(); err != nil {
		t.Fatalf("installed command failed: %v (%s)", err, output)
	}
	got, err := os.ReadFile(arguments)
	if err != nil {
		t.Fatal(err)
	}
	want := "hook\nsession-start\n--runtime\ncodex\n--adapter-source\nmaestro\n--orchestration-state\n.bcgos/maestro-orchestration-state.json\n"
	if string(got) != want {
		t.Fatalf("arguments = %q, want %q", got, want)
	}
}

func TestQuoteCommandPathCoversWindowsAndPOSIXPaths(t *testing.T) {
	if got, want := quoteCommandPathFor("windows", `C:\Program Files\Maestro\bcgos.exe`), `"C:\Program Files\Maestro\bcgos.exe"`; got != want {
		t.Fatalf("windows command = %q, want %q", got, want)
	}
	if got, want := quoteCommandPathFor("darwin", "/opt/Maestro's Tools/bcgos"), `'/opt/Maestro'"'"'s Tools/bcgos'`; got != want {
		t.Fatalf("POSIX command = %q, want %q", got, want)
	}
}

func TestInstallRejectsMalformedSessionStartInsteadOfOverwriting(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, ".codex", "hooks.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"hooks":{"SessionStart":"invalid"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Install("codex", workspace, "/opt/maestro/bcgos"); err == nil {
		t.Fatal("Install accepted malformed SessionStart")
	}
	data, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(data), "invalid") {
		t.Fatalf("config changed: %s, %v", data, err)
	}
}

func TestInstallRejectsMalformedSessionStartGroupInsteadOfOverwriting(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, ".codex", "hooks.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"hooks":{"SessionStart":["invalid"]}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Install("codex", workspace, "/opt/maestro/bcgos"); err == nil {
		t.Fatal("Install accepted malformed group")
	}
	data, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(data), "invalid") {
		t.Fatalf("config changed: %s, %v", data, err)
	}
}

func TestCodexInstallOwnsCompleteSynchronousLifecycle(t *testing.T) {
	workspace := t.TempDir()
	if _, err := Install("codex", workspace, "/opt/maestro/bcgos"); err != nil {
		t.Fatal(err)
	}
	config, err := read(filepath.Join(workspace, ".codex", "hooks.json"))
	if err != nil {
		t.Fatal(err)
	}
	hooks, err := hooksMap(config)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range []string{"SessionStart", "UserPromptSubmit", "PreToolUse", "PostToolUse", "Stop"} {
		groups, err := groupsForEvent(hooks, event)
		if err != nil || len(groups) == 0 {
			t.Fatalf("%s groups = %#v, %v", event, groups, err)
		}
		found := false
		for _, group := range groups {
			for _, raw := range group.(map[string]any)["hooks"].([]any) {
				entry := raw.(map[string]any)
				if !isOwnedEventCommand("codex", event, entry["command"]) {
					continue
				}
				found = true
				if entry["timeout"] != float64(5) {
					t.Fatalf("%s timeout = %#v", event, entry["timeout"])
				}
				if async, _ := entry["async"].(bool); async {
					t.Fatalf("%s must remain synchronous", event)
				}
			}
		}
		if !found {
			t.Fatalf("owned %s binding not found", event)
		}
	}
}

func TestClaudeInstallOwnsCompleteLifecycleWithBlockingStop(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, ".claude", "settings.local.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"hooks":{"PreToolUse":[{"hooks":[{"type":"command","command":"other"}]}]}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Install("claude", workspace, "/opt/maestro/bcgos"); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(workspace, ".claude", "agents"))
	if err != nil || len(entries) != 5 {
		t.Fatalf("managed native agents = %v, %v", entries, err)
	}
	config, err := read(path)
	if err != nil {
		t.Fatal(err)
	}
	hooks, err := hooksMap(config)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range []string{"SessionStart", "UserPromptSubmit", "PreToolUse", "PostToolUse", "Stop", "SubagentStart", "SubagentStop"} {
		groups, err := groupsForEvent(hooks, event)
		if err != nil || len(groups) == 0 {
			t.Fatalf("%s groups = %#v, %v", event, groups, err)
		}
		found := false
		for _, group := range groups {
			for _, raw := range group.(map[string]any)["hooks"].([]any) {
				entry := raw.(map[string]any)
				if !isOwnedEventCommand("claude", event, entry["command"]) {
					continue
				}
				found = true
				if entry["timeout"] != float64(5) {
					t.Fatalf("%s timeout = %#v", event, entry["timeout"])
				}
				wantAsync := event == "PostToolUse"
				if got, _ := entry["async"].(bool); got != wantAsync {
					t.Fatalf("%s async = %v, want %v", event, got, wantAsync)
				}
			}
		}
		if !found {
			t.Fatalf("owned %s binding not found", event)
		}
	}
	data, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(data), `"command": "other"`) {
		t.Fatalf("unrelated hook was not preserved: %s, %v", data, err)
	}
}

func TestClaudeInspectRequiresEveryOwnedLifecycleBinding(t *testing.T) {
	workspace := t.TempDir()
	if _, err := Install("claude", workspace, "/opt/maestro/bcgos"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(workspace, ".claude", "settings.local.json")
	config, err := read(path)
	if err != nil {
		t.Fatal(err)
	}
	hooks := config["hooks"].(map[string]any)
	delete(hooks, "Stop")
	if err := write(path, config); err != nil {
		t.Fatal(err)
	}
	status, err := Inspect("claude", workspace)
	if err != nil || status.State != "absent" {
		t.Fatalf("status = %#v, %v", status, err)
	}
}

func TestClaudeUninstallRemovesOnlyOwnedLifecycleBindings(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, ".claude", "settings.local.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"other"}]}]}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Install("claude", workspace, "/opt/maestro/bcgos"); err != nil {
		t.Fatal(err)
	}
	if _, err := Uninstall("claude", workspace); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(data), `"command": "other"`) || strings.Contains(string(data), "--adapter-source maestro") {
		t.Fatalf("config after uninstall = %s, %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(workspace, ".claude", "agents", "case-agent.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("managed Claude agent survived uninstall: %v", err)
	}
}

func TestClaudeInspectReportsPartialWhenManagedAgentProjectionIsMissing(t *testing.T) {
	workspace := t.TempDir()
	if _, err := Install("claude", workspace, "/opt/maestro/bcgos"); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(workspace, ".claude", "agents", "walter.md")); err != nil {
		t.Fatal(err)
	}
	status, err := Inspect("claude", workspace)
	if err != nil || status.State != "partial" {
		t.Fatalf("status=%#v err=%v", status, err)
	}
}

func TestClaudeUninstallRefusesEditedAgentBeforeRemovingHooks(t *testing.T) {
	workspace := t.TempDir()
	if _, err := Install("claude", workspace, "/opt/maestro/bcgos"); err != nil {
		t.Fatal(err)
	}
	agentPath := filepath.Join(workspace, ".claude", "agents", "walter.md")
	if err := os.WriteFile(agentPath, []byte("user replacement\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Uninstall("claude", workspace); err == nil {
		t.Fatal("edited agent was removed")
	}
	settings, err := os.ReadFile(filepath.Join(workspace, ".claude", "settings.local.json"))
	if err != nil || !strings.Contains(string(settings), "--adapter-source maestro") {
		t.Fatalf("hooks changed before agent preflight: %s, %v", settings, err)
	}
}

func TestClaudeInstallRollsBackProjectedAgentsWhenSettingsWriteFails(t *testing.T) {
	workspace := t.TempDir()
	settingsPath := filepath.Join(workspace, ".claude", "settings.local.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o700); err != nil {
		t.Fatal(err)
	}
	original := []byte(`{"hooks":{"PreToolUse":[{"hooks":[{"type":"command","command":"other"}]}]}}`)
	if err := os.WriteFile(settingsPath, original, 0o600); err != nil {
		t.Fatal(err)
	}
	priorWrite := writeRuntimeConfig
	writeRuntimeConfig = func(string, map[string]any) error { return errors.New("injected settings write failure") }
	t.Cleanup(func() { writeRuntimeConfig = priorWrite })

	if _, err := Install("claude", workspace, "/opt/maestro/bcgos"); err == nil || !strings.Contains(err.Error(), "injected settings write failure") {
		t.Fatalf("Install error = %v", err)
	}
	settings, err := os.ReadFile(settingsPath)
	if err != nil || string(settings) != string(original) {
		t.Fatalf("settings after rollback = %s, %v", settings, err)
	}
	for _, name := range managedClaudeAgentFiles {
		if _, err := os.Stat(filepath.Join(workspace, ".claude", "agents", name)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("managed agent %s survived rollback: %v", name, err)
		}
	}
}

func TestClaudeUninstallRollsBackHooksAndAgentsWhenProjectionRemovalFails(t *testing.T) {
	workspace := t.TempDir()
	if _, err := Install("claude", workspace, "/opt/maestro/bcgos"); err != nil {
		t.Fatal(err)
	}
	priorUninstall := uninstallClaudeAgents
	uninstallClaudeAgents = func(workspace string) (claudeagents.Status, error) {
		status, err := claudeagents.Uninstall(workspace)
		if err != nil {
			return status, err
		}
		return status, errors.New("injected agent removal failure")
	}
	t.Cleanup(func() { uninstallClaudeAgents = priorUninstall })

	if _, err := Uninstall("claude", workspace); err == nil || !strings.Contains(err.Error(), "injected agent removal failure") {
		t.Fatalf("Uninstall error = %v", err)
	}
	status, err := Inspect("claude", workspace)
	if err != nil || status.State != "installed" {
		t.Fatalf("status after rollback = %#v, %v", status, err)
	}
}

func TestStateSnapshotRestoresClaudeAgentsAfterDownstreamFailure(t *testing.T) {
	workspace := t.TempDir()
	snapshot, err := CaptureState("claude", workspace)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Install("claude", workspace, "/opt/maestro/bcgos"); err != nil {
		t.Fatal(err)
	}
	if err := snapshot.Restore(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(workspace, ".claude", "settings.local.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("settings survived downstream rollback: %v", err)
	}
	for _, name := range managedClaudeAgentFiles {
		if _, err := os.Stat(filepath.Join(workspace, ".claude", "agents", name)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("managed agent %s survived downstream rollback: %v", name, err)
		}
	}
}
