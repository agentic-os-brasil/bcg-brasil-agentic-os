package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	baseruntime "github.com/agentic-os-brasil/bcg-brasil-agentic-os/bundles/base/runtime"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/macosadapter"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/maintenance"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

// runMaintenance exposes the platform-neutral maintenance contract to native
// adapters and humans. It is intentionally read-only while executors remain
// unavailable; a catalog or adapter presence is not evidence of execution.
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
		jobs := schedulerJobsForTrigger(strings.TrimSpace(*trigger))
		worker := maintenance.Worker{Catalog: catalog, Scheduler: scheduler.Store{Root: filepath.Join(root, "maintenance", "scheduler")}, Receipts: maintenance.Store{Root: filepath.Join(root, "maintenance", "receipts")}, Jobs: jobs, Handlers: map[string]maintenance.Handler{}, Deadline: 2 * time.Minute}
		report, err := worker.Run(context.Background(), maintenance.WakeRequest{WorkspaceID: strings.TrimSpace(*workspace), OwnerID: "bcgos-presence", Now: time.Now().UTC(), Attended: *attended})
		if err != nil {
			_ = writeJSON(out, map[string]any{"schema_version": 1, "state": maintenance.Unavailable, "agent_id": "darwin", "scope": "health/maestro-system", "trigger": *trigger, "native_schedulers": "disabled", "reason": err.Error() + "; no receipt was emitted"}, errOut)
			return ExitUnavailable
		}
		wakeState, wakeReason, exitCode := report.State, "", ExitOK
		if len(worker.Handlers) == 0 {
			wakeState, wakeReason, exitCode = maintenance.Unavailable, "no qualified local handlers are enrolled; no receipt was emitted by this wake", ExitUnavailable
		}
		for _, receipt := range report.Receipts {
			if receipt.State == maintenance.ReceiptUnavailable {
				exitCode, wakeState, wakeReason = ExitUnavailable, maintenance.Unavailable, "unavailable work remains due; its receipt is not scheduler success"
			}
		}
		code := writeJSON(out, map[string]any{"state": wakeState, "reason": wakeReason, "trigger": *trigger, "native_schedulers": "disabled_until_explicit_install_and_qualification", "worker": report}, errOut)
		if code != ExitOK {
			return code
		}
		return exitCode
	case "canary":
		flags := newFlagSet("maintenance canary", errOut)
		installMacOS := flags.Bool("install-macos", false, "explicitly install the per-user LaunchAgent adapter")
		home := flags.String("home", "", "isolated user home fixture")
		if err := flags.Parse(args[1:]); err != nil || rejectPositionals(flags, errOut) {
			fmt.Fprintln(errOut, "usage: bcgos maintenance canary [--install-macos --home PATH]")
			return ExitUsage
		}
		if !*installMacOS {
			return writeJSON(out, map[string]any{"schema_version": 1, "state": "attended_local_only", "agent_id": "darwin", "scope": "health/maestro-system", "interactive_and_housekeeping_identity": "darwin", "native_schedulers": "disabled", "worker_invocation": "qualified_housekeeping_executor", "model_inline": false}, errOut)
		}
		if runtime.GOOS != "darwin" {
			return reportError(errOut, errors.New("macOS LaunchAgent install is unavailable on this platform"))
		}
		if strings.TrimSpace(*home) == "" {
			*home, _ = os.UserHomeDir()
		}
		executable, err := os.Executable()
		if err != nil {
			return reportError(errOut, err)
		}
		logRoot := filepath.Join(*home, "Library", "Logs")
		status, err := macosadapter.Install(*home, macosadapter.Spec{Label: "com.bcg.maestro.maintenance", Program: executable, Arguments: []string{"maintenance", "wake", "--trigger", "presence"}, StartInterval: 900, RunAtLoad: true, StandardOutPath: filepath.Join(logRoot, "bcgos-maintenance.stdout.log"), StandardErrPath: filepath.Join(logRoot, "bcgos-maintenance.stderr.log")}, true)
		if err != nil {
			return reportError(errOut, err)
		}
		return writeJSON(out, status, errOut)
	default:
		fmt.Fprintln(errOut, "usage: bcgos maintenance <catalog|status|wake|canary> [--trigger presence|daily|weekly|monthly|event]")
		return ExitUsage
	}
}

func maintenanceStatus(catalog maintenance.Catalog) map[string]any {
	return map[string]any{
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
