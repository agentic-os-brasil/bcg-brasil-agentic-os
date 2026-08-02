package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
)

func TestAdapterVerifyEmitsStructuredFailClosedReport(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	workspacePath := filepath.Join(root, "workspace")
	dataRoot := filepath.Join(root, "owner-data")
	if _, err := workspace.Initialize(workspace.Options{WorkspacePath: workspacePath, DataRoot: dataRoot}); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	code := runAdapterWithDataRoot([]string{"verify", "--runtime", "codex", workspacePath}, &output, &output, func() (string, error) {
		return dataRoot, nil
	})
	if code != ExitFailure || !strings.Contains(output.String(), `"schema_version": 1`) ||
		!strings.Contains(output.String(), `"state": "failed"`) ||
		!strings.Contains(output.String(), `"id": "install_state"`) ||
		!strings.Contains(output.String(), `"native_observation": "not_observed"`) {
		t.Fatalf("adapter verify exit=%d output=%s", code, output.String())
	}
}

func TestAdapterVerifyRejectsAlternateRuntimeAndExecutable(t *testing.T) {
	for _, args := range [][]string{
		{"verify", "--runtime", "claude"},
		{"verify", "--runtime", "codex", "--executable", "/tmp/arbitrary"},
	} {
		var output bytes.Buffer
		if code := runAdapterWithDataRoot(args, &output, &output, func() (string, error) { return t.TempDir(), nil }); code != ExitUsage {
			t.Fatalf("args=%v exit=%d output=%s", args, code, output.String())
		}
	}
}
