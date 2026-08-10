package cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/activationpolicy"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/adaptercfg"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/agentidentity"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/canary"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/execution"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/lifecycle"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/maestro"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/maintenance"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/memory"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ownerctx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/sessionstart"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/skillrouting"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
)

func TestCanaryReportCommandIsLocalOnly(t *testing.T) {
	root := t.TempDir()
	if err := (canary.Store{Root: filepath.Join(root, "canary")}).Append(canary.Receipt{RecordedAt: time.Now().UTC(), Event: canary.EventFirstValue, Outcome: canary.OutcomeSucceeded, Duration: canary.DurationUnderFiveMinutes}); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if code := runCanary([]string{"report"}, &output, &output, func() (string, error) { return root, nil }); code != ExitOK || !strings.Contains(output.String(), `"receipt_count": 1`) {
		t.Fatalf("report = %d %s", code, output.String())
	}
}

func TestWorkListAndCheckpointRequireExplicitStdin(t *testing.T) {
	root := t.TempDir()
	dataRoot := func() (string, error) { return root, nil }
	workspacePath := filepath.Join(t.TempDir(), "case-list")
	if _, err := workspace.Initialize(workspace.Options{WorkspacePath: workspacePath, DataRoot: root}); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	contract := `{"objective":"bounded list objective","initial_next_step":"start","criteria":[{"id":"check","type":"command_check","command":["go","version"]}],"allowed_refs":[]}`
	if code := runWork([]string{"create", "--workspace", workspacePath, "--stdin"}, strings.NewReader(contract), &output, &output, dataRoot); code != ExitOK {
		t.Fatalf("create exit = %d: %s", code, output.String())
	}
	var created execution.MutationReceipt
	if err := json.Unmarshal(output.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	output.Reset()
	if code := runWork([]string{"list", "--workspace", workspacePath}, strings.NewReader(""), &output, &output, dataRoot); code != ExitOK || !strings.Contains(output.String(), created.ItemID) || strings.Contains(output.String(), "bounded list objective") {
		t.Fatalf("list exit = %d: %s", code, output.String())
	}
	output.Reset()
	if code := runWork([]string{"checkpoint", "--workspace", workspacePath, "--item", created.ItemID, "--revision", "2", "--attempt", "attempt"}, strings.NewReader(`{"summary":"should not write"}`), &output, &output, dataRoot); code != ExitUsage {
		t.Fatalf("checkpoint without stdin exit = %d: %s", code, output.String())
	}
}

func TestOwnerAgentListIsReadOnlyAndStable(t *testing.T) {
	root := t.TempDir()
	var output bytes.Buffer
	if code := runOwnerWithInput([]string{"agent", "list"}, strings.NewReader(""), &output, &output, func() (string, error) { return root, nil }); code != ExitOK {
		t.Fatalf("owner agent list exit = %d: %s", code, output.String())
	}
	if !strings.Contains(output.String(), `"managed_agents"`) || !strings.Contains(output.String(), `"maestro"`) || !strings.Contains(output.String(), `"not_instantiated"`) {
		t.Fatalf("owner agent list = %s", output.String())
	}
	if _, err := os.Stat(filepath.Join(root, "agents")); !os.IsNotExist(err) {
		t.Fatalf("read-only list created agent state: %v", err)
	}
}

func TestMemoryCaptureStatusAndContextCommands(t *testing.T) {
	dataDir := t.TempDir()
	var output bytes.Buffer
	code := RunWithInput([]string{
		"memory", "capture",
		"--data-dir", dataDir,
		"--workspace", "case-a",
		"--kind", "decision",
		"--stdin",
		"--sanitized",
	}, strings.NewReader("sanitized evidence"), &output, &output)
	if code != 0 {
		t.Fatalf("capture exit = %d, output = %s", code, output.String())
	}
	capturePath := filepath.Join(dataDir, "memory", "workspaces", "case-a", "l1", "captures")
	entries, err := os.ReadDir(capturePath)
	if err != nil || len(entries) != 1 {
		t.Fatalf("capture files = %v, err = %v", entries, err)
	}

	output.Reset()
	code = Run([]string{"memory", "status", "--data-dir", dataDir, "--workspace", "case-a"}, &output, &output)
	if code != 0 || !strings.Contains(output.String(), `"state": "captured"`) || !strings.Contains(output.String(), `"dreaming": "daily_light_available_weekly_deep_unavailable"`) {
		t.Fatalf("status exit = %d, output = %s", code, output.String())
	}

	output.Reset()
	code = Run([]string{
		"memory", "context",
		"--data-dir", dataDir,
		"--workspace", "case-a",
		"--budget-l1", "100",
		"--budget-l2", "100",
		"--budget-l3", "100",
		"--budget-lifetime", "100",
	}, &output, &output)
	if code != 0 {
		t.Fatalf("context exit = %d, output = %s", code, output.String())
	}
	var bundle map[string]any
	if err := json.Unmarshal(output.Bytes(), &bundle); err != nil {
		t.Fatalf("context JSON: %v", err)
	}
	if diagnostics, ok := bundle["diagnostics"].([]any); !ok || len(diagnostics) != 4 {
		t.Fatalf("context diagnostics = %#v", bundle)
	}
}

func TestMemoryCaptureFailsClosedWithoutSanitizedAttestation(t *testing.T) {
	var output bytes.Buffer
	code := RunWithInput([]string{"memory", "capture", "--data-dir", t.TempDir(), "--workspace", "case-a", "--kind", "note", "--stdin"}, strings.NewReader("raw"), &output, &output)
	if code == 0 || !strings.Contains(output.String(), "--sanitized") {
		t.Fatalf("capture exit = %d, output = %s", code, output.String())
	}
}

func TestContinuousStatusFailsClosedForForgedCaptureAndMaintenanceReceipts(t *testing.T) {
	root := t.TempDir()
	workspaceID := "0123456789abcdef0123456789abcdef"
	captureDirectory := filepath.Join(root, "memory", "workspaces", workspaceID, "l1", "attested-captures")
	if err := os.MkdirAll(captureDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(captureDirectory, "2026-08-05.jsonl"), []byte(`{"schema_version":2`), 0o600); err != nil {
		t.Fatal(err)
	}
	if state, count := continuousMemoryStatus(root, workspaceID); state != "unavailable" || count != 0 {
		t.Fatalf("forged capture continuity state=%q count=%d", state, count)
	}
	enrollment := maintenance.CanaryEnrollment{SchemaVersion: maintenance.EnrollmentSchemaVersion, WorkspaceID: workspaceID, AgentID: "darwin", Home: "/tmp", Executable: "/bin/true", UID: "501", Timezone: "UTC", LaunchAgentLabel: "com.bcg.maestro.maintenance", Mode: "filesystem_only", EnrolledAt: time.Now().UTC(), Activated: []maintenance.Activation{{JobID: maintenance.MemoryCheckpointJobID, QualificationDigest: maintenance.QualificationDigest(maintenance.MemoryCheckpointJobID)}, {JobID: maintenance.MemoryLightDreamJobID, QualificationDigest: maintenance.QualificationDigest(maintenance.MemoryLightDreamJobID)}}}
	if err := maintenance.SaveCanaryEnrollment(root, enrollment); err != nil {
		t.Fatal(err)
	}
	receiptDirectory := filepath.Join(root, "maintenance", "receipts", "workspaces", workspaceID, "receipts", maintenance.MemoryCheckpointJobID)
	if err := os.MkdirAll(receiptDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(receiptDirectory, "forged--empty.json"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if configured, observed := continuousMaintenanceStatus(root, workspaceID); !configured || observed {
		t.Fatalf("forged maintenance receipt configured=%t observed=%t", configured, observed)
	}
}

func TestMemoryCaptureRejectsOversizedInput(t *testing.T) {
	dataDir := t.TempDir()
	var output bytes.Buffer
	code := RunWithInput([]string{
		"memory", "capture", "--data-dir", dataDir, "--workspace", "case-a", "--kind", "note", "--stdin", "--sanitized",
	}, strings.NewReader(strings.Repeat("x", (1<<20)+1)), &output, &output)
	if code == 0 || !strings.Contains(output.String(), "1 MiB") {
		t.Fatalf("capture exit = %d, output = %s", code, output.String())
	}
	if _, err := os.Stat(filepath.Join(dataDir, "memory", "workspaces", "case-a")); !os.IsNotExist(err) {
		t.Fatalf("oversized capture created workspace: %v", err)
	}
}

func TestMemoryContextRequiresEveryBudget(t *testing.T) {
	var output bytes.Buffer
	code := Run([]string{"memory", "context", "--data-dir", t.TempDir(), "--workspace", "case-a"}, &output, &output)
	if code == 0 || !strings.Contains(output.String(), "positive budget required") {
		t.Fatalf("context exit = %d, output = %s", code, output.String())
	}
}

func TestMemoryCommandsRejectResidualPositionals(t *testing.T) {
	dataDir := t.TempDir()
	tests := []struct {
		name string
		args []string
	}{
		{name: "capture", args: []string{"memory", "capture", "--data-dir", dataDir, "--workspace", "case-a", "--kind", "note", "--stdin", "--sanitized", "SECRET"}},
		{name: "status", args: []string{"memory", "status", "--data-dir", dataDir, "--workspace", "case-a", "typo"}},
		{name: "context", args: []string{"memory", "context", "--data-dir", dataDir, "--workspace", "case-a", "--budget-l1", "1", "--budget-l2", "1", "--budget-l3", "1", "--budget-lifetime", "1", "typo"}},
		{name: "dream", args: []string{"memory", "dream", "daily", "--data-dir", dataDir, "--workspace", "case-a", "typo"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			code := RunWithInput(test.args, strings.NewReader("safe"), &output, &output)
			if code == 0 || !strings.Contains(output.String(), "unexpected positional argument") {
				t.Fatalf("exit = %d, output = %s", code, output.String())
			}
		})
	}
	if _, err := os.Stat(filepath.Join(dataDir, "memory", "workspaces", "case-a")); !os.IsNotExist(err) {
		t.Fatalf("rejected capture created workspace: %v", err)
	}
}

func TestMemoryCLIReportsAllInvalidCommitsAsCorrupt(t *testing.T) {
	dataDir := t.TempDir()
	commits := filepath.Join(dataDir, "memory", "workspaces", "case-a", "commits")
	if err := os.MkdirAll(commits, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(commits, "20260720T120000.000000000Z-corrupt.json"), []byte(`{"broken":true}`), 0o600); err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	if code := Run([]string{"memory", "status", "--data-dir", dataDir, "--workspace", "case-a"}, &output, &output); code != 0 || !strings.Contains(output.String(), `"state": "corrupt"`) {
		t.Fatalf("status exit = %d, output = %s", code, output.String())
	}
	output.Reset()
	code := Run([]string{"memory", "context", "--data-dir", dataDir, "--workspace", "case-a", "--budget-l1", "1", "--budget-l2", "1", "--budget-l3", "1", "--budget-lifetime", "1"}, &output, &output)
	if code == 0 || !strings.Contains(output.String(), `"state":"error"`) || !strings.Contains(output.String(), "none is valid") {
		t.Fatalf("context exit = %d, output = %s", code, output.String())
	}
}

func TestMemoryDailyDreamExcludesManualCaptureAndWeeklyRemainsUnavailable(t *testing.T) {
	dataDir := t.TempDir()
	var output bytes.Buffer
	if code := RunWithInput([]string{"memory", "capture", "--data-dir", dataDir, "--workspace", "case-a", "--kind", "decision", "--stdin", "--sanitized"}, strings.NewReader("owner confirmation required"), &output, &output); code != ExitOK {
		t.Fatalf("capture exit=%d output=%s", code, output.String())
	}
	output.Reset()
	code := Run([]string{"memory", "dream", "daily", "--data-dir", dataDir, "--workspace", "case-a"}, &output, &output)
	if code != ExitOK || !strings.Contains(output.String(), `"capability": "memory_light_dreaming"`) || !strings.Contains(output.String(), `"state": "reviewed_no_change"`) || !strings.Contains(output.String(), "trusted capture-v2") {
		t.Fatalf("daily dream exit = %d, output = %s", code, output.String())
	}
	output.Reset()
	code = Run([]string{"memory", "dream", "weekly", "--data-dir", dataDir, "--workspace", "case-a"}, &output, &output)
	if code != ExitUnavailable || !strings.Contains(output.String(), `"capability": "memory_deep_dreaming"`) || !strings.Contains(output.String(), `"state": "unavailable"`) {
		t.Fatalf("weekly dream exit = %d, output = %s", code, output.String())
	}
}

func TestRecordAttestedSkillRoutePersistsOnlyTrustedMetadata(t *testing.T) {
	root := t.TempDir()
	selected := []skillrouting.Selection{{ID: "meeting-close", Reason: "secret prompt phrase", Pointer: "/secret/pointer"}}
	if err := recordAttestedSkillRoute(root, "claude", "case-a", "session-a", selected); err != nil {
		t.Fatal(err)
	}
	period := time.Now().UTC().Format("2006-01-02")
	path := filepath.Join(root, "memory", "workspaces", "case-a", "l1", "attested-captures", period+".jsonl")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "secret prompt phrase") || strings.Contains(string(body), "/secret/pointer") {
		t.Fatalf("capture leaked prompt rationale or pointer: %s", body)
	}
	var capture memory.Capture
	if err := json.Unmarshal(bytes.TrimSpace(body), &capture); err != nil {
		t.Fatal(err)
	}
	if capture.Text != "meeting-close" || capture.ProducerID != "claude.context-injection" {
		t.Fatalf("unexpected capture envelope: %#v", capture)
	}
	if err := (memory.CaptureAttestor{Root: filepath.Join(root, "memory")}).Verify(capture); err != nil {
		t.Fatalf("capture is not verifiably attested: %v", err)
	}
}

func TestVersionCommand(t *testing.T) {
	var output bytes.Buffer
	if code := Run([]string{"version"}, &output, &output); code != 0 || !strings.Contains(output.String(), "bcgos") {
		t.Fatalf("version exit = %d, output = %s", code, output.String())
	}
}

func TestIngestReportsUnavailableWithoutVerifiedRuntimePack(t *testing.T) {
	dataRoot, workspacePath := filepath.Join(t.TempDir(), "BCGOS"), t.TempDir()
	sourcePath := filepath.Join(workspacePath, "brief.docx")
	if err := os.WriteFile(sourcePath, []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if code := runInit([]string{workspacePath}, &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK {
		t.Fatalf("init exit = %d, output = %s", code, output.String())
	}
	output.Reset()
	code := runIngest([]string{"--workspace", workspacePath, "--source", sourcePath}, &output, &output, func() (string, error) { return dataRoot, nil })
	if code != ExitUnavailable {
		t.Fatalf("ingest exit = %d, output = %s", code, output.String())
	}
	var result map[string]any
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("ingest result is not JSON: %v; output=%s", err, output.String())
	}
	if result["status"] != "unavailable" || result["source_name"] != "brief.docx" {
		t.Fatalf("ingest result = %#v", result)
	}
	if _, err := os.Stat(filepath.Join(dataRoot, "ingestion", "artifacts")); !os.IsNotExist(err) {
		t.Fatalf("unavailable ingestion created artifacts: %v", err)
	}
}

func TestWorkspaceImportCLIRequiresApprovalAndExecutesSyntheticPlanTransactionally(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "external")
	destination := filepath.Join(root, "maestro")
	dataRoot := filepath.Join(root, "data")
	if err := os.MkdirAll(destination, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(source, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "notes.md"), []byte("synthetic workspace note"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "brief.docx"), []byte("synthetic document"), 0o600); err != nil {
		t.Fatal(err)
	}
	planPath := filepath.Join(root, "plan.json")
	approvalPath := filepath.Join(root, "approval.json")
	var output bytes.Buffer
	data := func() (string, error) { return dataRoot, nil }
	if code := runWorkspaceImport([]string{"import", "inspect", "--source", source}, &output, &output, data); code != ExitOK || !strings.Contains(output.String(), `"read_only": true`) {
		t.Fatalf("inspect exit = %d, output = %s", code, output.String())
	}
	output.Reset()
	if code := runWorkspaceImport([]string{"import", "plan", "--source", source, "--destination", destination, "--out", planPath}, &output, &output, data); code != ExitOK || !strings.Contains(output.String(), `"plan_digest"`) {
		t.Fatalf("plan exit = %d, output = %s", code, output.String())
	}
	output.Reset()
	if code := runWorkspaceImport([]string{"import", "approve", "--plan", planPath, "--approval", filepath.Join(root, "unused.json"), "--approved-by", "synthetic-owner", "--confirm", "NOPE"}, &output, &output, data); code == ExitOK {
		t.Fatal("approval without exact confirmation succeeded")
	}
	output.Reset()
	if code := runWorkspaceImport([]string{"import", "approve", "--plan", planPath, "--out", approvalPath, "--approved-by", "synthetic-owner", "--confirm", "IMPORT"}, &output, &output, data); code != ExitOK {
		t.Fatalf("approval exit = %d, output = %s", code, output.String())
	}
	output.Reset()
	if code := runWorkspaceImport([]string{"import", "execute", "--plan", planPath, "--approval", approvalPath}, &output, &output, data); code != ExitOK || !strings.Contains(output.String(), `"state": "executed"`) || !strings.Contains(output.String(), `"quarantined"`) {
		t.Fatalf("execute exit = %d, output = %s", code, output.String())
	}
	if body, err := os.ReadFile(filepath.Join(destination, "notes.md")); err != nil || string(body) != "synthetic workspace note" {
		t.Fatalf("copied note = %q, err=%v", body, err)
	}
	if _, err := os.Stat(filepath.Join(destination, "brief.docx")); !os.IsNotExist(err) {
		t.Fatalf("document was copied outside quarantine: %v", err)
	}
	entries, err := os.ReadDir(filepath.Join(dataRoot, "workspace-import", "receipts"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("receipts = %#v, err=%v", entries, err)
	}
	receiptPath := filepath.Join(dataRoot, "workspace-import", "receipts", entries[0].Name())
	output.Reset()
	if code := runWorkspaceImport([]string{"import", "rollback", "--plan", planPath, "--receipt", receiptPath, "--confirm", "ROLLBACK"}, &output, &output, data); code != ExitOK || !strings.Contains(output.String(), `"state": "rolled_back"`) {
		t.Fatalf("rollback exit = %d, output = %s", code, output.String())
	}
	if _, err := os.Stat(filepath.Join(destination, "notes.md")); !os.IsNotExist(err) {
		t.Fatalf("rollback left note: %v", err)
	}
}

func TestPrivateReleaseCommandsFailClosedWithoutApprovedSecureStore(t *testing.T) {
	tests := map[string][]string{
		"auth status":    {"auth", "status"},
		"auth login":     {"auth", "login"},
		"auth logout":    {"auth", "logout"},
		"update check":   {"update", "--check"},
		"update confirm": {"update", "--confirm", "0123456789abcdef0123456789abcdef"},
	}
	for name, args := range tests {
		t.Run(name, func(t *testing.T) {
			var output bytes.Buffer
			var errorOutput bytes.Buffer
			if code := Run(args, &output, &errorOutput); code != ExitUnavailable {
				t.Fatalf("Run(%v) exit = %d, want %d; out=%s err=%s", args, code, ExitUnavailable, output.String(), errorOutput.String())
			}
			var result struct {
				SchemaVersion int    `json:"schema_version"`
				State         string `json:"state"`
				Reason        string `json:"reason"`
			}
			if err := json.Unmarshal(output.Bytes(), &result); err != nil {
				t.Fatalf("output is not one JSON document: %v; output=%q", err, output.String())
			}
			if result.SchemaVersion != 1 || result.State != "unavailable" || result.Reason == "" {
				t.Fatalf("unexpected fail-closed result: %#v", result)
			}
			if strings.Contains(strings.ToLower(output.String()), "token") {
				t.Fatal("unavailable response mentioned credential material")
			}
		})
	}
}

func TestSessionStartHookOutputsBoundedNativeContext(t *testing.T) {
	dataRoot := filepath.Join(t.TempDir(), "local", "BCGOS")
	workspacePath := t.TempDir()
	var output bytes.Buffer
	if code := runInit([]string{workspacePath}, &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK {
		t.Fatalf("init exit = %d, output = %s", code, output.String())
	}
	output.Reset()
	code := runHook([]string{"session-start", "--runtime", "codex", "--adapter-source", "maestro", workspacePath}, &output, &output, func() (string, error) { return dataRoot, nil })
	if code != ExitOK || !strings.Contains(output.String(), `"hookEventName": "SessionStart"`) || !strings.Contains(output.String(), `\"runtime\":\"codex\"`) || !strings.Contains(output.String(), `\"availability_state\":\"enabled\"`) || !strings.Contains(output.String(), `\"memory\":{\"state\":\"empty\"`) || strings.Contains(output.String(), "memory context injection requires a runtime adapter") {
		t.Fatalf("hook exit = %d, output = %s", code, output.String())
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "Use the installed CLI silently") || !strings.Contains(output.String(), executable) {
		t.Fatalf("session hook did not expose the invoking Maestro CLI path: %s", output.String())
	}
}

func TestInstalledSessionStartUsesWorkspaceOrchestrationStateAndEnqueuesPresence(t *testing.T) {
	dataRoot, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	workspacePath, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if code := runInit([]string{workspacePath}, &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK {
		t.Fatal(output.String())
	}
	inspection, err := workspace.Inspect(workspacePath, dataRoot)
	if err != nil {
		t.Fatal(err)
	}
	previous := enqueueHookPresenceWake
	defer func() { enqueueHookPresenceWake = previous }()
	called := 0
	enqueueHookPresenceWake = func(workspaceID string) error {
		called++
		if workspaceID != inspection.WorkspaceID {
			t.Fatalf("presence workspace = %q, want %q", workspaceID, inspection.WorkspaceID)
		}
		return nil
	}
	output.Reset()
	code := runHook([]string{"session-start", "--runtime", "codex", "--adapter-source", "maestro", "--orchestration-state", ".bcgos/maestro-orchestration-state.json", workspacePath}, &output, &output, func() (string, error) { return dataRoot, nil })
	if code != ExitOK || called != 1 || !strings.Contains(output.String(), `"hookEventName": "SessionStart"`) {
		t.Fatalf("hook exit = %d, enqueue=%d, output=%s", code, called, output.String())
	}
	output.Reset()
	code = runHookWithInput([]string{"claude", "session-start", "--adapter-source", "maestro", "--orchestration-state", ".bcgos/maestro-orchestration-state.json", workspacePath}, strings.NewReader(""), &output, &output, func() (string, error) { return dataRoot, nil })
	if code != ExitOK || called != 2 || !strings.Contains(output.String(), `"hookEventName": "SessionStart"`) {
		t.Fatalf("Claude hook exit = %d, enqueue=%d, output=%s", code, called, output.String())
	}
}

func TestInstalledHookLeavesSafeActionToNativeFlowWhenOrchestrationStateIsSymlinked(t *testing.T) {
	dataRoot, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	workspacePath, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if code := runInit([]string{workspacePath}, &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK {
		t.Fatal(output.String())
	}
	for _, pointer := range []string{"../outside.json", ".bcgos/../outside.json"} {
		output.Reset()
		if code := runHook([]string{"session-start", "--runtime", "codex", "--adapter-source", "maestro", "--orchestration-state", pointer, workspacePath}, &output, &output, func() (string, error) { return dataRoot, nil }); code == ExitOK || !strings.Contains(output.String(), "orchestration state") {
			t.Fatalf("pointer %q accepted: exit=%d output=%s", pointer, code, output.String())
		}
	}
	outside := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(outside, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(workspacePath, ".bcgos", "maestro-orchestration-state.json")
	// Workspace bootstrap now creates the initial valid state eagerly. Remove
	// it here so this test can inject the malicious symlink it is meant to
	// reject.
	if err := os.Remove(statePath); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, statePath); err != nil {
		t.Fatal(err)
	}
	output.Reset()
	input := `{"session_id":"session-a","tool_name":"Bash","tool_input":{"command":"echo safe"}}`
	code := runHookWithInput([]string{"codex", "pre-action-guard", "--adapter-source", "maestro", "--orchestration-state", ".bcgos/maestro-orchestration-state.json", workspacePath}, strings.NewReader(input), &output, &output, func() (string, error) { return dataRoot, nil })
	if code != ExitOK || strings.TrimSpace(output.String()) != "{}" {
		t.Fatalf("symlink guard = %d %s", code, output.String())
	}
	if err := os.Remove(statePath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(statePath, []byte(`{"unknown_field":true}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	output.Reset()
	code = runHook([]string{"session-start", "--runtime", "codex", "--adapter-source", "maestro", "--orchestration-state", ".bcgos/maestro-orchestration-state.json", workspacePath}, &output, &output, func() (string, error) { return dataRoot, nil })
	if code == ExitOK || !strings.Contains(output.String(), "decode orchestration state") {
		t.Fatalf("malformed state accepted: exit=%d output=%s", code, output.String())
	}
}

func TestInstalledHookRejectsMissingOrchestrationStateWithRemediation(t *testing.T) {
	dataRoot, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	workspacePath, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if code := runInit([]string{workspacePath}, &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK {
		t.Fatal(output.String())
	}
	if err := os.Remove(filepath.Join(workspacePath, ".bcgos", "maestro-orchestration-state.json")); err != nil {
		t.Fatal(err)
	}
	output.Reset()
	code := runHook([]string{"session-start", "--runtime", "codex", "--adapter-source", "maestro", "--orchestration-state", ".bcgos/maestro-orchestration-state.json", workspacePath}, &output, &output, func() (string, error) { return dataRoot, nil })
	if code == ExitOK || !strings.Contains(output.String(), "orchestration state is missing") || !strings.Contains(output.String(), "bcgos init") {
		t.Fatalf("missing state accepted without remediation: exit=%d output=%s", code, output.String())
	}
}

func TestInstalledGuardDoesNotCoupleReadOnlyBCGOSDiagnosticsToWorkspaceState(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, runtimeName := range []string{"claude", "codex"} {
		t.Run(runtimeName, func(t *testing.T) {
			command := fmt.Sprintf("%q doctor", executable)
			body, err := json.Marshal(map[string]any{"session_id": "session-a", "tool_name": "Bash", "tool_input": map[string]string{"command": command}})
			if err != nil {
				t.Fatal(err)
			}
			var output bytes.Buffer
			rootCalled := false
			code := runHookWithInput(
				[]string{runtimeName, "pre-action-guard", "--adapter-source", "maestro", "--orchestration-state", ".bcgos/maestro-orchestration-state.json", "/workspace-that-must-not-be-inspected"},
				bytes.NewReader(body),
				&output,
				&output,
				func() (string, error) {
					rootCalled = true
					return "", errors.New("read-only diagnostics must not inspect workspace state")
				},
			)
			if code != ExitOK || rootCalled || strings.Contains(output.String(), `"permissionDecision": "deny"`) {
				t.Fatalf("guard = %d, rootCalled=%v, output=%s", code, rootCalled, output.String())
			}
		})
	}
}

func TestPreActionGuardLeavesIncompleteMetadataToNativeFlowAndProtectsRootRemoval(t *testing.T) {
	for _, runtimeName := range []string{"claude", "codex"} {
		t.Run(runtimeName, func(t *testing.T) {
			for _, input := range []string{
				`{"session_id":"session-a","tool_name":"Bash","tool_input":{}}`,
				`{"session_id":"session-a","tool_name":"Bash","tool_input":{"command":""}}`,
			} {
				var output bytes.Buffer
				code := runHookWithInput([]string{runtimeName, "pre-action-guard", "--adapter-source", "maestro", "/workspace-that-must-not-be-inspected"}, strings.NewReader(input), &output, &output, func() (string, error) {
					return "", errors.New("incomplete metadata must not inspect workspace state")
				})
				if code != ExitOK || strings.Contains(output.String(), `"permissionDecision"`) {
					t.Fatalf("incomplete metadata was blocked: code=%d output=%s", code, output.String())
				}
			}

			var output bytes.Buffer
			input := `{"session_id":"session-a","tool_name":"UnknownTool","tool_input":{"command":"rm -rf /"}}`
			code := runHookWithInput([]string{runtimeName, "pre-action-guard", "--adapter-source", "maestro", "/workspace-that-must-not-be-inspected"}, strings.NewReader(input), &output, &output, func() (string, error) {
				return "", errors.New("protected-root check must not inspect workspace state")
			})
			if code != ExitOK || !strings.Contains(output.String(), `"permissionDecision": "deny"`) {
				t.Fatalf("unknown tool root removal was not denied: code=%d output=%s", code, output.String())
			}
		})
	}
}

func TestPreActionGuardExplainsHowToRetryChainedRemoval(t *testing.T) {
	for _, runtimeName := range []string{"claude", "codex"} {
		t.Run(runtimeName, func(t *testing.T) {
			input := `{"session_id":"session-a","tool_name":"Bash","tool_input":{"command":"mv note.md archived.md && ls && rm archived.md"}}`
			var output bytes.Buffer
			code := runHookWithInput(
				[]string{runtimeName, "pre-action-guard", "--adapter-source", "maestro", "/workspace-that-must-not-be-inspected"},
				strings.NewReader(input), &output, &output,
				func() (string, error) { return "", errors.New("chained removal must not inspect workspace state") },
			)
			if code != ExitOK || !strings.Contains(output.String(), `"permissionDecision": "deny"`) ||
				!strings.Contains(output.String(), "Run each shell step separately") ||
				!strings.Contains(output.String(), "removal") {
				t.Fatalf("guard did not provide actionable recovery: code=%d output=%s", code, output.String())
			}
		})
	}
}

func TestLifecycleReceiptCheckExplainsBoundedHistory(t *testing.T) {
	root := t.TempDir()
	workspaceID := strings.Repeat("a", 32)
	for index := 0; index <= lifecycle.MaximumDiagnosticReceiptEntries; index++ {
		receipt := lifecycle.Receipt{
			SchemaVersion: 1,
			Runtime:       "claude",
			Event:         lifecycle.StopFinalize,
			State:         "observed",
			Provenance:    lifecycle.AdapterCommand,
			IdempotencyKey: lifecycle.IdempotencyKey(
				"canary-bounded-history", strconv.Itoa(index),
			),
		}
		if _, err := lifecycle.Record(root, workspaceID, receipt); err != nil {
			t.Fatal(err)
		}
	}
	check := lifecycleReceiptCheck(root, workspaceID)
	if check.State != "warning" || !strings.Contains(check.Message, "64-entry diagnostic window") ||
		!strings.Contains(check.Message, "remain local") || !strings.Contains(check.Message, "native qualification") {
		t.Fatalf("bounded history guidance = %#v", check)
	}
}

func TestInstalledGuardLeavesHomonymousDiagnosticToNativeFlowWhenStateCannotBeValidated(t *testing.T) {
	for _, runtimeName := range []string{"claude", "codex"} {
		t.Run(runtimeName, func(t *testing.T) {
			input := `{"session_id":"session-a","tool_name":"Bash","tool_input":{"command":"/tmp/attacker/bcgos doctor"}}`
			var output bytes.Buffer
			rootCalled := false
			code := runHookWithInput(
				[]string{runtimeName, "pre-action-guard", "--adapter-source", "maestro", "--orchestration-state", ".bcgos/maestro-orchestration-state.json", "/workspace-that-must-not-be-inspected"},
				strings.NewReader(input), &output, &output,
				func() (string, error) {
					rootCalled = true
					return "", errors.New("ordinary local commands must not inspect orchestration state")
				},
			)
			if code != ExitOK || rootCalled || strings.TrimSpace(output.String()) != "{}" {
				t.Fatalf("guard = %d rootCalled=%v output=%s", code, rootCalled, output.String())
			}
		})
	}
}

func TestPostActionReceiptIdentityIncludesValidatedOrchestrationSnapshot(t *testing.T) {
	dataRoot, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	workspacePath, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if code := runInit([]string{workspacePath}, &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK {
		t.Fatal(output.String())
	}
	inspection, err := workspace.Inspect(workspacePath, dataRoot)
	if err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(workspacePath, ".bcgos", "maestro-orchestration-state.json")
	input := `{"session_id":"session-a","tool_use_id":"toolu_a","tool_name":"Bash","tool_input":{"command":"sensitive"}}`
	for _, policy := range []string{"policy-a", "policy-b"} {
		if err := os.WriteFile(statePath, []byte(`{"policy_sha256":"`+policy+`","branch_id":"","scope_id":"","scope_kind":"","root_id":"","updated":"0001-01-01T00:00:00Z","fence_epoch":0}`+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		output.Reset()
		code := runHookWithInput([]string{"codex", "post-action-receipt", "--adapter-source", "maestro", "--orchestration-state", ".bcgos/maestro-orchestration-state.json", workspacePath}, strings.NewReader(input), &output, &output, func() (string, error) { return dataRoot, nil })
		if code != ExitOK || !strings.Contains(output.String(), `"continue": true`) {
			t.Fatalf("policy %s receipt = %d %s", policy, code, output.String())
		}
	}
	receipts, err := os.ReadDir(filepath.Join(dataRoot, "runtime", "receipts", inspection.WorkspaceID))
	if err != nil || len(receipts) != 2 {
		t.Fatalf("snapshot-bound receipt count = %d, err=%v", len(receipts), err)
	}
}

func TestClaudeGuardFailsClosedBeforeWorkspaceInspection(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{name: "malformed", input: `{"tool_name":`},
		{name: "oversized", input: `{"tool_name":"Bash","tool_input":{"command":"echo ` + strings.Repeat("x", (64<<10)+1) + `"}}`},
		{name: "evaluation failure", input: `{"tool_name":"Bash","tool_input":{"command":"rm -rf \"/"}}`},
		{name: "unsupported parameter expansion", input: `{"tool_name":"Bash","tool_input":{"command":"rm -rf ${UNSET:-/}"}}`},
		{name: "unsupported root glob", input: `{"tool_name":"Bash","tool_input":{"command":"rm -rf /*"}}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			rootCalled := false
			code := runHookWithInput(
				[]string{"claude", "pre-action-guard", "--adapter-source", "maestro", "/workspace-that-must-not-be-inspected"},
				strings.NewReader(test.input),
				&output,
				&output,
				func() (string, error) {
					rootCalled = true
					return "", errors.New("workspace inspection must not run")
				},
			)
			if code != ExitOK || rootCalled || !strings.Contains(output.String(), `"permissionDecision": "deny"`) ||
				!strings.Contains(output.String(), "Nothing was changed") {
				t.Fatalf("guard = %d, rootCalled=%v, output=%s", code, rootCalled, output.String())
			}
		})
	}
}

func TestClaudeLifecycleHooksRemainUnavailableWhileRecordingMetadataOnlyEvidence(t *testing.T) {
	dataRoot := filepath.Join(t.TempDir(), "local", "BCGOS")
	workspacePath := t.TempDir()
	var output bytes.Buffer
	if code := runInit([]string{workspacePath}, &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK {
		t.Fatal(output.String())
	}
	output.Reset()
	if code := runHookWithInput([]string{"claude", "context-injection", "--adapter-source", "maestro", workspacePath}, strings.NewReader(`{}`), &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK || !strings.Contains(output.String(), `"hookEventName": "UserPromptSubmit"`) {
		t.Fatalf("context hook = %d %s", code, output.String())
	}
	output.Reset()
	receiptInput := `{"session_id":"session-a","tool_use_id":"toolu_a","tool_name":"Bash","tool_input":{"command":"sensitive client command"}}`
	if code := runHookWithInput([]string{"claude", "post-action-receipt", "--adapter-source", "maestro", workspacePath}, strings.NewReader(receiptInput), &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK || !strings.Contains(output.String(), `"continue": true`) {
		t.Fatalf("receipt hook = %d %s", code, output.String())
	}
	output.Reset()
	if code := runHookWithInput([]string{"claude", "stop-finalization", "--adapter-source", "maestro", workspacePath}, strings.NewReader(`{"session_id":"session-a"}`), &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK || !strings.Contains(output.String(), `"continue": true`) {
		t.Fatalf("stop hook = %d %s", code, output.String())
	}
	inspection, err := workspace.Inspect(workspacePath, dataRoot)
	if err != nil {
		t.Fatal(err)
	}
	receiptRoot := filepath.Join(dataRoot, "runtime", "receipts", inspection.WorkspaceID)
	entries, err := os.ReadDir(receiptRoot)
	if err != nil || len(entries) != 2 {
		t.Fatalf("receipt entries = %v, %v", entries, err)
	}
	for _, entry := range entries {
		body, err := os.ReadFile(filepath.Join(receiptRoot, entry.Name()))
		if err != nil || strings.Contains(string(body), "sensitive client command") ||
			strings.Contains(string(body), "session-a") || strings.Contains(string(body), workspacePath) {
			t.Fatalf("unsafe receipt = %s, %v", body, err)
		}
		if !strings.Contains(string(body), `"provenance": "adapter_command"`) {
			t.Fatalf("receipt did not state unverified adapter provenance: %s", body)
		}
	}
	output.Reset()
	if code := runMaestroWithInput([]string{"status", workspacePath}, strings.NewReader(""), &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK || !strings.Contains(output.String(), `"adapter_observed": true`) || !strings.Contains(output.String(), `"post_action_observe"`) {
		t.Fatalf("continuous runtime evidence = %d %s", code, output.String())
	}
	output.Reset()
	if code := runDoctor([]string{workspacePath}, &output, &output, func() (string, error) { return dataRoot, nil }, func(string) bool { return true }); code != ExitOK ||
		!strings.Contains(output.String(), `"id": "lifecycle_receipts"`) ||
		!strings.Contains(output.String(), "adapter-command lifecycle receipt") ||
		!strings.Contains(output.String(), "native-session conformance") ||
		!strings.Contains(output.String(), `"state": "unavailable"`) {
		t.Fatalf("doctor = %d %s", code, output.String())
	}
}

func TestCodexLifecycleHooksUseSharedContractsWithoutNativePromotion(t *testing.T) {
	dataRoot := filepath.Join(t.TempDir(), "local", "BCGOS")
	workspacePath := t.TempDir()
	var output bytes.Buffer
	if code := runInit([]string{workspacePath}, &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK {
		t.Fatal(output.String())
	}
	output.Reset()
	if code := runHookWithInput([]string{"codex", "context-injection", "--adapter-source", "maestro", workspacePath}, strings.NewReader(`{}`), &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK || !strings.Contains(output.String(), `"hookEventName": "UserPromptSubmit"`) {
		t.Fatalf("Codex context hook = %d %s", code, output.String())
	}
	output.Reset()
	guardInput := `{"session_id":"session-a","tool_name":"Bash","tool_input":{"command":"rm -rf /"}}`
	if code := runHookWithInput([]string{"codex", "pre-action-guard", "--adapter-source", "maestro", workspacePath}, strings.NewReader(guardInput), &output, &output, func() (string, error) { return "", errors.New("workspace inspection must not run") }); code != ExitOK || !strings.Contains(output.String(), `"permissionDecision": "deny"`) {
		t.Fatalf("Codex guard = %d %s", code, output.String())
	}
	output.Reset()
	receiptInput := `{"session_id":"session-a","tool_use_id":"toolu_a","tool_name":"Bash","tool_input":{"command":"sensitive client command"}}`
	if code := runHookWithInput([]string{"codex", "post-action-receipt", "--adapter-source", "maestro", workspacePath}, strings.NewReader(receiptInput), &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK || !strings.Contains(output.String(), `"continue": true`) {
		t.Fatalf("Codex receipt hook = %d %s", code, output.String())
	}
	output.Reset()
	if code := runHookWithInput([]string{"codex", "stop-finalization", "--adapter-source", "maestro", workspacePath}, strings.NewReader(`{"session_id":"session-a"}`), &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK || !strings.Contains(output.String(), `"continue": true`) {
		t.Fatalf("Codex stop hook = %d %s", code, output.String())
	}
}

func TestContextRoutingAndExternalConfirmationHaveClaudeCodexParity(t *testing.T) {
	for _, runtimeName := range []string{"claude", "codex"} {
		t.Run(runtimeName, func(t *testing.T) {
			dataRoot := filepath.Join(t.TempDir(), "local", "BCGOS")
			workspacePath := t.TempDir()
			var output bytes.Buffer
			if code := runInit([]string{workspacePath}, &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK {
				t.Fatal(output.String())
			}
			if err := agentidentity.Save(dataRoot, agentidentity.Profile{SchemaVersion: 1, OwnerID: "owner-enrolled", Confirmed: true, UpdatedAt: time.Now().UTC(), Selections: []agentidentity.Selection{{Role: "maestro", DisplayName: "Maestro", Emoji: "🎼", OwnerID: "owner-enrolled", OwnershipScope: "system"}}}); err != nil {
				t.Fatal(err)
			}
			completeQuickOwnerOnboarding(t, dataRoot)
			output.Reset()
			if code := runAdapterWithDataRoot([]string{"install", "--runtime", runtimeName, workspacePath}, &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK {
				t.Fatalf("adapter install = %d %s", code, output.String())
			}

			prompt := `{"session_id":"session-a","prompt":"Please use $case-kickoff for this request"}`
			output.Reset()
			if code := runHookWithInput([]string{runtimeName, "context-injection", "--adapter-source", "maestro", workspacePath}, strings.NewReader(prompt), &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK || !strings.Contains(output.String(), "case-kickoff") || !strings.Contains(output.String(), "explicit_skill_reference") || strings.Contains(output.String(), "name: case-kickoff") {
				t.Fatalf("context routing = %d %s", code, output.String())
			}

			request := `{"session_id":"session-a","tool_name":"Bash","tool_input":{"command":"git push origin refs/heads/topic"}}`
			output.Reset()
			if code := runHookWithInput([]string{runtimeName, "pre-action-guard", "--adapter-source", "maestro", workspacePath}, strings.NewReader(request), &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK || !strings.Contains(output.String(), `"permissionDecision": "deny"`) {
				t.Fatalf("challenge = %d %s", code, output.String())
			}
			match := regexp.MustCompile(`CONFIRM MAESTRO [a-f0-9]{32}`).FindString(output.String())
			if match == "" {
				t.Fatalf("challenge phrase missing: %s", output.String())
			}
			confirmation := fmt.Sprintf(`{"session_id":"session-a","prompt":%q}`, match)
			output.Reset()
			if code := runHookWithInput([]string{runtimeName, "context-injection", "--adapter-source", "maestro", workspacePath}, strings.NewReader(confirmation), &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK || !strings.Contains(output.String(), "confirmed") {
				t.Fatalf("confirmation = %d %s", code, output.String())
			}
			output.Reset()
			if code := runHookWithInput([]string{runtimeName, "pre-action-guard", "--adapter-source", "maestro", workspacePath}, strings.NewReader(request), &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK || strings.Contains(output.String(), `"permissionDecision": "deny"`) {
				t.Fatalf("authorized one-shot = %d %s", code, output.String())
			}
			output.Reset()
			if code := runHookWithInput([]string{runtimeName, "pre-action-guard", "--adapter-source", "maestro", workspacePath}, strings.NewReader(request), &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK || !strings.Contains(output.String(), `"permissionDecision": "deny"`) {
				t.Fatalf("replay = %d %s", code, output.String())
			}
		})
	}
}

func TestLifecycleKeepsPendingOnboardingOnTheGovernedGuide(t *testing.T) {
	for _, runtimeName := range []string{"claude", "codex"} {
		t.Run(runtimeName, func(t *testing.T) {
			dataRoot := filepath.Join(t.TempDir(), "local", "BCGOS")
			workspacePath := t.TempDir()
			var output bytes.Buffer
			if code := runInit([]string{workspacePath}, &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK {
				t.Fatal(output.String())
			}
			output.Reset()
			if code := runAdapterWithDataRoot([]string{"install", "--runtime", runtimeName, workspacePath}, &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK {
				t.Fatalf("adapter install = %d %s", code, output.String())
			}

			output.Reset()
			if code := runHookWithInput([]string{runtimeName, "session-start", "--adapter-source", "maestro", workspacePath}, strings.NewReader(""), &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK ||
				!strings.Contains(output.String(), "maestro-onboarding") ||
				!strings.Contains(output.String(), "deterministic_onboarding_state") {
				t.Fatalf("session start did not select the governed onboarding guide = %d %s", code, output.String())
			}

			prompt := `{"session_id":"session-a","prompt":"Please use $case-kickoff before onboarding"}`
			output.Reset()
			if code := runHookWithInput([]string{runtimeName, "context-injection", "--adapter-source", "maestro", workspacePath}, strings.NewReader(prompt), &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK ||
				!strings.Contains(output.String(), "maestro-onboarding") ||
				strings.Contains(output.String(), "case-kickoff") {
				t.Fatalf("pending onboarding routed an unrelated Case method = %d %s", code, output.String())
			}
		})
	}
}

func TestContextRoutingCanSelectGovernedIngestionAndPriorWorkMethods(t *testing.T) {
	for _, runtimeName := range []string{"claude", "codex"} {
		t.Run(runtimeName, func(t *testing.T) {
			dataRoot := filepath.Join(t.TempDir(), "local", "BCGOS")
			workspacePath := t.TempDir()
			var output bytes.Buffer
			if code := runInit([]string{workspacePath}, &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK {
				t.Fatal(output.String())
			}
			completeQuickOwnerOnboarding(t, dataRoot)
			output.Reset()
			if code := runAdapterWithDataRoot([]string{"install", "--runtime", runtimeName, workspacePath}, &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK {
				t.Fatalf("adapter install = %d %s", code, output.String())
			}

			prompt := `{"session_id":"session-a","prompt":"Use $find-prior-work and $ingest-content for these authorized sources"}`
			output.Reset()
			if code := runHookWithInput([]string{runtimeName, "context-injection", "--adapter-source", "maestro", workspacePath}, strings.NewReader(prompt), &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK ||
				!strings.Contains(output.String(), "find-prior-work") ||
				!strings.Contains(output.String(), "ingest-content") {
				t.Fatalf("governed source methods were not routed = %d %s", code, output.String())
			}
		})
	}
}

func TestSessionStartReorientsToCheckpointStateWithoutExecutionContent(t *testing.T) {
	dataRoot := filepath.Join(t.TempDir(), "local", "BCGOS")
	workspacePath := t.TempDir()
	var output bytes.Buffer
	if code := runInit([]string{workspacePath}, &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK {
		t.Fatal(output.String())
	}
	completeQuickOwnerOnboarding(t, dataRoot)
	output.Reset()
	if code := runAdapterWithDataRoot([]string{"install", "--runtime", "codex", workspacePath}, &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK {
		t.Fatalf("adapter install = %d %s", code, output.String())
	}
	inspection, err := workspace.Inspect(workspacePath, dataRoot)
	if err != nil {
		t.Fatal(err)
	}
	store := execution.Store{Root: dataRoot}
	created, err := store.Create(execution.CreateInput{WorkspaceID: inspection.WorkspaceID, Objective: "secret client objective", InitialNextStep: "private next step", Criteria: []execution.Criterion{{ID: "criterion-a", Type: execution.CriterionCommandCheck, Command: []string{"go", "version"}}}})
	if err != nil {
		t.Fatal(err)
	}
	started, err := store.Start(inspection.WorkspaceID, created.Contract.ItemID, created.State.StateRevision)
	if err != nil {
		t.Fatal(err)
	}

	output.Reset()
	if code := runHookWithInput([]string{"codex", "session-start", "--adapter-source", "maestro", workspacePath}, strings.NewReader(""), &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK || !strings.Contains(output.String(), "CONTINUOUS USE STATUS") || !strings.Contains(output.String(), "checkpoint=missing") || !strings.Contains(output.String(), "bcgos work next --active") || strings.Contains(output.String(), created.Contract.ItemID) || strings.Contains(output.String(), "secret client objective") {
		t.Fatalf("pre-checkpoint SessionStart = %d %s", code, output.String())
	}

	if _, err := store.Checkpoint(inspection.WorkspaceID, created.Contract.ItemID, execution.CheckpointInput{ExpectedRevision: started.State.StateRevision, AttemptID: started.State.ActiveAttemptID, Summary: "private checkpoint body", NextStep: "private resume step"}); err != nil {
		t.Fatal(err)
	}
	output.Reset()
	if code := runHookWithInput([]string{"codex", "session-start", "--adapter-source", "maestro", workspacePath}, strings.NewReader(""), &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK || !strings.Contains(output.String(), "checkpoint=available") || !strings.Contains(output.String(), "resume through a new fenced attempt") || strings.Contains(output.String(), "private checkpoint body") || strings.Contains(output.String(), created.Contract.ItemID) {
		t.Fatalf("checkpointed SessionStart = %d %s", code, output.String())
	}
}

func completeQuickOwnerOnboarding(t *testing.T, root string) {
	t.Helper()
	if _, err := ownerctx.Initialize(root); err != nil {
		t.Fatal(err)
	}
	status, err := ownerctx.SelectOnboardingTrack(root, ownerctx.OnboardingTrackQuick)
	if err != nil {
		t.Fatal(err)
	}
	for _, facetID := range status.Onboarding.Remaining {
		facet := status.Facets[facetID]
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(facet.Path)), []byte("# "+facetID+"\n\nOwner-reviewed value.\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	status, err = ownerctx.Inspect(root)
	if err != nil || status.Onboarding.State != "review_required" {
		t.Fatalf("onboarding review state = %#v, %v", status.Onboarding, err)
	}
	if _, err := ownerctx.ConfirmOnboarding(root, status.Onboarding.ReviewDigest); err != nil {
		t.Fatal(err)
	}
}

func TestAdapterInstallBootstrapsARequestedNewWorkspacePath(t *testing.T) {
	root := t.TempDir()
	dataRoot := filepath.Join(root, "local", "BCGOS")
	workspacePath := filepath.Join(root, "new-workspace")
	var output bytes.Buffer
	if code := runAdapterWithDataRoot([]string{"install", "--runtime", "codex", workspacePath}, &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK {
		t.Fatalf("new workspace adapter install = %d %s", code, output.String())
	}
	if _, err := os.Stat(filepath.Join(workspacePath, ".bcgos", "workspace.json")); err != nil {
		t.Fatalf("new workspace manifest missing: %v", err)
	}
}

func TestAdapterCommandsInstallAndRemoveOnlyOwnedEntry(t *testing.T) {
	workspacePath := t.TempDir()
	dataRoot := filepath.Join(t.TempDir(), "local", "BCGOS")
	var output bytes.Buffer
	if code := runAdapterWithDataRoot([]string{"install", "--runtime", "codex", workspacePath}, &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK || !strings.Contains(output.String(), `"state": "installed"`) {
		t.Fatalf("install = %d %s", code, output.String())
	}
	if _, err := os.Stat(filepath.Join(workspacePath, ".bcgos", "maestro-orchestration-state.json")); err != nil {
		t.Fatalf("orchestration state was not bootstrapped: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dataRoot, "owner", "registry.json")); err != nil {
		t.Fatalf("owner context was not bootstrapped: %v", err)
	}
	if body, err := os.ReadFile(filepath.Join(workspacePath, "AGENTS.md")); err != nil || !strings.Contains(string(body), "Memória e persistência") {
		t.Fatalf("runtime orientation = %q, %v", body, err)
	}
	if body, err := os.ReadFile(filepath.Join(workspacePath, ".codex", "skills", "dream-memory", "SKILL.md")); err != nil || !strings.Contains(string(body), "name: dream-memory") {
		t.Fatalf("installed skill = %q, %v", body, err)
	}
	output.Reset()
	if code := runAdapterWithDataRoot([]string{"uninstall", "--runtime", "codex", workspacePath}, &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK || !strings.Contains(output.String(), `"state": "removed"`) {
		t.Fatalf("remove = %d %s", code, output.String())
	}
}

func TestAdapterInstallRepairsMissingStateBeforeSessionHook(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dataRoot := filepath.Join(root, "local", "BCGOS")
	workspacePath := filepath.Join(root, "workspace")
	var output bytes.Buffer
	if code := runInit([]string{workspacePath}, &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK {
		t.Fatal(output.String())
	}
	statePath := filepath.Join(workspacePath, ".bcgos", "maestro-orchestration-state.json")
	if err := os.Remove(statePath); err != nil {
		t.Fatal(err)
	}
	output.Reset()
	if code := runAdapterWithDataRoot([]string{"install", "--runtime", "claude", workspacePath}, &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK {
		t.Fatalf("adapter install did not repair state: %d %s", code, output.String())
	}
	if _, err := os.Stat(statePath); err != nil {
		t.Fatalf("repaired state missing: %v", err)
	}
	output.Reset()
	if code := runHookWithInput([]string{"claude", "session-start", "--adapter-source", "maestro", "--orchestration-state", ".bcgos/maestro-orchestration-state.json", workspacePath}, strings.NewReader(""), &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK || !strings.Contains(output.String(), `"hookEventName": "SessionStart"`) {
		t.Fatalf("session hook after repair = %d %s", code, output.String())
	}
}

func TestAdapterInstallPreservesAuthorizedSynchronizedWorkspace(t *testing.T) {
	root := t.TempDir()
	dataRoot := filepath.Join(root, "local", "BCGOS")
	workspacePath := filepath.Join(root, "OneDrive", "workspace")
	if _, err := workspace.Initialize(workspace.Options{WorkspacePath: workspacePath, DataRoot: dataRoot, AllowSynchronizedRoot: true}); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if code := runAdapterWithDataRoot([]string{"install", "--runtime", "claude", workspacePath}, &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK {
		t.Fatalf("authorized synchronized install = %d %s", code, output.String())
	}
}

func TestAdapterInstallRejectsRedirectedBootstrapRegistries(t *testing.T) {
	tests := []struct {
		name string
		path func(dataRoot string, workspacePath string) string
	}{
		{name: "owner", path: func(dataRoot, _ string) string { return filepath.Join(dataRoot, "owner", "registry.json") }},
		{name: "workspace agent", path: func(dataRoot, workspacePath string) string {
			inspection, err := workspace.Inspect(workspacePath, dataRoot)
			if err != nil {
				return ""
			}
			return filepath.Join(dataRoot, "workspaces", inspection.WorkspaceID, "agent", "agent.json")
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dataRoot := filepath.Join(t.TempDir(), "local", "BCGOS")
			workspacePath := t.TempDir()
			var output bytes.Buffer
			if code := runInit([]string{workspacePath}, &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK {
				t.Fatal(output.String())
			}
			registryPath := test.path(dataRoot, workspacePath)
			if registryPath == "" {
				t.Fatal("test registry path is empty")
			}
			outside := filepath.Join(t.TempDir(), "registry.json")
			body, err := os.ReadFile(registryPath)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(outside, body, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(registryPath); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, registryPath); err != nil {
				t.Skipf("symlink unavailable: %v", err)
			}
			output.Reset()
			if code := runAdapterWithDataRoot([]string{"install", "--runtime", "claude", workspacePath}, &output, &output, func() (string, error) { return dataRoot, nil }); code == ExitOK {
				t.Fatalf("redirected %s registry was accepted: %s", test.name, output.String())
			}
		})
	}
}

func TestDoctorSeparatesConfiguredAdapterFromRuntimeCapability(t *testing.T) {
	dataRoot, workspacePath := filepath.Join(t.TempDir(), "BCGOS"), t.TempDir()
	var output bytes.Buffer
	if code := runInit([]string{workspacePath}, &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK {
		t.Fatal(output.String())
	}
	if _, err := adaptercfg.Install("codex", workspacePath, "/opt/maestro/bcgos"); err != nil {
		t.Fatal(err)
	}
	output.Reset()
	if code := runDoctor([]string{workspacePath}, &output, &output, func() (string, error) { return dataRoot, nil }, func(string) bool { return true }); code != ExitOK || !strings.Contains(output.String(), `"id": "codex_adapter"`) || !strings.Contains(output.String(), `"state": "configured"`) || !strings.Contains(output.String(), `"context_inject"`) || !strings.Contains(output.String(), `"state": "unavailable"`) {
		t.Fatalf("doctor = %d %s", code, output.String())
	}
}

func TestSessionResolveReadsOnlyAuthorizedOwnerPointer(t *testing.T) {
	dataRoot, workspacePath := filepath.Join(t.TempDir(), "BCGOS"), t.TempDir()
	var output bytes.Buffer
	if code := runInit([]string{workspacePath}, &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK {
		t.Fatal(output.String())
	}
	output.Reset()
	if code := runOwner([]string{"init"}, &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK {
		t.Fatal(output.String())
	}
	output.Reset()
	code := runSessionResolve([]string{"--pointer", "owner/self/voice.md", "--purpose", "session", "--budget-bytes", "512", workspacePath}, &output, &output, func() (string, error) { return dataRoot, nil })
	if code != ExitOK || !strings.Contains(output.String(), `"state": "available"`) || !strings.Contains(output.String(), "# Voice") {
		t.Fatalf("resolve = %d %s", code, output.String())
	}
}

func TestSkillsIndexCommandExposesManagedPointers(t *testing.T) {
	var output bytes.Buffer
	if code := Run([]string{"skills", "index"}, &output, &output); code != ExitOK || !strings.Contains(output.String(), `"schema_version": 1`) || !strings.Contains(output.String(), `"dream-memory"`) || strings.Contains(output.String(), "Daily dreaming cannot") {
		t.Fatalf("skills index exit = %d, output = %s", code, output.String())
	}
}

func TestBundlesPlanMarksDataPracticeOptional(t *testing.T) {
	var output bytes.Buffer
	if code := Run([]string{"bundles", "plan", "--track", "data-science"}, &output, &output); code != ExitOK || !strings.Contains(output.String(), `"state": "optional"`) || !strings.Contains(output.String(), `"id": "tech-core"`) {
		t.Fatalf("bundles plan exit = %d, output = %s", code, output.String())
	}
}

func TestBundlesPlanMarksEngineeringCoreOptional(t *testing.T) {
	var output bytes.Buffer
	if code := Run([]string{"bundles", "plan", "--track", "software-engineering"}, &output, &output); code != ExitOK || !strings.Contains(output.String(), `"state": "optional"`) || !strings.Contains(output.String(), `"id": "tech-core"`) {
		t.Fatalf("bundles plan exit = %d, output = %s", code, output.String())
	}
}

func TestBundlesPlanKeepsClassicConsultingOnTheBaseBundle(t *testing.T) {
	var output bytes.Buffer
	if code := Run([]string{"bundles", "plan", "--track", "consulting"}, &output, &output); code != ExitOK || !strings.Contains(output.String(), `"state": "base_only"`) || strings.Contains(output.String(), `"id": "tech-core"`) {
		t.Fatalf("bundles plan exit = %d, output = %s", code, output.String())
	}
}

func TestBundlesRecommendUsesDeclaredFunctionAndDoesNotActivate(t *testing.T) {
	var output bytes.Buffer
	if code := Run([]string{"bundles", "recommend", "--function", "Engenheira de software"}, &output, &output); code != ExitOK ||
		!strings.Contains(output.String(), `"state": "recommended"`) ||
		!strings.Contains(output.String(), `"bundle": "tech-core"`) ||
		!strings.Contains(output.String(), `"software-engineering"`) {
		t.Fatalf("recommendation exit = %d, output = %s", code, output.String())
	}
	output.Reset()
	if code := Run([]string{"bundles", "recommend", "--function", "Consultora de estratégia"}, &output, &output); code != ExitOK ||
		!strings.Contains(output.String(), `"state": "ask"`) ||
		!strings.Contains(output.String(), "deseja incluir skills de tecnologia") {
		t.Fatalf("ambiguous recommendation exit = %d, output = %s", code, output.String())
	}
}

func TestProfileCommandsSwitchTheCanonicalUserPreference(t *testing.T) {
	dataRoot := filepath.Join(t.TempDir(), "local", "BCGOS")
	var output bytes.Buffer
	if code := runProfile([]string{"show"}, &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK || !strings.Contains(output.String(), `"profile": "standard"`) {
		t.Fatalf("profile show exit = %d, output = %s", code, output.String())
	}

	output.Reset()
	if code := runProfile([]string{"set", "advanced"}, &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK || !strings.Contains(output.String(), `"profile": "advanced"`) || !strings.Contains(output.String(), `"source": "configured"`) {
		t.Fatalf("profile set exit = %d, output = %s", code, output.String())
	}

	output.Reset()
	if code := runProfile([]string{"show"}, &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK || !strings.Contains(output.String(), `"profile": "advanced"`) {
		t.Fatalf("profile persisted exit = %d, output = %s", code, output.String())
	}
}

func TestOwnerCommandsExposeFacetsAndColdStartInterview(t *testing.T) {
	dataRoot := filepath.Join(t.TempDir(), "local", "BCGOS")
	var output bytes.Buffer
	if code := runOwner([]string{"init"}, &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK || !strings.Contains(output.String(), `"voice"`) || !strings.Contains(output.String(), `"psychological-profile"`) {
		t.Fatalf("owner init exit = %d, output = %s", code, output.String())
	}
	output.Reset()
	if code := runOwner([]string{"interview"}, &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK || !strings.Contains(output.String(), `"cold_start"`) || strings.Contains(output.String(), `"psychological-profile"`) {
		t.Fatalf("owner interview exit = %d, output = %s", code, output.String())
	}
	output.Reset()
	if code := runOwnerWithInput([]string{"onboarding", "answer", "--facet", "owner-identity", "--body", "# Identity\n\nToo early.", "--confirm"}, strings.NewReader(""), &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK || !strings.Contains(output.String(), `"track": "selection_required"`) || !strings.Contains(output.String(), "Resposta registrada") {
		t.Fatalf("onboarding answer before track selection = %d, output = %s", code, output.String())
	}
	output.Reset()
	if code := runOwner([]string{"onboarding", "select", "--track", "quick", "--confirm"}, &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK || !strings.Contains(output.String(), `"track": "quick"`) || !strings.Contains(output.String(), `"estimated_minutes": 10`) {
		t.Fatalf("quick onboarding selection = %d, output = %s", code, output.String())
	}
	output.Reset()
	if code := runOwnerWithInput([]string{"onboarding", "answer", "--facet", "owner-identity", "--body", "# Identity\n\nDaniel.", "--confirm"}, strings.NewReader(""), &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK || !strings.Contains(output.String(), `"state": "in_progress"`) || !strings.Contains(output.String(), `"personal-context"`) {
		t.Fatalf("one-shot onboarding answer = %d, output = %s", code, output.String())
	}
	output.Reset()
	if code := runOwnerWithInput([]string{"onboarding", "answer", "--facet", "communication-style", "--body", "# Communication\n\nDirect.", "--confirm"}, strings.NewReader(""), &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK || !strings.Contains(output.String(), "Resposta registrada") {
		t.Fatalf("out-of-order onboarding answer = %d, output = %s", code, output.String())
	}
	output.Reset()
	if code := runOwnerWithInput([]string{"onboarding", "answer", "--facet", "personal-context", "--body", strings.Repeat("x", maximumOwnerFacetBytes+1), "--confirm"}, strings.NewReader(""), &output, &output, func() (string, error) { return dataRoot, nil }); code == ExitOK || !strings.Contains(output.String(), "exceeds 1 MiB") {
		t.Fatalf("oversized onboarding answer body = %d, output = %s", code, output.String())
	}
	output.Reset()
	if code := runOwner([]string{"interview", "quick"}, &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK || !strings.Contains(output.String(), `"track": "quick"`) || !strings.Contains(output.String(), `"source_intake"`) || !strings.Contains(output.String(), "CV do BCG") || !strings.Contains(output.String(), "LinkedIn") || !strings.Contains(output.String(), `"owner-identity"`) || !strings.Contains(output.String(), `"personal-context"`) || !strings.Contains(output.String(), `"preferences"`) || strings.Contains(output.String(), `"decision-rules"`) {
		t.Fatalf("quick interview = %d, output = %s", code, output.String())
	}
}

func TestOwnerRefineAppliesEligibleFacetFromStdinAndRequiresConfirmationToRevert(t *testing.T) {
	dataRoot := filepath.Join(t.TempDir(), "local", "BCGOS")
	var output bytes.Buffer
	if code := runOwner([]string{"init"}, &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK {
		t.Fatalf("owner init exit = %d, output = %s", code, output.String())
	}
	output.Reset()
	code := runOwnerWithInput([]string{"refine", "submit", "--facet", "voice", "--evidence", "owner approved three drafts", "--stdin"}, strings.NewReader("# Voice\n\nClear and decisive.\n"), &output, &output, func() (string, error) { return dataRoot, nil })
	if code != ExitOK || !strings.Contains(output.String(), `"state": "proposed"`) || strings.Contains(output.String(), `"audit_id"`) {
		t.Fatalf("refine submit exit = %d, output = %s", code, output.String())
	}
	var proposal struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(output.Bytes(), &proposal); err != nil || proposal.ID == "" {
		t.Fatalf("proposal receipt = %s, err = %v", output.String(), err)
	}
	output.Reset()
	code = runOwner([]string{"refine", "apply", "--confirm", proposal.ID}, &output, &output, func() (string, error) { return dataRoot, nil })
	if code != ExitOK || !strings.Contains(output.String(), `"state": "applied"`) || !strings.Contains(output.String(), `"audit_id"`) {
		t.Fatalf("confirmed apply exit = %d, output = %s", code, output.String())
	}
	var receipt struct {
		AuditID string `json:"audit_id"`
	}
	if err := json.Unmarshal(output.Bytes(), &receipt); err != nil || receipt.AuditID == "" {
		t.Fatalf("refine receipt = %s, err = %v", output.String(), err)
	}
	output.Reset()
	code = runOwner([]string{"refine", "revert", receipt.AuditID}, &output, &output, func() (string, error) { return dataRoot, nil })
	if code == ExitOK || !strings.Contains(output.String(), "--confirm") {
		t.Fatalf("unconfirmed revert exit = %d, output = %s", code, output.String())
	}
}

func TestOwnerSelfControlsProjectAndPersistOnlyConfirmedMetadata(t *testing.T) {
	root := t.TempDir()
	var output bytes.Buffer
	dataRoot := func() (string, error) { return root, nil }
	if code := runOwnerWithInput([]string{"init"}, strings.NewReader(""), &output, &output, dataRoot); code != ExitOK {
		t.Fatalf("owner init = %d: %s", code, output.String())
	}
	output.Reset()
	if code := runOwnerWithInput([]string{"self", "snapshot"}, strings.NewReader(""), &output, &output, dataRoot); code != ExitOK {
		t.Fatalf("self snapshot = %d: %s", code, output.String())
	}
	var snapshot struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(output.Bytes(), &snapshot); err != nil || snapshot.Version == "" {
		t.Fatalf("snapshot = %s, err = %v", output.String(), err)
	}
	sum := sha256.Sum256([]byte("source"))
	input := `{"schema_version":1,"signal":"explicit_correction","facet":"voice","claim":"concise","evidence_type":"owner_correction","source_event":"interaction.completed","source_digest":"` + hex.EncodeToString(sum[:]) + `","episode_id":"episode-owner","scope_kind":"global","scope_id":"owner","confidence":0.9,"sensitivity":"professional","expires_at":"` + time.Now().UTC().Add(time.Hour).Format(time.RFC3339) + `","material":true,"declassified_global":true}`
	output.Reset()
	if code := runOwnerWithInput([]string{"self", "observe", "--stdin", "--confirm"}, strings.NewReader(input), &output, &output, dataRoot); code != ExitOK || !strings.Contains(output.String(), `"persisted": true`) {
		t.Fatalf("self observe = %d: %s", code, output.String())
	}
	output.Reset()
	if code := runOwnerWithInput([]string{"self", "observations"}, strings.NewReader(""), &output, &output, dataRoot); code != ExitOK || !strings.Contains(output.String(), `"persisted": true`) {
		t.Fatalf("self observations = %d: %s", code, output.String())
	}
	output.Reset()
	if code := runOwnerWithInput([]string{"self", "snapshot", "delete", snapshot.Version, "--confirm"}, strings.NewReader(""), &output, &output, dataRoot); code != ExitOK {
		t.Fatalf("self snapshot delete = %d: %s", code, output.String())
	}
}

func TestOwnerPromptHistoryControlsKeepBodiesOutOfInspectReceipts(t *testing.T) {
	root := t.TempDir()
	dataRoot := func() (string, error) { return root, nil }
	var output bytes.Buffer
	if code := runOwnerWithInput([]string{"init"}, strings.NewReader(""), &output, &output, dataRoot); code != ExitOK {
		t.Fatalf("owner init = %d: %s", code, output.String())
	}
	output.Reset()
	if code := runOwnerWithInput([]string{"prompt-history", "add", "--scope-kind", "case", "--scope-id", "case-a", "--language", "pt-BR", "--session-id", "session-a", "--stdin", "--confirm"}, strings.NewReader("user-only prompt"), &output, &output, dataRoot); code != ExitOK || strings.Contains(output.String(), "user-only prompt") {
		t.Fatalf("prompt history add = %d: %s", code, output.String())
	}
	output.Reset()
	if code := runOwnerWithInput([]string{"prompt-history", "inspect"}, strings.NewReader(""), &output, &output, dataRoot); code != ExitOK || strings.Contains(output.String(), "user-only prompt") {
		t.Fatalf("prompt history inspect = %d: %s", code, output.String())
	}
	output.Reset()
	if code := runOwnerWithInput([]string{"prompt-history", "export", "--confirm"}, strings.NewReader(""), &output, &output, dataRoot); code != ExitOK || !strings.Contains(output.String(), "user-only prompt") {
		t.Fatalf("prompt history export = %d: %s", code, output.String())
	}
}

func TestMaestroDispatchBoundaryRecordsPromptPlansAndPersistsMetadataOnlyChain(t *testing.T) {
	root := t.TempDir()
	dataRoot := func() (string, error) { return root, nil }
	input := `{"authenticated_owner":true,"owner_id":"owner","dispatch_id":"dispatch-cli-1","prompt":"prepare case alpha","language":"en-US","source":"cli","session_id":"session-dispatch","working_language":"en-US","current_language":"en-US","plan":{"schema_version":1,"intent_class":"case_execution","scope_kind":"case","scope_id":"case-alpha","account_scope_id":"account-alpha","sensitivity":"internal","materiality":"none","health_governance_intent":"none","available_registered_agents":[{"id":"account-agent-alpha","role":"client_account_agent","scope_kind":"account","scope_id":"account-alpha","authorization_digest":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","state_snapshot_digest":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","available":true},{"id":"case-agent-alpha","role":"case_agent","scope_kind":"case","scope_id":"case-alpha","parent_scope_kind":"account","parent_scope_id":"account-alpha","authorization_digest":"cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc","state_snapshot_digest":"dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd","available":true}]}}`
	input = addTestCapabilityDigests(input)
	var output bytes.Buffer
	if code := runMaestroWithInput([]string{"dispatch", "--stdin"}, strings.NewReader(input), &output, &output, dataRoot); code != ExitOK {
		t.Fatalf("Maestro dispatch = %d: %s", code, output.String())
	}
	if strings.Contains(output.String(), "prepare case alpha") || !strings.Contains(output.String(), "dispatch_boundary_model_pending_input") || !strings.Contains(output.String(), `"durable_dispatch_epoch": 1`) {
		t.Fatalf("dispatch boundary leaked body or missed pending-input result: %s", output.String())
	}
	chains, err := filepath.Glob(filepath.Join(root, "owner", "maestro", "chains", "*.json"))
	if err != nil || len(chains) != 1 {
		t.Fatalf("persisted chain files = %v, err=%v", chains, err)
	}
	output.Reset()
	if code := runMaestroWithInput([]string{"dispatch", "--stdin"}, strings.NewReader(input), &output, &output, dataRoot); code != ExitOK || !strings.Contains(output.String(), `"durable_dispatch_epoch": 1`) {
		t.Fatalf("same occurrence was not idempotent: code=%d output=%s", code, output.String())
	}
	history, err := ownerctx.InspectPromptHistory(root)
	if err != nil || len(history) != 1 {
		t.Fatalf("same JSON retry duplicated prompt history: entries=%d err=%v", len(history), err)
	}
	changedPrompt := strings.Replace(input, `"prompt":"prepare case alpha"`, `"prompt":"changed prompt"`, 1)
	output.Reset()
	if code := runMaestroWithInput([]string{"dispatch", "--stdin"}, strings.NewReader(changedPrompt), &output, &output, dataRoot); code == ExitOK {
		t.Fatal("same dispatch occurrence accepted changed prompt")
	}
	history, err = ownerctx.InspectPromptHistory(root)
	if err != nil || len(history) != 1 || history[0].SHA256 != maestro.SHA256Hex("prepare case alpha") {
		t.Fatalf("changed occurrence mutated prompt history: entries=%#v err=%v", history, err)
	}
	distinctOccurrence := strings.Replace(input, `"dispatch_id":"dispatch-cli-1"`, `"dispatch_id":"dispatch-cli-2"`, 1)
	output.Reset()
	if code := runMaestroWithInput([]string{"dispatch", "--stdin"}, strings.NewReader(distinctOccurrence), &output, &output, dataRoot); code != ExitOK || !strings.Contains(output.String(), `"durable_dispatch_epoch": 2`) {
		t.Fatalf("distinct occurrence did not advance the durable epoch: code=%d output=%s", code, output.String())
	}

	selfRoot := t.TempDir()
	selfDataRoot := func() (string, error) { return selfRoot, nil }
	selfInput := strings.Replace(input, `"prompt":"prepare case alpha"`, `"self_signal":{"signal":"explicit_correction","facet":"communication-style","claim":"prefers_concise","evidence_type":"owner_correction","confidence":1,"sensitivity":"professional","owner_confirmed":true},"prompt":"prepare case alpha"`, 1)
	output.Reset()
	if code := runMaestroWithInput([]string{"dispatch", "--stdin"}, strings.NewReader(selfInput), &output, &output, selfDataRoot); code != ExitOK {
		t.Fatalf("self-signal dispatch = %d: %s", code, output.String())
	}
	firstObservations, err := ownerctx.ListObservations(selfRoot)
	if err != nil || len(firstObservations) != 1 {
		t.Fatalf("first self-signal observation = %#v, err=%v", firstObservations, err)
	}
	firstObservationID := firstObservations[0].ID
	output.Reset()
	if code := runMaestroWithInput([]string{"dispatch", "--stdin"}, strings.NewReader(selfInput), &output, &output, selfDataRoot); code != ExitOK {
		t.Fatalf("self-signal retry = %d: %s", code, output.String())
	}
	secondObservations, err := ownerctx.ListObservations(selfRoot)
	if err != nil || len(secondObservations) != 1 || secondObservations[0].ID != firstObservationID {
		t.Fatalf("self-signal retry was not idempotent: %#v, err=%v", secondObservations, err)
	}
}

func TestMaestroDispatchExecutesDeterministicAccountCaseWalterLoop(t *testing.T) {
	root := t.TempDir()
	dataRoot := func() (string, error) { return root, nil }
	digest := func(seed string) string { return maestro.SHA256Hex(seed) }
	agent := func(id, role, kind, scope, parentKind, parentScope string) maestro.RegisteredAgent {
		return maestro.RegisteredAgent{
			ID: id, Role: role, ScopeKind: kind, ScopeID: scope,
			ParentScopeKind: parentKind, ParentScopeID: parentScope,
			AuthorizationDigest: digest(id + "-authorization"),
			CapabilityDigest:    digest(id + "-capability"),
			StateSnapshotDigest: digest(id + "-state"), Available: true,
		}
	}
	request := maestroDispatchRequest{
		AuthenticatedOwner: true, OwnerID: "owner", DispatchID: "dispatch-orchestration",
		Prompt: "prepare strategic case", Language: "en-US", Source: "cli", SessionID: "orchestration-session",
		Plan: maestro.Input{
			SchemaVersion: 1, IntentClass: maestro.IntentCase, ScopeKind: "case", ScopeID: "case-alpha",
			AccountScopeID: "account-alpha", Sensitivity: maestro.SensitivityInternal, Materiality: maestro.MaterialityReview,
			StrategicImplication: true, ClientImplication: true, HealthIntent: maestro.HealthNone,
			AvailableAgents: []maestro.RegisteredAgent{
				agent("account-agent-alpha", "client_account_agent", "account", "account-alpha", "", ""),
				agent("case-agent-alpha", "case_agent", "case", "case-alpha", "account", "account-alpha"),
				agent("walter", "reviewer", "review", "review", "", ""),
			},
		},
	}
	outputDigest := digest("case-output")
	request.AgentEvents = []maestro.AgentEvent{
		{AgentID: "account-agent-alpha", Decision: "approve"},
		{AgentID: "case-agent-alpha", Decision: "return", ContentDigest: outputDigest},
		{AgentID: "account-agent-alpha", Decision: "approve", ContentDigest: outputDigest},
		{AgentID: "walter", Decision: "approve", ContentDigest: outputDigest},
	}
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if code := runMaestroWithInput([]string{"dispatch", "--stdin"}, bytes.NewReader(body), &output, &output, dataRoot); code != ExitOK {
		t.Fatalf("orchestrated dispatch = %d: %s", code, output.String())
	}
	var receipt maestro.DispatchBoundaryReceipt
	if err := json.Unmarshal(output.Bytes(), &receipt); err != nil {
		t.Fatal(err)
	}
	if receipt.Outcome != "orchestration_contract_complete" || receipt.State != maestro.StageFinal || receipt.OrchestrationStage != maestro.StageFinal || receipt.AgentEventCount != 4 {
		t.Fatalf("orchestration receipt = %#v", receipt)
	}
	chainFiles, err := filepath.Glob(filepath.Join(root, "owner", "maestro", "chains", "*.json"))
	if err != nil || len(chainFiles) != 1 {
		t.Fatalf("chain files = %v, err=%v", chainFiles, err)
	}
	chainBody, err := os.ReadFile(chainFiles[0])
	if err != nil || !strings.Contains(string(chainBody), `"stage": "final"`) || !strings.Contains(string(chainBody), `"stage": "walter_review"`) {
		t.Fatalf("persisted orchestration chain = %s, err=%v", chainBody, err)
	}
}

func TestMaestroDispatchPromptFailureLeavesPreparedBoundaryForRecovery(t *testing.T) {
	root := t.TempDir()
	dataRoot := func() (string, error) { return root, nil }
	input := `{"authenticated_owner":true,"owner_id":"owner","dispatch_id":"dispatch-prompt-failure","prompt":"prepare case alpha","language":"en-US","source":"cli","session_id":"session-prompt-failure","working_language":"en-US","current_language":"en-US","plan":{"schema_version":1,"intent_class":"case_execution","scope_kind":"case","scope_id":"case-alpha","account_scope_id":"account-alpha","sensitivity":"internal","materiality":"none","health_governance_intent":"none","available_registered_agents":[{"id":"account-agent-alpha","role":"client_account_agent","scope_kind":"account","scope_id":"account-alpha","authorization_digest":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","state_snapshot_digest":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","available":true},{"id":"case-agent-alpha","role":"case_agent","scope_kind":"case","scope_id":"case-alpha","parent_scope_kind":"account","parent_scope_id":"account-alpha","authorization_digest":"cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc","state_snapshot_digest":"dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd","available":true}]}}`
	input = addTestCapabilityDigests(input)
	original := recordUserPromptFunc
	recordUserPromptFunc = func(string, ownerctx.PromptHistoryInput) (ownerctx.PromptHistoryReceipt, error) {
		return ownerctx.PromptHistoryReceipt{}, errors.New("injected prompt failure")
	}
	defer func() { recordUserPromptFunc = original }()
	var output bytes.Buffer
	if code := runMaestroWithInput([]string{"dispatch", "--stdin"}, strings.NewReader(input), &output, &output, dataRoot); code == ExitOK {
		t.Fatal("injected prompt failure was accepted")
	}
	receipts, err := filepath.Glob(filepath.Join(root, "owner", "maestro", "dispatch", "receipts", "*.json"))
	if err != nil || len(receipts) != 1 {
		t.Fatalf("prepared dispatch receipt count = %v, err=%v", receipts, err)
	}
	body, err := os.ReadFile(receipts[0])
	if err != nil {
		t.Fatal(err)
	}
	var state maestro.DispatchBoundaryState
	if err := json.Unmarshal(body, &state); err != nil || state.Status != "prepared" || !state.FinishedAt.IsZero() {
		t.Fatalf("prompt failure left false-finished boundary: %#v, err=%v", state, err)
	}
	if entries, err := ownerctx.InspectPromptHistory(root); err != nil || len(entries) != 0 {
		t.Fatalf("prompt failure mutated history: %#v, err=%v", entries, err)
	}
	chainFiles, err := filepath.Glob(filepath.Join(root, "owner", "maestro", "chains", "*.json"))
	if err != nil || len(chainFiles) != 0 {
		t.Fatalf("prompt failure left chain state: %v, err=%v", chainFiles, err)
	}
	recordUserPromptFunc = original
	output.Reset()
	if code := runMaestroWithInput([]string{"dispatch", "--stdin"}, strings.NewReader(input), &output, &output, dataRoot); code != ExitOK || !strings.Contains(output.String(), `"durable_dispatch_epoch": 1`) {
		t.Fatalf("prepared occurrence did not recover: code=%d output=%s", code, output.String())
	}
	if entries, err := ownerctx.InspectPromptHistory(root); err != nil || len(entries) != 1 {
		t.Fatalf("recovery prompt count = %#v, err=%v", entries, err)
	}
}

func addTestCapabilityDigests(input string) string {
	return strings.ReplaceAll(input, `"state_snapshot_digest":"`, `"capability_digest":"eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee","state_snapshot_digest":"`)
}

func TestMaestroDispatchValidatesBeforeRecordingPromptOrChain(t *testing.T) {
	root := t.TempDir()
	dataRoot := func() (string, error) { return root, nil }
	invalid := `{"authenticated_owner":true,"owner_id":"owner","dispatch_id":"dispatch-invalid","prompt":"should not persist"}`
	var output bytes.Buffer
	if code := runMaestroWithInput([]string{"dispatch", "--stdin"}, strings.NewReader(invalid), &output, &output, dataRoot); code == ExitOK {
		t.Fatal("invalid plan unexpectedly dispatched")
	}
	if _, err := os.Stat(filepath.Join(root, "owner", "prompt-history", "entries.jsonl")); !os.IsNotExist(err) {
		t.Fatalf("invalid plan left prompt history: %v", err)
	}
	if chains, err := filepath.Glob(filepath.Join(root, "owner", "maestro", "chains", "*.json")); err != nil || len(chains) != 0 {
		t.Fatalf("invalid plan left chains: %v, err=%v", chains, err)
	}

	output.Reset()
	valid := `{"authenticated_owner":true,"owner_id":"owner","dispatch_id":"dispatch-translation-failure","prompt":"translate current","language":"en-US","source":"cli","session_id":"translation-failure","working_language":"pt-BR","current_language":"en-US","plan":{"schema_version":1,"intent_class":"case_execution","scope_kind":"case","scope_id":"case-alpha","account_scope_id":"account-alpha","sensitivity":"internal","materiality":"none","health_governance_intent":"none","available_registered_agents":[{"id":"account-agent-alpha","role":"client_account_agent","scope_kind":"account","scope_id":"account-alpha","authorization_digest":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","state_snapshot_digest":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","available":true},{"id":"case-agent-alpha","role":"case_agent","scope_kind":"case","scope_id":"case-alpha","parent_scope_kind":"account","parent_scope_id":"account-alpha","authorization_digest":"cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc","state_snapshot_digest":"dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd","available":true}]}}`
	valid = addTestCapabilityDigests(valid)
	if code := runMaestroWithInput([]string{"dispatch", "--stdin"}, strings.NewReader(valid), &output, &output, dataRoot); code == ExitOK {
		t.Fatal("translation failure unexpectedly dispatched")
	}
	if entries, err := ownerctx.InspectPromptHistory(root); err == nil && len(entries) != 0 {
		t.Fatalf("translation failure recorded prompt: %#v", entries)
	}
	if chains, err := filepath.Glob(filepath.Join(root, "owner", "maestro", "chains", "*.json")); err != nil || len(chains) != 0 {
		t.Fatalf("translation failure left chains: %v, err=%v", chains, err)
	}
}

func TestMaestroDispatchRejectsUnknownAuthorityLookingFieldsBeforeMutation(t *testing.T) {
	root := t.TempDir()
	dataRoot := func() (string, error) { return root, nil }
	input := `{"authenticated_owner":true,"owner_id":"owner","dispatch_id":"dispatch-unknown","prompt":"should not persist","authority_grant":"case_agent"}`
	var output bytes.Buffer
	if code := runMaestroWithInput([]string{"dispatch", "--stdin"}, strings.NewReader(input), &output, &output, dataRoot); code == ExitOK {
		t.Fatal("unknown authority-looking field was accepted")
	}
	if entries, err := ownerctx.InspectPromptHistory(root); err == nil && len(entries) != 0 {
		t.Fatalf("unknown field left prompt history: %#v", entries)
	}
	if chains, err := filepath.Glob(filepath.Join(root, "owner", "maestro", "chains", "*.json")); err != nil || len(chains) != 0 {
		t.Fatalf("unknown field left chains: %v, err=%v", chains, err)
	}
	if receipts, err := filepath.Glob(filepath.Join(root, "owner", "maestro", "dispatch", "receipts", "*.json")); err != nil || len(receipts) != 0 {
		t.Fatalf("unknown field left dispatch receipts: %v, err=%v", receipts, err)
	}
}

func TestMaestroWalterDefaultFacetAllowlistExcludesPsychologicalProfile(t *testing.T) {
	root := t.TempDir()
	if _, err := ownerctx.Initialize(root); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "owner", "self", "psychological-profile.md"), []byte("sensitive owner profile"), 0o600); err != nil {
		t.Fatal(err)
	}
	snapshot, err := ownerctx.ProjectSnapshot(root, append([]string(nil), maestroWalterFacetAllowlist...))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := snapshot.Facets["psychological-profile"]; ok {
		t.Fatal("ordinary Maestro Walter context included psychological profile")
	}
	for _, facet := range maestroWalterFacetAllowlist {
		if _, ok := snapshot.Facets[facet]; !ok {
			t.Fatalf("Walter allowlist omitted expected facet %q", facet)
		}
	}
}

func TestMaestroWalterActivityDoesNotInventSelfEvidence(t *testing.T) {
	root := t.TempDir()
	if _, err := ownerctx.Initialize(root); err != nil {
		t.Fatal(err)
	}
	input := maestro.Input{SchemaVersion: 1, IntentClass: maestro.IntentCase, ScopeKind: "case", ScopeID: "case-self-negative", AccountScopeID: "account-self-negative", Sensitivity: maestro.SensitivityInternal, Materiality: maestro.MaterialityReview, HealthIntent: maestro.HealthNone, AvailableAgents: []maestro.RegisteredAgent{
		{ID: "account-self-negative", Role: "client_account_agent", ScopeKind: "account", ScopeID: "account-self-negative", AuthorizationDigest: strings.Repeat("a", 64), CapabilityDigest: strings.Repeat("b", 64), StateSnapshotDigest: strings.Repeat("c", 64), Available: true},
		{ID: "case-self-negative", Role: "case_agent", ScopeKind: "case", ScopeID: "case-self-negative", ParentScopeKind: "account", ParentScopeID: "account-self-negative", AuthorizationDigest: strings.Repeat("d", 64), CapabilityDigest: strings.Repeat("e", 64), StateSnapshotDigest: strings.Repeat("f", 64), Available: true},
		{ID: "walter", Role: "reviewer", ScopeKind: "review", ScopeID: "review", AuthorizationDigest: strings.Repeat("1", 64), CapabilityDigest: strings.Repeat("2", 64), StateSnapshotDigest: strings.Repeat("3", 64), Available: true},
	}}
	input.Materiality = maestro.MaterialityReview
	plan, err := maestro.PlanFor(input)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := ownerctx.ProjectSnapshot(root, append([]string(nil), maestroWalterFacetAllowlist...))
	if err != nil {
		t.Fatal(err)
	}
	packet, err := maestro.BuildIntentReviewPacket("make the high leverage recommendation", plan, "draft", nil, snapshot, nil, "executive", "high", "hard_to_reverse", "")
	if err != nil {
		t.Fatal(err)
	}
	request := maestroDispatchRequest{AuthenticatedOwner: true, OwnerID: "owner", DispatchID: "dispatch-self-negative", Plan: input}
	scopeKind, scopeID, err := mapPlannerObservationScope(request.Plan.ScopeKind, request.Plan.ScopeID)
	if err != nil {
		t.Fatal(err)
	}
	receipt, evaluation, err := ownerctx.AppendObservation(root, maestroDispatchObservation(request, plan, packet, scopeKind, scopeID, time.Now().UTC()))
	if err != nil || !evaluation.Evaluated || evaluation.Persist || receipt.Persisted {
		t.Fatalf("Walter activity was treated as self evidence: evaluation=%#v receipt=%#v err=%v", evaluation, receipt, err)
	}
	observations, err := ownerctx.ListObservations(root)
	if err != nil || len(observations) != 0 {
		t.Fatalf("Walter activity persisted a self observation: %#v, err=%v", observations, err)
	}
}

func TestMaestroSelfSignalContractRejectsUnsafeSemanticVariants(t *testing.T) {
	valid := &maestroSelfSignal{Signal: ownerctx.SignalExplicitCorrection, Facet: "communication-style", Claim: "prefers_concise", EvidenceType: "owner_correction", Confidence: 1, Sensitivity: "professional", OwnerConfirmed: true}
	if err := validateMaestroSelfSignal(valid); err != nil {
		t.Fatal(err)
	}
	variants := []struct {
		name string
		edit func(*maestroSelfSignal)
	}{
		{name: "unsupported signal", edit: func(signal *maestroSelfSignal) { signal.Signal = ownerctx.SignalObservedPattern }},
		{name: "unknown facet", edit: func(signal *maestroSelfSignal) { signal.Facet = "unsupported-facet" }},
		{name: "generated evidence", edit: func(signal *maestroSelfSignal) { signal.EvidenceType = "generated_output" }},
		{name: "client evidence", edit: func(signal *maestroSelfSignal) { signal.EvidenceType = "client_document" }},
		{name: "unconfirmed", edit: func(signal *maestroSelfSignal) { signal.OwnerConfirmed = false }},
	}
	for _, variant := range variants {
		t.Run(variant.name, func(t *testing.T) {
			candidate := *valid
			variant.edit(&candidate)
			if err := validateMaestroSelfSignal(&candidate); err == nil {
				t.Fatal("unsafe self signal was accepted")
			}
		})
	}
	generic := ownerctx.ObservationInput{SchemaVersion: 1, Signal: ownerctx.SignalExplicitEndorsement, Facet: "communication-style", Claim: "ok", EvidenceType: "owner_endorsement", SourceEvent: "interaction.completed", SourceDigest: maestro.SHA256Hex("self-signal-generic"), EpisodeID: "self-signal-generic", ScopeKind: "case", ScopeID: "case-self-negative", Confidence: 1, Sensitivity: "professional", ExpiresAt: time.Now().UTC().Add(time.Hour), AuthenticatedOwner: true, Material: true, OwnerConfirmed: true}
	if _, err := ownerctx.EvaluateInteraction(generic); err == nil {
		t.Fatal("generic OK endorsement was accepted")
	}
}

func TestMaestroDispatchRejectsGenericEndorsementBeforeAnyMutation(t *testing.T) {
	root := t.TempDir()
	request := maestroDispatchRequest{
		AuthenticatedOwner: true, OwnerID: "owner", DispatchID: "generic-endorsement", Prompt: "answer directly", Language: "en-US", Source: "cli", SessionID: "generic-session",
		SelfSignal: &maestroSelfSignal{Signal: ownerctx.SignalExplicitEndorsement, Facet: "communication-style", Claim: "okay", EvidenceType: "owner_endorsement", Confidence: 1, Sensitivity: "professional", OwnerConfirmed: true},
		Plan:       maestro.Input{SchemaVersion: 1, IntentClass: maestro.IntentDirectAnswer, ScopeKind: "workspace", ScopeID: "workspace-a", Sensitivity: maestro.SensitivityInternal, Materiality: maestro.MaterialityNone, HealthIntent: maestro.HealthNone},
	}
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if code := runMaestroWithInput([]string{"dispatch", "--stdin"}, bytes.NewReader(body), &output, &output, func() (string, error) { return root, nil }); code == ExitOK {
		t.Fatalf("generic endorsement was accepted: %s", output.String())
	}
	if _, err := os.Stat(filepath.Join(root, "owner")); !os.IsNotExist(err) {
		t.Fatalf("generic endorsement mutated owner state: %v", err)
	}
}

func TestMaestroDispatchKeepsEarlierSameSessionHistoryAndCurrentIsNotDuplicated(t *testing.T) {
	root := t.TempDir()
	dataRoot := func() (string, error) { return root, nil }
	if _, err := ownerctx.Initialize(root); err != nil {
		t.Fatal(err)
	}
	if _, err := ownerctx.RecordUserPrompt(root, ownerctx.PromptHistoryInput{OwnerID: "owner", Prompt: "prepare case alpha", Language: "en-US", Source: "cli", SessionID: "session-dispatch", ScopeKind: ownerctx.PromptScopeCase, ScopeID: "case-alpha", ContentKind: "user_prompt"}); err != nil {
		t.Fatal(err)
	}
	input := `{"authenticated_owner":true,"owner_id":"owner","dispatch_id":"dispatch-cli-2","prompt":"prepare case alpha","language":"en-US","source":"cli","session_id":"session-dispatch","working_language":"en-US","current_language":"en-US","plan":{"schema_version":1,"intent_class":"case_execution","scope_kind":"case","scope_id":"case-alpha","account_scope_id":"account-alpha","sensitivity":"internal","materiality":"none","health_governance_intent":"none","available_registered_agents":[{"id":"account-agent-alpha","role":"client_account_agent","scope_kind":"account","scope_id":"account-alpha","authorization_digest":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","state_snapshot_digest":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","available":true},{"id":"case-agent-alpha","role":"case_agent","scope_kind":"case","scope_id":"case-alpha","parent_scope_kind":"account","parent_scope_id":"account-alpha","authorization_digest":"cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc","state_snapshot_digest":"dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd","available":true}]}}`
	input = addTestCapabilityDigests(input)
	var output bytes.Buffer
	if code := runMaestroWithInput([]string{"dispatch", "--stdin"}, strings.NewReader(input), &output, &output, dataRoot); code != ExitOK || !strings.Contains(output.String(), `"history_count": 1`) {
		t.Fatalf("earlier same-session prompt was suppressed or current duplicated: code=%d output=%s", code, output.String())
	}
}

func TestMaestroDispatchActionRoutesUseExplicitOwnerObservationScopeMapping(t *testing.T) {
	digest := func(seed string) string { return maestro.SHA256Hex(seed) }
	agent := func(id, role, kind, scope string) maestro.RegisteredAgent {
		return maestro.RegisteredAgent{ID: id, Role: role, ScopeKind: kind, ScopeID: scope, AuthorizationDigest: digest(id + "-authorization"), CapabilityDigest: digest(id + "-capability"), StateSnapshotDigest: digest(id + "-state"), Available: true}
	}
	tests := []struct {
		name  string
		input maestro.Input
		stage maestro.Stage
	}{
		{name: "direct answer", input: maestro.Input{SchemaVersion: 1, IntentClass: maestro.IntentDirectAnswer, ScopeKind: "workspace", ScopeID: "workspace-a", Sensitivity: maestro.SensitivityInternal, Materiality: maestro.MaterialityNone, HealthIntent: maestro.HealthNone}, stage: maestro.StageFinal},
		{name: "account", input: maestro.Input{SchemaVersion: 1, IntentClass: maestro.IntentAccount, ScopeKind: "account", ScopeID: "client-alpha", Sensitivity: maestro.SensitivityInternal, Materiality: maestro.MaterialityNone, HealthIntent: maestro.HealthNone, AvailableAgents: []maestro.RegisteredAgent{agent("account-agent", "client_account_agent", "account", "client-alpha")}}, stage: maestro.StageAccountAdvisory},
		{name: "PA", input: maestro.Input{SchemaVersion: 1, IntentClass: maestro.IntentAdvisory, ScopeKind: "practice", ScopeID: "fpa", Sensitivity: maestro.SensitivityInternal, Materiality: maestro.MaterialityNone, HealthIntent: maestro.HealthNone, AvailableAgents: []maestro.RegisteredAgent{agent("pa-expert", "pa_expert", "practice", "fpa")}}, stage: maestro.StagePAExpert},
		{name: "Walter", input: maestro.Input{SchemaVersion: 1, IntentClass: maestro.IntentReview, ScopeKind: "review", ScopeID: "review", Sensitivity: maestro.SensitivityInternal, Materiality: maestro.MaterialityNone, HealthIntent: maestro.HealthNone, AvailableAgents: []maestro.RegisteredAgent{agent("walter", "reviewer", "review", "review")}}, stage: maestro.StageWalterReview},
		{name: "Darwin", input: maestro.Input{SchemaVersion: 1, IntentClass: maestro.IntentHealth, ScopeKind: "health", ScopeID: "system", Sensitivity: maestro.SensitivityInternal, Materiality: maestro.MaterialityNone, HealthIntent: maestro.HealthSystem, AvailableAgents: []maestro.RegisteredAgent{agent("darwin", "governance_analyst", "health", "system")}}, stage: maestro.StageDarwinHealth},
		{name: "errand", input: maestro.Input{SchemaVersion: 1, IntentClass: maestro.IntentErrand, ScopeKind: "errand", ScopeID: "errand-a", Sensitivity: maestro.SensitivityInternal, Materiality: maestro.MaterialityNone, HealthIntent: maestro.HealthNone, SimpleReversible: true, ExecutionOnly: true, AvailableAgents: []maestro.RegisteredAgent{agent("errand", "errand_helper", "errand", "errand-a")}}, stage: maestro.StageErrandExecution},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			root := t.TempDir()
			request := maestroDispatchRequest{AuthenticatedOwner: true, OwnerID: "owner", DispatchID: "route-" + strings.ToLower(strings.ReplaceAll(testCase.name, " ", "-")), Prompt: "route " + testCase.name, Language: "en-US", Source: "cli", SessionID: "route-session", Plan: testCase.input}
			body, err := json.Marshal(request)
			if err != nil {
				t.Fatal(err)
			}
			var output bytes.Buffer
			if code := runMaestroWithInput([]string{"dispatch", "--stdin"}, bytes.NewReader(body), &output, &output, func() (string, error) { return root, nil }); code != ExitOK {
				t.Fatalf("dispatch exit=%d output=%s", code, output.String())
			}
			var receipt maestro.DispatchBoundaryReceipt
			if err := json.Unmarshal(output.Bytes(), &receipt); err != nil {
				t.Fatalf("receipt=%q err=%v", output.String(), err)
			}
			if receipt.State != testCase.stage {
				t.Fatalf("receipt state=%s want=%s", receipt.State, testCase.stage)
			}
			observations, err := ownerctx.ListObservations(root)
			if err != nil || len(observations) != 0 {
				t.Fatalf("default route activity became self evidence: %#v err=%v", observations, err)
			}
		})
	}
}

func TestPlannerObservationScopeMappingIsClosedAndNonAuthoritative(t *testing.T) {
	for _, scope := range []string{"control", "workspace", "case", "account", "practice", "review", "health", "errand"} {
		kind, id, err := mapPlannerObservationScope(scope, "scope-a")
		if err != nil || id == "" {
			t.Fatalf("scope %q mapping = %q/%q err=%v", scope, kind, id, err)
		}
		if scope == "case" && kind != ownerctx.PromptScopeCase || scope == "account" && kind != ownerctx.PromptScopeAccount || scope == "workspace" && kind != ownerctx.PromptScopeWorkspace {
			t.Fatalf("native owner scope mapping changed for %q: %q/%q", scope, kind, id)
		}
		if scope != "case" && scope != "account" && scope != "workspace" && kind != ownerctx.PromptScopeWorkspace {
			t.Fatalf("planner-only scope %q did not project to workspace metadata scope: %q/%q", scope, kind, id)
		}
	}
	if _, _, err := mapPlannerObservationScope("unknown", "scope-a"); err == nil {
		t.Fatal("unknown planner scope was projected into owner scope")
	}
}

func TestAtlasCommandsBootstrapOnlyPrivateOwnerAndWorkspaceRoots(t *testing.T) {
	root := t.TempDir()
	dataRoot := filepath.Join(root, "local", "BCGOS")
	workspacePath := filepath.Join(root, "Developer", "case-a")
	var output bytes.Buffer
	if code := runInit([]string{workspacePath}, &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK {
		t.Fatalf("workspace init exit = %d, output = %s", code, output.String())
	}
	output.Reset()
	if code := runAtlas([]string{"init", workspacePath}, &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK || !strings.Contains(output.String(), `"managed": {`) || !strings.Contains(output.String(), `"state": "unavailable"`) || !strings.Contains(output.String(), `"workspace": {`) {
		t.Fatalf("atlas init exit = %d, output = %s", code, output.String())
	}
	if info, err := os.Stat(filepath.Join(workspacePath, "brain", "tasks", "README.md")); err != nil || info.IsDir() {
		t.Fatalf("visible task stub = %v", err)
	}
}

func TestSessionPacketReportsPointersWithoutOwnerFacetBodies(t *testing.T) {
	root := t.TempDir()
	dataRoot := filepath.Join(root, "local", "BCGOS")
	workspacePath := filepath.Join(root, "Developer", "case-a")
	var output bytes.Buffer
	if code := runInit([]string{workspacePath}, &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK {
		t.Fatalf("workspace init exit = %d, output = %s", code, output.String())
	}
	output.Reset()
	if code := runOwner([]string{"init"}, &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK {
		t.Fatalf("owner init exit = %d, output = %s", code, output.String())
	}
	output.Reset()
	if code := runAtlas([]string{"init", workspacePath}, &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK {
		t.Fatalf("atlas init exit = %d, output = %s", code, output.String())
	}
	output.Reset()
	if code := runSession([]string{"packet", workspacePath}, &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK || !strings.Contains(output.String(), `"schema_version": 1`) || !strings.Contains(output.String(), `"catalog_pointer": "bundles/base/skills/catalog.json"`) || strings.Contains(output.String(), "Descreva como voce quer falar") {
		t.Fatalf("session packet exit = %d, output = %s", code, output.String())
	}
}

func TestSessionBridgeProducesTheSameBoundedAdapterInputForEachRuntime(t *testing.T) {
	root := t.TempDir()
	dataRoot := filepath.Join(root, "local", "BCGOS")
	workspacePath := filepath.Join(root, "Developer", "case-a")
	var output bytes.Buffer
	if code := runInit([]string{workspacePath}, &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK {
		t.Fatalf("workspace init exit = %d, output = %s", code, output.String())
	}
	output.Reset()
	if code := runSession([]string{"bridge", "--runtime", "claude", workspacePath}, &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK || !strings.Contains(output.String(), `"event": "session_start"`) || !strings.Contains(output.String(), `"runtime": "claude"`) || !strings.Contains(output.String(), `"availability_state": "enabled"`) {
		t.Fatalf("Claude bridge exit = %d, output = %s", code, output.String())
	}
	output.Reset()
	if code := runSession([]string{"bridge", "--runtime", "codex", workspacePath}, &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK || !strings.Contains(output.String(), `"runtime": "codex"`) || strings.Contains(output.String(), "Descreva como voce quer falar") {
		t.Fatalf("Codex bridge exit = %d, output = %s", code, output.String())
	}
}

func TestExecutionHandoffAcrossTwoSessionsRecoversAndCompletesFirstSessionArtifact(t *testing.T) {
	root := t.TempDir()
	dataRoot := func() (string, error) { return root, nil }
	workspacePath := filepath.Join(t.TempDir(), "case-a")
	if _, err := workspace.Initialize(workspace.Options{WorkspacePath: workspacePath, DataRoot: root}); err != nil {
		t.Fatal(err)
	}

	const objective = "Secret contract body that must never enter Session Context."
	const summary = "Completed: Session A drafted delivery.md. Pending: recover it and finish the delivery."
	const nextStep = "Session B should recover delivery.md and finalize it."
	const artifactRef = "bcgos://workspace/delivery.md"
	const sessionADraft = "# Delivery\n\nDraft started in session A.\n"
	const sessionBFinal = "# Delivery\n\nCompleted after recovery in session B.\n"
	artifactPath := filepath.Join(workspacePath, "delivery.md")
	contract := `{
	  "objective": "` + objective + `",
	  "initial_next_step": "Start in session A.",
	  "criteria": [{"id": "delivery", "type": "artifact_snapshot", "target_ref": "` + artifactRef + `"}],
	  "allowed_refs": ["` + artifactRef + `"]
	}`

	var sessionA bytes.Buffer
	if code := runWork([]string{"create", "--workspace", workspacePath, "--stdin"}, strings.NewReader(contract), &sessionA, &sessionA, dataRoot); code != ExitOK {
		t.Fatalf("session A create exit = %d, output = %s", code, sessionA.String())
	}
	var created execution.MutationReceipt
	if err := json.Unmarshal(sessionA.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	sessionA.Reset()
	if code := runWork([]string{"start", "--workspace", workspacePath, "--item", created.ItemID, "--revision", strconv.Itoa(created.StateRevision)}, strings.NewReader(""), &sessionA, &sessionA, dataRoot); code != ExitOK {
		t.Fatalf("session A start exit = %d, output = %s", code, sessionA.String())
	}
	var started execution.MutationReceipt
	if err := json.Unmarshal(sessionA.Bytes(), &started); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(artifactPath, []byte(sessionADraft), 0o600); err != nil {
		t.Fatal(err)
	}
	checkpoint := `{"summary":"` + summary + `","next_step":"` + nextStep + `","artifact_refs":["` + artifactRef + `"]}`
	sessionA.Reset()
	if code := runWork([]string{"checkpoint", "--workspace", workspacePath, "--item", created.ItemID, "--revision", strconv.Itoa(started.StateRevision), "--attempt", started.AttemptID, "--stdin"}, strings.NewReader(checkpoint), &sessionA, &sessionA, dataRoot); code != ExitOK {
		t.Fatalf("session A checkpoint exit = %d, output = %s", code, sessionA.String())
	}
	var checkpointed execution.MutationReceipt
	if err := json.Unmarshal(sessionA.Bytes(), &checkpointed); err != nil {
		t.Fatal(err)
	}
	sessionA.Reset()
	if code := runWork([]string{"pause", "--workspace", workspacePath, "--item", created.ItemID, "--revision", strconv.Itoa(checkpointed.StateRevision), "--attempt", started.AttemptID}, strings.NewReader(""), &sessionA, &sessionA, dataRoot); code != ExitOK {
		t.Fatalf("session A pause exit = %d, output = %s", code, sessionA.String())
	}
	var paused execution.MutationReceipt
	if err := json.Unmarshal(sessionA.Bytes(), &paused); err != nil {
		t.Fatal(err)
	}

	envelopes := make(map[string]sessionstart.Envelope)
	for _, runtimeName := range []string{"claude", "codex"} {
		var sessionB bytes.Buffer
		if code := runSession([]string{"bridge", "--runtime", runtimeName, workspacePath}, &sessionB, &sessionB, dataRoot); code != ExitOK {
			t.Fatalf("session B %s bridge exit = %d, output = %s", runtimeName, code, sessionB.String())
		}
		var envelope sessionstart.Envelope
		if err := json.Unmarshal(sessionB.Bytes(), &envelope); err != nil {
			t.Fatal(err)
		}
		pointer := envelope.Packet.Execution.Active
		if pointer.Path != execution.ActivePointerPath || !pointer.Available || pointer.State != execution.ActivePointerAvailable {
			t.Fatalf("%s active pointer = %#v", runtimeName, pointer)
		}
		for _, prohibited := range []string{objective, summary, nextStep, created.ItemID, started.AttemptID} {
			if strings.Contains(sessionB.String(), prohibited) {
				t.Fatalf("%s Session Context leaked %q: %s", runtimeName, prohibited, sessionB.String())
			}
		}
		envelopes[runtimeName] = envelope
	}
	if !reflect.DeepEqual(envelopes["claude"].Packet, envelopes["codex"].Packet) {
		t.Fatalf("runtime packets differ: claude=%#v codex=%#v", envelopes["claude"].Packet, envelopes["codex"].Packet)
	}

	var sessionB bytes.Buffer
	if code := runWork([]string{"next", "--workspace", workspacePath, "--active"}, strings.NewReader(""), &sessionB, &sessionB, dataRoot); code != ExitOK {
		t.Fatalf("session B next exit = %d, output = %s", code, sessionB.String())
	}
	var next execution.NextProjection
	if err := json.Unmarshal(sessionB.Bytes(), &next); err != nil {
		t.Fatal(err)
	}
	if next.ItemID != created.ItemID || next.StateRevision != paused.StateRevision || next.Summary != summary || next.NextStep != nextStep {
		t.Fatalf("session B next = %#v", next)
	}
	if len(next.ArtifactRefs) != 1 || next.ArtifactRefs[0] != artifactRef {
		t.Fatalf("session B recovered artifact references = %#v", next.ArtifactRefs)
	}
	recoveredArtifactPath := filepath.Join(workspacePath, filepath.FromSlash(strings.TrimPrefix(next.ArtifactRefs[0], "bcgos://workspace/")))
	if artifact, err := os.ReadFile(recoveredArtifactPath); err != nil || string(artifact) != sessionADraft {
		t.Fatalf("session B did not recover session A artifact = %q, err = %v", artifact, err)
	}
	sessionB.Reset()
	if code := runWork([]string{"resume", "--workspace", workspacePath, "--item", next.ItemID, "--revision", strconv.Itoa(next.StateRevision)}, strings.NewReader(""), &sessionB, &sessionB, dataRoot); code != ExitOK {
		t.Fatalf("session B resume exit = %d, output = %s", code, sessionB.String())
	}
	var resumed execution.MutationReceipt
	if err := json.Unmarshal(sessionB.Bytes(), &resumed); err != nil {
		t.Fatal(err)
	}
	if resumed.AttemptID == "" || resumed.AttemptID == started.AttemptID {
		t.Fatalf("session B reused stale attempt: %#v", resumed)
	}
	sessionB.Reset()
	code := runWork([]string{"checkpoint", "--workspace", workspacePath, "--item", created.ItemID, "--revision", strconv.Itoa(resumed.StateRevision), "--attempt", started.AttemptID, "--stdin"}, strings.NewReader(checkpoint), &sessionB, &sessionB, dataRoot)
	if code == ExitOK || !strings.Contains(sessionB.String(), execution.ErrAttemptConflict.Error()) {
		t.Fatalf("stale session A writer exit = %d, output = %s", code, sessionB.String())
	}
	if err := os.WriteFile(recoveredArtifactPath, []byte(sessionBFinal), 0o600); err != nil {
		t.Fatal(err)
	}
	sessionB.Reset()
	if code := runWork([]string{"evidence", "--workspace", workspacePath, "--item", created.ItemID, "--revision", strconv.Itoa(resumed.StateRevision), "--attempt", resumed.AttemptID, "--criterion", "delivery"}, strings.NewReader(""), &sessionB, &sessionB, dataRoot); code != ExitOK || !strings.Contains(sessionB.String(), `"outcome": "passed"`) || strings.Contains(sessionB.String(), sessionBFinal) {
		t.Fatalf("session B evidence exit = %d, output = %s", code, sessionB.String())
	}
	var evidenced execution.EvidenceReceiptOutput
	if err := json.Unmarshal(sessionB.Bytes(), &evidenced); err != nil {
		t.Fatal(err)
	}
	sessionB.Reset()
	if code := runWork([]string{"complete", "--workspace", workspacePath, "--item", created.ItemID, "--revision", strconv.Itoa(evidenced.StateRevision), "--attempt", resumed.AttemptID}, strings.NewReader(""), &sessionB, &sessionB, dataRoot); code != ExitOK || !strings.Contains(sessionB.String(), `"state": "completed"`) {
		t.Fatalf("session B complete exit = %d, output = %s", code, sessionB.String())
	}
	assertExecutionMutationPrivate(t, sessionB.String())
	if artifact, err := os.ReadFile(recoveredArtifactPath); err != nil || string(artifact) != sessionBFinal {
		t.Fatalf("recovered artifact = %q, err = %v", artifact, err)
	}
}

func TestWorkCreateContractIsDiscoverableWithoutEngineeringReflection(t *testing.T) {
	var output bytes.Buffer
	dataRootCalled := false
	dataRoot := func() (string, error) {
		dataRootCalled = true
		return "", errors.New("schema inspection must not require local state")
	}
	if code := runWork([]string{"schema"}, strings.NewReader(""), &output, &output, dataRoot); code != ExitOK || dataRootCalled {
		t.Fatalf("work schema exit=%d dataRootCalled=%v output=%s", code, dataRootCalled, output.String())
	}
	for _, expected := range []string{"initial_next_step", "artifact_snapshot", "command_check", "bcgos://workspace/result.md", "go test", "allowed_refs"} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("work schema omitted %q: %s", expected, output.String())
		}
	}

	output.Reset()
	if code := runWork([]string{"create", "--help"}, strings.NewReader(""), &output, &output, dataRoot); code != ExitOK || dataRootCalled ||
		!strings.Contains(output.String(), "bcgos work schema") || !strings.Contains(output.String(), "artifact_snapshot") {
		t.Fatalf("work create help is not actionable: exit=%d dataRootCalled=%v output=%s", code, dataRootCalled, output.String())
	}
}

func assertExecutionMutationPrivate(t *testing.T, body string) {
	t.Helper()
	for _, prohibited := range []string{
		"objective", "criteria", "summary", "next_step", "blocker",
		"artifact_refs", "allowed_refs", "walter_reviews",
	} {
		if strings.Contains(body, `"`+prohibited+`"`) {
			t.Fatalf("mutation receipt leaked %q: %s", prohibited, body)
		}
	}
}

func TestInitPersistsTheSelectedInteractionProfile(t *testing.T) {
	root := t.TempDir()
	dataRoot := filepath.Join(root, "local", "BCGOS")
	var output bytes.Buffer
	if code := runInit([]string{"--profile", "power", filepath.Join(root, "workspace")}, &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK || !strings.Contains(output.String(), `"profile": "power"`) {
		t.Fatalf("init exit = %d, output = %s", code, output.String())
	}
	output.Reset()
	if code := runProfile([]string{"show"}, &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK || !strings.Contains(output.String(), `"profile": "power"`) || !strings.Contains(output.String(), `"source": "configured"`) {
		t.Fatalf("profile after init exit = %d, output = %s", code, output.String())
	}
}

func TestWorkspaceAgentCommandsCreateAndExposeTheGuidedInterview(t *testing.T) {
	root := t.TempDir()
	dataRoot := filepath.Join(root, "local", "BCGOS")
	workspacePath := filepath.Join(root, "workspace")
	var output bytes.Buffer
	if code := runInit([]string{workspacePath}, &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK || !strings.Contains(output.String(), `"case_agent"`) || !strings.Contains(output.String(), `"workspace-agent-`) || !strings.Contains(output.String(), `"identity_compatibility": "migration_compatibility"`) || !strings.Contains(output.String(), `"agent_stub"`) || !strings.Contains(output.String(), `"runtime_state": "unavailable"`) {
		t.Fatalf("init exit = %d, output = %s", code, output.String())
	}
	output.Reset()
	if code := runWorkspaceAgent([]string{"interview", workspacePath}, &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK || !strings.Contains(output.String(), `"case_agent_setup"`) || !strings.Contains(output.String(), `"decision_and_horizon"`) || !strings.Contains(output.String(), `"handoff"`) {
		t.Fatalf("workspace-agent interview exit = %d, output = %s", code, output.String())
	}
}

func TestAgentIdentityInterviewAndPersonalizationAreExplicit(t *testing.T) {
	root := t.TempDir()
	dataRoot := filepath.Join(root, "local", "BCGOS")
	var output bytes.Buffer
	if code := runAgentWithInput([]string{"interview"}, strings.NewReader(""), &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK ||
		!strings.Contains(output.String(), `"agent_identity_setup"`) ||
		!strings.Contains(output.String(), `"ownership_explanation"`) ||
		!strings.Contains(output.String(), `"default_emoji"`) ||
		!strings.Contains(output.String(), `"client_account_agent"`) {
		t.Fatalf("identity interview = %d, output = %s", code, output.String())
	}
	profile := `{"schema_version":1,"owner_id":"daniel","confirmed":true,"updated_at":"2026-07-28T00:00:00Z","capability_tracks":["software-engineering"],"selections":[{"role":"maestro","agent_id":"maestro","display_name":"Maestro","emoji":"🎼","owner_id":"daniel","ownership_scope":"system"},{"role":"client_account_agent","agent_id":"client-account-agent-acme","display_name":"Compass","emoji":"🧭","owner_id":"daniel","ownership_scope":"account"},{"role":"case_agent","agent_id":"case-agent-pricing","display_name":"Forge","emoji":"⚙️","owner_id":"daniel","ownership_scope":"case"}]}`
	output.Reset()
	if code := draftAndConfirmAgentProfile(t, dataRoot, profile, &output); code != ExitOK || !strings.Contains(output.String(), `"state": "applied"`) {
		t.Fatalf("identity personalize = %d, output = %s", code, output.String())
	}
	output.Reset()
	if code := runAgentWithInput([]string{"identity"}, strings.NewReader(""), &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK || !strings.Contains(output.String(), `"ownership_scope": "case"`) {
		t.Fatalf("identity status = %d, output = %s", code, output.String())
	}
}

func TestAgentIdentityFullRosterUsesCanonicalProfileEnvelope(t *testing.T) {
	root := t.TempDir()
	dataRoot := filepath.Join(root, "local", "BCGOS")
	var output bytes.Buffer
	profiles := []string{
		`{"schema_version":1,"owner_id":"rafa-menezes","confirmed":true,"capability_tracks":["technical-explorer","software-engineering","data-science","data-engineering","ai-engineering"],"selections":[{"role":"maestro","display_name":"Maestro","emoji":"🎼","owner_id":"rafa-menezes","ownership_scope":"system"},{"role":"client_account_agent","agent_id":"client-account-agent-rafa-menezes","display_name":"Account Partner","emoji":"🤝","owner_id":"rafa-menezes","ownership_scope":"account"},{"role":"case_agent","agent_id":"case-agent-rafa-menezes","display_name":"Case Lead","emoji":"⚙️","owner_id":"rafa-menezes","ownership_scope":"case"},{"role":"quality_guardian","display_name":"Gamma Guardian","emoji":"🧪","owner_id":"rafa-menezes","ownership_scope":"quality_longitudinal"},{"role":"pa_expert","display_name":"PA Expert","emoji":"🧠","owner_id":"rafa-menezes","ownership_scope":"pa_expert_registry"}]}`,
		`{"schema_version":1,"owner_id":"rafa-menezes","confirmed":true,"capability_tracks":["technical-explorer","software-engineering","data-science","data-engineering","ai-engineering"],"selections":[{"role":"maestro","display_name":"Maestro","emoji":"🎼","owner_id":"rafa-menezes","ownership_scope":"system"},{"role":"client_account_agent","agent_id":"client-account-agent-rafa-menezes","display_name":"Account Partner","emoji":"🤝","owner_id":"rafa-menezes","ownership_scope":"account"},{"role":"case_agent","agent_id":"case-agent-rafa-menezes","display_name":"Case Lead","emoji":"⚙️","owner_id":"rafa-menezes","ownership_scope":"case"},{"role":"walter","display_name":"Walter","emoji":"🦉","owner_id":"rafa-menezes","ownership_scope":"governance"},{"role":"quality_guardian","display_name":"Gamma Guardian","emoji":"🧪","owner_id":"rafa-menezes","ownership_scope":"quality_longitudinal"},{"role":"pa_expert","display_name":"PA Expert","emoji":"🧠","owner_id":"rafa-menezes","ownership_scope":"pa_expert_registry"}]}`,
		`{"schema_version":1,"owner_id":"rafa-menezes","confirmed":true,"capability_tracks":["technical-explorer","software-engineering","data-science","data-engineering","ai-engineering"],"selections":[{"role":"maestro","display_name":"Maestro","emoji":"🎼","owner_id":"rafa-menezes","ownership_scope":"system"},{"role":"client_account_agent","agent_id":"client-account-agent-rafa-menezes","display_name":"Account Partner","emoji":"🤝","owner_id":"rafa-menezes","ownership_scope":"account"},{"role":"case_agent","agent_id":"case-agent-rafa-menezes","display_name":"Case Lead","emoji":"⚙️","owner_id":"rafa-menezes","ownership_scope":"case"},{"role":"walter","display_name":"Walter","emoji":"🦉","owner_id":"rafa-menezes","ownership_scope":"governance"},{"role":"darwin","display_name":"Darwin","emoji":"🧬","owner_id":"rafa-menezes","ownership_scope":"governance"},{"role":"quality_guardian","display_name":"Gamma Guardian","emoji":"🧪","owner_id":"rafa-menezes","ownership_scope":"quality_longitudinal"},{"role":"pa_expert","display_name":"PA Expert","emoji":"🧠","owner_id":"rafa-menezes","ownership_scope":"pa_expert_registry"}]}`,
	}
	for index, profile := range profiles {
		if code := draftAndConfirmAgentProfile(t, dataRoot, profile, &output); code != ExitOK ||
			!strings.Contains(output.String(), `"state": "applied"`) {
			t.Fatalf("identity roster step %d = %d, output = %s", index+1, code, output.String())
		}
	}
	output.Reset()
	if code := runAgentWithInput([]string{"identity"}, strings.NewReader(""), &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK ||
		!strings.Contains(output.String(), `"role": "quality_guardian"`) ||
		!strings.Contains(output.String(), `"display_name": "Account Partner"`) {
		t.Fatalf("full identity status = %d, output = %s", code, output.String())
	}
}

func TestInterviewSelectionActivatesEngineeringProjection(t *testing.T) {
	root := t.TempDir()
	dataRoot := filepath.Join(root, "local", "BCGOS")
	workspacePath := filepath.Join(root, "workspace")
	if err := os.MkdirAll(workspacePath, 0o755); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	profile := `{"schema_version":1,"owner_id":"daniel","confirmed":true,"capability_tracks":["software-engineering"],"selections":[{"role":"maestro","display_name":"Maestro","emoji":"🎼","owner_id":"daniel","ownership_scope":"system"}]}`
	if code := draftAndConfirmAgentProfile(t, dataRoot, profile, &output); code != ExitOK {
		t.Fatalf("personalize = %d %s", code, output.String())
	}
	output.Reset()
	if code := runAdapterWithDataRoot([]string{"install", "--runtime", "codex", workspacePath}, &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK || !strings.Contains(output.String(), `"skill_count": 31`) {
		t.Fatalf("optional adapter install = %d %s", code, output.String())
	}
	for _, skillID := range []string{"maestro-onboarding", "review-explain-change", "spec-driven-delivery", "test-and-evidence"} {
		if _, err := os.Stat(filepath.Join(workspacePath, ".codex", "skills", skillID, "SKILL.md")); err != nil {
			t.Fatalf("engineering skill %s was not projected: %v", skillID, err)
		}
	}
}

func TestInterviewSelectionActivatesDataProjection(t *testing.T) {
	root := t.TempDir()
	dataRoot := filepath.Join(t.TempDir(), "local", "BCGOS")
	workspacePath := filepath.Join(root, "workspace")
	if err := os.MkdirAll(workspacePath, 0o755); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	profile := `{"schema_version":1,"owner_id":"daniel","confirmed":true,"capability_tracks":["data-science"],"selections":[{"role":"maestro","display_name":"Maestro","emoji":"🎼","owner_id":"daniel","ownership_scope":"system"}]}`
	if code := draftAndConfirmAgentProfile(t, dataRoot, profile, &output); code != ExitOK {
		t.Fatalf("data personalize = %d %s", code, output.String())
	}
	output.Reset()
	if code := runAdapterWithDataRoot([]string{"install", "--runtime", "codex", workspacePath}, &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK || !strings.Contains(output.String(), `"skill_count": 31`) {
		t.Fatalf("data adapter install = %d %s", code, output.String())
	}
	for _, skillID := range []string{"maestro-onboarding", "review-explain-change", "spec-driven-delivery", "test-and-evidence", "data-pipeline-quality", "data-science-evaluation", "reproducible-data-run"} {
		if _, err := os.Stat(filepath.Join(workspacePath, ".codex", "skills", skillID, "SKILL.md")); err != nil {
			t.Fatalf("data selection did not project all skills; missing %s: %v", skillID, err)
		}
	}
}

func draftAndConfirmAgentProfile(t *testing.T, dataRoot, profile string, output *bytes.Buffer) int {
	t.Helper()
	output.Reset()
	code := runAgentWithInput([]string{"personalize", "draft", "--stdin", "--consent", "--no-client-data"}, strings.NewReader(profile), output, output, func() (string, error) { return dataRoot, nil })
	if code != ExitOK {
		return code
	}
	var draft agentidentity.ProfileDraft
	if err := json.Unmarshal(output.Bytes(), &draft); err != nil {
		t.Fatalf("decode identity draft: %v (%s)", err, output.String())
	}
	output.Reset()
	return runAgentWithInput([]string{"personalize", "confirm", "--id", draft.ID, "--digest", draft.ReviewDigest, "--confirm"}, strings.NewReader(""), output, output, func() (string, error) { return dataRoot, nil })
}

func TestDarwinHeadlessHousekeepingUsesTheScopedAgentContract(t *testing.T) {
	dataRoot := filepath.Join(t.TempDir(), "local", "BCGOS")
	t.Setenv("BCGOS_MAESTRO_CAPABILITY", "maestro-test-cap")
	t.Setenv("BCGOS_DARWIN_CAPABILITY", "darwin-test-cap")
	t.Setenv("BCGOS_RECOVERY_CAPABILITY", "recovery-test-cap")
	packet := `{"schema_version":1,"window_id":"cli-window","runtime":"claude","mode":"interactive","observations":[{"code":"state_stale","severity":"low","count":1}]}`
	var output bytes.Buffer
	code := runAgentWithInput([]string{"darwin", "housekeeping", "--stdin"}, strings.NewReader(packet), &output, &output, func() (string, error) { return dataRoot, nil })
	if code != ExitOK || !strings.Contains(output.String(), `"agent_id": "darwin"`) || !strings.Contains(output.String(), `"mode": "headless_housekeeping"`) || !strings.Contains(output.String(), `"emoji": "🧬"`) {
		t.Fatalf("exit=%d output=%s", code, output.String())
	}
	receipts, err := os.ReadDir(filepath.Join(dataRoot, "darwin", "receipts"))
	if err != nil || len(receipts) != 1 {
		t.Fatalf("receipts=%v err=%v", receipts, err)
	}
}

func TestDarwinHeadlessHousekeepingRequiresExplicitCapabilities(t *testing.T) {
	t.Setenv("BCGOS_MAESTRO_CAPABILITY", "")
	t.Setenv("BCGOS_DARWIN_CAPABILITY", "")
	t.Setenv("BCGOS_RECOVERY_CAPABILITY", "")
	var output bytes.Buffer
	code := runAgentWithInput([]string{"darwin", "housekeeping", "--stdin"}, strings.NewReader(`{}`), &output, &output, func() (string, error) { return t.TempDir(), nil })
	if code == ExitOK || !strings.Contains(output.String(), "requires BCGOS_MAESTRO_CAPABILITY") {
		t.Fatalf("exit=%d output=%s", code, output.String())
	}
}

func TestAgentScaffoldCommandRejectsRetiredWorkspaceChildRole(t *testing.T) {
	root := t.TempDir()
	dataRoot := filepath.Join(root, "local", "BCGOS")
	workspacePath := filepath.Join(root, "workspace")
	var output bytes.Buffer
	if code := runInit([]string{workspacePath}, &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK {
		t.Fatalf("init exit = %d, output = %s", code, output.String())
	}
	inspection, err := workspace.Inspect(workspacePath, dataRoot)
	if err != nil {
		t.Fatal(err)
	}
	parent := "workspace-agent-" + inspection.WorkspaceID
	output.Reset()
	code := runAgent([]string{
		"scaffold",
		"--id", "retired-research",
		"--role", "retired_specialist_role",
		"--scope-kind", "workspace",
		"--scope", inspection.WorkspaceID,
		"--parent", parent,
		"--parent-role", "workspace_agent",
	}, &output, &output, func() (string, error) { return dataRoot, nil })
	if code == ExitOK {
		t.Fatalf("retired workspace child role was accepted: output=%s", output.String())
	}
}

func TestAgentScaffoldCommandRejectsRetiredPracticeChain(t *testing.T) {
	root := t.TempDir()
	dataRoot := filepath.Join(root, "local", "BCGOS")
	var output bytes.Buffer
	code := runAgent([]string{
		"scaffold",
		"--id", "practice-agent-insurance",
		"--role", "practice_agent",
		"--scope-kind", "practice",
		"--scope", "insurance",
		"--parent", "maestro",
		"--parent-role", "hub",
	}, &output, &output, func() (string, error) { return dataRoot, nil })
	if code == ExitOK || !strings.Contains(strings.ToLower(output.String()), "deprecated") {
		t.Fatalf("retired practice scaffold exit = %d, output = %s", code, output.String())
	}
}

func TestAgentHirePlanDeclassifyAndVerifyCommands(t *testing.T) {
	root := t.TempDir()
	dataRoot := filepath.Join(root, "local", "BCGOS")
	canonRelative := filepath.Join("pa-experts", "pa-expert-fpa-pricing", "canon.md")
	canonPath := filepath.Join(dataRoot, canonRelative)
	if err := os.MkdirAll(filepath.Dir(canonPath), 0o700); err != nil {
		t.Fatal(err)
	}
	canon := []byte("# Pricing PA expert canon\n")
	if err := os.WriteFile(canonPath, canon, 0o600); err != nil {
		t.Fatal(err)
	}
	canonDigest := sha256.Sum256(canon)
	canonSHA256 := hex.EncodeToString(canonDigest[:])
	var output bytes.Buffer
	code := runAgent([]string{
		"hire",
		"--id", "pa-expert-fpa-pricing",
		"--role", "pa_expert",
		"--scope-kind", "practice",
		"--scope", "pricing",
		"--parent", "maestro",
		"--parent-role", "hub",
		"--owner", "pa-expert-curator",
		"--mandate", "Advise with the governed pricing canon.",
		"--canon", filepath.ToSlash(canonRelative),
		"--canon-sha256", canonSHA256,
		"--expert-kind", "FPA",
		"--expert-version", "1.0.0",
	}, &output, &output, func() (string, error) { return dataRoot, nil })
	if code != ExitOK || !strings.Contains(output.String(), `"expert_kind": "FPA"`) ||
		!strings.Contains(output.String(), `"runtime_state": "unavailable"`) ||
		!strings.Contains(output.String(), `"expert_lifecycle": "draft"`) {
		t.Fatalf("PA expert hire exit = %d, output = %s", code, output.String())
	}

	planInput := `{"envelope":{"schema_version":1,"episode_id":"episode-cli","owner":"case_agent","posture":"balanced","consequence":"medium","reversibility":"reversible","sensitivity":"internal","knowledge_need":"none"}}`
	output.Reset()
	code = runAgentWithInput([]string{"plan", "--stdin"}, strings.NewReader(planInput), &output, &output, func() (string, error) { return dataRoot, nil })
	var plan activationpolicy.RoutePlan
	if code != ExitOK || json.Unmarshal(output.Bytes(), &plan) != nil ||
		plan.Route != activationpolicy.D1Targeted || len(plan.Experts) != 0 ||
		!plan.RequiresAssurance || plan.MayAuthorizeDispatch {
		t.Fatalf("activation plan exit = %d, output = %s", code, output.String())
	}

	request := activationpolicy.AdvisoryRequest{
		SchemaVersion: 1, RequestID: activationpolicy.OpaqueAdvisoryRequestID("advisory-cli"),
		EpisodeSHA256: activationpolicy.SHA256Hex([]byte(plan.EpisodeID)),
		PlanSHA256:    plan.PlanSHA256,
		Expert: activationpolicy.PAExpert{
			ID: "pa-expert-fpa-pricing", Kind: activationpolicy.ExpertFPA,
			Version: "1.0.0", CanonSHA256: canonSHA256,
			Lifecycle: activationpolicy.Published,
		},
		QuestionCode: "pricing-strategy", Classification: activationpolicy.Internal,
		Facts: []activationpolicy.AdvisoryFact{{
			Code: "market-signal", Classification: activationpolicy.Internal,
			ValueCode: "abstracted-market-signal",
		}},
		OutputSections: []string{"findings", "challenges"},
		Attestation: activationpolicy.DeclassificationAttestation{
			ExporterID: "maestro", NoClientIdentifiers: true,
			NoStakeholderIdentifiers: true, NoRawExcerpts: true, NoScopedPointers: true,
		},
	}
	requestBody, _ := json.Marshal(activationAdvisoryInput{
		Envelope: activationpolicy.IntentEnvelope{
			SchemaVersion: 1, EpisodeID: "episode-cli", Owner: activationpolicy.OwnerCase,
			Posture: activationpolicy.Balanced, Consequence: activationpolicy.Medium,
			Reversibility: activationpolicy.Reversible, Sensitivity: activationpolicy.Internal,
			KnowledgeNeed: activationpolicy.Functional,
		},
		Request: request,
	})
	output.Reset()
	code = runAgentWithInput([]string{"declassify", "--stdin"}, bytes.NewReader(requestBody), &output, &output, func() (string, error) { return dataRoot, nil })
	if code == ExitOK {
		t.Fatal("draft PA expert was treated as published")
	}

	completion := activationCompletionInput{
		Envelope: activationpolicy.IntentEnvelope{
			SchemaVersion: 1, EpisodeID: "episode-cli", Owner: activationpolicy.OwnerCase,
			Posture: activationpolicy.Balanced, Consequence: activationpolicy.Medium,
			Reversibility: activationpolicy.Reversible, Sensitivity: activationpolicy.Internal,
			KnowledgeNeed: activationpolicy.None,
		},
		Plan: plan,
		Receipts: []activationpolicy.CompletionReceipt{
			{SchemaVersion: 1, EpisodeID: plan.EpisodeID, PlanSHA256: plan.PlanSHA256, Kind: activationpolicy.OwnerReceipt, ActorID: "case_agent", EvidenceAuthority: "unverified_breadcrumb"},
			{SchemaVersion: 1, EpisodeID: plan.EpisodeID, PlanSHA256: plan.PlanSHA256, Kind: activationpolicy.AssuranceReceipt, ActorID: "walter", EvidenceAuthority: "unverified_breadcrumb"},
		},
	}
	completionBody, _ := json.Marshal(completion)
	output.Reset()
	code = runAgentWithInput([]string{"verify", "--stdin"}, bytes.NewReader(completionBody), &output, &output, func() (string, error) { return dataRoot, nil })
	if code != ExitOK || !strings.Contains(output.String(), `"state": "shadow_evaluated"`) ||
		!strings.Contains(output.String(), `"may_complete_execution": false`) {
		t.Fatalf("verify exit = %d, output = %s", code, output.String())
	}

	tampered := completion
	tampered.Plan.Route = activationpolicy.D0Direct
	tampered.Plan.Experts = nil
	tampered.Plan.RequiresAssurance = false
	tampered.Plan.AssuranceAgentID = ""
	tampered.Plan.PlanSHA256 = activationpolicy.PlanDigest(tampered.Plan)
	tampered.Receipts = []activationpolicy.CompletionReceipt{{
		SchemaVersion: 1, EpisodeID: tampered.Plan.EpisodeID,
		PlanSHA256: tampered.Plan.PlanSHA256,
		Kind:       activationpolicy.OwnerReceipt, ActorID: "case_agent",
		EvidenceAuthority: "unverified_breadcrumb",
	}}
	tamperedBody, _ := json.Marshal(tampered)
	output.Reset()
	code = runAgentWithInput([]string{"verify", "--stdin"}, bytes.NewReader(tamperedBody), &output, &output, func() (string, error) { return dataRoot, nil })
	if code == ExitOK {
		t.Fatal("rehashed caller-tampered plan bypassed deterministic recomputation")
	}
}

func TestWorkspaceAgentResearchAndEconomicCommandsPersistGovernedArtifacts(t *testing.T) {
	root := t.TempDir()
	dataRoot := filepath.Join(root, "local", "BCGOS")
	workspacePath := filepath.Join(root, "workspace")
	var output bytes.Buffer
	if code := runInit([]string{workspacePath}, &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK {
		t.Fatalf("init exit = %d, output = %s", code, output.String())
	}
	output.Reset()
	briefInput := `{"reviewed_by":"owner","classification":"confidential","mandate":"support a decision","objectives":["recommendation"],"stakeholders":["sponsor"],"constraints":["four weeks"],"bullish":[{"statement":"upside","evidence":["public signal"],"assumptions":["adoption grows"],"counter_evidence":["weak conversion"],"invalidation_signals":["demand declines"]}],"bearish":[{"statement":"downside","evidence":["public risk"],"assumptions":["cost stays high"],"counter_evidence":["efficiency improves"],"invalidation_signals":["cost falls"]}],"research_questions":["public market size"]}`
	code := runWorkspaceAgentWithInput([]string{"brief", "submit", "--stdin", workspacePath}, strings.NewReader(briefInput), &output, &output, func() (string, error) { return dataRoot, nil })
	if code != ExitOK || !strings.Contains(output.String(), `"brief_id"`) {
		t.Fatalf("brief submit exit = %d, output = %s", code, output.String())
	}

	output.Reset()
	planInput := `{"purpose":"public market context","valid_until":"2027-07-25T12:00:00Z","max_queries":1,"query_themes":["market size"],"sources":["ibge.gov.br"]}`
	code = runWorkspaceAgentWithInput([]string{"research", "plan", "--stdin", workspacePath}, strings.NewReader(planInput), &output, &output, func() (string, error) { return dataRoot, nil })
	var plan struct {
		PlanID string `json:"plan_id"`
	}
	if code != ExitOK || json.Unmarshal(output.Bytes(), &plan) != nil || plan.PlanID == "" {
		t.Fatalf("research plan exit = %d, output = %s", code, output.String())
	}

	output.Reset()
	code = runWorkspaceAgentWithInput([]string{"research", "approve", "--plan", plan.PlanID, "--approved-by", "owner", "--confirm", workspacePath}, strings.NewReader(""), &output, &output, func() (string, error) { return dataRoot, nil })
	if code != ExitOK || !strings.Contains(output.String(), `"state": "approved"`) {
		t.Fatalf("research approve exit = %d, output = %s", code, output.String())
	}

	output.Reset()
	queryInput := `{"plan_id":"` + plan.PlanID + `","query":"market size"}`
	code = runWorkspaceAgentWithInput([]string{"research", "query", "--stdin", workspacePath}, strings.NewReader(queryInput), &output, &output, func() (string, error) { return dataRoot, nil })
	if code != ExitOK || !strings.Contains(output.String(), `"slot": 1`) {
		t.Fatalf("research query exit = %d, output = %s", code, output.String())
	}

	output.Reset()
	evidenceInput := `{"plan_id":"` + plan.PlanID + `","query":"market size","source_url":"https://www.ibge.gov.br/example","retrieved_at":"2026-07-25T12:00:00Z","valid_until":"2027-07-25T12:00:00Z","claim":"Public fact","evidence_strength":"primary","classification":"public"}`
	code = runWorkspaceAgentWithInput([]string{"research", "record", "--stdin", workspacePath}, strings.NewReader(evidenceInput), &output, &output, func() (string, error) { return dataRoot, nil })
	if code != ExitOK || !strings.Contains(output.String(), `"state": "recorded"`) {
		t.Fatalf("research record exit = %d, output = %s", code, output.String())
	}

	output.Reset()
	snapshotInput := `{"as_of":"2026-07-25T12:00:00Z","claims":[{"statement":"Public macro claim","classification":"public","source_urls":["https://www.bcb.gov.br/example"]}],"sources":[{"url":"https://www.bcb.gov.br/example","retrieved_at":"2026-07-25T12:00:00Z"}],"attestation":{}}`
	code = runWorkspaceAgentWithInput([]string{"economic", "import", "--stdin", "--attested-public", "--attested-by", "owner", "--confirm-no-workspace-derivation"}, strings.NewReader(snapshotInput), &output, &output, func() (string, error) { return dataRoot, nil })
	var snapshot struct {
		SnapshotID string `json:"snapshot_id"`
	}
	if code != ExitOK || json.Unmarshal(output.Bytes(), &snapshot) != nil || snapshot.SnapshotID == "" {
		t.Fatalf("economic import exit = %d, output = %s", code, output.String())
	}
	output.Reset()
	code = runWorkspaceAgentWithInput([]string{"economic", "attach", "--snapshot", snapshot.SnapshotID, workspacePath}, strings.NewReader(""), &output, &output, func() (string, error) { return dataRoot, nil })
	if code != ExitOK || !strings.Contains(output.String(), `"state": "attached"`) {
		t.Fatalf("economic attach exit = %d, output = %s", code, output.String())
	}
}

func TestWorkspaceAgentFirstValueCommandsProduceResumableArtifact(t *testing.T) {
	root := t.TempDir()
	dataRoot := filepath.Join(root, "local", "BCGOS")
	workspacePath := filepath.Join(root, "workspace")
	var output bytes.Buffer
	if code := runInit([]string{workspacePath}, &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK {
		t.Fatalf("init exit = %d, output = %s", code, output.String())
	}

	output.Reset()
	code := runWorkspaceAgentWithInput([]string{"value", "start", workspacePath}, strings.NewReader(""), &output, &output, func() (string, error) { return dataRoot, nil })
	var started struct {
		RunID string `json:"run_id"`
	}
	if code != ExitOK || json.Unmarshal(output.Bytes(), &started) != nil || started.RunID == "" {
		t.Fatalf("start exit = %d, output = %s", code, output.String())
	}

	input := `{"brief":{"reviewed_by":"owner","classification":"internal","mandate":"support a pilot decision","decision":"choose the scope","time_horizon":"two weeks","objectives":["recommend a scope"],"stakeholders":["sponsor"],"materials":["approved notes"],"constraints":["no external research"],"success_signals":["sponsor can decide"],"open_questions":["who owns delivery"],"bullish":[{"statement":"upside","evidence":["signal"],"assumptions":["adoption"],"counter_evidence":["risk"],"invalidation_signals":["decline"]}],"bearish":[{"statement":"downside","evidence":["risk"],"assumptions":["cost"],"counter_evidence":["efficiency"],"invalidation_signals":["cost falls"]}]},"plan":[{"outcome":"confirm scope","owner":"sponsor","completion_criterion":"scope recorded"}],"artifact_title":"Pilot decision brief","next_step":"Review scope with sponsor","next_owner":"project lead"}`
	output.Reset()
	code = runWorkspaceAgentWithInput([]string{"value", "submit", "--run", started.RunID, "--stdin", workspacePath}, strings.NewReader(input), &output, &output, func() (string, error) { return dataRoot, nil })
	var receipt struct {
		Artifact struct {
			Path string `json:"path"`
		} `json:"artifact"`
	}
	if code != ExitOK || json.Unmarshal(output.Bytes(), &receipt) != nil || !strings.HasPrefix(receipt.Artifact.Path, filepath.Join(workspacePath, "brain", "deliverables")+string(os.PathSeparator)) {
		t.Fatalf("submit exit = %d, receipt = %#v, output = %s", code, receipt, output.String())
	}
	if _, err := os.Stat(receipt.Artifact.Path); err != nil {
		t.Fatalf("governed artifact: %v", err)
	}

	output.Reset()
	code = runWorkspaceAgentWithInput([]string{"value", "status", workspacePath}, strings.NewReader(""), &output, &output, func() (string, error) { return dataRoot, nil })
	if code != ExitOK || !strings.Contains(output.String(), `"next_step": "Review scope with sponsor"`) {
		t.Fatalf("status exit = %d, output = %s", code, output.String())
	}

	output.Reset()
	code = runWorkspaceAgentWithInput([]string{"value", "start", workspacePath}, strings.NewReader(""), &output, &output, func() (string, error) { return dataRoot, nil })
	var incompleteRun struct {
		RunID string `json:"run_id"`
	}
	if code != ExitOK || json.Unmarshal(output.Bytes(), &incompleteRun) != nil || incompleteRun.RunID == "" {
		t.Fatalf("second start exit = %d, output = %s", code, output.String())
	}
	invalidInput := strings.Replace(input, `"constraints":["no external research"]`, `"constraints":[]`, 1)
	output.Reset()
	code = runWorkspaceAgentWithInput([]string{"value", "submit", "--run", incompleteRun.RunID, "--stdin", workspacePath}, strings.NewReader(invalidInput), &output, &output, func() (string, error) { return dataRoot, nil })
	if code != ExitFailure || !strings.Contains(output.String(), "first-value brief is incomplete") {
		t.Fatalf("incomplete submit exit = %d, output = %s", code, output.String())
	}
}

func TestInitPersistsStandardWhenNoProfileIsSelected(t *testing.T) {
	root := t.TempDir()
	dataRoot := filepath.Join(root, "local", "BCGOS")
	var output bytes.Buffer
	if code := runInit([]string{filepath.Join(root, "workspace")}, &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK {
		t.Fatalf("init exit = %d, output = %s", code, output.String())
	}
	output.Reset()
	if code := runProfile([]string{"show"}, &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK || !strings.Contains(output.String(), `"profile": "standard"`) || !strings.Contains(output.String(), `"source": "configured"`) {
		t.Fatalf("profile after default init exit = %d, output = %s", code, output.String())
	}
}

func TestProductStatusAndDoctorDescribeReadyWorkspace(t *testing.T) {
	root := t.TempDir()
	workspacePath := filepath.Join(root, "Developer", "case-a")
	dataRoot := filepath.Join(root, "local", "BCGOS")
	var output bytes.Buffer
	if code := runInit([]string{workspacePath}, &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK {
		t.Fatalf("init exit = %d, output = %s", code, output.String())
	}

	output.Reset()
	if code := runProductStatus([]string{workspacePath}, &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK || !strings.Contains(output.String(), `"state": "ready"`) || !strings.Contains(output.String(), `"brain_readable": true`) || !strings.Contains(output.String(), `"profile": "standard"`) || !strings.Contains(output.String(), `"continuous_use"`) || !strings.Contains(output.String(), `"configured"`) || !strings.Contains(output.String(), `"adapter_observed"`) || !strings.Contains(output.String(), `"native_qualified"`) || !strings.Contains(output.String(), `"unavailable"`) {
		t.Fatalf("status exit = %d, output = %s", code, output.String())
	}
	var initialized struct {
		WorkspaceID string `json:"workspace_id"`
	}
	manifestBody, err := os.ReadFile(filepath.Join(workspacePath, ".bcgos", "workspace.json"))
	if err != nil || json.Unmarshal(manifestBody, &initialized) != nil || initialized.WorkspaceID == "" {
		t.Fatalf("workspace ID unavailable: body=%s err=%v", manifestBody, err)
	}
	output.Reset()
	if code := runProductStatus([]string{initialized.WorkspaceID}, &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK || !strings.Contains(output.String(), `"state": "ready"`) || !strings.Contains(output.String(), workspacePath) {
		t.Fatalf("status by workspace ID exit = %d, output = %s", code, output.String())
	}
	output.Reset()
	if code := runDoctor([]string{initialized.WorkspaceID}, &output, &output, func() (string, error) { return dataRoot, nil }, func(string) bool { return false }); code != ExitOK || !strings.Contains(output.String(), `"state": "ready"`) {
		t.Fatalf("doctor by workspace ID exit = %d, output = %s", code, output.String())
	}

	output.Reset()
	if code := runMaestroWithInput([]string{"status", initialized.WorkspaceID}, strings.NewReader(""), &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK || !strings.Contains(output.String(), `"calibration"`) || !strings.Contains(output.String(), `"complete_calibration"`) || strings.Contains(output.String(), "professional-role.md") {
		t.Fatalf("Maestro status exit = %d, output = %s", code, output.String())
	}

	output.Reset()
	available := func(name string) bool { return name == "claude" }
	if code := runDoctor([]string{workspacePath}, &output, &output, func() (string, error) { return dataRoot, nil }, available); code != ExitOK || !strings.Contains(output.String(), `"claude_code"`) || !strings.Contains(output.String(), `"runtime_capabilities"`) || !strings.Contains(output.String(), `"context_inject"`) || !strings.Contains(output.String(), `"interaction_profile"`) || !strings.Contains(output.String(), `"codex"`) || !strings.Contains(output.String(), `"unavailable"`) {
		t.Fatalf("doctor exit = %d, output = %s", code, output.String())
	}
}

func TestDoctorExplainsUninitializedWorkspace(t *testing.T) {
	root := t.TempDir()
	var output bytes.Buffer
	code := runDoctor([]string{filepath.Join(root, "not-initialized")}, &output, &output, func() (string, error) { return filepath.Join(root, "local", "BCGOS"), nil }, func(string) bool { return false })
	if code != ExitOK || !strings.Contains(output.String(), `"state": "action_required"`) || !strings.Contains(output.String(), "bcgos init") {
		t.Fatalf("doctor exit = %d, output = %s", code, output.String())
	}
}

func TestDoctorExplainsMissingRuntimeDependencies(t *testing.T) {
	root := t.TempDir()
	dataRoot := filepath.Join(root, "local", "BCGOS")
	workspacePath := filepath.Join(root, "workspace")
	var output bytes.Buffer
	if code := runInit([]string{workspacePath}, &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK {
		t.Fatal(output.String())
	}
	if err := os.Remove(filepath.Join(workspacePath, ".bcgos", "maestro-orchestration-state.json")); err != nil {
		t.Fatal(err)
	}
	output.Reset()
	if code := runDoctor([]string{workspacePath}, &output, &output, func() (string, error) { return dataRoot, nil }, func(string) bool { return true }); code != ExitOK ||
		!strings.Contains(output.String(), `"id": "runtime_dependencies"`) || !strings.Contains(output.String(), `"state": "action_required"`) || !strings.Contains(output.String(), "bcgos init") {
		t.Fatalf("doctor did not expose missing dependencies: exit=%d output=%s", code, output.String())
	}
}

func TestFederationEnrollmentIsOneTimeAndRevocable(t *testing.T) {
	dataRoot := t.TempDir()
	root := func() (string, error) { return dataRoot, nil }
	var output bytes.Buffer
	if code := runFederation([]string{"enroll", "--accept-federated-improvement-contract", "--bridge-endpoint", "https://bridge.maestro.example/federation/v1/batches"}, &output, &output, root); code != ExitOK {
		t.Fatalf("enroll exit = %d, output = %s", code, output.String())
	}
	if !strings.Contains(output.String(), `"automatic_export": true`) || strings.Contains(output.String(), "installation_id") {
		t.Fatalf("enroll output = %s", output.String())
	}
	output.Reset()
	if code := runFederation([]string{"status"}, &output, &output, root); code != ExitOK || !strings.Contains(output.String(), `"state": "enrolled"`) {
		t.Fatalf("status exit = %d, output = %s", code, output.String())
	}
	output.Reset()
	if code := runFederation([]string{"revoke"}, &output, &output, root); code != ExitOK || !strings.Contains(output.String(), `"state": "revoked"`) {
		t.Fatalf("revoke exit = %d, output = %s", code, output.String())
	}
}

func TestFederationEnrollmentRequiresExplicitContractAcceptance(t *testing.T) {
	var output bytes.Buffer
	code := runFederation([]string{"enroll", "--bridge-endpoint", "https://bridge.maestro.example/federation/v1/batches"}, &output, &output, func() (string, error) { return t.TempDir(), nil })
	if code != ExitUsage || !strings.Contains(output.String(), "--accept-federated-improvement-contract") {
		t.Fatalf("exit = %d, output = %s", code, output.String())
	}
}
