// Command bcgos-bootstrap is the stable activation and rollback helper.
// It is intentionally separate from the self-updating bcgos executable.
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installtx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/processwait"
)

var Version = "0.0.0-dev"

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	switch os.Args[1] {
	case "activate":
		activate(os.Args[2:])
	case "rollback":
		rollback(os.Args[2:])
	case "version":
		fmt.Println("bcgos-bootstrap " + Version)
	default:
		usage()
	}
}

func activate(args []string) {
	flags := flag.NewFlagSet("activate", flag.ExitOnError)
	plan := flags.String("plan", "", "verified activation plan")
	waitPID := flags.Int("wait-pid", 0, "CLI process to wait for before activation")
	timeout := flags.Duration("wait-timeout", 2*time.Minute, "maximum process-exit wait")
	_ = flags.Parse(args)
	if *plan == "" || flags.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: bcgos-bootstrap activate --plan FILE [--wait-pid PID]")
		os.Exit(2)
	}
	if *waitPID > 0 {
		fatalIf(processwait.UntilExit(*waitPID, *timeout))
	}
	fatalIf(installtx.Activate(*plan, installtx.ActivateOptions{}))
	fmt.Println("Maestro activation complete")
}

func rollback(args []string) {
	flags := flag.NewFlagSet("rollback", flag.ExitOnError)
	managedRoot := flags.String("managed-root", "", "managed Maestro root")
	dataRoot := flags.String("data-root", "", "owner-data root")
	_ = flags.Parse(args)
	if *managedRoot == "" || *dataRoot == "" || flags.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: bcgos-bootstrap rollback --managed-root DIR --data-root DIR")
		os.Exit(2)
	}
	fatalIf(installtx.Rollback(*managedRoot, *dataRoot, nil))
	fmt.Println("Maestro rollback complete")
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: bcgos-bootstrap <activate|rollback|version> [options]")
	os.Exit(2)
}

func fatalIf(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
