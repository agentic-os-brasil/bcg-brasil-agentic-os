package cli

import (
	"bytes"
	"encoding/json"
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
