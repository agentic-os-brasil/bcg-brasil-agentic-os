package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installer"
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
	// Use a supported target explicitly so the package-default contract is
	// exercised consistently on Linux CI as well as the pilot platforms.
	platform := "darwin"
	bootstrapper := filepath.Join(root, "bcgos-bootstrap_0.1.0_"+platform+"_"+runtime.GOARCH)
	if err := os.WriteFile(bootstrapper, []byte("seed"), 0o700); err != nil {
		t.Fatal(err)
	}
	options := options{}
	if err := resolveDefaultsAt(&options, root, platform, runtime.GOARCH, "/Users/pilot", ""); err != nil {
		t.Fatal(err)
	}
	if options.bootstrapper != bootstrapper || options.wizardDir != filepath.Join(root, "wizard") || options.releaseDir != filepath.Join(root, "release") || options.authorityRegistry != filepath.Join(root, "authority-registry.json") {
		t.Fatalf("package defaults = %#v", options)
	}
	if options.managedRoot != "/Users/pilot/Library/Application Support/Maestro" || options.dataRoot != "/Users/pilot/Library/Application Support/BCGOS" {
		t.Fatalf("user-space roots = %q, %q", options.managedRoot, options.dataRoot)
	}
}
