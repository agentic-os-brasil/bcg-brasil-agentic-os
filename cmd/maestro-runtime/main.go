// maestro-runtime is the managed ZIP helper, not an end-user CLI.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/userlevel"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/zipmigration"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/zipruntime"
)

func run(args []string, in io.Reader, out, diagnostic io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(diagnostic, "managed runtime command required")
		return 2
	}
	if len(args) == 1 && args[0] == "preflight" {
		if err := userlevel.EnsureNotElevated(); err != nil {
			fmt.Fprintln(diagnostic, err)
			return 2
		}
		fmt.Fprintln(out, `{"state":"non_elevated","native_qualification":"unattested"}`)
		return 0
	}
	if args[0] != "migration" {
		if err := zipruntime.Run(args, in, out); err != nil {
			fmt.Fprintln(diagnostic, err)
			return 2
		}
		return 0
	}
	flags := flag.NewFlagSet("migration", flag.ContinueOnError)
	flags.SetOutput(diagnostic)
	project := flags.String("project", "", "installation root")
	dry := flags.Bool("dry-run", false, "inventory without writes")
	status := flags.Bool("status", false, "validate current receipt without writes")
	rollback := flags.String("rollback", "", "revoke this attempt without changing original or copied data")
	resolve := flags.String("resolve-legacy", "", "resolve a validated legacy namespace")
	if err := flags.Parse(args[1:]); err != nil {
		return 2
	}
	count := 0
	for _, set := range []bool{*dry, *status, *rollback != "", *resolve != ""} {
		if set {
			count++
		}
	}
	if *project == "" || flags.NArg() != 0 || count > 1 {
		fmt.Fprintln(diagnostic, "migration requires --project and at most one mode")
		return 2
	}
	result := zipmigration.Run(*project, zipmigration.Options{DryRun: *dry, Status: *status, Rollback: *rollback, ResolveLegacy: *resolve})
	if err := json.NewEncoder(out).Encode(result); err != nil {
		return 2
	}
	if result.State == "committed" || result.State == "not_needed" || result.State == "rolled_back" || (*dry && result.State == "planned") {
		return 0
	}
	return 2
}
func main() { os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr)) }
