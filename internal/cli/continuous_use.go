package cli

import (
	"os"
	"path/filepath"
	"sort"

	basememory "github.com/agentic-os-brasil/bcg-brasil-agentic-os/bundles/base/memory"
	baseruntime "github.com/agentic-os-brasil/bcg-brasil-agentic-os/bundles/base/runtime"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/adaptercfg"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/continuoususe"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/execution"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/lifecycle"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/maintenance"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/memory"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ownerctx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/runtimecap"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/runtimeprojection"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
)

func buildContinuousUseStatus(root string, inspection workspace.Inspection, owner ownerctx.Status) (continuoususe.Status, execution.ActiveContinuity, error) {
	active := execution.ActiveContinuity{State: execution.ActivePointerUnavailable, CheckpointState: execution.CheckpointUnavailable}
	if inspection.WorkspaceID != "" {
		resolved, err := (execution.Store{Root: root}).ActiveContinuity(inspection.WorkspaceID)
		if err != nil {
			active = execution.ActiveContinuity{State: execution.ActivePointerAmbiguous, CheckpointState: execution.CheckpointUnavailable}
		} else {
			active = resolved
		}
	}

	calibrationState := owner.Onboarding.State
	calibrationTrack := owner.Onboarding.Track
	if calibrationState == "" {
		calibrationState = "required"
	}
	if calibrationTrack == "" {
		calibrationTrack = "selection_required"
	}
	openTasksState, openTasksCount := owner.OpenTasks.State, owner.OpenTasks.Count
	if openTasksState == "" {
		openTasksState = "unavailable"
		openTasksCount = 0
	}
	memoryState, attestedSignals := continuousMemoryStatus(root, inspection.WorkspaceID)
	maintenanceConfigured, maintenanceObserved := continuousMaintenanceStatus(root, inspection.WorkspaceID)
	runtimes := continuousRuntimeEvidence(root, inspection)

	status, err := continuoususe.Build(continuoususe.Source{
		WorkspaceState: inspection.State, CalibrationState: calibrationState, CalibrationTrack: calibrationTrack,
		OpenTasksState: openTasksState, OpenTasksCount: openTasksCount,
		OpenWorkState: active.State, WorkState: string(active.WorkState), CheckpointState: active.CheckpointState,
		MemoryState: memoryState, AttestedSignalFiles: attestedSignals,
		MaintenanceConfigured: maintenanceConfigured, MaintenanceObserved: maintenanceObserved,
		Runtimes: runtimes,
	})
	return status, active, err
}

func activePointerFromContinuity(active execution.ActiveContinuity) execution.ActivePointer {
	return execution.ActivePointer{Path: active.Path, Available: active.Available, State: active.State}
}

func continuousMemoryStatus(root, workspaceID string) (string, int) {
	if root == "" || workspaceID == "" {
		return "unavailable", 0
	}
	policy, err := basememory.Policy()
	if err != nil {
		return "unavailable", 0
	}
	report, err := (&memory.Engine{Root: filepath.Join(root, "memory"), Policy: policy}).Status(workspaceID)
	if err != nil {
		return "unavailable", 0
	}
	switch report.State {
	case "ready":
		return "available", report.AttestedCaptureFiles
	case "empty", "captured":
		return "empty", report.AttestedCaptureFiles
	default:
		return "unavailable", report.AttestedCaptureFiles
	}
}

func continuousMaintenanceStatus(root, workspaceID string) (bool, bool) {
	path := filepath.Join(root, "maintenance", "canary-enrollment.json")
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return false, false
	}
	enrollment, err := maintenance.LoadCanaryEnrollment(root)
	if err != nil || enrollment.WorkspaceID != workspaceID {
		return false, false
	}
	activated := map[string]bool{}
	for _, activation := range enrollment.Activated {
		activated[activation.JobID] = true
	}
	configured := activated[maintenance.MemoryCheckpointJobID] && activated[maintenance.MemoryLightDreamJobID]
	if !configured {
		return false, false
	}
	for _, jobID := range []string{maintenance.MemoryCheckpointJobID, maintenance.MemoryLightDreamJobID} {
		directory := filepath.Join(root, "maintenance", "receipts", "workspaces", workspaceID, "receipts", jobID)
		entries, readErr := os.ReadDir(directory)
		if readErr != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() && entry.Type()&os.ModeSymlink == 0 && filepath.Ext(entry.Name()) == ".json" {
				return true, true
			}
		}
	}
	return true, false
}

func continuousRuntimeEvidence(root string, inspection workspace.Inspection) []continuoususe.RuntimeSource {
	manifest, manifestErr := baseruntime.Manifest()
	result := make([]continuoususe.RuntimeSource, 0, 2)
	for _, runtimeName := range []string{"claude", "codex"} {
		configured := false
		reason := "exact runtime projection and lifecycle bindings are not installed"
		adapterStatus, adapterErr := adaptercfg.Inspect(runtimeName, inspection.WorkspacePath)
		projectionStatus, projectionErr := runtimeprojection.Inspect(runtimeName, inspection.WorkspacePath)
		if adapterErr == nil && projectionErr == nil && adapterStatus.State == "installed" && projectionStatus.State == "installed" {
			configured = true
			reason = "qualifying native-session evidence is pending"
		}
		summary, summaryErr := lifecycle.DiagnoseRuntime(root, inspection.WorkspaceID, runtimeName)
		observed := summaryErr == nil && summary.State == "observed"
		events := append([]string(nil), summary.Events...)
		sort.Strings(events)
		nativeQualified := false
		if manifestErr == nil {
			nativeQualified = continuousRuntimeNativeQualified(manifest, runtimeName)
		}
		if nativeQualified && !configured {
			nativeQualified = false
			reason = "the previously qualified runtime is not currently configured for this workspace"
		}
		result = append(result, continuoususe.RuntimeSource{
			Runtime: runtimeName, Configured: configured, AdapterObserved: observed,
			NativeQualified: nativeQualified, ObservedEvents: events, UnavailableReason: reason,
		})
	}
	return result
}

func continuousRuntimeNativeQualified(manifest runtimecap.Manifest, runtimeName string) bool {
	required := map[string]bool{
		"session_start": false, "context_injection": false, "pre_action_guard": false,
		"post_action_observe": false, "stop_finalize": false,
	}
	for _, capability := range manifest.Capabilities {
		if _, tracked := required[capability.ID]; !tracked {
			continue
		}
		runtimeContract, ok := capability.Runtimes[runtimeName]
		if !ok || !runtimeContract.NativeQualified {
			return false
		}
		required[capability.ID] = true
	}
	for _, qualified := range required {
		if !qualified {
			return false
		}
	}
	return true
}
