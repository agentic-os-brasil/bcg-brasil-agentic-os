package installreadiness

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/adaptercfg"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/agentscaffold"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installtx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ownerctx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/runtimeprojection"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspaceagent"
)

func TestVerifyAcceptsOnlyCanonicalConfiguredCodexInstall(t *testing.T) {
	fixture := newReadinessFixture(t, nil)
	report, err := Verify(fixture.options())
	if err != nil {
		t.Fatal(err)
	}
	if report.SchemaVersion != 1 || report.State != "verified" || !report.Ready ||
		report.Runtime != "codex" || report.EvidenceClass != "configured" ||
		report.NativeObservation != "not_observed" || report.CapabilityState != "unavailable" {
		t.Fatalf("unexpected readiness report: %#v", report)
	}
	if report.WorkspacePath != fixture.workspace || report.InstalledCLI != fixture.cli || report.WorkspaceID == "" {
		t.Fatalf("report identities drifted: %#v", report)
	}
	if len(report.Lifecycle) != 5 {
		t.Fatalf("lifecycle bindings = %d, want 5", len(report.Lifecycle))
	}
	wantEvents := map[string]string{
		"session_start": "SessionStart", "context_inject": "UserPromptSubmit",
		"pre_action_guard": "PreToolUse", "post_action_observe": "PostToolUse",
		"stop_finalize": "Stop",
	}
	for _, binding := range report.Lifecycle {
		if wantEvents[binding.SemanticEvent] != binding.NativeEvent || binding.CapabilityState != "unavailable" ||
			!binding.Configured || binding.AdapterObserved || binding.NativeQualified ||
			!strings.HasPrefix(binding.Command, quoteTestPath(fixture.cli)+" hook ") {
			t.Fatalf("unsafe lifecycle binding: %#v", binding)
		}
		delete(wantEvents, binding.SemanticEvent)
	}
	if len(wantEvents) != 0 {
		t.Fatalf("missing lifecycle events: %#v", wantEvents)
	}
	checks := make(map[string]Check, len(report.Checks))
	for _, check := range report.Checks {
		checks[check.ID] = check
	}
	if checks["case_agent"].CanonicalRole != "case_agent" || checks["case_agent"].IdentityCompatibility != "" {
		t.Fatalf("Case Agent readiness check is not canonical: %#v", checks["case_agent"])
	}
	if checks["agent_scaffold"].CanonicalRole != "case_agent" || checks["agent_scaffold"].IdentityCompatibility != "migration_compatibility" ||
		!strings.Contains(checks["agent_scaffold"].Message, "legacy workspace-agent ID") {
		t.Fatalf("legacy scaffold was not explicitly marked as migration compatibility: %#v", checks["agent_scaffold"])
	}
}

func TestVerifyAcceptsOnlyCanonicalConfiguredClaudeInstall(t *testing.T) {
	fixture := newReadinessFixtureForRuntime(t, "claude", nil)
	report, err := Verify(fixture.options())
	if err != nil {
		t.Fatal(err)
	}
	if report.Runtime != "claude" || !report.Ready || len(report.Lifecycle) != 5 {
		t.Fatalf("unexpected Claude readiness report: %#v", report)
	}
	for _, binding := range report.Lifecycle {
		if !strings.HasPrefix(binding.Command, quoteTestPath(fixture.cli)+" hook claude ") ||
			!binding.Configured || binding.AdapterObserved || binding.NativeQualified {
			t.Fatalf("unsafe Claude lifecycle binding: %#v", binding)
		}
	}
}

func TestVerifyRejectsMissingAndTamperedSurfaces(t *testing.T) {
	tests := []struct {
		name     string
		failedID string
		mutate   func(*testing.T, readinessFixture)
	}{
		{name: "missing workspace manifest", failedID: "workspace_identity", mutate: func(t *testing.T, f readinessFixture) {
			removeFile(t, filepath.Join(f.workspace, ".bcgos", "workspace.json"))
		}},
		{name: "missing AGENTS", failedID: "runtime_projection", mutate: func(t *testing.T, f readinessFixture) {
			removeFile(t, filepath.Join(f.workspace, "AGENTS.md"))
		}},
		{name: "tampered AGENTS", failedID: "runtime_projection", mutate: func(t *testing.T, f readinessFixture) {
			path := filepath.Join(f.workspace, "AGENTS.md")
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			body = []byte(strings.Replace(string(body), "O Maestro é o Agentic OS profissional", "conteúdo alterado", 1))
			if err := os.WriteFile(path, body, 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "tampered projection identity", failedID: "runtime_projection", mutate: func(t *testing.T, f readinessFixture) {
			path := filepath.Join(f.workspace, runtimeprojection.ManifestRelativePath)
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			body = []byte(strings.Replace(string(body), `"orientation_path": "AGENTS.md"`, `"orientation_path": "other.md"`, 1))
			if err := os.WriteFile(path, body, 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "missing hooks", failedID: "runtime_hooks", mutate: func(t *testing.T, f readinessFixture) {
			removeFile(t, filepath.Join(f.workspace, ".codex", "hooks.json"))
		}},
		{name: "missing orchestration state", failedID: "orchestration_state", mutate: func(t *testing.T, f readinessFixture) {
			removeFile(t, filepath.Join(f.workspace, ".bcgos", "maestro-orchestration-state.json"))
		}},
		{name: "mismatched hook command", failedID: "runtime_hooks", mutate: func(t *testing.T, f readinessFixture) {
			path := filepath.Join(f.workspace, ".codex", "hooks.json")
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			body = []byte(strings.Replace(string(body), "codex context-injection", "codex stop-finalization", 1))
			if err := os.WriteFile(path, body, 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "duplicate owned hook", failedID: "runtime_hooks", mutate: duplicateFirstHook},
		{name: "missing installed CLI", failedID: "installed_cli", mutate: func(t *testing.T, f readinessFixture) {
			removeFile(t, f.cli)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newReadinessFixture(t, nil)
			test.mutate(t, fixture)
			report, err := Verify(fixture.options())
			if err == nil || report.Ready || report.State != "failed" || failedCheck(report) != test.failedID {
				t.Fatalf("Verify() report=%#v err=%v, want failed check %s", report, err, test.failedID)
			}
		})
	}
}

func TestVerifyRejectsSymlinksAndMismatchedIdentities(t *testing.T) {
	t.Run("workspace alias", func(t *testing.T) {
		fixture := newReadinessFixture(t, nil)
		alias := filepath.Join(filepath.Dir(fixture.workspace), "workspace-alias")
		if err := os.Symlink(fixture.workspace, alias); err != nil {
			t.Skipf("symlink unavailable: %v", err)
		}
		options := fixture.options()
		options.WorkspacePath = alias
		report, err := Verify(options)
		if err == nil || failedCheck(report) != "workspace_path" {
			t.Fatalf("workspace symlink report=%#v err=%v", report, err)
		}
	})

	t.Run("hook config symlink", func(t *testing.T) {
		fixture := newReadinessFixture(t, nil)
		path := filepath.Join(fixture.workspace, ".codex", "hooks.json")
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		removeFile(t, path)
		target := filepath.Join(filepath.Dir(fixture.workspace), "outside-hooks.json")
		if err := os.WriteFile(target, body, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, path); err != nil {
			t.Skipf("symlink unavailable: %v", err)
		}
		report, err := Verify(fixture.options())
		if err == nil || failedCheck(report) != "runtime_hooks" {
			t.Fatalf("hooks symlink report=%#v err=%v", report, err)
		}
	})

	t.Run("installed CLI symlink", func(t *testing.T) {
		fixture := newReadinessFixture(t, nil)
		removeFile(t, fixture.cli)
		target := filepath.Join(filepath.Dir(fixture.managedRoot), "other-bcgos")
		writeExecutable(t, target)
		if err := os.Symlink(target, fixture.cli); err != nil {
			t.Skipf("symlink unavailable: %v", err)
		}
		report, err := Verify(fixture.options())
		if err == nil || failedCheck(report) != "installed_cli" {
			t.Fatalf("CLI symlink report=%#v err=%v", report, err)
		}
	})

	t.Run("CLI version mismatch", func(t *testing.T) {
		fixture := newReadinessFixture(t, nil)
		options := fixture.options()
		options.CLIVersion = "0.2.0"
		report, err := Verify(options)
		if err == nil || failedCheck(report) != "installed_cli" {
			t.Fatalf("version mismatch report=%#v err=%v", report, err)
		}
	})

	t.Run("managed root mismatch", func(t *testing.T) {
		fixture := newReadinessFixture(t, nil)
		state := fixture.installState()
		state.ManagedRoot = filepath.Join(filepath.Dir(fixture.managedRoot), "different-managed-root")
		if err := installtx.WriteState(fixture.dataRoot, state); err != nil {
			t.Fatal(err)
		}
		report, err := Verify(fixture.options())
		if err == nil || failedCheck(report) != "installed_cli" {
			t.Fatalf("managed-root mismatch report=%#v err=%v", report, err)
		}
	})

	t.Run("capability track mismatch", func(t *testing.T) {
		fixture := newReadinessFixture(t, nil)
		options := fixture.options()
		options.CapabilityTracks = []string{"data-science"}
		report, err := Verify(options)
		if err == nil || failedCheck(report) != "runtime_projection" {
			t.Fatalf("track mismatch report=%#v err=%v", report, err)
		}
	})
}

type readinessFixture struct {
	workspace, dataRoot, managedRoot, cli string
	tracks                                []string
	runtime                               string
}

func newReadinessFixture(t *testing.T, tracks []string) readinessFixture {
	return newReadinessFixtureForRuntime(t, "codex", tracks)
}

func newReadinessFixtureForRuntime(t *testing.T, runtimeName string, tracks []string) readinessFixture {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	fixture := readinessFixture{
		workspace:   filepath.Join(root, "workspace"),
		dataRoot:    filepath.Join(root, "owner-data"),
		managedRoot: filepath.Join(root, "Maestro"),
		tracks:      append([]string(nil), tracks...),
		runtime:     runtimeName,
	}
	cliName := "bcgos"
	if runtime.GOOS == "windows" {
		cliName += ".exe"
	}
	fixture.cli = filepath.Join(fixture.managedRoot, "bin", cliName)
	writeExecutable(t, fixture.cli)
	initialized, err := workspace.Initialize(workspace.Options{WorkspacePath: fixture.workspace, DataRoot: fixture.dataRoot})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ownerctx.Initialize(fixture.dataRoot); err != nil {
		t.Fatal(err)
	}
	if _, err := workspaceagent.Initialize(fixture.dataRoot, initialized.WorkspaceID); err != nil {
		t.Fatal(err)
	}
	if _, err := agentscaffold.Scaffold(fixture.dataRoot, agentscaffold.WorkspaceRequest(initialized.WorkspaceID)); err != nil {
		t.Fatal(err)
	}
	if _, err := runtimeprojection.InstallForTracks(runtimeName, fixture.workspace, fixture.tracks); err != nil {
		t.Fatal(err)
	}
	if _, err := adaptercfg.Install(runtimeName, fixture.workspace, fixture.cli); err != nil {
		t.Fatal(err)
	}
	if err := installtx.WriteState(fixture.dataRoot, fixture.installState()); err != nil {
		t.Fatal(err)
	}
	return fixture
}

func (fixture readinessFixture) installState() installtx.State {
	targetOS := runtime.GOOS
	if targetOS != "windows" && targetOS != "darwin" {
		targetOS = "darwin"
	}
	targetArch := runtime.GOARCH
	if targetArch != "amd64" && targetArch != "arm64" {
		targetArch = "amd64"
	}
	return installtx.State{
		SchemaVersion: 2, ManagedRoot: fixture.managedRoot,
		Release: "0.1.0", Channel: "canary", CLIVersion: "0.1.0", BundleVersion: "0.1.0",
		TargetOS: targetOS, TargetArch: targetArch, ActivatedAt: time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC),
	}
}

func (fixture readinessFixture) options() Options {
	return Options{
		WorkspacePath: fixture.workspace, DataRoot: fixture.dataRoot,
		ExecutablePath: fixture.cli, CLIVersion: "0.1.0", CapabilityTracks: fixture.tracks, Runtime: fixture.runtime,
	}
}

func failedCheck(report Report) string {
	for _, check := range report.Checks {
		if check.State == "fail" {
			return check.ID
		}
	}
	return ""
}

func duplicateFirstHook(t *testing.T, fixture readinessFixture) {
	t.Helper()
	path := filepath.Join(fixture.workspace, ".codex", "hooks.json")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err := json.Unmarshal(body, &config); err != nil {
		t.Fatal(err)
	}
	hooks := config["hooks"].(map[string]any)
	groups := hooks["SessionStart"].([]any)
	group := groups[0].(map[string]any)
	entries := group["hooks"].([]any)
	group["hooks"] = append(entries, entries[0])
	body, err = json.MarshalIndent(config, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(body, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeExecutable(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("bcgos-test-binary"), 0o700); err != nil {
		t.Fatal(err)
	}
}

func removeFile(t *testing.T, path string) {
	t.Helper()
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
}

func quoteTestPath(path string) string {
	if runtime.GOOS == "windows" {
		return `"` + strings.ReplaceAll(path, `"`, `\"`) + `"`
	}
	return "'" + strings.ReplaceAll(path, "'", "'\"'\"'") + "'"
}
