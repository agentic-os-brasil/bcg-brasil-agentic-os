package cli

import (
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspacemigration"
)

func runWorkspaceMigration(args []string, out, errOut io.Writer, dataRoot func() (string, error)) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		fmt.Fprintln(out, "usage: bcgos workspace-migration status --runtime claude|codex [workspace-path]")
		return ExitOK
	}
	if args[0] != "status" {
		fmt.Fprintln(errOut, "usage: bcgos workspace-migration status --runtime claude|codex [workspace-path]")
		return ExitUsage
	}
	flags := flag.NewFlagSet("workspace-migration status", flag.ContinueOnError)
	flags.SetOutput(errOut)
	runtimeName := flags.String("runtime", "", "workspace runtime")
	if err := flags.Parse(args[1:]); err != nil || len(flags.Args()) > 1 || strings.TrimSpace(*runtimeName) == "" {
		fmt.Fprintln(errOut, "usage: bcgos workspace-migration status --runtime claude|codex [workspace-path]")
		return ExitUsage
	}
	root, err := dataRoot()
	if err != nil {
		return reportError(errOut, err)
	}
	path := "."
	if len(flags.Args()) == 1 {
		path = flags.Args()[0]
	}
	inspection, err := workspacemigration.Inspect(workspacemigration.PlanOptions{WorkspacePath: path, DataRoot: root, Runtime: *runtimeName})
	if err != nil {
		return reportError(errOut, err)
	}
	return writeJSON(out, struct {
		Capability workspacemigration.Status     `json:"capability"`
		Inspection workspacemigration.Inspection `json:"inspection"`
	}{Capability: workspacemigration.CapabilityStatus(), Inspection: inspection}, errOut)
}
