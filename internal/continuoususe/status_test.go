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
	if !status.Runtimes[0].Configured || status.Runtimes[0].AdapterObserved || status.Runtimes[0].NativeQualified || status.Runtimes[0].Unavailable || status.Runtimes[0].State != EvidenceConfigured {
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
	if runtime.State != EvidenceAdapterObserved || !runtime.Configured || !runtime.AdapterObserved || runtime.NativeQualified || runtime.Unavailable {
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
	// Native qualification remains evidence-gated even when an adapter is observed.
	status2, err := Build(Source{WorkspaceState: "ready", CalibrationState: "complete", CalibrationTrack: "quick", OpenTasksState: "empty", OpenWorkState: "unavailable", MemoryState: "empty", Runtimes: []RuntimeSource{{Runtime: "claude", NativeQualified: true}}})
	if err == nil || status2.Runtimes != nil {
		t.Fatalf("unconfigured native qualification should be rejected: status=%#v err=%v", status2, err)
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
	// Flip Unavailable to true on configured evidence — violates the evidence invariant.
	status.Runtimes[0].Unavailable = true
	if err := status.Validate(); err == nil {
		t.Fatal("tampered unavailable flag on configured evidence was accepted")
	}
	status, err = Build(Source{
		WorkspaceState: "ready", CalibrationState: "complete", CalibrationTrack: "quick",
		OpenTasksState: "empty", OpenWorkState: "unavailable", MemoryState: "empty",
		Runtimes: []RuntimeSource{{Runtime: "claude", Configured: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	status.Runtimes[0].State = EvidenceOperational
	if err := status.Validate(); err == nil {
		t.Fatal("configured runtime was promoted to operational evidence")
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

func TestStatusRejectsUnboundedLabelsReasonsAndHistoricalPayloads(t *testing.T) {
	status, err := Build(Source{
		WorkspaceState: "ready", CalibrationState: "complete", CalibrationTrack: "quick",
		OpenTasksState: "empty", OpenWorkState: "unavailable", MemoryState: "empty",
		Runtimes: []RuntimeSource{{Runtime: "claude", Configured: true}},
	})
	if err != nil {
		t.Fatal(err)
	}

	tampered := status
	tampered.Calibration.Track = strings.Repeat("history-", 1024)
	if err := tampered.Validate(); err == nil {
		t.Fatal("unbounded calibration label was accepted")
	}
	tampered = status
	tampered.NextActions = append([]NextAction(nil), status.NextActions...)
	tampered.NextActions[0].Reason = strings.Repeat("receipt body ", 1024)
	if err := tampered.Validate(); err == nil {
		t.Fatal("historical body was accepted through a next-action reason")
	}
	tampered = status
	tampered.Runtimes = append([]RuntimeEvidence(nil), status.Runtimes...)
	tampered.Runtimes[0].Reason = strings.Repeat("receipt body ", 1024)
	if err := tampered.Validate(); err == nil {
		t.Fatal("historical body was accepted through runtime evidence")
	}
	if _, err := Build(Source{
		WorkspaceState: "ready", CalibrationState: "complete", CalibrationTrack: "custom-track-with-unbounded-label",
		OpenTasksState: "empty", OpenWorkState: "unavailable", MemoryState: "empty",
	}); err == nil {
		t.Fatal("caller-defined calibration track was accepted")
	}
}

func TestMaximalStatusHasFixedCompactShapeWithoutHistoryOrReceiptBodies(t *testing.T) {
	events := []string{"session_start", "context_inject", "pre_action_guard", "post_action_observe", "stop_finalize"}
	status, err := Build(Source{
		WorkspaceState: "unavailable", CalibrationState: "required", CalibrationTrack: "selection_required",
		OpenTasksState: "available", OpenTasksCount: int(^uint(0) >> 1),
		OpenWorkState: "ambiguous", MemoryState: "available", AttestedSignalFiles: int(^uint(0) >> 1),
		MaintenanceConfigured: true, MaintenanceObserved: true,
		Runtimes: []RuntimeSource{
			{Runtime: "claude", Configured: true, AdapterObserved: true, ObservedEvents: events, UnavailableReason: ReasonNativeSessionPending},
			{Runtime: "codex", Configured: true, AdapterObserved: true, ObservedEvents: events, UnavailableReason: ReasonNativeSessionPending},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	if len(body) > MaximumSerializedStatusBytes {
		t.Fatalf("bounded status grew to %d bytes: %s", len(body), body)
	}
	for _, prohibited := range []string{"receipt_body", "receipt_history", "checkpoint_body", "objective", "item_id", "attempt_id", "transcript"} {
		if strings.Contains(string(body), prohibited) {
			t.Fatalf("bounded status exposed %q: %s", prohibited, body)
		}
	}
}
