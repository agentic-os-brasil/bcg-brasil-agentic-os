package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/setupauth"
)

func TestSetupApplyBlocksElevatedProcessBeforeAnyMutation(t *testing.T) {
	original := ensureUserLevelProcess
	ensureUserLevelProcess = func() error { return errors.New("elevated Windows process") }
	t.Cleanup(func() { ensureUserLevelProcess = original })

	root := t.TempDir()
	workspacePath := filepath.Join(root, "workspace")
	var output bytes.Buffer
	code := runSetup([]string{"apply", "--workspace", workspacePath, "--runtime", "claude", "--confirm"}, &output, &output,
		func() (string, error) { return filepath.Join(root, "data"), nil },
		func() (setupauth.Identity, error) { return setupauth.DeriveIdentity("pilot", "device"), nil },
	)
	if code != ExitFailure {
		t.Fatalf("code = %d, output = %s", code, output.String())
	}
	var report setupApplyReport
	if err := json.Unmarshal(output.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.State != setupBlocked || len(report.Stages) != 1 || report.Stages[0].ID != "process_identity" {
		t.Fatalf("report = %#v", report)
	}
	if _, err := os.Stat(workspacePath); !os.IsNotExist(err) {
		t.Fatalf("elevated setup mutated workspace: %v", err)
	}
}

func TestSetupApplyRequiresOneConfirmationBeforeFirstMutation(t *testing.T) {
	root := t.TempDir()
	dataRoot := filepath.Join(root, "data")
	workspacePath := filepath.Join(root, "workspace")
	var output bytes.Buffer
	code := runSetup([]string{"apply", "--workspace", workspacePath, "--runtime", "codex"}, &output, &output,
		func() (string, error) { return dataRoot, nil },
		func() (setupauth.Identity, error) { return setupauth.DeriveIdentity("pilot", "device"), nil },
	)
	if code != ExitFailure {
		t.Fatalf("code = %d, output = %s", code, output.String())
	}
	var report setupApplyReport
	if err := json.Unmarshal(output.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.State != setupBlocked || len(report.Stages) != 1 || report.Stages[0].ID != "authorization" {
		t.Fatalf("report = %#v", report)
	}
	if _, err := os.Stat(workspacePath); !os.IsNotExist(err) {
		t.Fatalf("first apply mutated workspace before confirmation: %v", err)
	}
}

func TestSetupApplyCompletesLocallyAndReusesGrantWithoutAnotherConfirmation(t *testing.T) {
	root := t.TempDir()
	dataRoot := filepath.Join(root, "data")
	workspacePath := filepath.Join(root, "workspace")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	identity := func() (setupauth.Identity, error) { return setupauth.DeriveIdentity("pilot", "device"), nil }
	resolveRoot := func() (string, error) { return dataRoot, nil }
	var first bytes.Buffer
	code := runSetup([]string{"apply", "--workspace", workspacePath, "--runtime", "codex", "--executable", executable, "--confirm"}, &first, &first, resolveRoot, identity)
	if code != ExitOK {
		t.Fatalf("first code = %d, output = %s", code, first.String())
	}
	var firstReport setupApplyReport
	if err := json.Unmarshal(first.Bytes(), &firstReport); err != nil {
		t.Fatal(err)
	}
	if firstReport.State != setupComplete && firstReport.State != setupCompleteWithExternalPending {
		t.Fatalf("first report = %#v", firstReport)
	}
	if firstReport.Authorization.State != setupauth.StateActive || firstReport.WorkspaceID == "" {
		t.Fatalf("first authorization = %#v", firstReport.Authorization)
	}

	var second bytes.Buffer
	code = runSetup([]string{"apply", "--workspace", workspacePath, "--runtime", "codex", "--executable", executable}, &second, &second, resolveRoot, identity)
	if code != ExitOK {
		t.Fatalf("second code = %d, output = %s", code, second.String())
	}
	var secondReport setupApplyReport
	if err := json.Unmarshal(second.Bytes(), &secondReport); err != nil {
		t.Fatal(err)
	}
	if secondReport.Authorization.GrantDigest != firstReport.Authorization.GrantDigest {
		t.Fatalf("grant changed: first=%s second=%s", firstReport.Authorization.GrantDigest, secondReport.Authorization.GrantDigest)
	}
	wantAlreadyReady := map[string]bool{"workspace_initialize": false, "authorization": false, "runtime_adapter": false}
	for _, stage := range secondReport.Stages {
		if _, ok := wantAlreadyReady[stage.ID]; ok && stage.Status == "already_ready" {
			wantAlreadyReady[stage.ID] = true
		}
	}
	for stage, ready := range wantAlreadyReady {
		if !ready {
			t.Fatalf("stage %s was not idempotent: %#v", stage, secondReport.Stages)
		}
	}
}
