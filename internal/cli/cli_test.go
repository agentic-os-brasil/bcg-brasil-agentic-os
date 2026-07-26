package cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/adaptercfg"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/execution"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/sessionstart"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

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
	for _, skillID := range []string{"extract-work-items", "grill-me", "investigate", "meeting-to-work-items", "storyline", "wayfinder"} {
		if !strings.Contains(output.String(), `"`+skillID+`"`) {
			t.Fatalf("skills index is missing Wave 1 skill %q: %s", skillID, output.String())
		}
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

func TestExecutionHandoffAcrossTwoSessionsUsesOnlyTheActivePointer(t *testing.T) {
	root := t.TempDir()
	dataRoot := func() (string, error) { return root, nil }
	workspacePath := filepath.Join(t.TempDir(), "case-a")
	if _, err := workspace.Initialize(workspace.Options{WorkspacePath: workspacePath, DataRoot: root}); err != nil {
		t.Fatal(err)
	}

	const objective = "Secret contract body that must never enter Session Context."
	const summary = "Session A left a bounded checkpoint."
	const nextStep = "Session B should resume from this bounded next action."
	contract := `{
	  "objective": "` + objective + `",
	  "initial_next_step": "Start in session A.",
	  "criteria": [{"id": "tests", "type": "command_check", "command": ["go", "version"]}],
	  "allowed_refs": []
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
	checkpoint := `{"summary":"` + summary + `","next_step":"` + nextStep + `","artifact_refs":[]}`
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
	if code := runInit([]string{workspacePath}, &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK || !strings.Contains(output.String(), `"workspace_agent"`) || !strings.Contains(output.String(), `"workspace-agent-`) || !strings.Contains(output.String(), `"agent_stub"`) || !strings.Contains(output.String(), `"runtime_state": "unavailable"`) {
		t.Fatalf("init exit = %d, output = %s", code, output.String())
	}
	output.Reset()
	if code := runWorkspaceAgent([]string{"interview", workspacePath}, &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK || !strings.Contains(output.String(), `"workspace_agent_setup"`) || !strings.Contains(output.String(), `"research"`) {
		t.Fatalf("workspace-agent interview exit = %d, output = %s", code, output.String())
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
