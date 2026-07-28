package cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/activationpolicy"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/adaptercfg"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/canary"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/execution"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/sessionstart"
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
	if code != 0 || !strings.Contains(output.String(), `"state": "captured"`) || !strings.Contains(output.String(), `"dreaming": "unavailable"`) {
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

func TestMemoryDreamReportsUnavailableWithoutAdapter(t *testing.T) {
	var output bytes.Buffer
	code := Run([]string{"memory", "dream", "daily", "--data-dir", t.TempDir(), "--workspace", "case-a"}, &output, &output)
	if code != ExitUnavailable || !strings.Contains(output.String(), `"capability": "memory_dreaming"`) || !strings.Contains(output.String(), `"state": "unavailable"`) {
		t.Fatalf("dream exit = %d, output = %s", code, output.String())
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
	if code != ExitOK || !strings.Contains(output.String(), `"hookEventName": "SessionStart"`) || !strings.Contains(output.String(), `\"runtime\":\"codex\"`) || !strings.Contains(output.String(), `\"injection_state\":\"unavailable\"`) {
		t.Fatalf("hook exit = %d, output = %s", code, output.String())
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

func TestAdapterCommandsInstallAndRemoveOnlyOwnedEntry(t *testing.T) {
	workspacePath := t.TempDir()
	var output bytes.Buffer
	if code := runAdapter([]string{"install", "--runtime", "codex", workspacePath}, &output, &output); code != ExitOK || !strings.Contains(output.String(), `"state": "installed"`) {
		t.Fatalf("install = %d %s", code, output.String())
	}
	output.Reset()
	if code := runAdapter([]string{"uninstall", "--runtime", "codex", workspacePath}, &output, &output); code != ExitOK || !strings.Contains(output.String(), `"state": "removed"`) {
		t.Fatalf("remove = %d %s", code, output.String())
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

func TestBundlesPlanExplainsThatDataBundlesAreNotActivated(t *testing.T) {
	var output bytes.Buffer
	if code := Run([]string{"bundles", "plan", "--track", "data-science"}, &output, &output); code != ExitOK || !strings.Contains(output.String(), `"state": "unavailable"`) || !strings.Contains(output.String(), `"id": "engineering-core"`) || !strings.Contains(output.String(), "not implemented") {
		t.Fatalf("bundles plan exit = %d, output = %s", code, output.String())
	}
}

func TestBundlesPlanKeepsClassicConsultingOnTheBaseBundle(t *testing.T) {
	var output bytes.Buffer
	if code := Run([]string{"bundles", "plan", "--track", "consulting"}, &output, &output); code != ExitOK || !strings.Contains(output.String(), `"state": "base_only"`) || strings.Contains(output.String(), `"id": "data-practice"`) {
		t.Fatalf("bundles plan exit = %d, output = %s", code, output.String())
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
	if _, err := os.Stat(filepath.Join(workspacePath, "brain", "tasks")); !os.IsNotExist(err) {
		t.Fatalf("unexpected task taxonomy: %v", err)
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
	if code := runSession([]string{"bridge", "--runtime", "claude", workspacePath}, &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK || !strings.Contains(output.String(), `"event": "session_start"`) || !strings.Contains(output.String(), `"runtime": "claude"`) || !strings.Contains(output.String(), `"injection_state": "unavailable"`) {
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
	if code := runInit([]string{workspacePath}, &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK || !strings.Contains(output.String(), `"case_agent"`) || !strings.Contains(output.String(), `"workspace-agent-`) || !strings.Contains(output.String(), `"agent_stub"`) || !strings.Contains(output.String(), `"runtime_state": "unavailable"`) {
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
	profile := `{"schema_version":1,"owner_id":"daniel","confirmed":true,"updated_at":"2026-07-28T00:00:00Z","selections":[{"role":"client_account_agent","agent_id":"client-account-agent-acme","display_name":"Compass","emoji":"🧭","owner_id":"daniel","ownership_scope":"account"},{"role":"case_agent","agent_id":"case-agent-pricing","display_name":"Forge","emoji":"⚙️","owner_id":"daniel","ownership_scope":"case"}]}`
	output.Reset()
	if code := runAgentWithInput([]string{"personalize", "--stdin"}, strings.NewReader(profile), &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK || !strings.Contains(output.String(), `"display_name": "Compass"`) {
		t.Fatalf("identity personalize = %d, output = %s", code, output.String())
	}
	output.Reset()
	if code := runAgentWithInput([]string{"identity"}, strings.NewReader(""), &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK || !strings.Contains(output.String(), `"ownership_scope": "case"`) {
		t.Fatalf("identity status = %d, output = %s", code, output.String())
	}
}

func TestAgentScaffoldCommandCreatesAndInspectsAWorkspaceSpecialist(t *testing.T) {
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
		"--id", "capability-research",
		"--role", "capability_specialist",
		"--scope-kind", "workspace",
		"--scope", inspection.WorkspaceID,
		"--parent", parent,
		"--parent-role", "workspace_agent",
	}, &output, &output, func() (string, error) { return dataRoot, nil })
	if code != ExitOK || !strings.Contains(output.String(), `"agent_id": "capability-research"`) ||
		!strings.Contains(output.String(), `"input_contract": "minimum_work_packet"`) ||
		!strings.Contains(output.String(), `"runtime_state": "unavailable"`) {
		t.Fatalf("agent scaffold exit = %d, output = %s", code, output.String())
	}
	output.Reset()
	if code := runAgent([]string{"status", "--id", "capability-research"}, &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK || !strings.Contains(output.String(), `"available": true`) {
		t.Fatalf("agent status exit = %d, output = %s", code, output.String())
	}
}

func TestAgentScaffoldCommandCreatesPracticeAndSubjectChain(t *testing.T) {
	root := t.TempDir()
	dataRoot := filepath.Join(root, "local", "BCGOS")
	canonRelative := filepath.Join("practices", "insurance", "canon.md")
	canonPath := filepath.Join(dataRoot, canonRelative)
	if err := os.MkdirAll(filepath.Dir(canonPath), 0o700); err != nil {
		t.Fatal(err)
	}
	canon := []byte("# Insurance canon\n")
	if err := os.WriteFile(canonPath, canon, 0o600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(canon)
	var output bytes.Buffer
	code := runAgent([]string{
		"scaffold",
		"--id", "practice-agent-insurance",
		"--role", "practice_agent",
		"--scope-kind", "practice",
		"--scope", "insurance",
		"--parent", "maestro",
		"--parent-role", "hub",
		"--owner", "practice-owner",
		"--mandate", "Maintain the governed insurance canon.",
		"--canon", filepath.ToSlash(canonRelative),
		"--canon-sha256", hex.EncodeToString(digest[:]),
	}, &output, &output, func() (string, error) { return dataRoot, nil })
	if code != ExitOK || !strings.Contains(output.String(), `"role": "practice_agent"`) {
		t.Fatalf("practice scaffold exit = %d, output = %s", code, output.String())
	}
	output.Reset()
	code = runAgent([]string{
		"scaffold",
		"--id", "subject-insurance",
		"--role", "subject_specialist",
		"--scope-kind", "practice",
		"--scope", "insurance",
		"--parent", "practice-agent-insurance",
		"--parent-role", "practice_agent",
	}, &output, &output, func() (string, error) { return dataRoot, nil })
	if code != ExitOK || !strings.Contains(output.String(), `"role": "subject_specialist"`) {
		t.Fatalf("subject scaffold exit = %d, output = %s", code, output.String())
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
		SchemaVersion: 1, RequestID: "advisory-cli",
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
			NoStakeholderIdentifiers: true, NoRawExcerpts: true,
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
	if code := runProductStatus([]string{workspacePath}, &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK || !strings.Contains(output.String(), `"state": "ready"`) || !strings.Contains(output.String(), `"brain_readable": true`) || !strings.Contains(output.String(), `"profile": "standard"`) {
		t.Fatalf("status exit = %d, output = %s", code, output.String())
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
