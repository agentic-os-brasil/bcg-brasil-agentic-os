package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/maintenance"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
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

func TestCanaryLaunchAgentRequestsNativeIdleObservation(t *testing.T) {
	spec := canaryLaunchAgentSpec("/Users/example", "/Applications/BCGOS/bin/bcgos", "maestro-system")
	joined := strings.Join(spec.Arguments, " ")
	if !strings.Contains(joined, "--idle-state auto") {
		t.Fatalf("LaunchAgent arguments do not request native idle observation: %q", joined)
	}
}

func TestCheckpointAndLightDreamHandlersAreOperableButDeepDreamRemainsUnavailable(t *testing.T) {
	enrollment := maintenance.CanaryEnrollment{Activated: []maintenance.Activation{{JobID: maintenance.MemoryCheckpointJobID, QualificationDigest: maintenance.QualificationDigest(maintenance.MemoryCheckpointJobID)}, {JobID: maintenance.MemoryLightDreamJobID, QualificationDigest: maintenance.QualificationDigest(maintenance.MemoryLightDreamJobID)}}}
	handlers, qualification, activated := maintenanceHandlers(t.TempDir(), "maestro-system", enrollment, true)
	if handlers[maintenance.MemoryCheckpointJobID] == nil || qualification[maintenance.MemoryCheckpointJobID] == "" || !containsString(activated, maintenance.MemoryCheckpointJobID) {
		t.Fatalf("checkpoint not operable: handlers=%#v qualification=%#v activated=%#v", handlers, qualification, activated)
	}
	if handlers[maintenance.MemoryLightDreamJobID] == nil || qualification[maintenance.MemoryLightDreamJobID] == "" || handlers[maintenance.MemoryDeepDreamJobID] != nil {
		t.Fatalf("light/deep dream activation drifted: %#v", handlers)
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

func TestPresencePlannerIncludesOnlyExplicitlyActivatedJobs(t *testing.T) {
	jobs := schedulerJobsForTrigger("presence")
	activated := []string{"darwin-housekeeping-daily", "darwin-deep-weekly"}
	planned := activatedSchedulerJobs(jobs, activated)
	if len(planned) != 2 {
		t.Fatalf("planned jobs=%#v, want exactly the two enrolled Darwin jobs", planned)
	}
	for _, job := range planned {
		if job.ID == maintenance.WalterSelfReviewWeeklyJobID || job.ID == maintenance.DarwinStructuralProposalJobID {
			t.Fatalf("inactive/model-backed job entered presence plan: %#v", job)
		}
	}

	enrolledAt := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	now := time.Date(2026, 8, 2, 10, 0, 0, 0, time.UTC)
	due, err := scheduler.PlanDue(planned, enrolledAt, nil, now)
	if err != nil {
		t.Fatal(err)
	}
	for _, occurrence := range due {
		if occurrence.JobID == maintenance.WalterSelfReviewWeeklyJobID || occurrence.JobID == maintenance.DarwinStructuralProposalJobID {
			t.Fatalf("inactive occurrence entered presence plan: %#v", occurrence)
		}
	}
}

func TestPresencePlannerWithNoEnrollmentPlansNothing(t *testing.T) {
	planned := activatedSchedulerJobs(schedulerJobsForTrigger("presence"), nil)
	if len(planned) != 0 {
		t.Fatalf("unenrolled presence wake planned jobs: %#v", planned)
	}
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
	if result["idle_eligibility"] != "explicit_evidence_required_unknown_fails_closed" || result["memory_checkpoint"] != "locally_qualified_only_after_canary_enrollment" || result["memory_dreaming"] != "daily_light_locally_qualified_weekly_deep_unavailable" || result["pulse_interval_seconds"] != float64(900) {
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

func TestCanaryInstallBindsExactInitializedWorkspaceAndRunningExecutable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("macOS LaunchAgent fixtures require POSIX executable paths")
	}
	home := t.TempDir()
	currentHome, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	dataRoot, err := canaryDataRoot(home, currentHome)
	if err != nil {
		t.Fatal(err)
	}
	workspacePath := filepath.Join(t.TempDir(), "customer-acme-workspace")
	initialized, err := workspace.Initialize(workspace.Options{WorkspacePath: workspacePath, DataRoot: dataRoot})
	if err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	resolvedExecutable, err := filepath.EvalSymlinks(executable)
	if err != nil {
		t.Fatal(err)
	}
	args := []string{"maintenance", "canary", "install-macos", "--confirm", "--home", home, "--workspace-path", workspacePath, "--executable", executable}
	for attempt := 0; attempt < 2; attempt++ {
		var output bytes.Buffer
		if code := Run(args, &output, &output); code != ExitOK {
			t.Fatalf("install attempt %d exit=%d output=%s", attempt+1, code, output.String())
		}
	}
	plistPath := filepath.Join(home, "Library", "LaunchAgents", "com.bcg.maestro.maintenance.plist")
	body, err := os.ReadFile(plistPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, expected := range []string{resolvedExecutable, initialized.WorkspaceID, "maintenance", "wake", "presence"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("plist missing %q: %s", expected, text)
		}
	}
	for _, forbidden := range []string{workspacePath, "customer-acme", "walter-self-review", "model", "token", "secret"} {
		if strings.Contains(strings.ToLower(text), strings.ToLower(forbidden)) {
			t.Fatalf("plist contains forbidden value %q: %s", forbidden, text)
		}
	}
	enrollment, err := maintenance.LoadCanaryEnrollment(dataRoot)
	if err != nil {
		t.Fatal(err)
	}
	if enrollment.WorkspaceID != initialized.WorkspaceID || enrollment.Executable != resolvedExecutable || enrollment.Mode != "filesystem_only" {
		t.Fatalf("enrollment=%#v", enrollment)
	}
	if len(enrollment.Activated) != 4 {
		t.Fatalf("activated jobs=%#v", enrollment.Activated)
	}
	for _, activation := range enrollment.Activated {
		if activation.JobID == "walter-self-review-weekly" || activation.JobID == maintenance.MemoryDeepDreamJobID {
			t.Fatalf("model-backed job activated: %#v", activation)
		}
	}
	activatedIDs := make([]string, 0, len(enrollment.Activated))
	for _, activation := range enrollment.Activated {
		activatedIDs = append(activatedIDs, activation.JobID)
	}
	if !containsString(activatedIDs, maintenance.MemoryCheckpointJobID) || !containsString(activatedIDs, maintenance.MemoryLightDreamJobID) {
		t.Fatalf("memory continuity jobs were not activated: %#v", enrollment.Activated)
	}

	var status bytes.Buffer
	if code := Run([]string{"maintenance", "canary", "status", "--home", home}, &status, &status); code != ExitOK || !strings.Contains(status.String(), `"binding_state": "exact"`) || !strings.Contains(status.String(), `"model_backed_capabilities": "unavailable"`) {
		t.Fatalf("status exit/output=%s", status.String())
	}
	for attempt := 0; attempt < 2; attempt++ {
		var output bytes.Buffer
		if code := Run([]string{"maintenance", "canary", "uninstall", "--confirm", "--home", home}, &output, &output); code != ExitOK {
			t.Fatalf("uninstall attempt %d exit=%d output=%s", attempt+1, code, output.String())
		}
	}
}

func TestCanaryInstallRejectsUninitializedWorkspaceAndExecutableSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("macOS LaunchAgent fixtures require POSIX executable paths")
	}
	home := t.TempDir()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	code := Run([]string{"maintenance", "canary", "install-macos", "--confirm", "--home", home, "--workspace-path", t.TempDir(), "--executable", executable}, &output, &output)
	if code != ExitFailure || !strings.Contains(output.String(), "initialized") {
		t.Fatalf("uninitialized workspace exit=%d output=%s", code, output.String())
	}
	symlink := filepath.Join(t.TempDir(), "bcgos")
	if err := os.Symlink(executable, symlink); err != nil {
		t.Fatal(err)
	}
	currentHome, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	dataRoot, err := canaryDataRoot(home, currentHome)
	if err != nil {
		t.Fatal(err)
	}
	workspacePath := filepath.Join(t.TempDir(), "workspace")
	if _, err := workspace.Initialize(workspace.Options{WorkspacePath: workspacePath, DataRoot: dataRoot}); err != nil {
		t.Fatal(err)
	}
	output.Reset()
	code = Run([]string{"maintenance", "canary", "install-macos", "--confirm", "--home", home, "--workspace-path", workspacePath, "--executable", symlink}, &output, &output)
	if code != ExitFailure || !strings.Contains(output.String(), "symlink") {
		t.Fatalf("executable symlink exit=%d output=%s", code, output.String())
	}
}

func TestCanaryLaunchctlRequiresExplicitCurrentMacOSUserScope(t *testing.T) {
	var output bytes.Buffer
	code := Run([]string{"maintenance", "canary", "status", "--home", t.TempDir(), "--launchctl"}, &output, &output)
	if code != ExitFailure || !strings.Contains(output.String(), "current macOS user") {
		t.Fatalf("launchctl fixture exit=%d output=%s", code, output.String())
	}
}

func TestCanaryStatusFailsClosedWhenPlistWorkspaceBindingIsTampered(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("macOS LaunchAgent fixtures require POSIX executable paths")
	}
	home := t.TempDir()
	currentHome, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	dataRoot, err := canaryDataRoot(home, currentHome)
	if err != nil {
		t.Fatal(err)
	}
	workspacePath := filepath.Join(t.TempDir(), "workspace")
	initialized, err := workspace.Initialize(workspace.Options{WorkspacePath: workspacePath, DataRoot: dataRoot})
	if err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if code := Run([]string{"maintenance", "canary", "install-macos", "--confirm", "--home", home, "--workspace-path", workspacePath, "--executable", executable}, &output, &output); code != ExitOK {
		t.Fatalf("install exit=%d output=%s", code, output.String())
	}
	plistPath := filepath.Join(home, "Library", "LaunchAgents", "com.bcg.maestro.maintenance.plist")
	body, err := os.ReadFile(plistPath)
	if err != nil {
		t.Fatal(err)
	}
	body = bytes.Replace(body, []byte(initialized.WorkspaceID), []byte("fedcba9876543210fedcba9876543210"), 1)
	if err := os.WriteFile(plistPath, body, 0o600); err != nil {
		t.Fatal(err)
	}
	output.Reset()
	if code := Run([]string{"maintenance", "canary", "status", "--home", home}, &output, &output); code != ExitFailure || !strings.Contains(output.String(), "binding") {
		t.Fatalf("tampered status exit=%d output=%s", code, output.String())
	}
}
