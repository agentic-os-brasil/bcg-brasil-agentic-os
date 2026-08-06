package cli

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
)

func TestWorkspaceMigrationStatusExposesFailClosedExecutionBoundary(t *testing.T) {
	root := t.TempDir()
	workspacePath := filepath.Join(root, "workspace")
	dataRoot := filepath.Join(root, "data")
	if _, err := workspace.Initialize(workspace.Options{WorkspacePath: workspacePath, DataRoot: dataRoot}); err != nil {
		t.Fatal(err)
	}
	var output, errorOutput bytes.Buffer
	code := runWorkspaceMigration([]string{"status", "--runtime", "codex", workspacePath}, &output, &errorOutput, func() (string, error) { return dataRoot, nil })
	if code != ExitOK {
		t.Fatalf("status exit = %d; out=%s err=%s", code, output.String(), errorOutput.String())
	}
	var result struct {
		Capability struct {
			State     string `json:"state"`
			Execution string `json:"execution"`
		} `json:"capability"`
		Inspection struct {
			State string `json:"state"`
		} `json:"inspection"`
	}
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Inspection.State != "valid" || result.Capability.State != "pending_core_activation" || result.Capability.Execution != "unavailable" {
		t.Fatalf("unexpected migration status: %#v", result)
	}
}
