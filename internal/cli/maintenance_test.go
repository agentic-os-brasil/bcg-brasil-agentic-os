package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/maintenance"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

func TestContinuityScheduleSeparatesCheckpointAndDreams(t *testing.T) {
	jobs := schedulerJobsForTrigger("presence")
	byID := map[string]scheduler.Job{}
	for _, job := range jobs {
		byID[job.ID] = job
	}
	for _, id := range []string{maintenance.MemoryCheckpointJobID, maintenance.MemoryLightDreamJobID} {
		job := byID[id]
		if job.Cadence != scheduler.Interval || job.IntervalHours != 3 || job.MaxCatchUp != 1 {
			t.Fatalf("%s schedule=%#v", id, job)
		}
	}
	deep := byID[maintenance.MemoryDeepDreamJobID]
	if deep.Cadence != scheduler.Weekly || deep.Weekday != time.Sunday || deep.MaxCatchUp != 1 {
		t.Fatalf("deep dream schedule=%#v", deep)
	}
}

func TestCheckpointHandlerIsOperableButDreamsRemainUnavailable(t *testing.T) {
	enrollment := maintenance.CanaryEnrollment{Activated: []maintenance.Activation{{JobID: maintenance.MemoryCheckpointJobID, QualificationDigest: maintenance.QualificationDigest(maintenance.MemoryCheckpointJobID)}}}
	handlers, qualification, activated := maintenanceHandlers(t.TempDir(), "maestro-system", enrollment, true)
	if handlers[maintenance.MemoryCheckpointJobID] == nil || qualification[maintenance.MemoryCheckpointJobID] == "" || !containsString(activated, maintenance.MemoryCheckpointJobID) {
		t.Fatalf("checkpoint not operable: handlers=%#v qualification=%#v activated=%#v", handlers, qualification, activated)
	}
	if handlers[maintenance.MemoryLightDreamJobID] != nil || handlers[maintenance.MemoryDeepDreamJobID] != nil {
		t.Fatalf("model-backed dreams were promoted: %#v", handlers)
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
	if result["idle_eligibility"] != "explicit_evidence_required_unknown_fails_closed" || result["memory_checkpoint"] != "locally_qualified_only_after_canary_enrollment" || result["memory_dreaming"] != "unavailable_without_synthesis_adapter" || result["pulse_interval_seconds"] != float64(900) {
		t.Fatalf("continuity capability truth drifted: %#v", result)
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
