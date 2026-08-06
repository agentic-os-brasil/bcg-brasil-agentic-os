package workspacemigration

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/adaptercfg"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/runtimeprojection"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
)

func TestBuildConfirmAndApplyPreservesAuthoredWorkspaceContent(t *testing.T) {
	workspacePath, dataRoot := initializedWorkspace(t)
	orientationPath := filepath.Join(workspacePath, "CLAUDE.md")
	if err := os.WriteFile(orientationPath, []byte("# regras locais\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	hooksPath := filepath.Join(workspacePath, ".claude", "settings.local.json")
	if err := os.MkdirAll(filepath.Dir(hooksPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hooksPath, []byte(`{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"user-hook"}]}]}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	plan, err := BuildPlan(PlanOptions{
		WorkspacePath: workspacePath, DataRoot: dataRoot, Runtime: "claude",
		Executable: "/opt/maestro/v2/bcgos", TargetRelease: "0.2.0", TargetBundle: "0.2.0",
	})
	if err != nil || plan.Execution != "available" || plan.SourceState != StateValid {
		t.Fatalf("BuildPlan() = %#v, %v", plan, err)
	}
	if err := StagePlan(dataRoot, plan); err != nil {
		t.Fatal(err)
	}
	confirmed, err := Confirm(dataRoot, plan.ID, testCore(), testTime())
	if err != nil || confirmed.PlanID != plan.ID {
		t.Fatalf("Confirm() = %#v, %v", confirmed, err)
	}
	receipt, err := Apply(dataRoot, plan.ID, testCore(), testTime())
	if err != nil || receipt.State != "applied" {
		t.Fatalf("Apply() = %#v, %v", receipt, err)
	}
	orientation, err := os.ReadFile(orientationPath)
	if err != nil || !strings.HasPrefix(string(orientation), "# regras locais\n") || !strings.Contains(string(orientation), runtimeprojection.OrientationBegin) {
		t.Fatalf("authored orientation was not preserved: %q (%v)", orientation, err)
	}
	hooks, err := os.ReadFile(hooksPath)
	if err != nil || !strings.Contains(string(hooks), "user-hook") || !strings.Contains(string(hooks), "--adapter-source maestro") {
		t.Fatalf("authored hook was not preserved: %q (%v)", hooks, err)
	}
}

func TestApplyFailureRestoresBoundedSnapshot(t *testing.T) {
	workspacePath, dataRoot := initializedWorkspace(t)
	plan, err := BuildPlan(PlanOptions{
		WorkspacePath: workspacePath, DataRoot: dataRoot, Runtime: "codex",
		Executable: "/opt/maestro/v2/bcgos", TargetRelease: "0.2.0", TargetBundle: "0.2.0",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := StagePlan(dataRoot, plan); err != nil {
		t.Fatal(err)
	}
	if _, err := Confirm(dataRoot, plan.ID, testCore(), testTime()); err != nil {
		t.Fatal(err)
	}
	original := installAdapter
	installAdapter = func(runtimeName, path, executable string) (adaptercfg.Status, error) {
		if _, err := original(runtimeName, path, executable); err != nil {
			return adaptercfg.Status{}, err
		}
		return adaptercfg.Status{}, errors.New("synthetic interruption after adapter write")
	}
	defer func() { installAdapter = original }()

	receipt, err := Apply(dataRoot, plan.ID, testCore(), testTime())
	if err == nil || receipt.State != "rolled_back" || !receipt.Restored {
		t.Fatalf("failed Apply() = %#v, %v", receipt, err)
	}
	if _, statErr := os.Stat(filepath.Join(workspacePath, ".codex", "hooks.json")); !os.IsNotExist(statErr) {
		t.Fatalf("failed migration left adapter files behind: %v", statErr)
	}
}

func TestRecoverInterruptedMigrationRestoresSnapshot(t *testing.T) {
	workspacePath, dataRoot := initializedWorkspace(t)
	plan, err := BuildPlan(PlanOptions{
		WorkspacePath: workspacePath, DataRoot: dataRoot, Runtime: "claude",
		Executable: "/opt/maestro/v2/bcgos", TargetRelease: "0.2.0", TargetBundle: "0.2.0",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := StagePlan(dataRoot, plan); err != nil {
		t.Fatal(err)
	}
	confirmed, err := Confirm(dataRoot, plan.ID, testCore(), testTime())
	if err != nil {
		t.Fatal(err)
	}
	partial := filepath.Join(workspacePath, "CLAUDE.md")
	if err := os.WriteFile(partial, []byte("partial managed write\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(planRoot(dataRoot, plan.ID), "execution.json"), execution{
		SchemaVersion: SchemaVersion, PlanID: plan.ID, State: "applying", SnapshotPath: confirmed.SnapshotPath, StartedAt: testTime(),
	}, 0o600); err != nil {
		t.Fatal(err)
	}
	receipt, err := Recover(dataRoot, plan.ID, testTime())
	if err != nil || receipt.State != "rolled_back" || !receipt.Restored {
		t.Fatalf("Recover() = %#v, %v", receipt, err)
	}
	if _, statErr := os.Stat(partial); !os.IsNotExist(statErr) {
		t.Fatalf("interrupted recovery kept a file absent from the snapshot: %v", statErr)
	}
}

func TestInspectClassifiesLegacyAndIncompleteWorkspaces(t *testing.T) {
	root := t.TempDir()
	dataRoot := filepath.Join(root, "data")
	legacy := filepath.Join(root, "legacy")
	if err := os.MkdirAll(filepath.Join(legacy, ".bcgos"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, ".bcgos", "runtime-projection.json"), []byte(`{"legacy":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	legacyInspection, err := Inspect(PlanOptions{WorkspacePath: legacy, DataRoot: dataRoot, Runtime: "codex"})
	if err != nil || legacyInspection.State != StateLegacy {
		t.Fatalf("legacy inspection = %#v, %v", legacyInspection, err)
	}
	incompletePath, _ := initializedWorkspaceAt(t, filepath.Join(root, "incomplete"), dataRoot)
	if err := os.Remove(filepath.Join(incompletePath, "brain", "README.md")); err != nil {
		t.Fatal(err)
	}
	incomplete, err := Inspect(PlanOptions{WorkspacePath: incompletePath, DataRoot: dataRoot, Runtime: "codex"})
	if err != nil || incomplete.State != StateIncomplete {
		t.Fatalf("incomplete inspection = %#v, %v", incomplete, err)
	}
	plan, err := BuildPlan(PlanOptions{WorkspacePath: incompletePath, DataRoot: dataRoot, Runtime: "codex", Executable: "/opt/bcgos", TargetRelease: "0.2.0", TargetBundle: "0.2.0"})
	if err != nil || plan.Execution != "unavailable" {
		t.Fatalf("incomplete plan = %#v, %v", plan, err)
	}
}

func initializedWorkspace(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	return initializedWorkspaceAt(t, filepath.Join(root, "workspace"), filepath.Join(root, "data"))
}

func initializedWorkspaceAt(t *testing.T, workspacePath, dataRoot string) (string, string) {
	t.Helper()
	if _, err := workspace.Initialize(workspace.Options{WorkspacePath: workspacePath, DataRoot: dataRoot}); err != nil {
		t.Fatal(err)
	}
	return workspacePath, dataRoot
}

func testCore() CoreActivation {
	return CoreActivation{Authority: StableBootstrapper, Activated: true, Release: "0.2.0", BundleVersion: "0.2.0", ManagedRoot: "/opt/maestro", StateDigest: strings.Repeat("a", 64)}
}

func testTime() time.Time {
	return time.Date(2026, 8, 5, 21, 0, 0, 0, time.UTC)
}
