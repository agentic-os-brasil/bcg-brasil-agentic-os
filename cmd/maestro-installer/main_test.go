package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
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

func TestWizardOpenDataReportsMissingDataRoot(t *testing.T) {
	handler := wizardHandler(options{dataRoot: filepath.Join(t.TempDir(), "missing")})
	request := httptest.NewRequest(http.MethodPost, "/api/open-data", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
	if !strings.Contains(recorder.Body.String(), "pasta de dados ainda não existe") {
		t.Fatalf("body = %s", recorder.Body.String())
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

func TestResolveDefaultsUsesUserSpaceRootsWhenPackageIsComplete(t *testing.T) {
	root := t.TempDir()
	bootstrapper := filepath.Join(root, "bcgos-bootstrap_0.1.0_"+runtime.GOOS+"_"+runtime.GOARCH)
	if runtime.GOOS == "windows" {
		bootstrapper += ".exe"
	}
	if err := os.WriteFile(bootstrapper, []byte("seed"), 0o700); err != nil {
		t.Fatal(err)
	}
	options := options{}
	if err := resolveDefaultsAt(&options, root, runtime.GOOS, runtime.GOARCH, "/Users/pilot", ""); err != nil {
		t.Fatal(err)
	}
	if options.bootstrapper != bootstrapper || options.wizardDir != filepath.Join(root, "wizard") || options.releaseDir != filepath.Join(root, "release") || options.authorityRegistry != filepath.Join(root, "authority-registry.json") {
		t.Fatalf("package defaults = %#v", options)
	}
	if options.managedRoot != "/Users/pilot/Library/Application Support/Maestro" || options.dataRoot != "/Users/pilot/Library/Application Support/BCGOS" {
		t.Fatalf("user-space roots = %q, %q", options.managedRoot, options.dataRoot)
	}
}
