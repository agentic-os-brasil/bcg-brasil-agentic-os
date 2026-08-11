package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installer"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
)

func TestWizardHandlerKeepsStateReadOnlyAndActionsPostOnly(t *testing.T) {
	handler := wizardHandler(options{})
	tests := []struct {
		name   string
		method string
		path   string
		want   int
	}{
		{"state rejects post", http.MethodPost, "/api/state", http.StatusMethodNotAllowed},
		{"verify rejects get", http.MethodGet, "/api/verify", http.StatusMethodNotAllowed},
		{"install rejects get", http.MethodGet, "/api/install", http.StatusMethodNotAllowed},
		{"open data rejects get", http.MethodGet, "/api/open-data", http.StatusMethodNotAllowed},
		{"create workspace rejects get", http.MethodGet, "/api/create-workspace", http.StatusMethodNotAllowed},
		{"workspace flow select rejects get", http.MethodGet, "/api/workspace-flow/select", http.StatusMethodNotAllowed},
		{"workspace flow analyze rejects get", http.MethodGet, "/api/workspace-flow/analyze", http.StatusMethodNotAllowed},
		{"workspace flow confirm rejects get", http.MethodGet, "/api/workspace-flow/confirm", http.StatusMethodNotAllowed},
		{"launch runtime rejects get", http.MethodGet, "/api/launch-runtime", http.StatusMethodNotAllowed},
		{"close rejects get", http.MethodGet, "/api/close", http.StatusMethodNotAllowed},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(test.method, test.path, nil)
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			if recorder.Code != test.want {
				t.Fatalf("status = %d, want %d", recorder.Code, test.want)
			}
		})
	}
}

func TestFolderChooserCommandSupportsWindowsAndKeepsPromptLocal(t *testing.T) {
	command, arguments, err := folderChooserCommand("windows", "Escolha a fonte do Maestro")
	if err != nil {
		t.Fatal(err)
	}
	if command != "powershell.exe" {
		t.Fatalf("command = %q, want powershell.exe", command)
	}
	joined := strings.Join(arguments, " ")
	for _, want := range []string{"-STA", "System.Windows.Forms.FolderBrowserDialog", "Escolha a fonte do Maestro", "SelectedPath"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("windows chooser arguments do not contain %q: %s", want, joined)
		}
	}
	if strings.Contains(joined, "ExecutionPolicy") || strings.Contains(joined, "Bypass") {
		t.Fatalf("windows chooser must preserve the local PowerShell execution policy: %s", joined)
	}
	if _, _, err := folderChooserCommand("linux", "Escolha"); err == nil {
		t.Fatal("unsupported platform unexpectedly returned a chooser")
	}
}

func TestWizardLaunchRuntimeChoosesReadyWorkspaceAndUsesApprovedTarget(t *testing.T) {
	root := t.TempDir()
	dataRoot := filepath.Join(root, "data")
	workspacePath := filepath.Join(root, "workspace")
	if _, err := workspace.Initialize(workspace.Options{WorkspacePath: workspacePath, DataRoot: dataRoot}); err != nil {
		t.Fatal(err)
	}
	var launchedRuntime, launchedWorkspace string
	handler := wizardHandler(options{
		dataRoot:     dataRoot,
		sessionToken: "test-token",
		runtimeTargets: func() []runtimeTarget {
			return []runtimeTarget{{ID: "claude", Label: "Abrir no Claude Code"}}
		},
		workspacePath: func() (string, error) { return workspacePath, nil },
		launchRuntime: func(runtimeID, path string) error {
			launchedRuntime, launchedWorkspace = runtimeID, path
			return nil
		},
	})
	request := httptest.NewRequest(http.MethodPost, "/api/launch-runtime", strings.NewReader(`{"runtime":"claude"}`))
	request.Header.Set("X-Maestro-Session", "test-token")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if launchedRuntime != "claude" || launchedWorkspace != workspacePath {
		t.Fatalf("launch = %q %q", launchedRuntime, launchedWorkspace)
	}
}

func TestWizardLaunchRuntimeRejectsAnUnavailableTarget(t *testing.T) {
	handler := wizardHandler(options{
		sessionToken:   "test-token",
		runtimeTargets: func() []runtimeTarget { return nil },
	})
	request := httptest.NewRequest(http.MethodPost, "/api/launch-runtime", strings.NewReader(`{"runtime":"codex"}`))
	request.Header.Set("X-Maestro-Session", "test-token")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "não está disponível") {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestCodexWorkspaceLinkKeepsTheAbsoluteWorkspacePath(t *testing.T) {
	workspacePath := "/Users/pilot/Projects/maestro workspace"
	value, err := url.Parse(codexWorkspaceLink(workspacePath))
	if err != nil {
		t.Fatal(err)
	}
	prompt := value.Query().Get("prompt")
	if value.Scheme != "codex" || value.Host != "new" || value.Query().Get("path") != workspacePath || prompt != maestroCodexKickoffPrompt {
		t.Fatalf("deep link = %q", value.String())
	}
	if !strings.Contains(prompt, "Olá, Maestro! 🎼\n\n") || !strings.Contains(prompt, "\n\n🧭 Para começar") {
		t.Fatalf("kickoff prompt should preserve paragraph structure: %q", prompt)
	}
}

func TestClaudeCodeWorkspaceLinkKeepsTheAbsoluteWorkspacePath(t *testing.T) {
	workspacePath := "/Users/pilot/Projects/maestro workspace"
	value, err := url.Parse(claudeCodeWorkspaceLink(workspacePath))
	if err != nil {
		t.Fatal(err)
	}
	prompt := value.Query().Get("q")
	if value.Scheme != "claude" || value.Host != "code" || value.Path != "/new" || value.Query().Get("folder") != workspacePath || prompt != maestroClaudeKickoffPrompt {
		t.Fatalf("deep link = %q", value.String())
	}
	if !strings.Contains(prompt, "Olá, Maestro! 🎼\n\n") || !strings.Contains(prompt, "\n\n🧭 Para começar") {
		t.Fatalf("kickoff prompt should preserve paragraph structure: %q", prompt)
	}
	if !strings.Contains(prompt, "guia instalado de\nMaestro Onboarding") ||
		!strings.Contains(prompt, "não crie AGENTS.md") ||
		strings.Contains(prompt, "execute agora a skill /maestro-onboarding") {
		t.Fatalf("Claude kickoff must use the materialized guide and reject fabricated fallback files: %q", prompt)
	}
}

func TestClaudeCodeLaunchLinkSupportsWindowsDesktopHandoff(t *testing.T) {
	workspacePath := `C:\Users\pilot\Developer\maestro-os`
	link, supported := claudeCodeLaunchLink("windows", workspacePath)
	if !supported {
		t.Fatal("Windows Claude Desktop handoff should be supported")
	}
	parsed, err := url.Parse(link)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Scheme != "claude" || parsed.Host != "code" || parsed.Query().Get("folder") != workspacePath || parsed.Query().Get("q") != maestroClaudeKickoffPrompt {
		t.Fatalf("windows deep link = %q", link)
	}
	if _, supported := claudeCodeLaunchLink("linux", workspacePath); supported {
		t.Fatal("unsupported platform unexpectedly received a Claude Desktop handoff")
	}
}

func TestWindowsClaudeDesktopCandidatesIncludePerUserInstall(t *testing.T) {
	candidates := windowsClaudeDesktopCandidates(`C:\Users\pilot\AppData\Local`, `C:\Program Files`, `C:\Program Files (x86)`)
	want := filepath.Join(`C:\Users\pilot\AppData\Local`, "Programs", "Claude", "Claude.exe")
	if !containsString(candidates, want) {
		t.Fatalf("candidates = %q, want %q", candidates, want)
	}
}

func TestRuntimeLaunchSupportDoesNotAdvertiseCodexOnWindows(t *testing.T) {
	if runtimeLaunchSupported("windows", "codex") {
		t.Fatal("Windows Codex must not be offered until a Desktop handoff exists")
	}
	if !runtimeLaunchSupported("windows", "claude") {
		t.Fatal("Windows Claude Desktop handoff should remain supported")
	}
	if !runtimeLaunchSupported("darwin", "codex") {
		t.Fatal("macOS Codex handoff should remain supported")
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

func TestWizardCreatesTheDefaultWorkspaceWithoutTouchingAnImportSource(t *testing.T) {
	root := t.TempDir()
	dataRoot := filepath.Join(root, "data")
	workspacePath := filepath.Join(root, "Developer", "maestro-os")
	managedRoot := filepath.Join(root, "managed")
	cliPath := filepath.Join(managedRoot, "bin", "bcgos")
	if runtime.GOOS == "windows" {
		cliPath += ".exe"
	}
	if err := os.MkdirAll(filepath.Dir(cliPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cliPath, []byte("test"), 0o700); err != nil {
		t.Fatal(err)
	}
	handler := wizardHandler(options{
		dataRoot:      dataRoot,
		managedRoot:   managedRoot,
		sessionToken:  "test-token",
		workspacePath: func() (string, error) { return workspacePath, nil },
		configureWorkspace: func(options, string) (workspaceActivation, error) {
			return workspaceActivation{
				State:       "ready",
				Lifecycle:   lifecycleActivation{State: "configured", Events: []string{"SessionStart", "UserPromptSubmit", "PreToolUse", "PostToolUse", "Stop"}, HookReview: "owner_review_required"},
				Maintenance: maintenanceActivation{State: "active_loaded_enabled", NativeObserved: true, ModelBacked: "unavailable"},
			}, nil
		},
		authorizeSetup: func(options, string) (workspaceSetupAuthorization, error) {
			return workspaceSetupAuthorization{State: "active", GrantDigest: strings.Repeat("a", 64)}, nil
		},
	})
	request := httptest.NewRequest(http.MethodPost, "/api/create-workspace", strings.NewReader(`{"import_existing":false,"authorize_setup":true}`))
	request.Header.Set("X-Maestro-Session", "test-token")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Status            string            `json:"status"`
		WorkspacePath     string            `json:"workspace_path"`
		WorkspaceID       string            `json:"workspace_id"`
		Prompt            string            `json:"prompt"`
		DeepLinks         map[string]string `json:"deeplinks"`
		AdapterState      string            `json:"adapter_state"`
		ReadinessState    string            `json:"readiness_state"`
		SchedulerState    string            `json:"scheduler_state"`
		ReadyForRuntime   bool              `json:"ready_for_runtime"`
		DiagnosticCommand string            `json:"diagnostic_command"`
		DataRoot          string            `json:"data_root"`
		MemoryCommand     string            `json:"memory_status_command"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Status != "ready" || response.WorkspacePath != workspacePath || response.WorkspaceID == "" ||
		response.Prompt != maestroClaudeKickoffPrompt ||
		response.DeepLinks["claude_desktop"] == "" || response.DeepLinks["claude_code_desktop"] == "" || response.DeepLinks["codex"] == "" ||
		response.AdapterState != "configured" || response.ReadinessState != "ready" ||
		response.SchedulerState != "active_loaded_enabled" || !response.ReadyForRuntime ||
		response.DataRoot != dataRoot || !strings.Contains(response.MemoryCommand, "memory status --data-dir") || !strings.Contains(response.MemoryCommand, dataRoot) || !strings.Contains(response.MemoryCommand, "--workspace") || !strings.Contains(response.MemoryCommand, response.WorkspaceID) ||
		!strings.Contains(response.DiagnosticCommand, workspacePath) {
		t.Fatalf("workspace diagnostic = %#v", response)
	}
	inspection, err := workspace.Inspect(workspacePath, dataRoot)
	if err != nil || inspection.State != "ready" {
		t.Fatalf("workspace = %#v, err = %v", inspection, err)
	}
	if _, err := os.Stat(filepath.Join(dataRoot, "agents", "instances", "workspace-agent-"+inspection.WorkspaceID, "instance.json")); err != nil {
		t.Fatalf("workspace agent scaffold missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dataRoot, "owner", "registry.json")); err != nil {
		t.Fatalf("owner context bootstrap missing: %v", err)
	}
	for _, path := range []string{
		filepath.Join(dataRoot, "memory", "workspaces", inspection.WorkspaceID, "l1", "captures"),
		filepath.Join(dataRoot, "memory", "workspaces", inspection.WorkspaceID, "l1", "attested-captures"),
		filepath.Join(dataRoot, "memory", "workspaces", inspection.WorkspaceID, "commits"),
	} {
		if info, err := os.Stat(path); err != nil || !info.IsDir() {
			t.Fatalf("installed memory/daily bootstrap missing at %s: info=%v err=%v", path, info, err)
		}
	}
	for _, path := range []string{
		filepath.Join(workspacePath, "brain", "daily", "index.md"),
		filepath.Join(workspacePath, "brain", "daily", time.Now().Format("2006-01-02")+".md"),
	} {
		if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() {
			t.Fatalf("installed daily bootstrap missing at %s: info=%v err=%v", path, info, err)
		}
	}
	if _, err := os.Stat(filepath.Join(workspacePath, "brain", "organization", "bcg", "README.md")); err != nil {
		t.Fatalf("BCG organizational scaffold missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workspacePath, "agents", "client-accounts", "acme-example.md")); err != nil {
		t.Fatalf("ACME example scaffold missing: %v", err)
	}
	if !strings.Contains(recorder.Body.String(), `"activation"`) || !strings.Contains(recorder.Body.String(), `"active_loaded_enabled"`) || !strings.Contains(recorder.Body.String(), `"model_backed":"unavailable"`) {
		t.Fatalf("activation evidence missing from response: %s", recorder.Body.String())
	}
}

func TestAuthorizeWorkspaceSetupRecordsGrantWithoutLaunchingTheCLI(t *testing.T) {
	root := t.TempDir()
	dataRoot := filepath.Join(root, "data")
	workspacePath := filepath.Join(root, "Developer", "maestro-os")
	if _, err := workspace.Initialize(workspace.Options{WorkspacePath: workspacePath, DataRoot: dataRoot}); err != nil {
		t.Fatal(err)
	}
	grant, err := authorizeWorkspaceSetup(options{dataRoot: dataRoot}, workspacePath)
	if err != nil {
		t.Fatal(err)
	}
	if grant.State != "active" || len(grant.GrantDigest) != 64 {
		t.Fatalf("grant=%#v", grant)
	}
}

func TestWizardRendersInstalledDataRootAndMemoryStatusCommand(t *testing.T) {
	index, err := os.ReadFile(filepath.Join("..", "..", "installers", "wizard", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	app, err := os.ReadFile(filepath.Join("..", "..", "installers", "wizard", "app.js"))
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"handoff-data-root", "handoff-memory-command"} {
		if !strings.Contains(string(index), expected) || !strings.Contains(string(app), expected) {
			t.Fatalf("wizard handoff does not render %s", expected)
		}
	}
}

func TestWizardKeepsWorkspaceReadyWhenAuthorizationFails(t *testing.T) {
	root := t.TempDir()
	dataRoot := filepath.Join(root, "data")
	workspacePath := filepath.Join(root, "Developer", "maestro-os")
	configured := false
	handler := wizardHandler(options{
		dataRoot:     dataRoot,
		sessionToken: "test-token",
		workspacePath: func() (string, error) {
			return workspacePath, nil
		},
		authorizeSetup: func(options, string) (workspaceSetupAuthorization, error) {
			return workspaceSetupAuthorization{}, errors.New("approval service unavailable")
		},
		configureWorkspace: func(options, string) (workspaceActivation, error) {
			configured = true
			return workspaceActivation{State: "ready"}, nil
		},
	})
	request := httptest.NewRequest(http.MethodPost, "/api/create-workspace", strings.NewReader(`{"import_existing":false,"authorize_setup":true}`))
	request.Header.Set("X-Maestro-Session", "test-token")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"ready_for_runtime":true`) ||
		!strings.Contains(recorder.Body.String(), `"setup_state":"ready_with_warnings"`) || !strings.Contains(recorder.Body.String(), "approval service unavailable") || !configured {
		t.Fatalf("authorization failure blocked the usable workspace: status=%d configured=%v body=%s", recorder.Code, configured, recorder.Body.String())
	}
	inspection, err := workspace.Inspect(workspacePath, dataRoot)
	if err != nil || inspection.WorkspaceID == "" {
		t.Fatalf("bootstrap state should remain inspectable for retry: %#v err=%v", inspection, err)
	}
}

func TestWizardKeepsWorkspaceReadyWhenRuntimeConfigurationAndReceiptFail(t *testing.T) {
	root := t.TempDir()
	dataRoot := filepath.Join(root, "data")
	workspacePath := filepath.Join(root, "Developer", "maestro-os")
	handler := wizardHandler(options{
		dataRoot:     dataRoot,
		sessionToken: "test-token",
		workspacePath: func() (string, error) {
			return workspacePath, nil
		},
		authorizeSetup: func(options, string) (workspaceSetupAuthorization, error) {
			return workspaceSetupAuthorization{State: "active", GrantDigest: strings.Repeat("a", 64)}, nil
		},
		configureWorkspace: func(options, string) (workspaceActivation, error) {
			return workspaceActivation{}, errors.New("adapter verify unavailable")
		},
		newReceiptID: func() (string, error) {
			return "", errors.New("entropy unavailable")
		},
	})
	request := httptest.NewRequest(http.MethodPost, "/api/create-workspace", strings.NewReader(`{"import_existing":false,"authorize_setup":true}`))
	request.Header.Set("X-Maestro-Session", "test-token")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"ready_for_runtime":true`) ||
		!strings.Contains(recorder.Body.String(), `"receipt_state":"pending"`) ||
		!strings.Contains(recorder.Body.String(), "adapter verify unavailable") || !strings.Contains(recorder.Body.String(), "entropy unavailable") {
		t.Fatalf("advisory setup failures blocked handoff: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Handoff  workspaceHandoff `json:"handoff"`
		Warnings []string         `json:"warnings"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Handoff.WorkspacePath != workspacePath || response.Handoff.Prompt == "" || len(response.Warnings) != 2 || len(response.Handoff.Diagnostics) < 2 {
		t.Fatalf("handoff = %#v warnings=%#v", response.Handoff, response.Warnings)
	}
}

func TestWizardDefersSourceSelectionUntilWorkspaceBootstrapCompletes(t *testing.T) {
	root := t.TempDir()
	dataRoot := filepath.Join(root, "data")
	workspacePath := filepath.Join(root, "Developer", "maestro-os")
	sourcePath := filepath.Join(root, "prior-material")
	if err := os.MkdirAll(sourcePath, 0o700); err != nil {
		t.Fatal(err)
	}
	chooserSawWorkspace := false
	handler := wizardHandler(options{
		dataRoot:     dataRoot,
		managedRoot:  filepath.Join(root, "managed"),
		sessionToken: "test-token",
		workspacePath: func() (string, error) {
			return workspacePath, nil
		},
		chooseImportSource: func() (string, error) {
			_, err := os.Stat(filepath.Join(workspacePath, ".bcgos", "workspace.json"))
			chooserSawWorkspace = err == nil
			return sourcePath, err
		},
		configureWorkspace: func(options, string) (workspaceActivation, error) {
			return workspaceActivation{State: "ready", Lifecycle: lifecycleActivation{State: "configured"}, Maintenance: maintenanceActivation{State: "active_loaded_enabled"}}, nil
		},
		authorizeSetup: func(options, string) (workspaceSetupAuthorization, error) {
			return workspaceSetupAuthorization{State: "active", GrantDigest: strings.Repeat("b", 64)}, nil
		},
	})
	request := httptest.NewRequest(http.MethodPost, "/api/create-workspace", strings.NewReader(`{"import_existing":true,"authorize_setup":true}`))
	request.Header.Set("X-Maestro-Session", "test-token")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !chooserSawWorkspace || !strings.Contains(recorder.Body.String(), `"source_registered":true`) {
		t.Fatalf("status=%d chooserSawWorkspace=%v body=%s", recorder.Code, chooserSawWorkspace, recorder.Body.String())
	}
	if _, err := os.Stat(filepath.Join(workspacePath, ".bcgos", "import-intake.json")); err != nil {
		t.Fatalf("post-bootstrap source pointer missing: %v", err)
	}
}

func TestWizardKeepsWorkspaceReadyWhenSourceSelectionFails(t *testing.T) {
	root := t.TempDir()
	dataRoot := filepath.Join(root, "data")
	workspacePath := filepath.Join(root, "Developer", "maestro-os")
	handler := wizardHandler(options{
		dataRoot:     dataRoot,
		sessionToken: "test-token",
		workspacePath: func() (string, error) {
			return workspacePath, nil
		},
		chooseImportSource: func() (string, error) {
			return "", errors.New("chooser unavailable")
		},
		configureWorkspace: func(options, string) (workspaceActivation, error) {
			return workspaceActivation{State: "ready"}, nil
		},
		authorizeSetup: func(options, string) (workspaceSetupAuthorization, error) {
			return workspaceSetupAuthorization{State: "active", GrantDigest: strings.Repeat("b", 64)}, nil
		},
	})
	request := httptest.NewRequest(http.MethodPost, "/api/create-workspace", strings.NewReader(`{"import_existing":true,"authorize_setup":true}`))
	request.Header.Set("X-Maestro-Session", "test-token")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"ready_for_runtime":true`) ||
		!strings.Contains(recorder.Body.String(), `"source_registered":false`) || !strings.Contains(recorder.Body.String(), "chooser unavailable") {
		t.Fatalf("source chooser failure blocked handoff: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestWizardKeepsWorkspaceReadyWhenImportIntentWriteFails(t *testing.T) {
	root := t.TempDir()
	dataRoot := filepath.Join(root, "data")
	workspacePath := filepath.Join(root, "Developer", "maestro-os")
	sourcePath := filepath.Join(root, "prior-material")
	if err := os.MkdirAll(sourcePath, 0o700); err != nil {
		t.Fatal(err)
	}
	handler := wizardHandler(options{
		dataRoot:     dataRoot,
		sessionToken: "test-token",
		workspacePath: func() (string, error) {
			return workspacePath, nil
		},
		chooseImportSource: func() (string, error) {
			if err := os.MkdirAll(filepath.Join(workspacePath, ".bcgos", "import-intake.json"), 0o700); err != nil {
				return "", err
			}
			return sourcePath, nil
		},
		configureWorkspace: func(options, string) (workspaceActivation, error) {
			return workspaceActivation{State: "ready"}, nil
		},
		authorizeSetup: func(options, string) (workspaceSetupAuthorization, error) {
			return workspaceSetupAuthorization{State: "active", GrantDigest: strings.Repeat("b", 64)}, nil
		},
	})
	request := httptest.NewRequest(http.MethodPost, "/api/create-workspace", strings.NewReader(`{"import_existing":true,"authorize_setup":true}`))
	request.Header.Set("X-Maestro-Session", "test-token")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"ready_for_runtime":true`) ||
		!strings.Contains(recorder.Body.String(), `"source_registered":false`) || !strings.Contains(recorder.Body.String(), "intenção de ingestão") {
		t.Fatalf("import intent failure blocked handoff: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestConfigureWorkspaceRuntimeRunsIdempotentReadinessAndNativeMaintenance(t *testing.T) {
	root := t.TempDir()
	managedRoot := filepath.Join(root, "managed")
	dataRoot := filepath.Join(root, "data")
	workspacePath := filepath.Join(root, "Developer", "maestro-os")
	cliPath := filepath.Join(managedRoot, "bin", "bcgos")
	if err := os.MkdirAll(filepath.Dir(cliPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cliPath, []byte("test"), 0o700); err != nil {
		t.Fatal(err)
	}
	initialized, err := workspace.Initialize(workspace.Options{WorkspacePath: workspacePath, DataRoot: dataRoot})
	if err != nil {
		t.Fatal(err)
	}
	var calls [][]string
	runner := commandRunnerFunc(func(_ context.Context, executable string, arguments []string) ([]byte, error) {
		call := append([]string{executable}, arguments...)
		calls = append(calls, call)
		switch (len(calls)-1)%6 + 1 {
		case 1:
			return []byte(`{"state":"initialized"}`), nil
		case 2:
			return []byte(`{"runtime":"claude","state":"installed","projection":{"state":"installed"}}`), nil
		case 3:
			return []byte(`{"workspace":{"state":"ready","workspace_id":"` + initialized.WorkspaceID + `","workspace_path":"` + workspacePath + `"}}`), nil
		case 4:
			return []byte(`{"runtime":"claude","state":"installed","projection":{"state":"installed"}}`), nil
		case 5:
			return []byte(`{"runtime":"claude","state":"verified"}`), nil
		case 6:
			return []byte(`{"state":"enrolled","enrollment":{"workspace_id":"` + initialized.WorkspaceID + `"},"launch_agent":{"state":"active_loaded_enabled","file_present":true,"loaded":true,"enabled":true,"native_qualified":true}}`), nil
		default:
			t.Fatalf("unexpected command: %v", call)
			return nil, nil
		}
	})

	for attempt := 0; attempt < 2; attempt++ {
		activation, err := configureWorkspaceRuntime(options{managedRoot: managedRoot, commandRunner: runner}, workspacePath)
		if err != nil {
			t.Fatal(err)
		}
		if activation.State != "ready" || activation.Lifecycle.State != "configured" || activation.Lifecycle.StartSession != "configured" || activation.Lifecycle.HookReview != "owner_review_required" || activation.Lifecycle.NativeObserved != "pending_first_session" {
			t.Fatalf("lifecycle activation = %#v", activation)
		}
		if activation.Maintenance.State != "active_loaded_enabled" || !activation.Maintenance.NativeObserved || activation.Maintenance.ModelBacked != "unavailable" {
			t.Fatalf("maintenance activation = %#v", activation)
		}
	}
	wantOnce := [][]string{
		{cliPath, "init", workspacePath},
		{cliPath, "adapter", "install", "--runtime", "claude", "--executable", cliPath, workspacePath},
		{cliPath, "status", workspacePath},
		{cliPath, "adapter", "status", "--runtime", "claude", workspacePath},
		{cliPath, "adapter", "verify", "--runtime", "claude", workspacePath},
		{cliPath, "maintenance", "canary", "install-macos", "--workspace-path", workspacePath, "--executable", cliPath, "--confirm", "--launchctl"},
	}
	want := append(append([][]string{}, wantOnce...), wantOnce...)
	if len(calls) != len(want) {
		t.Fatalf("calls = %#v", calls)
	}
	for index := range want {
		if strings.Join(calls[index], "\x00") != strings.Join(want[index], "\x00") {
			t.Fatalf("call %d = %#v, want %#v", index, calls[index], want[index])
		}
	}
}

func TestConfigureWorkspaceRuntimeReportsAdapterVerifyFailureAndContinuesMaintenance(t *testing.T) {
	root := t.TempDir()
	managedRoot := filepath.Join(root, "managed")
	dataRoot := filepath.Join(root, "data")
	workspacePath := filepath.Join(root, "Developer", "maestro-os")
	cliPath := filepath.Join(managedRoot, "bin", "bcgos")
	if err := os.MkdirAll(filepath.Dir(cliPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cliPath, []byte("test"), 0o700); err != nil {
		t.Fatal(err)
	}
	initialized, err := workspace.Initialize(workspace.Options{WorkspacePath: workspacePath, DataRoot: dataRoot})
	if err != nil {
		t.Fatal(err)
	}
	call := 0
	runner := commandRunnerFunc(func(_ context.Context, _ string, arguments []string) ([]byte, error) {
		call++
		switch call {
		case 1:
			return []byte(`{"state":"initialized"}`), nil
		case 2, 4:
			return []byte(`{"runtime":"claude","state":"installed","projection":{"state":"installed"}}`), nil
		case 3:
			return []byte(`{"workspace":{"state":"ready","workspace_id":"` + initialized.WorkspaceID + `","workspace_path":"` + workspacePath + `"}}`), nil
		case 5:
			return []byte("verify diagnostic"), errors.New("verify unavailable")
		case 6:
			return []byte(`{"state":"enrolled","enrollment":{"workspace_id":"` + initialized.WorkspaceID + `"},"launch_agent":{"state":"active_loaded_enabled","file_present":true,"loaded":true,"enabled":true,"native_qualified":true}}`), nil
		default:
			return nil, errors.New("unexpected call: " + strings.Join(arguments, " "))
		}
	})
	activation, err := configureWorkspaceRuntimeForPlatform(options{managedRoot: managedRoot, commandRunner: runner}, workspacePath, "darwin")
	if err != nil || activation.State != "ready" || activation.Lifecycle.State != "configured" || activation.Maintenance.State != "active_loaded_enabled" ||
		!strings.Contains(strings.Join(activation.Diagnostics, " "), "verify diagnostic") {
		t.Fatalf("activation = %#v, error = %v", activation, err)
	}
}

func TestPrimaryRuntimeDefaultsToClaudeAndAllowsExplicitCodex(t *testing.T) {
	for _, test := range []struct {
		name  string
		input string
		want  string
		err   bool
	}{
		{name: "default", want: "claude"},
		{name: "explicit Codex", input: "codex", want: "codex"},
		{name: "unsupported", input: "other", err: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := primaryRuntime(options{primaryRuntime: test.input})
			if (err != nil) != test.err || got != test.want {
				t.Fatalf("primaryRuntime(%q) = %q, %v; want %q, error=%v", test.input, got, err, test.want, test.err)
			}
		})
	}
}

func TestConfigureWorkspaceRuntimeReportsLaunchdDegradationWithoutBlocking(t *testing.T) {
	root := t.TempDir()
	managedRoot := filepath.Join(root, "managed")
	dataRoot := filepath.Join(root, "data")
	workspacePath := filepath.Join(root, "Developer", "maestro-os")
	cliPath := filepath.Join(managedRoot, "bin", "bcgos")
	if err := os.MkdirAll(filepath.Dir(cliPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cliPath, []byte("test"), 0o700); err != nil {
		t.Fatal(err)
	}
	initialized, err := workspace.Initialize(workspace.Options{WorkspacePath: workspacePath, DataRoot: dataRoot})
	if err != nil {
		t.Fatal(err)
	}
	call := 0
	runner := commandRunnerFunc(func(_ context.Context, _ string, _ []string) ([]byte, error) {
		call++
		switch call {
		case 1:
			return []byte(`{"runtime":"claude","state":"installed","projection":{"state":"installed"}}`), nil
		case 2:
			return []byte(`{"runtime":"claude","state":"installed","projection":{"state":"installed"}}`), nil
		case 3:
			return []byte(`{"workspace":{"state":"ready","workspace_id":"` + initialized.WorkspaceID + `"}}`), nil
		case 4:
			return []byte(`{"runtime":"claude","state":"installed","projection":{"state":"installed"}}`), nil
		case 5:
			return []byte(`{"runtime":"claude","state":"verified"}`), nil
		case 6:
			return []byte(`{"state":"enrolled","enrollment":{"workspace_id":"` + initialized.WorkspaceID + `"},"launch_agent":{"state":"file_present_native_qualification_pending","file_present":true,"native_qualified":false}}`), nil
		default:
			return nil, errors.New("unexpected call")
		}
	})

	activation, err := configureWorkspaceRuntime(options{managedRoot: managedRoot, commandRunner: runner}, workspacePath)
	if err != nil || activation.State != "ready" || len(activation.Diagnostics) == 0 ||
		!strings.Contains(strings.Join(activation.Diagnostics, " "), "launchd ainda não ficou ativo") {
		t.Fatalf("activation = %#v, error = %v", activation, err)
	}
}

func TestConfigureWorkspaceRuntimeCanarySimpleActivatesBeforeNativeQualification(t *testing.T) {
	root := t.TempDir()
	managedRoot := filepath.Join(root, "managed")
	dataRoot := filepath.Join(root, "data")
	workspacePath := filepath.Join(root, "Developer", "maestro-os")
	cliPath := filepath.Join(managedRoot, "bin", "bcgos")
	if err := os.MkdirAll(filepath.Dir(cliPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cliPath, []byte("test"), 0o700); err != nil {
		t.Fatal(err)
	}
	initialized, err := workspace.Initialize(workspace.Options{WorkspacePath: workspacePath, DataRoot: dataRoot})
	if err != nil {
		t.Fatal(err)
	}
	call := 0
	runner := commandRunnerFunc(func(_ context.Context, _ string, _ []string) ([]byte, error) {
		call++
		switch call {
		case 1, 2:
			return []byte(`{"runtime":"claude","state":"installed","projection":{"state":"installed"}}`), nil
		case 3:
			return []byte(`{"workspace":{"state":"ready","workspace_id":"` + initialized.WorkspaceID + `"}}`), nil
		case 4:
			return []byte(`{"runtime":"claude","state":"installed","projection":{"state":"installed"}}`), nil
		case 5:
			return []byte(`{"runtime":"claude","state":"verified"}`), nil
		case 6:
			return []byte(`{"state":"enrolled","enrollment":{"workspace_id":"` + initialized.WorkspaceID + `"},"launch_agent":{"state":"active_loaded_enabled","file_present":true,"loaded":true,"enabled":true,"native_qualified":false}}`), nil
		default:
			return nil, errors.New("unexpected call")
		}
	})

	activation, err := configureWorkspaceRuntimeForPlatform(options{
		managedRoot: managedRoot, commandRunner: runner,
	}, workspacePath, "darwin")
	if err != nil {
		t.Fatal(err)
	}
	if activation.State != "ready" || !activation.Maintenance.NativeObserved || activation.Maintenance.NativeQualified ||
		!strings.Contains(strings.Join(activation.Diagnostics, " "), "observação nativa") {
		t.Fatalf("activation = %#v", activation)
	}
}

func TestConfigureWorkspaceRuntimeOnWindowsCompletesWithoutMacOSMaintenance(t *testing.T) {
	root := t.TempDir()
	managedRoot := filepath.Join(root, "managed")
	dataRoot := filepath.Join(root, "data")
	workspacePath := filepath.Join(root, "Developer", "maestro-os")
	cliPath := filepath.Join(managedRoot, "bin", "bcgos")
	if err := os.MkdirAll(filepath.Dir(cliPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cliPath, []byte("test"), 0o700); err != nil {
		t.Fatal(err)
	}
	initialized, err := workspace.Initialize(workspace.Options{WorkspacePath: workspacePath, DataRoot: dataRoot})
	if err != nil {
		t.Fatal(err)
	}
	var calls [][]string
	runner := commandRunnerFunc(func(_ context.Context, executable string, arguments []string) ([]byte, error) {
		calls = append(calls, append([]string{executable}, arguments...))
		switch len(calls) {
		case 1:
			return []byte(`{"state":"initialized"}`), nil
		case 2, 4:
			return []byte(`{"runtime":"claude","state":"installed","projection":{"state":"installed"}}`), nil
		case 3:
			return []byte(`{"workspace":{"state":"ready","workspace_id":"` + initialized.WorkspaceID + `","workspace_path":"` + workspacePath + `"}}`), nil
		case 5:
			return []byte(`{"runtime":"claude","state":"verified"}`), nil
		default:
			return nil, errors.New("unexpected maintenance command on Windows")
		}
	})
	activation, err := configureWorkspaceRuntimeForPlatform(options{managedRoot: managedRoot, commandRunner: runner}, workspacePath, "windows")
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 5 {
		t.Fatalf("calls = %#v; Windows must stop after adapter verification", calls)
	}
	if activation.State != "ready" || activation.Lifecycle.State != "configured" ||
		activation.Maintenance.State != "runtime_scheduler_optional" ||
		activation.Maintenance.NativeObserved || activation.Maintenance.Schedule != "not_configured" {
		t.Fatalf("Windows activation = %#v", activation)
	}
}

func TestWizardStateReportsOnlyARegularInstalledCLI(t *testing.T) {
	managedRoot := filepath.Join(t.TempDir(), "managed")
	cliPath := filepath.Join(managedRoot, "bin", "bcgos")
	if runtime.GOOS == "windows" {
		cliPath += ".exe"
	}
	if err := os.MkdirAll(filepath.Dir(cliPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cliPath, []byte("test"), 0o700); err != nil {
		t.Fatal(err)
	}
	handler := wizardHandler(options{managedRoot: managedRoot})
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/state", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var state struct {
		Installed bool   `json:"installed"`
		CLIPath   string `json:"cli_path"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &state); err != nil {
		t.Fatal(err)
	}
	if !state.Installed || state.CLIPath != cliPath {
		t.Fatalf("installed state = %#v, want %q", state, cliPath)
	}
}

func TestWizardStateDisclosesControlledWindowsLocalBeta(t *testing.T) {
	handler := wizardHandler(options{nativeTrustMode: installer.NativeTrustWindowsLocalBeta})
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/state", nil))
	var state struct {
		Trust     string `json:"trust"`
		LocalBeta bool   `json:"local_beta"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &state); err != nil {
		t.Fatal(err)
	}
	if state.Trust != "windows_local_beta" || !state.LocalBeta {
		t.Fatalf("local beta state = %#v", state)
	}
}

func TestWizardCloseRequiresSessionAndInvokesShutdown(t *testing.T) {
	closed := false
	handler := wizardHandler(options{sessionToken: "test-token", shutdown: func() { closed = true }})
	request := httptest.NewRequest(http.MethodPost, "/api/close", nil)
	request.Header.Set("X-Maestro-Session", "test-token")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if !closed {
		t.Fatal("close endpoint did not invoke the session shutdown")
	}
	if !strings.Contains(recorder.Body.String(), `"closing"`) {
		t.Fatalf("body = %s", recorder.Body.String())
	}
}

func TestWizardLifecycleDrainsActiveMutationAndRejectsNewWork(t *testing.T) {
	lifecycle := newWizardLifecycle()
	if !lifecycle.beginMutation() {
		t.Fatal("first mutation was rejected")
	}
	if !lifecycle.requestClose() {
		t.Fatal("first close was not accepted")
	}
	if lifecycle.requestClose() {
		t.Fatal("repeated close was not idempotent")
	}
	if lifecycle.beginMutation() {
		t.Fatal("new mutation was accepted after close request")
	}
	drained := make(chan error, 1)
	go func() { drained <- lifecycle.waitDrained(context.Background()) }()
	select {
	case <-drained:
		t.Fatal("lifecycle drained before active mutation completed")
	case <-time.After(20 * time.Millisecond):
	}
	lifecycle.endMutation()
	select {
	case err := <-drained:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("lifecycle did not drain active mutation")
	}
}

func TestWizardOpenDataReportsMissingDataRoot(t *testing.T) {
	handler := wizardHandler(options{dataRoot: filepath.Join(t.TempDir(), "missing"), sessionToken: "test-token"})
	request := httptest.NewRequest(http.MethodPost, "/api/open-data", nil)
	request.Header.Set("X-Maestro-Session", "test-token")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
	if !strings.Contains(recorder.Body.String(), "pasta de dados ainda não existe") {
		t.Fatalf("body = %s", recorder.Body.String())
	}
}

func TestWizardMutationsRequireSessionToken(t *testing.T) {
	handler := wizardHandler(options{sessionToken: "expected"})
	request := httptest.NewRequest(http.MethodPost, "/api/verify", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusForbidden)
	}
}

func TestWizardMutationsRejectWrongOrigin(t *testing.T) {
	handler := wizardHandler(options{sessionToken: "expected", origin: "http://127.0.0.1:1234"})
	request := httptest.NewRequest(http.MethodPost, "/api/verify", nil)
	request.Header.Set("X-Maestro-Session", "expected")
	request.Header.Set("Origin", "http://evil.example")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusForbidden)
	}
}

func TestSimulationRunsConfirmationBoundInstall(t *testing.T) {
	root := t.TempDir()
	managedRoot := filepath.Join(root, "managed")
	dataRoot := filepath.Join(root, "data")
	handler := wizardHandler(options{
		simulate: true, simulationRoot: root, managedRoot: managedRoot, dataRoot: dataRoot,
		sessionToken: "test-token",
	})
	verifyRequest := httptest.NewRequest(http.MethodPost, "/api/verify", nil)
	verifyRequest.Header.Set("X-Maestro-Session", "test-token")
	verifyRecorder := httptest.NewRecorder()
	handler.ServeHTTP(verifyRecorder, verifyRequest)
	if verifyRecorder.Code != http.StatusOK {
		t.Fatalf("verify status = %d, body = %s", verifyRecorder.Code, verifyRecorder.Body.String())
	}
	var plan installer.Plan
	if err := json.Unmarshal(verifyRecorder.Body.Bytes(), &plan); err != nil {
		t.Fatal(err)
	}
	if plan.PlanDigest == "" || plan.Release != "0.1.0-simulation" {
		t.Fatalf("simulation plan = %#v", plan)
	}
	installRequest := httptest.NewRequest(http.MethodPost, "/api/install", bytes.NewBufferString(`{"plan_digest":"`+plan.PlanDigest+`"}`))
	installRequest.Header.Set("X-Maestro-Session", "test-token")
	installRecorder := httptest.NewRecorder()
	handler.ServeHTTP(installRecorder, installRequest)
	if installRecorder.Code != http.StatusOK {
		t.Fatalf("install status = %d, body = %s", installRecorder.Code, installRecorder.Body.String())
	}
	if _, err := os.Stat(filepath.Join(managedRoot, "INSTALL-REHEARSAL.txt")); err != nil {
		t.Fatal(err)
	}
}

func TestSimulationInstallRejectsWrongPlanDigest(t *testing.T) {
	root := t.TempDir()
	handler := wizardHandler(options{simulate: true, simulationRoot: root, managedRoot: filepath.Join(root, "managed"), dataRoot: filepath.Join(root, "data"), sessionToken: "test-token"})
	verifyRequest := httptest.NewRequest(http.MethodPost, "/api/verify", nil)
	verifyRequest.Header.Set("X-Maestro-Session", "test-token")
	verifyRecorder := httptest.NewRecorder()
	handler.ServeHTTP(verifyRecorder, verifyRequest)
	installRequest := httptest.NewRequest(http.MethodPost, "/api/install", bytes.NewBufferString(`{"plan_digest":"wrong"}`))
	installRequest.Header.Set("X-Maestro-Session", "test-token")
	installRecorder := httptest.NewRecorder()
	handler.ServeHTTP(installRecorder, installRequest)
	if installRecorder.Code != http.StatusConflict {
		t.Fatalf("install status = %d, want %d", installRecorder.Code, http.StatusConflict)
	}
}

func TestResolveDefaultsRequiresOneNativeBootstrapper(t *testing.T) {
	root := t.TempDir()
	options := options{wizardDir: filepath.Join(root, "wizard"), releaseDir: filepath.Join(root, "release"), authorityRegistry: filepath.Join(root, "registry.json")}
	if err := os.MkdirAll(options.wizardDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(options.releaseDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(options.authorityRegistry, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := resolveDefaultsAt(&options, root, runtime.GOOS, runtime.GOARCH, "/Users/pilot", ""); err == nil {
		t.Fatal("resolveDefaults accepted a package without a native bootstrapper")
	}
}

func TestResolveDefaultsRejectsIncompleteInstallerPackage(t *testing.T) {
	root := t.TempDir()
	bootstrapper := filepath.Join(root, "bcgos-bootstrap_0.1.0_"+runtime.GOOS+"_"+runtime.GOARCH)
	if runtime.GOOS == "windows" {
		bootstrapper += ".exe"
	}
	if err := os.WriteFile(bootstrapper, []byte("seed"), 0o700); err != nil {
		t.Fatal(err)
	}
	options := options{}
	err := resolveDefaultsAt(&options, root, runtime.GOOS, runtime.GOARCH, "/Users/pilot", "")
	if err == nil || !strings.Contains(err.Error(), "installer_package_incomplete") {
		t.Fatalf("resolveDefaults accepted an incomplete package: %v", err)
	}
}

func TestResolveBuildTrustProfile(t *testing.T) {
	tests := []struct {
		name, profile, issuer, keyID, registrySHA, bootstrapperSHA string
		wantMode                                                   installer.NativeTrustMode
		wantErr                                                    bool
	}{
		{name: "strict defaults", profile: "strict", wantMode: installer.NativeTrustStrict},
		{name: "canary simple", profile: "canary-simple", wantMode: installer.NativeTrustCanarySimple},
		{name: "empty defaults", wantMode: installer.NativeTrustStrict},
		{name: "partial beta fails closed", profile: "windows-local-beta", issuer: "beta", wantErr: true},
		{name: "strict rejects pins", profile: "strict", issuer: "beta", wantErr: true},
		{
			name: "complete beta", profile: "windows-local-beta", issuer: "maestro-beta-local", keyID: "beta-20260805",
			registrySHA: strings.Repeat("a", 64), bootstrapperSHA: strings.Repeat("b", 64),
			wantMode: installer.NativeTrustWindowsLocalBeta,
		},
		{name: "unknown profile", profile: "skip-signature", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mode, _, err := resolveBuildTrustProfile(test.profile, test.issuer, test.keyID, test.registrySHA, test.bootstrapperSHA)
			if (err != nil) != test.wantErr {
				t.Fatalf("resolveBuildTrustProfile() error = %v, wantErr %v", err, test.wantErr)
			}
			if !test.wantErr && mode != test.wantMode {
				t.Fatalf("mode = %q, want %q", mode, test.wantMode)
			}
		})
	}
}

func TestInstallerOptionsCarryBuildTrustProfile(t *testing.T) {
	input := options{
		nativeTrustMode: installer.NativeTrustWindowsLocalBeta,
		localBetaPins: installer.LocalBetaPins{
			AuthorityRegistrySHA256: strings.Repeat("a", 64), BootstrapperSHA256: strings.Repeat("b", 64),
			Issuer: "maestro-beta-local", KeyID: "beta-20260805",
		},
	}
	got := installerOptions(input)
	if got.NativeTrustMode != installer.NativeTrustWindowsLocalBeta || got.LocalBetaPins.Issuer != "maestro-beta-local" {
		t.Fatalf("installer options lost build trust profile: %#v", got)
	}
}

func TestResolvePreviewDefaultsDoesNotRequireReleaseInputs(t *testing.T) {
	options := options{}
	if err := resolvePreviewDefaults(&options); err != nil {
		t.Fatal(err)
	}
	if filepath.Base(options.wizardDir) != "wizard" {
		t.Fatalf("preview wizard dir = %q", options.wizardDir)
	}
	if options.releaseDir != "" || options.bootstrapper != "" || options.authorityRegistry != "" {
		t.Fatalf("preview unexpectedly resolved release inputs: %#v", options)
	}
}

func TestResolveSimulationDefaultsUsesIsolatedRoots(t *testing.T) {
	root := t.TempDir()
	options := options{wizardDir: filepath.Join(root, "wizard"), simulationRoot: filepath.Join(root, "sandbox")}
	if err := resolveSimulationDefaults(&options); err != nil {
		t.Fatal(err)
	}
	if options.managedRoot != filepath.Join(root, "sandbox", "managed") || options.dataRoot != filepath.Join(root, "sandbox", "data") {
		t.Fatalf("simulation roots = %q, %q", options.managedRoot, options.dataRoot)
	}
	if options.releaseDir != "" || options.bootstrapper != "" || options.authorityRegistry != "" {
		t.Fatalf("simulation unexpectedly resolved release inputs: %#v", options)
	}
}

func TestResolveDefaultsUsesUserSpaceRootsWhenPackageIsComplete(t *testing.T) {
	root := t.TempDir()
	// Exercise native Windows path semantics on Windows. Linux is a build
	// target, not a pilot installer target, so it exercises the Darwin package
	// contract with POSIX paths instead.
	platform, home, localAppData := "darwin", "/Users/pilot", ""
	wantManaged := "/Users/pilot/Library/Application Support/Maestro"
	wantData := "/Users/pilot/Library/Application Support/BCGOS"
	suffix := ""
	if runtime.GOOS == "windows" {
		platform = "windows"
		home = `C:\Users\pilot`
		localAppData = `C:\Users\pilot\AppData\Local`
		wantManaged = `C:\Users\pilot\AppData\Local\Maestro`
		wantData = `C:\Users\pilot\AppData\Local\BCGOS`
		suffix = ".exe"
	}
	bootstrapper := filepath.Join(root, "bcgos-bootstrap_0.1.0_"+platform+"_"+runtime.GOARCH+suffix)
	if err := os.WriteFile(bootstrapper, []byte("seed"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "wizard"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "release"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "authority-registry.json"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	options := options{}
	if err := resolveDefaultsAt(&options, root, platform, runtime.GOARCH, home, localAppData); err != nil {
		t.Fatal(err)
	}
	if options.bootstrapper != bootstrapper || options.wizardDir != filepath.Join(root, "wizard") || options.releaseDir != filepath.Join(root, "release") || options.authorityRegistry != filepath.Join(root, "authority-registry.json") {
		t.Fatalf("package defaults = %#v", options)
	}
	if options.managedRoot != wantManaged || options.dataRoot != wantData {
		t.Fatalf("user-space roots = %q, %q", options.managedRoot, options.dataRoot)
	}
}
