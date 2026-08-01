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

	baseruntime "github.com/agentic-os-brasil/bcg-brasil-agentic-os/bundles/base/runtime"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/darwin"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/macosadapter"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/maintenance"
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
		workspace := flags.String("workspace", "maestro-system", "bounded maintenance workspace")
		attended := flags.Bool("attended", false, "grant attended local Canary authority")
		if err := flags.Parse(args[1:]); err != nil || rejectPositionals(flags, errOut) {
			fmt.Fprintln(errOut, "usage: bcgos maintenance wake --trigger presence|daily|weekly|monthly|event [--workspace ID] [--attended]")
			return ExitUsage
		}
		if _, err := catalog.ForTrigger(strings.TrimSpace(*trigger)); err != nil {
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
		jobs := schedulerJobsForTrigger(strings.TrimSpace(*trigger))
		enrollment, enrollmentErr := maintenance.LoadCanaryEnrollment(root)
		if enrollmentErr == nil && (enrollment.WorkspaceID != strings.TrimSpace(*workspace) || !samePathCLI(enrollment.Home, currentHome)) {
			enrollmentErr = errors.New("Canary enrollment is bound to a different workspace or home")
		}
		handlers, qualification, activated := maintenanceHandlers(root, strings.TrimSpace(*workspace), enrollment, enrollmentErr == nil)
		worker := maintenance.Worker{Catalog: catalog, Scheduler: scheduler.Store{Root: filepath.Join(root, "maintenance", "scheduler")}, Receipts: maintenance.Store{Root: filepath.Join(root, "maintenance", "receipts")}, Jobs: jobs, Handlers: handlers, LocalQualification: qualification, ActivatedJobs: activated, Deadline: 2 * time.Minute}
		timezone := ""
		if enrollmentErr == nil {
			timezone = enrollment.Timezone
		}
		report, err := worker.Run(context.Background(), maintenance.WakeRequest{WorkspaceID: strings.TrimSpace(*workspace), Timezone: timezone, OwnerID: "bcgos-presence", Now: time.Now(), Attended: *attended, Preauthorized: enrollmentErr == nil})
		if err != nil {
			_ = writeJSON(out, map[string]any{"schema_version": 1, "state": maintenance.Unavailable, "agent_id": "darwin", "scope": "health/maestro-system", "trigger": *trigger, "native_schedulers": "disabled", "reason": err.Error() + "; no receipt was emitted"}, errOut)
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
		code := writeJSON(out, map[string]any{"state": wakeState, "reason": wakeReason, "trigger": *trigger, "native_schedulers": "disabled_until_explicit_install_and_qualification", "worker": report}, errOut)
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

func maintenanceHandlers(root, workspace string, enrollment maintenance.CanaryEnrollment, enrolled bool) (map[string]maintenance.Handler, map[string]string, []string) {
	handlers := map[string]maintenance.Handler{}
	qualification, activated := map[string]string{}, []string{}
	if !enrolled {
		return handlers, qualification, activated
	}
	qualification, activated = maintenance.ActivationMaps(enrollment)
	schedulerStore := scheduler.Store{Root: filepath.Join(root, "maintenance", "scheduler")}
	builder := darwin.LocalProductHealthBuilder{Scheduler: schedulerStore, Workspace: workspace, Runtime: "runtime-neutral"}
	commandStore := maintenance.Store{Root: filepath.Join(root, "maintenance", "darwin-commands")}
	guard := darwin.ToolGuardFunc(func(call darwin.ToolCall) error {
		if call.Tool != "filesystem" || (call.Operation != "write" && call.Operation != "edit") || !strings.HasPrefix(call.Resource, "bcgos://health/maestro-system/") {
			return errors.New("Darwin Canary grant denied")
		}
		return nil
	})
	proposalStore := darwin.ProposalStore{Root: filepath.Join(root, "maintenance", "darwin-proposals")}
	handlers[darwin.HousekeepingJobID] = darwin.HousekeepingHandler{Build: builder, Guard: guard, Invoker: darwin.FilesystemInvoker{Root: filepath.Join(root, "maintenance", "darwin")}, Store: darwin.Store{Root: filepath.Join(root, "maintenance", "darwin")}, CommandStore: commandStore}
	handlers["darwin-deep-weekly"] = darwin.DeepReviewHandler{Build: builder, CommandStore: commandStore, ProposalStore: proposalStore}
	handlers["walter-self-review-weekly"] = maintenance.WalterWeeklyAdapter{}
	return handlers, qualification, activated
}

func runMaintenanceCanary(args []string, out, errOut io.Writer, catalog maintenance.Catalog) int {
	action := args[0]
	if action != "install-macos" && action != "status" && action != "pause" && action != "resume" && action != "uninstall" && action != "recover-quarantine" {
		fmt.Fprintln(errOut, "usage: bcgos maintenance canary <install-macos|status|pause|resume|uninstall|recover-quarantine> [--confirm] [--home PATH] [--workspace ID]")
		return ExitUsage
	}
	flags := newFlagSet("maintenance canary "+action, errOut)
	confirm := flags.Bool("confirm", false, "explicitly confirm the requested lifecycle mutation")
	home := flags.String("home", "", "current user home or isolated filesystem-only fixture")
	workspace := flags.String("workspace", "maestro-system", "bounded maintenance workspace")
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
	lifecycle := macosadapter.Lifecycle{Runner: macosadapter.ExecCommandRunner{}, UID: uid, CurrentHome: currentHome, Timeout: 15 * time.Second, Native: samePathCLI(*home, currentHome) && runtime.GOOS == "darwin"}
	if action == "install-macos" {
		program := "/usr/local/bin/bcgos"
		if runtime.GOOS == "darwin" {
			executable, execErr := os.Executable()
			if execErr != nil {
				return reportError(errOut, execErr)
			}
			program = executable
		}
		logRoot := filepath.Join(*home, "Library", "Logs")
		status, installErr := lifecycle.Install(context.Background(), *home, macosadapter.Spec{Label: "com.bcg.maestro.maintenance", Program: program, Arguments: []string{"maintenance", "wake", "--trigger", "presence", "--workspace", strings.TrimSpace(*workspace)}, StartInterval: 900, RunAtLoad: true, StandardOutPath: filepath.Join(logRoot, "bcgos-maintenance.stdout.log"), StandardErrPath: filepath.Join(logRoot, "bcgos-maintenance.stderr.log")}, true)
		if installErr != nil {
			return reportError(errOut, installErr)
		}
		timezone := currentTimezone()
		mode := "filesystem_only"
		if lifecycle.Native {
			mode = "native"
		}
		enrollment := maintenance.CanaryEnrollment{SchemaVersion: maintenance.EnrollmentSchemaVersion, WorkspaceID: strings.TrimSpace(*workspace), AgentID: "darwin", Home: filepath.Clean(*home), UID: uid, Timezone: timezone, LaunchAgentLabel: status.Label, Mode: mode, EnrolledAt: time.Now().In(mustLoadTimezone(timezone)), Activated: []maintenance.Activation{{JobID: darwin.HousekeepingJobID, QualificationDigest: maintenance.QualificationDigest(darwin.HousekeepingJobID)}, {JobID: "darwin-deep-weekly", QualificationDigest: maintenance.QualificationDigest("darwin-deep-weekly")}}}
		if err := maintenance.SaveCanaryEnrollment(root, enrollment); err != nil {
			_ = lifecycle.Uninstall(context.Background(), *home, status.Label)
			return reportError(errOut, err)
		}
		return writeJSON(out, map[string]any{"state": "enrolled", "enrollment": enrollment, "launch_agent": status}, errOut)
	}
	enrollment, enrollmentErr := maintenance.LoadCanaryEnrollment(root)
	if enrollmentErr == nil && !homeProvided {
		*home = enrollment.Home
		lifecycle.Native = enrollment.Mode == "native" && samePathCLI(*home, currentHome) && runtime.GOOS == "darwin"
	}
	var status macosadapter.LaunchAgentStatus
	switch action {
	case "status":
		status, err = lifecycle.Status(context.Background(), *home, "com.bcg.maestro.maintenance")
	case "pause":
		status, err = lifecycle.Pause(context.Background(), *home, "com.bcg.maestro.maintenance")
	case "resume":
		status, err = lifecycle.Resume(context.Background(), *home, "com.bcg.maestro.maintenance")
	case "uninstall":
		err = lifecycle.Uninstall(context.Background(), *home, "com.bcg.maestro.maintenance")
		if err == nil {
			_ = maintenance.DeleteCanaryEnrollment(root)
			status = macosadapter.LaunchAgentStatus{State: "not_present", Label: "com.bcg.maestro.maintenance"}
		}
	}
	if err != nil {
		return reportError(errOut, err)
	}
	result := map[string]any{"launch_agent": status, "enrollment_present": enrollmentErr == nil, "native_qualified": status.NativeQualified}
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
	for _, job := range schedulerJobsForTrigger("presence") {
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
	for _, receipt := range receipts {
		if receipt.State == scheduler.Unavailable {
			result["unavailable_count"] = result["unavailable_count"].(int) + 1
		}
		if receipt.AttemptedAt.After(time.Time{}) {
			last = map[string]any{"job_id": receipt.JobID, "state": receipt.State, "scheduled_for": receipt.ScheduledFor.UTC(), "attempted_at": receipt.AttemptedAt.UTC()}
		}
	}
	if len(last) > 0 {
		result["last_receipt"] = last
	}
	maintenanceStore := maintenance.Store{Root: filepath.Join(root, "maintenance", "receipts")}
	recoveryRequired, auditIncomplete, recoveryIntents := 0, 0, 0
	for _, job := range schedulerJobsForTrigger("presence") {
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
			if due, dueErr := scheduler.PlanDue(schedulerJobsForTrigger("presence"), schedulerEnrollment.EnrolledAt, receipts, time.Now().In(mustLoadTimezone(enrollment.Timezone))); dueErr == nil {
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
	lifecycle := macosadapter.Lifecycle{Runner: macosadapter.ExecCommandRunner{}, UID: uid, CurrentHome: currentHome, Timeout: 15 * time.Second, Native: enrollment.Mode == "native" && runtime.GOOS == "darwin" && samePathCLI(enrollment.Home, currentHome)}
	status, statusErr := lifecycle.Status(context.Background(), enrollment.Home, enrollment.LaunchAgentLabel)
	if statusErr != nil {
		result["lifecycle_status"] = map[string]any{"state": "status_unavailable", "native_qualified": false}
	} else {
		result["lifecycle_status"] = status
	}
	result["maintenance"] = maintenanceRuntimeStatus(root, catalog, enrollment)
	return result
}

func schedulerJobsForTrigger(trigger string) []scheduler.Job {
	daily := scheduler.Job{ID: "darwin-housekeeping-daily", Cadence: scheduler.Daily, LocalHour: 3, MaxCatchUp: 1}
	weekly := scheduler.Job{ID: "darwin-deep-weekly", Cadence: scheduler.Weekly, Weekday: time.Sunday, LocalHour: 4, MaxCatchUp: 1}
	walter := scheduler.Job{ID: "walter-self-review-weekly", Cadence: scheduler.Weekly, Weekday: time.Sunday, LocalHour: 5, MaxCatchUp: 1}
	monthly := scheduler.Job{ID: "darwin-structural-evolution-proposal", Cadence: scheduler.Monthly, DayOfMonth: 1, LocalHour: 5, MaxCatchUp: 1}
	switch trigger {
	case "daily":
		return []scheduler.Job{daily}
	case "weekly":
		return []scheduler.Job{weekly, walter}
	case "monthly":
		return []scheduler.Job{monthly}
	case "event":
		return nil
	default:
		return []scheduler.Job{daily, weekly, walter, monthly}
	}
}
