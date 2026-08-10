// Package continuoususe derives Maestro's cross-session status from existing
// local authorities. It owns no durable state and never receives professional
// content.
package continuoususe

import (
	"encoding/json"
	"errors"
	"sort"
)

const (
	StateReady          = "ready"
	StateActionRequired = "action_required"
	StateUnavailable    = "unavailable"

	EvidenceConfigured      = "configured"
	EvidenceAdapterObserved = "adapter-observed"
	EvidenceNativeQualified = "native-qualified"
	EvidenceOperational     = "operational"
	EvidenceUnavailable     = "unavailable"

	ActionCompleteCalibration  = "complete_calibration"
	ActionCheckpointActiveWork = "checkpoint_active_work"
	ActionResumeActiveWork     = "resume_active_work"
	ActionResolveAmbiguousWork = "resolve_ambiguous_work"
	ActionRepairWorkspace      = "repair_workspace"
	ActionInspectContinuity    = "inspect_continuity"

	ReasonNativePending            = "native_qualification_pending_or_unavailable"
	ReasonContextInjectionPending  = "native_context_injection_pending"
	ReasonSchedulerPending         = "native_scheduler_qualification_pending"
	ReasonRuntimeProjectionMissing = "runtime_projection_or_bindings_not_installed"
	ReasonNativeSessionPending     = "native_session_qualification_pending"
	ReasonRuntimeNotConfigured     = "qualified_runtime_not_configured"
	ReasonSourceUnavailable        = "continuous_use_source_unavailable"

	MaximumSerializedStatusBytes = 4 << 10

	activeExecutionPointer = "bcgos://execution/active"
)

var actionTemplates = map[string]NextAction{
	ActionCompleteCalibration:  {ID: ActionCompleteCalibration, Command: "bcgos owner onboarding status", Reason: "finish or confirm the deterministic owner calibration before routing ordinary work"},
	ActionCheckpointActiveWork: {ID: ActionCheckpointActiveWork, Command: "bcgos work next --active --workspace <workspace>", Reason: "resolve the active item, then write a bounded checkpoint before the next handoff"},
	ActionResumeActiveWork:     {ID: ActionResumeActiveWork, Command: "bcgos work next --active --workspace <workspace>", Reason: "resolve the bounded checkpoint and resume through a new fenced attempt"},
	ActionResolveAmbiguousWork: {ID: ActionResolveAmbiguousWork, Command: "bcgos work inspect --workspace <workspace> --item <id>", Reason: "multiple active items require an explicit item selection"},
	ActionRepairWorkspace:      {ID: ActionRepairWorkspace, Command: "bcgos doctor <workspace>", Reason: "workspace readiness must be restored before continuity can be trusted"},
	ActionInspectContinuity:    {ID: ActionInspectContinuity, Command: "bcgos maestro status <workspace>", Reason: "inspect the bounded continuity projection after source state changes"},
}

type Source struct {
	WorkspaceState        string
	CalibrationState      string
	CalibrationTrack      string
	OpenTasksState        string
	OpenTasksCount        int
	OpenWorkState         string
	WorkState             string
	CheckpointState       string
	MemoryState           string
	AttestedSignalFiles   int
	MaintenanceConfigured bool
	MaintenanceObserved   bool
	Runtimes              []RuntimeSource
}

type RuntimeSource struct {
	Runtime           string
	Configured        bool
	AdapterObserved   bool
	NativeQualified   bool
	ObservedEvents    []string
	UnavailableReason string
}

type Status struct {
	SchemaVersion int                `json:"schema_version"`
	State         string             `json:"state"`
	Calibration   Calibration        `json:"calibration"`
	OpenTasks     OpenTasks          `json:"open_tasks"`
	OpenWork      OpenWork           `json:"open_work"`
	Signals       SignalEvidence     `json:"bounded_signals"`
	Memory        MemoryStatus       `json:"memory"`
	Maintenance   CapabilityEvidence `json:"maintenance"`
	Runtimes      []RuntimeEvidence  `json:"runtimes"`
	NextActions   []NextAction       `json:"next_actions"`
}

type Calibration struct {
	State string `json:"state"`
	Track string `json:"track"`
}

type OpenTasks struct {
	State string `json:"state"`
	Count int    `json:"count"`
}

type OpenWork struct {
	Pointer         string `json:"pointer,omitempty"`
	Available       bool   `json:"available"`
	State           string `json:"state"`
	WorkState       string `json:"work_state,omitempty"`
	CheckpointState string `json:"checkpoint_state"`
}

type CapabilityEvidence struct {
	State           string `json:"state"`
	Configured      bool   `json:"configured"`
	AdapterObserved bool   `json:"adapter_observed"`
	NativeQualified bool   `json:"native_qualified"`
	Unavailable     bool   `json:"unavailable"`
	Reason          string `json:"reason,omitempty"`
}

type SignalEvidence struct {
	CapabilityEvidence
	AttestedFiles int `json:"attested_capture_files"`
}

type MemoryStatus struct {
	State string `json:"state"`
}

type RuntimeEvidence struct {
	Runtime string `json:"runtime"`
	CapabilityEvidence
	ObservedEvents []string `json:"observed_events,omitempty"`
}

type NextAction struct {
	ID      string `json:"id"`
	Command string `json:"command"`
	Reason  string `json:"reason"`
}

func Build(source Source) (Status, error) {
	if err := validateSource(source); err != nil {
		return Status{}, err
	}
	status := Status{
		SchemaVersion: 1,
		State:         StateReady,
		Calibration:   Calibration{State: source.CalibrationState, Track: source.CalibrationTrack},
		OpenTasks:     OpenTasks{State: source.OpenTasksState, Count: source.OpenTasksCount},
		OpenWork: OpenWork{
			Available: source.OpenWorkState == "available", State: source.OpenWorkState,
			WorkState: source.WorkState, CheckpointState: source.CheckpointState,
		},
		Memory: MemoryStatus{State: source.MemoryState},
	}
	if status.OpenWork.Available {
		status.OpenWork.Pointer = activeExecutionPointer
	}

	anyRuntimeConfigured := false
	for _, runtime := range source.Runtimes {
		anyRuntimeConfigured = anyRuntimeConfigured || runtime.Configured
		evidence := evidence(runtime.Configured, runtime.AdapterObserved, runtime.NativeQualified, runtime.UnavailableReason)
		events := append([]string(nil), runtime.ObservedEvents...)
		sort.Strings(events)
		status.Runtimes = append(status.Runtimes, RuntimeEvidence{Runtime: runtime.Runtime, CapabilityEvidence: evidence, ObservedEvents: events})
	}
	sort.Slice(status.Runtimes, func(left, right int) bool { return status.Runtimes[left].Runtime < status.Runtimes[right].Runtime })
	status.Signals = SignalEvidence{
		CapabilityEvidence: evidence(anyRuntimeConfigured, source.AttestedSignalFiles > 0, false, ReasonContextInjectionPending),
		AttestedFiles:      source.AttestedSignalFiles,
	}
	status.Maintenance = evidence(source.MaintenanceConfigured, source.MaintenanceObserved, false, ReasonSchedulerPending)

	if source.WorkspaceState != "ready" && source.WorkspaceState != "warning" {
		status.State = StateUnavailable
		status.NextActions = append(status.NextActions, actionTemplates[ActionRepairWorkspace])
	}
	if source.CalibrationState != "complete" {
		if status.State != StateUnavailable {
			status.State = StateActionRequired
		}
		status.NextActions = append(status.NextActions, actionTemplates[ActionCompleteCalibration])
	}
	switch source.OpenWorkState {
	case "ambiguous":
		if status.State != StateUnavailable {
			status.State = StateActionRequired
		}
		status.NextActions = append(status.NextActions, actionTemplates[ActionResolveAmbiguousWork])
	case "available":
		if source.CheckpointState == "missing" {
			if status.State != StateUnavailable {
				status.State = StateActionRequired
			}
			status.NextActions = append(status.NextActions, actionTemplates[ActionCheckpointActiveWork])
		} else {
			status.NextActions = append(status.NextActions, actionTemplates[ActionResumeActiveWork])
		}
	}
	if len(status.NextActions) == 0 {
		status.NextActions = []NextAction{actionTemplates[ActionInspectContinuity]}
	}
	if err := status.Validate(); err != nil {
		return Status{}, err
	}
	return status, nil
}

// Validate rejects projections whose bounded state or evidence ordering was
// altered after derivation. It deliberately validates only closed vocabulary;
// continuous-use status is not a channel for arbitrary runtime metadata.
func (status Status) Validate() error {
	if status.SchemaVersion != 1 || (status.State != StateReady && status.State != StateActionRequired && status.State != StateUnavailable) {
		return errors.New("continuous-use status header is invalid")
	}
	if err := validateSource(Source{
		WorkspaceState: "ready", CalibrationState: status.Calibration.State, CalibrationTrack: status.Calibration.Track,
		OpenTasksState: status.OpenTasks.State, OpenTasksCount: status.OpenTasks.Count,
		OpenWorkState: status.OpenWork.State, WorkState: status.OpenWork.WorkState,
		CheckpointState: status.OpenWork.CheckpointState, MemoryState: status.Memory.State,
		AttestedSignalFiles: status.Signals.AttestedFiles,
	}); err != nil {
		return err
	}
	if status.OpenWork.Available != (status.OpenWork.State == "available") {
		return errors.New("continuous-use open-work availability is inconsistent")
	}
	if status.OpenWork.Available && status.OpenWork.Pointer != activeExecutionPointer {
		return errors.New("continuous-use active-work pointer is invalid")
	}
	if !status.OpenWork.Available && status.OpenWork.Pointer != "" {
		return errors.New("continuous-use unresolved work exposed a pointer")
	}
	if err := validateEvidence(status.Signals.CapabilityEvidence); err != nil {
		return err
	}
	if status.Signals.AdapterObserved != (status.Signals.AttestedFiles > 0) {
		return errors.New("continuous-use bounded-signal count is inconsistent")
	}
	if err := validateEvidence(status.Maintenance); err != nil {
		return err
	}
	seenRuntimes := map[string]bool{}
	previousRuntime := ""
	validEvents := map[string]bool{
		"session_start": true, "context_inject": true, "pre_action_guard": true,
		"post_action_observe": true, "stop_finalize": true,
	}
	for _, runtime := range status.Runtimes {
		if (runtime.Runtime != "claude" && runtime.Runtime != "codex") || seenRuntimes[runtime.Runtime] || previousRuntime > runtime.Runtime {
			return errors.New("continuous-use runtime evidence is invalid, duplicated or unsorted")
		}
		seenRuntimes[runtime.Runtime] = true
		previousRuntime = runtime.Runtime
		if err := validateEvidence(runtime.CapabilityEvidence); err != nil {
			return err
		}
		previousEvent := ""
		for _, event := range runtime.ObservedEvents {
			if !validEvents[event] || previousEvent >= event {
				return errors.New("continuous-use observed lifecycle event is invalid, duplicated or unsorted")
			}
			previousEvent = event
		}
		if runtime.AdapterObserved != (len(runtime.ObservedEvents) > 0) {
			return errors.New("continuous-use runtime observation is inconsistent")
		}
	}
	if len(status.NextActions) == 0 {
		return errors.New("continuous-use status is missing a safe next action")
	}
	seenActions := map[string]bool{}
	for _, action := range status.NextActions {
		expected, valid := actionTemplates[action.ID]
		if !valid || seenActions[action.ID] || action != expected {
			return errors.New("continuous-use next action is invalid or duplicated")
		}
		seenActions[action.ID] = true
	}
	body, err := json.Marshal(status)
	if err != nil || len(body) > MaximumSerializedStatusBytes {
		return errors.New("continuous-use status exceeds its fixed serialization budget")
	}
	return nil
}

func validateEvidence(value CapabilityEvidence) error {
	validReasons := map[string]bool{
		ReasonNativePending: true, ReasonContextInjectionPending: true,
		ReasonSchedulerPending: true, ReasonRuntimeProjectionMissing: true,
		ReasonNativeSessionPending: true, ReasonRuntimeNotConfigured: true,
		ReasonSourceUnavailable: true,
	}
	switch {
	case value.State == EvidenceOperational:
		// operational is product availability, while native evidence remains separate.
		// Evidence ordering is still enforced so telemetry cannot claim impossible proof.
		if !value.Configured || value.Unavailable || value.Reason != "" || value.NativeQualified && !value.AdapterObserved {
			return errors.New("continuous-use capability evidence is inconsistent")
		}
	case value.State == EvidenceUnavailable:
		// unavailable may describe a configured-but-not-qualified capability; it must
		// not claim native observation or qualification without the prerequisites.
		if value.Unavailable == false || !validReasons[value.Reason] || value.NativeQualified && (!value.Configured || !value.AdapterObserved) {
			return errors.New("continuous-use capability evidence is inconsistent")
		}
	default:
		return errors.New("continuous-use capability evidence state is invalid")
	}
	return nil
}

func evidence(configured, observed, qualified bool, reason string) CapabilityEvidence {
	if configured {
		return CapabilityEvidence{
			State: EvidenceOperational, Configured: true,
			AdapterObserved: observed, NativeQualified: qualified,
			Unavailable: false, Reason: "",
		}
	}
	if reason == "" {
		reason = ReasonNativePending
	}
	return CapabilityEvidence{
		State: EvidenceUnavailable, Configured: false,
		AdapterObserved: observed, NativeQualified: qualified,
		Unavailable: true, Reason: reason,
	}
}

func validateSource(source Source) error {
	if source.WorkspaceState == "" {
		return errors.New("continuous-use workspace state is required")
	}
	switch source.CalibrationState {
	case "required", "in_progress", "review_required", "complete":
	default:
		return errors.New("continuous-use calibration state is invalid")
	}
	switch source.CalibrationTrack {
	case "selection_required", "quick", "complete":
	default:
		return errors.New("continuous-use calibration track is invalid")
	}
	switch source.OpenTasksState {
	case "available", "empty", "unavailable":
	default:
		return errors.New("continuous-use open-task state is invalid")
	}
	if source.OpenTasksCount < 0 || (source.OpenTasksState != "available" && source.OpenTasksCount != 0) {
		return errors.New("continuous-use open-task count is invalid")
	}
	switch source.OpenWorkState {
	case "available":
		if source.WorkState != "running" && source.WorkState != "paused" {
			return errors.New("continuous-use active work state is invalid")
		}
		if source.CheckpointState != "available" && source.CheckpointState != "missing" {
			return errors.New("continuous-use checkpoint state is invalid")
		}
	case "unavailable", "ambiguous":
		if source.WorkState != "" || source.CheckpointState != "" && source.CheckpointState != "unavailable" {
			return errors.New("continuous-use unresolved work exposed metadata")
		}
	default:
		return errors.New("continuous-use open-work state is invalid")
	}
	switch source.MemoryState {
	case "available", "empty", "unavailable":
	default:
		return errors.New("continuous-use memory state is invalid")
	}
	if source.AttestedSignalFiles < 0 || source.MaintenanceObserved && !source.MaintenanceConfigured {
		return errors.New("continuous-use evidence ordering is invalid")
	}
	seen := map[string]bool{}
	for _, runtime := range source.Runtimes {
		if (runtime.Runtime != "claude" && runtime.Runtime != "codex") || seen[runtime.Runtime] {
			return errors.New("continuous-use runtime evidence is invalid or duplicated")
		}
		seen[runtime.Runtime] = true
		if runtime.NativeQualified && (!runtime.Configured || !runtime.AdapterObserved) {
			return errors.New("continuous-use runtime evidence ordering is invalid")
		}
	}
	return nil
}
