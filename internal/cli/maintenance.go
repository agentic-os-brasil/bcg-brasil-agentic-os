package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	basememory "github.com/agentic-os-brasil/bcg-brasil-agentic-os/bundles/base/memory"
	baseruntime "github.com/agentic-os-brasil/bcg-brasil-agentic-os/bundles/base/runtime"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/darwin"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/macosadapter"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/maintenance"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/memory"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
)

// runMaintenance exposes the platform-neutral maintenance contract to native
// adapters and humans. A catalog or adapter presence is never evidence of
// execution; only an enrolled, qualified worker can emit terminal receipts.
func runMaintenance(args []string, out, errOut io.Writer) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		fmt.Fprintln(out, "usage: bcgos maintenance <catalog|status|wake|canary> [--trigger presence|daily|weekly|monthly|event]")
		return ExitOK
	}
	catalog, err := baseruntime.Maintenance()
	if err != nil {
		return reportError(errOut, err)
	}
	switch args[0] {
	case "catalog":
		if len(args) != 1 {
			fmt.Fprintln(errOut, "usage: bcgos maintenance catalog")
			return ExitUsage
		}
		return writeJSON(out, catalog, errOut)
	case "status":
		if len(args) != 1 {
			fmt.Fprintln(errOut, "usage: bcgos maintenance status")
			return ExitUsage
		}
		return writeJSON(out, maintenanceStatus(catalog), errOut)
	case "wake":
		flags := newFlagSet("maintenance wake", errOut)
		trigger := flags.String("trigger", "presence", "wake trigger")
		eventID := flags.String("event-id", "", "bounded source event identity; required for event wakes")
		workspace := flags.String("workspace", "maestro-system", "bounded maintenance workspace")
		attended := flags.Bool("attended", false, "grant attended local Canary authority")
		idleState := flags.String("idle-state", string(maintenance.IdleUnknown), "idle eligibility: auto, unknown, active or idle")
		if err := flags.Parse(args[1:]); err != nil || rejectPositionals(flags, errOut) {
			fmt.Fprintln(errOut, "usage: bcgos maintenance wake --trigger presence|daily|weekly|monthly|event [--event-id ID] [--workspace ID] [--idle-state auto|unknown|active|idle] [--attended]")
			return ExitUsage
		}
		trimmedTrigger, trimmedEventID := strings.TrimSpace(*trigger), strings.TrimSpace(*eventID)
		requestedIdle := strings.TrimSpace(*idleState)
		trimmedIdle := maintenance.IdleState(requestedIdle)
		idleObservation := "explicit"
		if requestedIdle == "auto" {
			trimmedIdle, idleObservation = observeNativeIdle(context.Background())
		}
		if trimmedIdle != maintenance.IdleUnknown && trimmedIdle != maintenance.IdleActive && trimmedIdle != maintenance.IdleConfirmed {
			fmt.Fprintln(errOut, "--idle-state must be auto, unknown, active or idle")
			return ExitUsage
		}
		if trimmedTrigger == "event" && trimmedEventID == "" {
			fmt.Fprintln(errOut, "maintenance wake --trigger event requires --event-id ID")
			return ExitUsage
		}
		if trimmedTrigger == "event" {
			if err := maintenance.ValidateEventID(trimmedEventID); err != nil {
				fmt.Fprintln(errOut, err)
				return ExitUsage
			}
		}
		if trimmedTrigger != "event" && trimmedEventID != "" {
			fmt.Fprintln(errOut, "--event-id is only valid with --trigger event")
			return ExitUsage
		}
		if _, err := catalog.ForTrigger(trimmedTrigger); err != nil {
			return reportError(errOut, err)
		}
		root, err := defaultDataRoot()
		if err != nil {
			return reportError(errOut, err)
		}
		currentHome, homeErr := os.UserHomeDir()
		if homeErr != nil {
			return reportError(errOut, homeErr)
		}
		enrollment, enrollmentErr := maintenance.LoadCanaryEnrollment(root)
		if enrollmentErr == nil && (enrollment.WorkspaceID != strings.TrimSpace(*workspace) || !samePathCLI(enrollment.Home, currentHome)) {
			enrollmentErr = errors.New("Canary enrollment is bound to a different workspace or home")
		}
		handlers, qualification, activated := maintenanceHandlers(root, strings.TrimSpace(*workspace), enrollment, enrollmentErr == nil)
		// The presence wake is an acceleration mechanism, not an activation
		// mechanism. Only jobs explicitly enrolled by the attended installer may
		// enter the scheduler plan. Catalog-only/model-backed jobs remain visible
		// as unavailable capabilities without generating a fresh unavailable
		// receipt on every RunAtLoad or interval wake.
		jobs := activatedSchedulerJobs(schedulerJobsForTrigger(trimmedTrigger), activated)
		worker := maintenance.Worker{Catalog: catalog, Scheduler: scheduler.Store{Root: filepath.Join(root, "maintenance", "scheduler")}, Receipts: maintenance.Store{Root: filepath.Join(root, "maintenance", "receipts")}, Jobs: jobs, Handlers: handlers, LocalQualification: qualification, ActivatedJobs: activated, Deadline: 2 * time.Minute}
		timezone := ""
		if enrollmentErr == nil {
			timezone = enrollment.Timezone
		}
		report, err := worker.Run(context.Background(), maintenance.WakeRequest{WorkspaceID: strings.TrimSpace(*workspace), Trigger: maintenance.Trigger(trimmedTrigger), EventID: trimmedEventID, Timezone: timezone, IdleState: trimmedIdle, OwnerID: "bcgos-presence", Now: time.Now(), Attended: *attended, Preauthorized: enrollmentErr == nil})
		if err != nil {
			_ = writeJSON(out, map[string]any{"schema_version": 1, "state": maintenance.Unavailable, "agent_id": "darwin", "scope": "health/maestro-system", "trigger": trimmedTrigger, "event_id": trimmedEventID, "native_schedulers": "disabled", "reason": err.Error() + "; no receipt was emitted"}, errOut)
			return ExitUnavailable
		}
		wakeState, wakeReason, exitCode := report.State, "", ExitOK
		if enrollmentErr != nil || len(worker.Handlers) == 0 {
			wakeState, wakeReason, exitCode = maintenance.Unavailable, "no qualified local handlers are enrolled; no receipt was emitted by this wake", ExitUnavailable
		}
		for _, receipt := range report.Receipts {
			if receipt.State == maintenance.ReceiptUnavailable {
				exitCode, wakeState, wakeReason = ExitUnavailable, maintenance.Unavailable, "unavailable work remains due; its receipt is not scheduler success"
			} else if receipt.State == maintenance.ReceiptFailed || receipt.State == maintenance.ReceiptTimedOut {
				exitCode, wakeState, wakeReason = ExitUnavailable, "completed_with_failures", "bounded handler failure remains due; recovery is available on a later wake"
			}
		}
		// A wake can arrive from launchd, an attended command or another approved
		// adapter. It must not claim that the native scheduler is disabled merely
		// because this bounded worker has no provenance for its caller. The
		// lifecycle status command remains the source of truth for that question.
		code := writeJSON(out, map[string]any{"state": wakeState, "reason": wakeReason, "trigger": trimmedTrigger, "event_id": trimmedEventID, "idle_observation": idleObservation, "native_schedulers": "wake_received; inspect maintenance canary status for lifecycle state", "worker": report}, errOut)
		if code != ExitOK {
			return code
		}
		return exitCode
	case "canary":
		if len(args) == 1 {
			return writeJSON(out, map[string]any{"schema_version": 1, "state": "attended_local_only", "agent_id": "darwin", "scope": "health/maestro-system", "interactive_and_housekeeping_identity": "darwin", "native_schedulers": "disabled", "worker_invocation": "qualified_housekeeping_executor", "model_inline": false}, errOut)
		}
		return runMaintenanceCanary(args[1:], out, errOut, catalog)
	default:
		fmt.Fprintln(errOut, "usage: bcgos maintenance <catalog|status|wake|canary> [--trigger presence|daily|weekly|monthly|event]")
		return ExitUsage
	}
}

func maintenanceHandlers(root, workspace string, enrollment maintenance.CanaryEnrollment, enrolled bool) (map[string]any, map[string]string, []string) {
	handlers := map[string]any{}
	qualification, activated := map[string]string{}, []string{}
	if !enrolled {
		return handlers, qualification, activated
	}
	qualification, activated = maintenance.ActivationMaps(enrollment)
	active := make(map[string]bool, len(activated))
	for _, jobID := range activated {
		active[jobID] = true
	}
	schedulerStore := scheduler.Store{Root: filepath.Join(root, "maintenance", "scheduler")}
	darwinRoot := filepath.Join(root, "maintenance", "darwin")
	builder := darwin.LocalProductHealthBuilder{Scheduler: schedulerStore, Workspace: workspace, Runtime: "runtime-neutral", ManagedStateRoot: darwinRoot, StateDocumentsRoot: filepath.Join(root, "maintenance", "state-documents")}
	commandStore := maintenance.Store{Root: filepath.Join(root, "maintenance", "darwin-commands")}
	guard := darwin.ToolGuardFunc(func(call darwin.ToolCall) error {
		if call.Tool != "filesystem" || (call.Operation != "write" && call.Operation != "edit") || !strings.HasPrefix(call.Resource, "bcgos://health/maestro-system/") {
			return errors.New("Darwin Canary grant denied")
		}
		return nil
	})
	proposalStore := darwin.ProposalStore{Root: filepath.Join(root, "maintenance", "darwin-proposals")}
	invoker := darwin.OperationalInvoker{Diagnostics: darwin.FilesystemInvoker{Root: darwinRoot}, Repairs: darwin.ManagedStateRepairInvoker{Root: darwinRoot}, Guard: guard}
	darwinStore := darwin.Store{Root: darwinRoot}
	if active[darwin.HousekeepingJobID] {
		handlers[darwin.HousekeepingJobID] = darwin.HousekeepingHandler{Build: builder, Guard: guard, Invoker: invoker, Store: darwinStore, CommandStore: commandStore}
	}
	if active[maintenance.MemoryCheckpointJobID] {
		handlers[maintenance.MemoryCheckpointJobID] = maintenance.MemoryCheckpointHandler{Scheduler: schedulerStore, Store: maintenance.ContinuityCheckpointStore{Root: filepath.Join(root, "maintenance", "checkpoints")}}
	}
	if active[maintenance.MemoryLightDreamJobID] {
		policy, policyErr := basememory.Policy()
		config, configErr := basememory.Runtime()
		if policyErr == nil && configErr == nil {
			memoryRoot := filepath.Join(root, "memory")
			attestor := memory.CaptureAttestor{Root: memoryRoot}
			engine := &memory.Engine{Root: memoryRoot, Policy: policy, Budgets: map[string]int{"L1": config.L1MaxRunes, "L2": 1, "L3": 1, "lifetime": 1}, MaxSourceBytes: config.L1MaxInputBytes, Synthesizer: memory.DeterministicL1Synthesizer{MaxRunes: config.L1MaxRunes, MaxEntries: config.L1MaxEntries, MaxInputBytes: config.L1MaxInputBytes, MaxInputEntries: config.L1MaxInputEntries, Attestor: attestor}, SynthesizerID: memory.DeterministicL1SynthesizerID}
			handlers[maintenance.MemoryLightDreamJobID] = maintenance.MemoryLightDreamHandler{Engine: engine}
		}
	}
	if active["darwin-deep-weekly"] {
		handlers["darwin-deep-weekly"] = darwin.DeepReviewHandler{Build: builder, Guard: guard, Invoker: invoker, Store: darwinStore, CommandStore: commandStore, ProposalStore: proposalStore}
	}
	if active[maintenance.WalterSelfReviewWeeklyJobID] {
		handlers[maintenance.WalterSelfReviewWeeklyJobID] = maintenance.WalterWeeklyAdapter{}
	}
	return handlers, qualification, activated
}

func runMaintenanceCanary(args []string, out, errOut io.Writer, catalog maintenance.Catalog) int {
	action := args[0]
	if action != "install-macos" && action != "status" && action != "pause" && action != "resume" && action != "uninstall" && action != "recover-quarantine" {
		fmt.Fprintln(errOut, "usage: bcgos maintenance canary <install-macos|status|pause|resume|uninstall|recover-quarantine> [--confirm] [--launchctl] [--home PATH] [--workspace-path PATH]")
		return ExitUsage
	}
	flags := newFlagSet("maintenance canary "+action, errOut)
	confirm := flags.Bool("confirm", false, "explicitly confirm the requested lifecycle mutation")
	launchctlRequested := flags.Bool("launchctl", false, "explicitly inspect or mutate the current macOS launchctl domain")
	home := flags.String("home", "", "current user home or isolated filesystem-only fixture")
	workspace := flags.String("workspace", "", "bounded maintenance workspace ID for quarantine recovery")
	workspacePath := flags.String("workspace-path", "", "exact initialized workspace path for scheduler enrollment")
	executable := flags.String("executable", "", "exact installed bcgos executable; must match the running process")
	jobID := flags.String("job-id", "", "exact quarantined job to recover")
	scheduledFor := flags.String("scheduled-for", "", "exact scheduled occurrence in RFC3339 format")
	reason := flags.String("reason", "", "operator recovery reason code")
	if err := flags.Parse(args[1:]); err != nil || rejectPositionals(flags, errOut) {
		return ExitUsage
	}
	mutating := action != "status"
	if mutating && !*confirm {
		return reportError(errOut, errors.New("Canary lifecycle mutation requires --confirm"))
	}
	currentHome, err := os.UserHomeDir()
	if err != nil {
		return reportError(errOut, err)
	}
	homeProvided := strings.TrimSpace(*home) != ""
	if !homeProvided {
		*home = currentHome
	}
	root, err := canaryDataRoot(*home, currentHome)
	if err != nil {
		return reportError(errOut, err)
	}
	if *launchctlRequested && (runtime.GOOS != "darwin" || !samePathCLI(*home, currentHome)) {
		return reportError(errOut, errors.New("--launchctl is available only for the current macOS user; no administrator domain is supported"))
	}
	if action == "recover-quarantine" {
		if strings.TrimSpace(*jobID) == "" || strings.TrimSpace(*scheduledFor) == "" || *reason != "operator_confirmed_process_gone" {
			return reportError(errOut, errors.New("quarantine recovery requires --job-id, --scheduled-for, --reason operator_confirmed_process_gone and --confirm"))
		}
		when, parseErr := time.Parse(time.RFC3339Nano, *scheduledFor)
		if parseErr != nil {
			return reportError(errOut, errors.New("scheduled-for must be RFC3339"))
		}
		store := scheduler.Store{Root: filepath.Join(root, "maintenance", "scheduler")}
		leases, listErr := store.QuarantinedLeases(strings.TrimSpace(*workspace))
		if listErr != nil {
			return reportError(errOut, listErr)
		}
		key := scheduler.ScheduledOccurrenceKey(strings.TrimSpace(*jobID), when)
		var target scheduler.Lease
		for _, lease := range leases {
			if lease.JobID == strings.TrimSpace(*jobID) && lease.OccurrenceKey == key {
				target = lease
				break
			}
		}
		if target.FenceToken == "" {
			return reportError(errOut, errors.New("quarantined occurrence was not found"))
		}
		now := time.Now().UTC()
		trigger := maintenance.TriggerDaily
		for _, job := range schedulerJobsForTrigger("default") {
			if job.ID == target.JobID {
				switch job.Cadence {
				case scheduler.Weekly:
					trigger = maintenance.TriggerWeekly
				case scheduler.Monthly:
					trigger = maintenance.TriggerMonthly
				}
			}
		}
		intent, auditErr := maintenance.NewRecoveryIntentReceipt(strings.TrimSpace(*workspace), target.JobID, trigger, when, now, target.FenceToken)
		if auditErr != nil {
			return reportError(errOut, auditErr)
		}
		receiptStore := maintenance.Store{Root: filepath.Join(root, "maintenance", "receipts")}
		if auditErr = receiptStore.AppendReceipt(intent); auditErr != nil {
			return reportError(errOut, auditErr)
		}
		if recoveryErr := store.RecoverQuarantinedLease(target, now); recoveryErr != nil {
			failed, buildErr := maintenance.NewRecoveryOutcomeReceipt(intent, now, "failed", maintenance.ReasonRecoveryFailed)
			if buildErr == nil {
				_ = receiptStore.AppendReceipt(failed)
			}
			return reportError(errOut, recoveryErr)
		}
		completed, buildErr := maintenance.NewRecoveryOutcomeReceipt(intent, now, "completed", maintenance.ReasonRecoveryCompleted)
		if buildErr != nil {
			return reportError(errOut, buildErr)
		}
		// The fenced removal succeeded. If completion audit publication fails,
		// preserve the committed-but-incomplete state as a scheduler diagnostic
		// and return a repair-required error rather than claiming clean success.
		if auditErr = receiptStore.AppendReceipt(completed); auditErr != nil {
			_ = store.AppendReceipt(strings.TrimSpace(*workspace), scheduler.Receipt{JobID: target.JobID, ScheduledFor: when, AttemptedAt: now, State: scheduler.Failed, Error: "recovery_committed_audit_incomplete"})
			return reportError(errOut, errors.New("recovery committed but completion audit is incomplete"))
		}
		if auditErr = store.AppendReceipt(strings.TrimSpace(*workspace), scheduler.Receipt{JobID: target.JobID, ScheduledFor: when, AttemptedAt: now, State: scheduler.Unavailable, Error: "quarantine_recovery_completed"}); auditErr != nil {
			return reportError(errOut, auditErr)
		}
		return writeJSON(out, map[string]any{"state": "quarantine_recovered", "job_id": target.JobID, "scheduled_for": when.UTC(), "reason": "operator_confirmed_process_gone", "audit_receipt": completed, "recovery_intent": intent}, errOut)
	}
	uid, uidErr := macosadapter.CurrentUID()
	if uidErr != nil {
		return reportError(errOut, uidErr)
	}
	lifecycle := macosadapter.Lifecycle{Runner: macosadapter.ExecCommandRunner{}, UID: uid, CurrentHome: currentHome, Timeout: 15 * time.Second, Native: *launchctlRequested}
	if action == "install-macos" {
		inspection, inspectErr := inspectCanaryWorkspace(strings.TrimSpace(*workspacePath), root)
		if inspectErr != nil {
			return reportError(errOut, inspectErr)
		}
		program, execErr := exactRunningExecutable(strings.TrimSpace(*executable))
		if execErr != nil {
			return reportError(errOut, execErr)
		}
		spec := canaryLaunchAgentSpec(*home, program, inspection.WorkspaceID)
		existing, enrollmentErr := maintenance.LoadCanaryEnrollment(root)
		freshEnrollment := errors.Is(enrollmentErr, os.ErrNotExist)
		if enrollmentErr != nil && !freshEnrollment {
			return reportError(errOut, fmt.Errorf("inspect existing scheduler enrollment: %w", enrollmentErr))
		}
		if enrollmentErr == nil {
			if existing.WorkspaceID != inspection.WorkspaceID || !samePathCLI(existing.Executable, program) || !samePathCLI(existing.Home, *home) {
				return reportError(errOut, errors.New("the single per-user scheduler is already bound to a different workspace, executable or home; uninstall it explicitly before rebinding"))
			}
			fileStatus, statusErr := macosadapter.ReadStatus(*home, canaryLaunchAgentLabel)
			if statusErr != nil {
				return reportError(errOut, statusErr)
			}
			if fileStatus.State != "not_installed" {
				if _, verifyErr := macosadapter.Verify(*home, spec); verifyErr != nil {
					return reportError(errOut, verifyErr)
				}
			}
		} else {
			fileStatus, statusErr := macosadapter.ReadStatus(*home, canaryLaunchAgentLabel)
			if statusErr != nil {
				return reportError(errOut, statusErr)
			}
			if fileStatus.State != "not_installed" {
				return reportError(errOut, errors.New("an unbound Maestro LaunchAgent plist already exists; refusing to replace it"))
			}
		}
		timezone := currentTimezone()
		mode := "filesystem_only"
		if lifecycle.Native {
			mode = "native"
		} else if enrollmentErr == nil && existing.Mode == "native" {
			mode = "native"
		}
		enrolledAt := time.Now().In(mustLoadTimezone(timezone))
		if enrollmentErr == nil {
			enrolledAt = existing.EnrolledAt
		}
		enrollment := maintenance.CanaryEnrollment{SchemaVersion: maintenance.EnrollmentSchemaVersion, WorkspaceID: inspection.WorkspaceID, AgentID: "darwin", Home: filepath.Clean(*home), Executable: program, UID: uid, Timezone: timezone, LaunchAgentLabel: canaryLaunchAgentLabel, Mode: mode, EnrolledAt: enrolledAt, Activated: []maintenance.Activation{{JobID: maintenance.MemoryCheckpointJobID, QualificationDigest: maintenance.QualificationDigest(maintenance.MemoryCheckpointJobID)}, {JobID: maintenance.MemoryLightDreamJobID, QualificationDigest: maintenance.QualificationDigest(maintenance.MemoryLightDreamJobID)}, {JobID: darwin.HousekeepingJobID, QualificationDigest: maintenance.QualificationDigest(darwin.HousekeepingJobID)}, {JobID: "darwin-deep-weekly", QualificationDigest: maintenance.QualificationDigest("darwin-deep-weekly")}}}
		// RunAtLoad may invoke the worker as soon as launchctl bootstraps the
		// plist. Persist the exact enrollment first, so that first wake sees the
		// same bounded authority as all subsequent interval wakes. On a failed
		// first install we remove this provisional record together with the
		// lifecycle so no authority survives a partial activation.
		if freshEnrollment {
			if err := maintenance.SaveCanaryEnrollment(root, enrollment); err != nil {
				return reportError(errOut, err)
			}
		}
		status, installErr := lifecycle.Install(context.Background(), *home, spec, true)
		if installErr != nil {
			if freshEnrollment {
				_ = maintenance.DeleteCanaryEnrollment(root)
			}
			return reportError(errOut, installErr)
		}
		if status.Label != canaryLaunchAgentLabel {
			if freshEnrollment {
				_ = lifecycle.Uninstall(context.Background(), *home, status.Label)
				_ = maintenance.DeleteCanaryEnrollment(root)
			}
			return reportError(errOut, errors.New("LaunchAgent returned an unexpected managed label"))
		}
		if !freshEnrollment {
			if err := maintenance.SaveCanaryEnrollment(root, enrollment); err != nil {
				return reportError(errOut, err)
			}
		}
		return writeJSON(out, map[string]any{"state": "enrolled", "binding_state": "exact", "model_backed_capabilities": "unavailable", "enrollment": enrollment, "launch_agent": status}, errOut)
	}
	enrollment, enrollmentErr := maintenance.LoadCanaryEnrollment(root)
	if enrollmentErr == nil && !homeProvided {
		*home = enrollment.Home
	}
	// A native enrollment is the durable record of the owner's prior opt-in.
	// Status may therefore inspect launchctl read-only without another flag;
	// lifecycle mutations still require the explicit --launchctl authority.
	lifecycle.Native = shouldObserveNativeEnrollment(action, *launchctlRequested, enrollment, enrollmentErr, *home, currentHome)
	fileStatus, fileErr := macosadapter.ReadStatus(*home, canaryLaunchAgentLabel)
	if fileErr != nil {
		return reportError(errOut, fileErr)
	}
	if enrollmentErr != nil && !errors.Is(enrollmentErr, os.ErrNotExist) {
		return reportError(errOut, fmt.Errorf("inspect scheduler enrollment: %w", enrollmentErr))
	}
	if enrollmentErr != nil && fileStatus.State != "not_installed" {
		return reportError(errOut, errors.New("LaunchAgent plist is present without its exact scheduler enrollment binding"))
	}
	if enrollmentErr == nil && fileStatus.State != "not_installed" {
		resolvedExecutable, executableErr := macosadapter.ResolveExecutable(enrollment.Executable)
		if executableErr != nil || !samePathCLI(resolvedExecutable, enrollment.Executable) {
			if executableErr == nil {
				executableErr = errors.New("enrolled scheduler executable no longer resolves to its exact path")
			}
			return reportError(errOut, executableErr)
		}
		if _, verifyErr := macosadapter.Verify(*home, canaryLaunchAgentSpec(*home, enrollment.Executable, enrollment.WorkspaceID)); verifyErr != nil {
			return reportError(errOut, verifyErr)
		}
	}
	if enrollmentErr == nil && enrollment.Mode == "native" && action != "status" && !*launchctlRequested {
		return reportError(errOut, errors.New("this scheduler was loaded with launchctl; pause, resume or uninstall requires explicit --launchctl so no orphan service is left behind"))
	}
	var status macosadapter.LaunchAgentStatus
	switch action {
	case "status":
		status, err = lifecycle.Status(context.Background(), *home, canaryLaunchAgentLabel)
	case "pause":
		status, err = lifecycle.Pause(context.Background(), *home, canaryLaunchAgentLabel)
	case "resume":
		status, err = lifecycle.Resume(context.Background(), *home, canaryLaunchAgentLabel)
	case "uninstall":
		err = lifecycle.Uninstall(context.Background(), *home, canaryLaunchAgentLabel)
		if err == nil {
			_ = maintenance.DeleteCanaryEnrollment(root)
			status = macosadapter.LaunchAgentStatus{State: "not_present", Label: canaryLaunchAgentLabel}
		}
	}
	if err != nil {
		return reportError(errOut, err)
	}
	if enrollmentErr == nil && *launchctlRequested {
		switch action {
		case "pause":
			enrollment.Mode = "filesystem_only"
		case "resume":
			enrollment.Mode = "native"
		}
		if action == "pause" || action == "resume" {
			if saveErr := maintenance.SaveCanaryEnrollment(root, enrollment); saveErr != nil {
				return reportError(errOut, errors.New("LaunchAgent lifecycle changed but its enrollment audit could not be updated"))
			}
		}
	}
	result := map[string]any{"launch_agent": status, "enrollment_present": enrollmentErr == nil, "native_qualified": status.NativeQualified, "binding_state": map[bool]string{true: "exact", false: "not_present"}[enrollmentErr == nil && fileStatus.State != "not_installed"], "model_backed_capabilities": "unavailable"}
	if enrollmentErr == nil {
		result["timezone"] = enrollment.Timezone
		result["activated_jobs"] = enrollment.Activated
		result["workspace_id"] = enrollment.WorkspaceID
		result["maintenance"] = maintenanceRuntimeStatus(root, catalog, enrollment)
	}
	return writeJSON(out, result, errOut)
}

func canaryDataRoot(home, currentHome string) (string, error) {
	if samePathCLI(home, currentHome) {
		return defaultDataRoot()
	}
	// A non-current --home is always an isolated fixture. Never use the
	// process user's LOCALAPPDATA/XDG state for it, including on Windows.
	if runtime.GOOS == "windows" {
		return filepath.Join(home, "AppData", "Local", "BCGOS"), nil
	}
	return workspace.DefaultDataRoot(runtime.GOOS, home, "", "")
}

const canaryLaunchAgentLabel = "com.bcg.maestro.maintenance"

func canaryLaunchAgentSpec(home, executable, workspaceID string) macosadapter.Spec {
	logRoot := filepath.Join(home, "Library", "Logs")
	return macosadapter.Spec{
		Label:           canaryLaunchAgentLabel,
		Program:         executable,
		Arguments:       []string{"maintenance", "wake", "--trigger", "presence", "--workspace", workspaceID, "--idle-state", "auto"},
		StartInterval:   900,
		RunAtLoad:       true,
		StandardOutPath: filepath.Join(logRoot, "bcgos-maintenance.stdout.log"),
		StandardErrPath: filepath.Join(logRoot, "bcgos-maintenance.stderr.log"),
	}
}

const maestroIdleThreshold = 5 * time.Minute

func observeNativeIdle(ctx context.Context) (maintenance.IdleState, string) {
	state, idleFor, err := macosadapter.ObserveIdle(ctx, maestroIdleThreshold)
	if err != nil {
		return maintenance.IdleUnknown, "native_unavailable_fail_closed"
	}
	if state == macosadapter.NativeIdleConfirmed {
		return maintenance.IdleConfirmed, fmt.Sprintf("native_hid_idle:%ds", int64(idleFor/time.Second))
	}
	return maintenance.IdleActive, fmt.Sprintf("native_hid_active:%ds", int64(idleFor/time.Second))
}

func exactRunningExecutable(requested string) (string, error) {
	running, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve running bcgos executable: %w", err)
	}
	running, err = macosadapter.ResolveExecutable(running)
	if err != nil {
		return "", err
	}
	if requested == "" {
		return running, nil
	}
	exact, err := macosadapter.ResolveExecutable(requested)
	if err != nil {
		return "", err
	}
	if !samePathCLI(exact, running) {
		return "", errors.New("--executable must identify the exact running installed bcgos executable")
	}
	return exact, nil
}

func inspectCanaryWorkspace(path, dataRoot string) (workspace.Inspection, error) {
	if path == "" {
		return workspace.Inspection{}, errors.New("install-macos requires --workspace-path for an initialized workspace")
	}
	inspection, err := workspace.Inspect(path, dataRoot)
	if err != nil {
		return workspace.Inspection{}, fmt.Errorf("inspect initialized workspace: %w", err)
	}
	if (inspection.State != "ready" && inspection.State != "warning") || inspection.WorkspaceID == "" {
		return workspace.Inspection{}, errors.New("workspace must be initialized and readable before scheduler enrollment")
	}
	return inspection, nil
}

func maintenanceRuntimeStatus(root string, catalog maintenance.Catalog, enrollment maintenance.CanaryEnrollment) map[string]any {
	store := scheduler.Store{Root: filepath.Join(root, "maintenance", "scheduler")}
	_, activated := maintenance.ActivationMaps(enrollment)
	active := make(map[string]bool, len(activated))
	for _, jobID := range activated {
		active[jobID] = true
	}
	catalogIDs := make(map[string]bool, len(catalog.Jobs))
	for _, job := range catalog.Jobs {
		catalogIDs[job.ID] = true
	}
	unavailableJobs := []string{}
	allPresenceJobs := schedulerJobsForTrigger("presence")
	plannedJobs := activatedSchedulerJobs(allPresenceJobs, activated)
	for _, job := range allPresenceJobs {
		if !active[job.ID] || !catalogIDs[job.ID] {
			unavailableJobs = append(unavailableJobs, job.ID)
		}
	}
	result := map[string]any{"due_count": 0, "unavailable_count": 0, "unavailable_jobs": unavailableJobs, "last_receipt": nil}
	quarantined, quarantineErr := store.QuarantinedLeases(enrollment.WorkspaceID)
	result["quarantined_count"] = len(quarantined)
	if quarantineErr == nil {
		metadata := make([]map[string]any, 0, len(quarantined))
		for _, lease := range quarantined {
			metadata = append(metadata, map[string]any{"job_id": lease.JobID, "occurrence_key": lease.OccurrenceKey, "owner_id": lease.OwnerID, "expires_at": lease.ExpiresAt.UTC()})
		}
		result["quarantined"] = metadata
	} else {
		result["quarantine_state"] = "unavailable"
	}
	receipts, err := store.Receipts(enrollment.WorkspaceID)
	if err != nil {
		return result
	}
	last := map[string]any{}
	var lastAttemptedAt time.Time
	schedulerAuditIncomplete := 0
	for _, receipt := range receipts {
		if receipt.State == scheduler.Unavailable {
			result["unavailable_count"] = result["unavailable_count"].(int) + 1
		}
		if receipt.Error == "recovery_committed_audit_incomplete" {
			schedulerAuditIncomplete++
		}
		if receipt.AttemptedAt.After(lastAttemptedAt) {
			lastAttemptedAt = receipt.AttemptedAt
			last = map[string]any{"job_id": receipt.JobID, "state": receipt.State, "scheduled_for": receipt.ScheduledFor.UTC(), "attempted_at": receipt.AttemptedAt.UTC()}
		}
	}
	if len(last) > 0 {
		result["last_receipt"] = last
	}
	maintenanceStore := maintenance.Store{Root: filepath.Join(root, "maintenance", "receipts")}
	recoveryRequired, auditIncomplete, recoveryIntents := 0, schedulerAuditIncomplete, 0
	for _, job := range allPresenceJobs {
		maintenanceReceipts, readErr := maintenanceStore.Receipts(enrollment.WorkspaceID, job.ID)
		if readErr != nil {
			continue
		}
		for _, receipt := range maintenanceReceipts {
			if receipt.State == maintenance.ReceiptRecoveryRequired {
				recoveryRequired++
			}
			switch receipt.RecoveryPhase {
			case "intent":
				recoveryIntents++
			case "audit_incomplete":
				auditIncomplete++
			}
		}
	}
	result["recovery_required_count"] = recoveryRequired
	result["recovery_intent_count"] = recoveryIntents
	result["recovery_audit_incomplete_count"] = auditIncomplete
	path := filepath.Join(root, "maintenance", "scheduler", "workspaces", enrollment.WorkspaceID, "enrollment.json")
	if body, readErr := os.ReadFile(path); readErr == nil {
		var schedulerEnrollment scheduler.Enrollment
		if json.Unmarshal(body, &schedulerEnrollment) == nil {
			if due, dueErr := scheduler.PlanDue(plannedJobs, schedulerEnrollment.EnrolledAt, receipts, time.Now().In(mustLoadTimezone(enrollment.Timezone))); dueErr == nil {
				result["due_count"] = len(due)
			}
		}
	}
	return result
}

func currentTimezone() string {
	if value := strings.TrimSpace(os.Getenv("TZ")); value != "" {
		if _, err := time.LoadLocation(value); err == nil {
			return value
		}
	}
	if link, err := os.Readlink("/etc/localtime"); err == nil {
		if index := strings.Index(link, "/zoneinfo/"); index >= 0 {
			value := link[index+len("/zoneinfo/"):]
			if _, err := time.LoadLocation(value); err == nil {
				return value
			}
		}
	}
	if value := time.Now().Location().String(); value != "" && value != "Local" {
		if _, err := time.LoadLocation(value); err == nil {
			return value
		}
	}
	return "UTC"
}

func mustLoadTimezone(value string) *time.Location {
	location, err := time.LoadLocation(value)
	if err != nil {
		return time.UTC
	}
	return location
}
func samePathCLI(left, right string) bool {
	left, _ = filepath.Abs(left)
	right, _ = filepath.Abs(right)
	return filepath.Clean(left) == filepath.Clean(right)
}

func maintenanceStatus(catalog maintenance.Catalog) map[string]any {
	result := map[string]any{
		"schema_version":                        catalog.SchemaVersion,
		"catalog_state":                         catalog.CatalogState,
		"executor_state":                        "runtime_worker_ready_for_explicit_qualified_handlers",
		"native_adapters":                       "macos_adapter_available_windows_unavailable",
		"automatic_maintenance":                 "presence_wake_derives_due_work_only",
		"darwin_agent_id":                       "darwin",
		"darwin_scope":                          "health/maestro-system",
		"interactive_and_housekeeping_identity": "darwin",
		"canary_activation":                     "attended_local_only",
		"native_schedulers":                     "disabled_until_explicit_install_and_qualification",
		"idle_eligibility":                      "explicit_evidence_required_unknown_fails_closed",
		"memory_checkpoint":                     "locally_qualified_only_after_canary_enrollment",
		"memory_dreaming":                       "daily_light_locally_qualified_weekly_deep_unavailable",
		"pulse_interval_seconds":                900,
		"job_count":                             len(catalog.Jobs),
		"reason":                                "unavailable model-backed jobs remain due; wake receipts never prove execution",
	}
	root, rootErr := defaultDataRoot()
	if rootErr != nil {
		result["enrollment_present"] = false
		result["lifecycle_status"] = map[string]any{"state": "data_root_unavailable"}
		return result
	}
	enrollment, enrollmentErr := maintenance.LoadCanaryEnrollment(root)
	result["enrollment_present"] = enrollmentErr == nil
	if enrollmentErr != nil {
		result["lifecycle_status"] = map[string]any{"state": "not_enrolled", "native_qualified": false}
		return result
	}
	result["timezone"] = enrollment.Timezone
	result["activated_jobs"] = enrollment.Activated
	currentHome, homeErr := os.UserHomeDir()
	uid, uidErr := macosadapter.CurrentUID()
	if homeErr != nil || uidErr != nil {
		result["lifecycle_status"] = map[string]any{"state": "current_user_unavailable", "native_qualified": false}
		return result
	}
	// Native enrollment already records attended opt-in. Aggregate status may
	// inspect that service read-only so it does not misreport a loaded scheduler
	// as pending merely because the caller omitted an implementation flag.
	lifecycle := macosadapter.Lifecycle{Runner: macosadapter.ExecCommandRunner{}, UID: uid, CurrentHome: currentHome, Timeout: 15 * time.Second, Native: enrollment.Mode == "native" && samePathCLI(enrollment.Home, currentHome)}
	if _, executableErr := macosadapter.ResolveExecutable(enrollment.Executable); executableErr != nil {
		result["lifecycle_status"] = map[string]any{"state": "executable_binding_invalid", "native_qualified": false}
		return result
	}
	if _, bindingErr := macosadapter.Verify(enrollment.Home, canaryLaunchAgentSpec(enrollment.Home, enrollment.Executable, enrollment.WorkspaceID)); bindingErr != nil {
		result["lifecycle_status"] = map[string]any{"state": "plist_binding_invalid", "native_qualified": false}
		return result
	}
	status, statusErr := lifecycle.Status(context.Background(), enrollment.Home, enrollment.LaunchAgentLabel)
	if statusErr != nil {
		result["lifecycle_status"] = map[string]any{"state": "status_unavailable", "native_qualified": false}
	} else {
		result["lifecycle_status"] = status
	}
	result["maintenance"] = maintenanceRuntimeStatus(root, catalog, enrollment)
	return result
}

func shouldObserveNativeEnrollment(action string, requested bool, enrollment maintenance.CanaryEnrollment, enrollmentErr error, home, currentHome string) bool {
	if requested {
		return true
	}
	return action == "status" && enrollmentErr == nil && enrollment.Mode == "native" && samePathCLI(enrollment.Home, home) && samePathCLI(home, currentHome)
}

func schedulerJobsForTrigger(trigger string) []scheduler.Job {
	checkpoint := scheduler.Job{ID: maintenance.MemoryCheckpointJobID, Cadence: scheduler.Interval, IntervalHours: 3, MaxCatchUp: 1}
	lightDream := scheduler.Job{ID: maintenance.MemoryLightDreamJobID, Cadence: scheduler.Interval, IntervalHours: 3, MaxCatchUp: 1}
	deepDream := scheduler.Job{ID: maintenance.MemoryDeepDreamJobID, Cadence: scheduler.Weekly, Weekday: time.Sunday, LocalHour: 2, MaxCatchUp: 1}
	daily := scheduler.Job{ID: "darwin-housekeeping-daily", Cadence: scheduler.Daily, LocalHour: 3, MaxCatchUp: 1}
	weekly := scheduler.Job{ID: "darwin-deep-weekly", Cadence: scheduler.Weekly, Weekday: time.Sunday, LocalHour: 4, MaxCatchUp: 1}
	walter := scheduler.Job{ID: "walter-self-review-weekly", Cadence: scheduler.Weekly, Weekday: time.Sunday, LocalHour: 5, MaxCatchUp: 1}
	monthly := scheduler.Job{ID: "darwin-structural-evolution-proposal", Cadence: scheduler.Monthly, DayOfMonth: 1, LocalHour: 5, MaxCatchUp: 1}
	switch trigger {
	case "daily":
		return []scheduler.Job{daily}
	case "weekly":
		return []scheduler.Job{deepDream, weekly, walter}
	case "monthly":
		return []scheduler.Job{monthly}
	case "event":
		return nil
	default:
		return []scheduler.Job{checkpoint, lightDream, deepDream, daily, weekly, walter, monthly}
	}
}

func activatedSchedulerJobs(jobs []scheduler.Job, activated []string) []scheduler.Job {
	active := make(map[string]bool, len(activated))
	for _, jobID := range activated {
		active[jobID] = true
	}
	filtered := make([]scheduler.Job, 0, len(jobs))
	for _, job := range jobs {
		if active[job.ID] {
			filtered = append(filtered, job)
		}
	}
	return filtered
}
