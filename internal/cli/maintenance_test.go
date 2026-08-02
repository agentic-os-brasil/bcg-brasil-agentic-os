package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMaintenanceStatusReportsWorkerAndNativeEvidence(t *testing.T) {
	var output bytes.Buffer
	if code := Run([]string{"maintenance", "status"}, &output, &output); code != ExitOK {
		t.Fatalf("status exit = %d, output = %s", code, output.String())
	}
	var result map[string]any
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result["executor_state"] != "runtime_worker_ready_for_explicit_qualified_handlers" || result["catalog_state"] != "catalog_only" || result["native_adapters"] != "macos_adapter_available_windows_unavailable" {
		t.Fatalf("unexpected status: %#v", result)
	}
}

func TestMaintenanceWakeFailsClosedWithoutReceipt(t *testing.T) {
	var output bytes.Buffer
	if code := Run([]string{"maintenance", "wake", "--trigger", "presence"}, &output, &output); code != ExitUnavailable {
		t.Fatalf("wake exit = %d, output = %s", code, output.String())
	}
	if !strings.Contains(output.String(), `"state": "unavailable"`) || !strings.Contains(output.String(), "no receipt") {
		t.Fatalf("unexpected wake output: %s", output.String())
	}
}

func TestMaintenanceEventWakeRequiresExplicitEventID(t *testing.T) {
	var output bytes.Buffer
	if code := Run([]string{"maintenance", "wake", "--trigger", "event"}, &output, &output); code != ExitUsage {
		t.Fatalf("event wake exit = %d, output = %s", code, output.String())
	}
	if !strings.Contains(output.String(), "requires --event-id") {
		t.Fatalf("unexpected event wake output: %s", output.String())
	}
}

func TestMaintenanceEventWakeRejectsMalformedEventIDBeforeStateAccess(t *testing.T) {
	var output bytes.Buffer
	if code := Run([]string{"maintenance", "wake", "--trigger", "event", "--event-id", "malformed event"}, &output, &output); code != ExitUsage {
		t.Fatalf("malformed event wake exit = %d, output = %s", code, output.String())
	}
	if !strings.Contains(output.String(), "bounded event ID") {
		t.Fatalf("unexpected malformed event wake output: %s", output.String())
	}
}

func TestCanaryFixtureUsesIsolatedDataRoot(t *testing.T) {
	currentHome, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	fixture := filepath.Join(t.TempDir(), "home")
	root, err := canaryDataRoot(fixture, currentHome)
	if err != nil {
		t.Fatal(err)
	}
	relative, err := filepath.Rel(fixture, root)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		t.Fatalf("fixture root escaped isolation: root=%q home=%q", root, fixture)
	}
	production, err := defaultDataRoot()
	if err != nil {
		t.Fatal(err)
	}
	if samePathCLI(root, production) {
		t.Fatalf("fixture root reused production root: %q", root)
	}
}
