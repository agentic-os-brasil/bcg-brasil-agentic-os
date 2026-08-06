package workspacemigration

import (
	"errors"
	"io"
	"os"
	"os/exec"
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
	confirmed, err := confirmInternal(dataRoot, plan.ID, testCore(), testTime())
	if err != nil || confirmed.PlanID != plan.ID {
		t.Fatalf("Confirm() = %#v, %v", confirmed, err)
	}
	receipt, err := applyInternal(dataRoot, plan.ID, testCore(), testTime())
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
	if _, err := confirmInternal(dataRoot, plan.ID, testCore(), testTime()); err != nil {
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

	receipt, err := applyInternal(dataRoot, plan.ID, testCore(), testTime())
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
	confirmed, err := confirmInternal(dataRoot, plan.ID, testCore(), testTime())
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
	receipt, err := recoverInternal(dataRoot, plan.ID, testTime())
	if err != nil || receipt.State != "rolled_back" || !receipt.Restored {
		t.Fatalf("Recover() = %#v, %v", receipt, err)
	}
	if _, statErr := os.Stat(partial); !os.IsNotExist(statErr) {
		t.Fatalf("interrupted recovery kept a file absent from the snapshot: %v", statErr)
	}
}

func TestPublicExecutionFailsClosedWithoutBootstrapperVerifier(t *testing.T) {
	workspacePath, dataRoot := initializedWorkspace(t)
	plan, err := BuildPlan(PlanOptions{WorkspacePath: workspacePath, DataRoot: dataRoot, Runtime: "codex", Executable: "/opt/bcgos", TargetRelease: "0.2.0", TargetBundle: "0.2.0"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Confirm(dataRoot, plan.ID, testCore(), testTime()); !errors.Is(err, errExecutionUnavailable) {
		t.Fatalf("Confirm() error = %v, want public execution unavailable", err)
	}
	if _, err := Apply(dataRoot, plan.ID, testCore(), testTime()); !errors.Is(err, errExecutionUnavailable) {
		t.Fatalf("Apply() error = %v, want public execution unavailable", err)
	}
	if _, err := Recover(dataRoot, plan.ID, testTime()); !errors.Is(err, errExecutionUnavailable) {
		t.Fatalf("Recover() error = %v, want public execution unavailable", err)
	}
}

func TestRecoveryRejectsDisplacedAndTamperedSnapshots(t *testing.T) {
	workspacePath, dataRoot := initializedWorkspace(t)
	plan, err := BuildPlan(PlanOptions{WorkspacePath: workspacePath, DataRoot: dataRoot, Runtime: "claude", Executable: "/opt/bcgos", TargetRelease: "0.2.0", TargetBundle: "0.2.0"})
	if err != nil {
		t.Fatal(err)
	}
	if err := StagePlan(dataRoot, plan); err != nil {
		t.Fatal(err)
	}
	confirmed, err := confirmInternal(dataRoot, plan.ID, testCore(), testTime())
	if err != nil {
		t.Fatal(err)
	}
	root := planRoot(dataRoot, plan.ID)
	displaced := filepath.Join(t.TempDir(), "snapshot.json")
	if err := writeJSON(filepath.Join(root, "execution.json"), execution{SchemaVersion: SchemaVersion, PlanID: plan.ID, State: "applying", SnapshotPath: displaced, StartedAt: testTime()}, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := recoverInternal(dataRoot, plan.ID, testTime()); err == nil {
		t.Fatal("recovery accepted a displaced snapshot path")
	}
	var value snapshot
	if err := readJSON(confirmed.SnapshotPath, &value); err != nil {
		t.Fatal(err)
	}
	value.PlanID = strings.Repeat("f", 32)
	if err := writeJSON(confirmed.SnapshotPath, value, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(root, "execution.json"), execution{SchemaVersion: SchemaVersion, PlanID: plan.ID, State: "applying", SnapshotPath: confirmed.SnapshotPath, StartedAt: testTime()}, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := recoverInternal(dataRoot, plan.ID, testTime()); err == nil {
		t.Fatal("recovery accepted a snapshot with a tampered plan ID")
	}
}

func TestInternalEngineRejectsSymlinkedManagedParent(t *testing.T) {
	workspacePath, dataRoot := initializedWorkspace(t)
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(workspacePath, ".codex")); err != nil {
		t.Fatal(err)
	}
	if _, err := BuildPlan(PlanOptions{WorkspacePath: workspacePath, DataRoot: dataRoot, Runtime: "codex", Executable: "/opt/bcgos", TargetRelease: "0.2.0", TargetBundle: "0.2.0"}); err == nil {
		t.Fatal("BuildPlan accepted a symlinked managed parent")
	}
	entries, err := os.ReadDir(outside)
	if err != nil || len(entries) != 0 {
		t.Fatalf("symlink target was modified: entries=%v err=%v", entries, err)
	}
}

func TestFailedProjectionRemovesNewManagedSkills(t *testing.T) {
	workspacePath, dataRoot := initializedWorkspace(t)
	tracks := []string{"software-engineering"}
	plan, err := BuildPlan(PlanOptions{WorkspacePath: workspacePath, DataRoot: dataRoot, Runtime: "codex", Executable: "/opt/bcgos", TargetRelease: "0.2.0", TargetBundle: "0.2.0", CapabilityTracks: tracks})
	if err != nil {
		t.Fatal(err)
	}
	if err := StagePlan(dataRoot, plan); err != nil {
		t.Fatal(err)
	}
	if _, err := confirmInternal(dataRoot, plan.ID, testCore(), testTime()); err != nil {
		t.Fatal(err)
	}
	original := installProjection
	installProjection = func(runtimeName, path string, selected []string) (runtimeprojection.Status, error) {
		status, err := original(runtimeName, path, selected)
		if err != nil {
			return status, err
		}
		return status, errors.New("synthetic failure after new skill projection")
	}
	defer func() { installProjection = original }()
	receipt, err := applyInternal(dataRoot, plan.ID, testCore(), testTime())
	if err == nil || receipt.State != "rolled_back" || !receipt.Restored {
		t.Fatalf("failed projection Apply() = %#v, %v", receipt, err)
	}
	paths, err := runtimeprojection.PlannedManagedPaths("codex", workspacePath, tracks)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		if strings.HasSuffix(path, "SKILL.md") {
			if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
				t.Fatalf("failed migration left prospective skill %s: %v", path, statErr)
			}
		}
	}
}

func TestAdapterGitExcludeIsBoundedAndRestored(t *testing.T) {
	workspacePath, dataRoot := initializedWorkspace(t)
	if err := os.MkdirAll(filepath.Join(workspacePath, ".git", "info"), 0o700); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("git", "-C", workspacePath, "init")
	command.Stdout, command.Stderr = io.Discard, io.Discard
	if err := command.Run(); err != nil {
		t.Fatal(err)
	}
	excludePath := filepath.Join(workspacePath, ".git", "info", "exclude")
	originalExclude := []byte("# authored exclude\n")
	if err := os.WriteFile(excludePath, originalExclude, 0o600); err != nil {
		t.Fatal(err)
	}
	plan, err := BuildPlan(PlanOptions{WorkspacePath: workspacePath, DataRoot: dataRoot, Runtime: "codex", Executable: "/opt/bcgos", TargetRelease: "0.2.0", TargetBundle: "0.2.0"})
	if err != nil {
		t.Fatal(err)
	}
	if err := StagePlan(dataRoot, plan); err != nil {
		t.Fatal(err)
	}
	confirmed, err := confirmInternal(dataRoot, plan.ID, testCore(), testTime())
	if err != nil {
		t.Fatal(err)
	}
	var value snapshot
	if err := readJSON(confirmed.SnapshotPath, &value); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, entry := range value.Entries {
		if filepath.Clean(entry.Path) == filepath.Clean(excludePath) {
			found = true
			if !entry.Exists || string(entry.Body) != string(originalExclude) {
				t.Fatalf("Git exclude snapshot = %#v", entry)
			}
		}
	}
	if !found {
		t.Fatal("Git exclude was not included in the bounded snapshot")
	}
	originalInstall := installAdapter
	installAdapter = func(runtimeName, path, executable string) (adaptercfg.Status, error) {
		if _, err := originalInstall(runtimeName, path, executable); err != nil {
			return adaptercfg.Status{}, err
		}
		return adaptercfg.Status{}, errors.New("synthetic failure after adapter and Git exclude writes")
	}
	defer func() { installAdapter = originalInstall }()
	receipt, err := applyInternal(dataRoot, plan.ID, testCore(), testTime())
	if err == nil || receipt.State != "rolled_back" || !receipt.Restored {
		t.Fatalf("failed adapter Apply() = %#v, %v", receipt, err)
	}
	restored, err := os.ReadFile(excludePath)
	if err != nil || string(restored) != string(originalExclude) {
		t.Fatalf("Git exclude was not restored: %q (%v)", restored, err)
	}
}

func TestAppliedReceiptPreventsRecoveryRollbackAndTerminalizes(t *testing.T) {
	workspacePath, dataRoot := initializedWorkspace(t)
	plan, err := BuildPlan(PlanOptions{WorkspacePath: workspacePath, DataRoot: dataRoot, Runtime: "codex", Executable: "/opt/bcgos", TargetRelease: "0.2.0", TargetBundle: "0.2.0"})
	if err != nil {
		t.Fatal(err)
	}
	if err := StagePlan(dataRoot, plan); err != nil {
		t.Fatal(err)
	}
	if _, err := confirmInternal(dataRoot, plan.ID, testCore(), testTime()); err != nil {
		t.Fatal(err)
	}
	originalRemove := removeExecutionMarkerFunc
	removeExecutionMarkerFunc = func(string) error { return errors.New("synthetic terminalization failure") }
	receipt, err := applyInternal(dataRoot, plan.ID, testCore(), testTime())
	removeExecutionMarkerFunc = originalRemove
	if err == nil || receipt.State != "applied" {
		t.Fatalf("terminalization Apply() = %#v, %v", receipt, err)
	}
	if _, statErr := os.Stat(filepath.Join(workspacePath, ".codex", "hooks.json")); statErr != nil {
		t.Fatalf("applied projection disappeared before recovery: %v", statErr)
	}
	recovered, err := recoverInternal(dataRoot, plan.ID, testTime())
	if err != nil || recovered.State != "applied" {
		t.Fatalf("recovery after applied receipt = %#v, %v", recovered, err)
	}
	if _, statErr := os.Stat(filepath.Join(planRoot(dataRoot, plan.ID), "execution.json")); !os.IsNotExist(statErr) {
		t.Fatalf("execution marker remained after recovery terminalization: %v", statErr)
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
