package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
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
