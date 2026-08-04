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
	if value.Scheme != "codex" || value.Host != "new" || value.Query().Get("path") != workspacePath || !strings.Contains(value.Query().Get("prompt"), "workspace Maestro") {
		t.Fatalf("deep link = %q", value.String())
	}
}

func TestClaudeCodeWorkspaceLinkKeepsTheAbsoluteWorkspacePath(t *testing.T) {
	workspacePath := "/Users/pilot/Projects/maestro workspace"
	value, err := url.Parse(claudeCodeWorkspaceLink(workspacePath))
	if err != nil {
		t.Fatal(err)
	}
	if value.Scheme != "claude" || value.Host != "code" || value.Path != "/new" || value.Query().Get("folder") != workspacePath || !strings.Contains(value.Query().Get("q"), "workspace Maestro") {
		t.Fatalf("deep link = %q", value.String())
	}
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
	})
	request := httptest.NewRequest(http.MethodPost, "/api/create-workspace", strings.NewReader(`{"import_existing":false}`))
	request.Header.Set("X-Maestro-Session", "test-token")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Status            string `json:"status"`
		WorkspacePath     string `json:"workspace_path"`
		AdapterState      string `json:"adapter_state"`
		ReadinessState    string `json:"readiness_state"`
		SchedulerState    string `json:"scheduler_state"`
		ReadyForRuntime   bool   `json:"ready_for_runtime"`
		DiagnosticCommand string `json:"diagnostic_command"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Status != "ready" || response.WorkspacePath != workspacePath ||
		response.AdapterState != "configured" || response.ReadinessState != "ready" ||
		response.SchedulerState != "active_loaded_enabled" || !response.ReadyForRuntime ||
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
	if !strings.Contains(recorder.Body.String(), `"activation"`) || !strings.Contains(recorder.Body.String(), `"active_loaded_enabled"`) || !strings.Contains(recorder.Body.String(), `"model_backed":"unavailable"`) {
		t.Fatalf("activation evidence missing from response: %s", recorder.Body.String())
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
		if activation.State != "ready" || activation.Lifecycle.State != "configured" || activation.Lifecycle.StartSession != "configured" || activation.Lifecycle.HookReview != "owner_review_required" || activation.Lifecycle.NativeObserved != "unavailable_pending_first_session" {
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

func TestConfigureWorkspaceRuntimeDoesNotDeclareReadyWhenLaunchdIsNotNative(t *testing.T) {
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

	_, err = configureWorkspaceRuntime(options{managedRoot: managedRoot, commandRunner: runner}, workspacePath)
	if err == nil || !strings.Contains(err.Error(), "launchd não ficou ativo") || !strings.Contains(err.Error(), "maintenance canary install-macos") || !strings.Contains(err.Error(), "--launchctl") {
		t.Fatalf("error = %v", err)
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
	if err := resolveDefaultsAt(&options, root, runtime.GOOS, runtime.GOARCH, "/Users/pilot", ""); err == nil {
		t.Fatal("resolveDefaults accepted a package without a native bootstrapper")
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
