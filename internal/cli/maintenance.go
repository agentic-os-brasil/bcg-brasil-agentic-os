package cli

import (
	"fmt"
	"io"
	"strings"

	baseruntime "github.com/agentic-os-brasil/bcg-brasil-agentic-os/bundles/base/runtime"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/maintenance"
)

// runMaintenance exposes the platform-neutral maintenance contract to native
// adapters and humans. It is intentionally read-only while executors remain
// unavailable; a catalog or adapter presence is not evidence of execution.
func runMaintenance(args []string, out, errOut io.Writer) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		fmt.Fprintln(out, "usage: bcgos maintenance <catalog|status|wake> [--trigger presence|daily|weekly|monthly|event]")
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
		if err := flags.Parse(args[1:]); err != nil || rejectPositionals(flags, errOut) {
			fmt.Fprintln(errOut, "usage: bcgos maintenance wake --trigger presence|daily|weekly|monthly|event")
			return ExitUsage
		}
		jobs, err := catalog.ForTrigger(strings.TrimSpace(*trigger))
		if err != nil {
			return reportError(errOut, err)
		}
		return writeUnavailableMaintenance(out, errOut, *trigger, jobs)
	default:
		fmt.Fprintln(errOut, "usage: bcgos maintenance <catalog|status|wake> [--trigger presence|daily|weekly|monthly|event]")
		return ExitUsage
	}
}

func maintenanceStatus(catalog maintenance.Catalog) map[string]any {
	return map[string]any{
		"schema_version":        catalog.SchemaVersion,
		"catalog_state":         catalog.CatalogState,
		"executor_state":        maintenance.Unavailable,
		"native_adapters":       maintenance.Unavailable,
		"automatic_maintenance": "prebuilt_contract_only",
		"job_count":             len(catalog.Jobs),
		"reason":                "owning maintenance executors and native scheduler installation are not yet qualified",
	}
}

func writeUnavailableMaintenance(out, errOut io.Writer, trigger string, jobs []maintenance.Job) int {
	code := writeJSON(out, map[string]any{
		"schema_version": 1,
		"state":          maintenance.Unavailable,
		"trigger":        trigger,
		"planned_jobs":   jobs,
		"reason":         "native maintenance executor is not installed; no work was run and no receipt was emitted",
	}, errOut)
	if code != ExitOK {
		return code
	}
	return ExitUnavailable
}
