package continuoususe

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBuildPrioritizesIncompleteCalibration(t *testing.T) {
	status, err := Build(Source{
		WorkspaceState:   "ready",
		CalibrationState: "required",
		CalibrationTrack: "selection_required",
		OpenTasksState:   "unavailable",
		OpenWorkState:    "unavailable",
		MemoryState:      "empty",
		Runtimes:         []RuntimeSource{{Runtime: "claude", Configured: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if status.State != StateActionRequired || status.Calibration.State != "required" || len(status.NextActions) == 0 || status.NextActions[0].ID != ActionCompleteCalibration {
		t.Fatalf("status = %#v", status)
	}
	if !status.Runtimes[0].Configured || status.Runtimes[0].AdapterObserved || status.Runtimes[0].NativeQualified || !status.Runtimes[0].Unavailable || status.Runtimes[0].State != EvidenceConfigured {
		t.Fatalf("runtime evidence = %#v", status.Runtimes[0])
	}
}

func TestBuildRequiresExplicitCheckpointForActiveWork(t *testing.T) {
	status, err := Build(Source{
		WorkspaceState:   "ready",
		CalibrationState: "complete",
		CalibrationTrack: "quick",
		OpenTasksState:   "available",
		OpenTasksCount:   2,
		OpenWorkState:    "available",
		WorkState:        "running",
		CheckpointState:  "missing",
		MemoryState:      "empty",
	})
	if err != nil {
		t.Fatal(err)
	}
	if status.State != StateActionRequired || status.OpenWork.Pointer != "bcgos://execution/active" || status.OpenWork.CheckpointState != "missing" || len(status.NextActions) == 0 || status.NextActions[0].ID != ActionCheckpointActiveWork {
		t.Fatalf("status = %#v", status)
	}
	body, err := json.Marshal(status.OpenWork)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "item_id") || strings.Contains(string(body), "attempt_id") {
		t.Fatalf("continuous-use projection leaked execution identity: %s", body)
	}
}

func TestBuildReportsObservedWithoutClaimingNativeQualification(t *testing.T) {
	status, err := Build(Source{
		WorkspaceState:        "ready",
		CalibrationState:      "complete",
		CalibrationTrack:      "complete",
		OpenTasksState:        "empty",
		OpenWorkState:         "available",
		WorkState:             "paused",
		CheckpointState:       "available",
		MemoryState:           "available",
		AttestedSignalFiles:   3,
		MaintenanceConfigured: true,
		MaintenanceObserved:   true,
		Runtimes:              []RuntimeSource{{Runtime: "codex", Configured: true, AdapterObserved: true, ObservedEvents: []string{"stop_finalize"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if status.State != StateReady || status.Signals.State != EvidenceAdapterObserved || status.Signals.AttestedFiles != 3 || !status.Maintenance.Configured || !status.Maintenance.AdapterObserved {
		t.Fatalf("status = %#v", status)
	}
	runtime := status.Runtimes[0]
	if runtime.State != EvidenceAdapterObserved || !runtime.Configured || !runtime.AdapterObserved || runtime.NativeQualified || !runtime.Unavailable {
		t.Fatalf("runtime evidence = %#v", runtime)
	}
	if len(status.NextActions) == 0 || status.NextActions[0].ID != ActionResumeActiveWork {
		t.Fatalf("next actions = %#v", status.NextActions)
	}
}

func TestBuildFailsClosedForAmbiguousWorkAndInvalidEvidence(t *testing.T) {
	status, err := Build(Source{WorkspaceState: "ready", CalibrationState: "complete", CalibrationTrack: "quick", OpenTasksState: "empty", OpenWorkState: "ambiguous", MemoryState: "unavailable"})
	if err != nil || status.State != StateActionRequired || status.OpenWork.Pointer != "" || status.NextActions[0].ID != ActionResolveAmbiguousWork {
		t.Fatalf("status = %#v, err = %v", status, err)
	}
	if _, err := Build(Source{WorkspaceState: "ready", CalibrationState: "complete", CalibrationTrack: "quick", OpenTasksState: "empty", OpenWorkState: "unavailable", MemoryState: "empty", Runtimes: []RuntimeSource{{Runtime: "claude", NativeQualified: true}}}); err == nil {
		t.Fatal("native qualification without configured and observed evidence was accepted")
	}
}

func TestValidateRejectsTamperedEvidenceAndExecutionMetadata(t *testing.T) {
	status, err := Build(Source{
		WorkspaceState: "ready", CalibrationState: "complete", CalibrationTrack: "quick",
		OpenTasksState: "empty", OpenWorkState: "unavailable", MemoryState: "empty",
		Runtimes: []RuntimeSource{{Runtime: "claude", Configured: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	status.Runtimes[0].NativeQualified = true
	status.Runtimes[0].AdapterObserved = false
	if err := status.Validate(); err == nil {
		t.Fatal("tampered native qualification was accepted")
	}

	status, err = Build(Source{
		WorkspaceState: "ready", CalibrationState: "complete", CalibrationTrack: "quick",
		OpenTasksState: "empty", OpenWorkState: "available", WorkState: "paused",
		CheckpointState: "available", MemoryState: "empty",
	})
	if err != nil {
		t.Fatal(err)
	}
	status.OpenWork.Pointer = "bcgos://execution/private-item-id"
	if err := status.Validate(); err == nil {
		t.Fatal("non-portable execution pointer was accepted")
	}
}
