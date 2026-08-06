package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspaceimport"
)

func runWorkspaceImport(args []string, out, errOut io.Writer, dataRoot func() (string, error)) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		fmt.Fprintln(out, "usage: bcgos workspace import <inspect|plan|approve|execute|rollback>")
		return ExitOK
	}
	if args[0] != "import" {
		fmt.Fprintln(errOut, "usage: bcgos workspace import <inspect|plan|approve|execute|rollback>")
		return ExitUsage
	}
	if len(args) < 2 {
		fmt.Fprintln(errOut, "usage: bcgos workspace import <inspect|plan|approve|execute|rollback>")
		return ExitUsage
	}
	return runWorkspaceImportCommand(args[1:], out, errOut, dataRoot)
}

func runWorkspaceImportCommand(args []string, out, errOut io.Writer, dataRoot func() (string, error)) int {
	command := args[0]
	flags := newFlagSet("workspace import "+command, errOut)
	source := flags.String("source", "", "external workspace source path")
	destination := flags.String("destination", "", "Maestro workspace destination path")
	planPath := flags.String("plan", "", "immutable plan JSON path")
	approvalPath := flags.String("approval", "", "explicit approval JSON path")
	receiptPath := flags.String("receipt", "", "execution receipt JSON path")
	outPath := flags.String("out", "", "write the JSON artifact to this path")
	approvedBy := flags.String("approved-by", "", "human approval identity")
	confirmation := flags.String("confirm", "", "exact confirmation token: IMPORT or ROLLBACK")
	maxEntries := flags.Int("max-entries", 0, "bounded inventory entry limit")
	maxDepth := flags.Int("max-depth", 0, "bounded inventory depth limit")
	maxFileBytes := flags.Int64("max-file-bytes", 0, "bounded per-file limit")
	maxTotalBytes := flags.Int64("max-total-bytes", 0, "bounded total inventory limit")
	if err := flags.Parse(args[1:]); err != nil || rejectPositionals(flags, errOut) {
		return ExitUsage
	}
	limits := workspaceimport.Limits{MaxEntries: *maxEntries, MaxDepth: *maxDepth, MaxFileBytes: *maxFileBytes, MaxTotalBytes: *maxTotalBytes}
	switch command {
	case "inspect":
		if strings.TrimSpace(*source) == "" {
			fmt.Fprintln(errOut, "usage: bcgos workspace import inspect --source PATH")
			return ExitUsage
		}
		inspection, err := workspaceimport.Inspect(*source, limits)
		if err != nil {
			_ = writeJSON(out, inspection, errOut)
			return ExitUsage
		}
		return writeJSON(out, inspection, errOut)
	case "plan":
		if strings.TrimSpace(*source) == "" || strings.TrimSpace(*destination) == "" {
			fmt.Fprintln(errOut, "usage: bcgos workspace import plan --source PATH --destination PATH [--out PATH]")
			return ExitUsage
		}
		plan, err := workspaceimport.BuildPlan(*source, *destination, limits)
		if err != nil {
			return reportError(errOut, err)
		}
		if *outPath != "" {
			if err := workspaceimport.SavePlan(*outPath, plan); err != nil {
				return reportError(errOut, err)
			}
		}
		return writeJSON(out, plan, errOut)
	case "approve":
		if strings.TrimSpace(*planPath) == "" || strings.TrimSpace(*approvedBy) == "" || *confirmation != workspaceimport.ConfirmImport {
			fmt.Fprintln(errOut, "usage: bcgos workspace import approve --plan PATH --approved-by ID --confirm IMPORT [--out PATH]")
			return ExitUsage
		}
		plan, err := workspaceimport.ReadPlan(*planPath)
		if err != nil {
			return reportError(errOut, err)
		}
		approval, err := workspaceimport.Approve(plan, *approvedBy, *confirmation)
		if err != nil {
			return reportError(errOut, err)
		}
		if *outPath != "" {
			if err := workspaceimport.SaveApproval(*outPath, approval); err != nil {
				return reportError(errOut, err)
			}
		}
		return writeJSON(out, approval, errOut)
	case "execute":
		if strings.TrimSpace(*planPath) == "" || strings.TrimSpace(*approvalPath) == "" {
			fmt.Fprintln(errOut, "usage: bcgos workspace import execute --plan PATH --approval PATH")
			return ExitUsage
		}
		root, err := dataRoot()
		if err != nil {
			return reportError(errOut, err)
		}
		plan, err := workspaceimport.ReadPlan(*planPath)
		if err != nil {
			return reportError(errOut, err)
		}
		approval, err := workspaceimport.ReadApproval(*approvalPath)
		if err != nil {
			return reportError(errOut, err)
		}
		receipt, err := workspaceimport.Execute(root, plan, approval)
		if err != nil {
			_ = writeJSON(out, receipt, errOut)
			return ExitFailure
		}
		return writeJSON(out, receipt, errOut)
	case "rollback":
		if strings.TrimSpace(*planPath) == "" || strings.TrimSpace(*receiptPath) == "" || *confirmation != workspaceimport.ConfirmRollback {
			fmt.Fprintln(errOut, "usage: bcgos workspace import rollback --plan PATH --receipt PATH --confirm ROLLBACK")
			return ExitUsage
		}
		root, err := dataRoot()
		if err != nil {
			return reportError(errOut, err)
		}
		plan, err := workspaceimport.ReadPlan(*planPath)
		if err != nil {
			return reportError(errOut, err)
		}
		receipt, err := workspaceimport.ReadReceipt(*receiptPath)
		if err != nil {
			return reportError(errOut, err)
		}
		rolled, err := workspaceimport.Rollback(root, plan, receipt, *confirmation)
		if err != nil {
			return reportError(errOut, err)
		}
		return writeJSON(out, rolled, errOut)
	default:
		fmt.Fprintln(errOut, "usage: bcgos workspace import <inspect|plan|approve|execute|rollback>")
		return ExitUsage
	}
}
