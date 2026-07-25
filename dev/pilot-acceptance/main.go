// Command pilot-acceptance creates isolated evidence or validates an approved
// corporate-device report. It never promotes a release.
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/dev/pilotacceptance"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	switch os.Args[1] {
	case "isolated":
		isolated(os.Args[2:])
	case "validate":
		validate(os.Args[2:])
	default:
		usage()
	}
}

func isolated(args []string) {
	flags := flag.NewFlagSet("isolated", flag.ExitOnError)
	runID := flags.String("run-id", "", "CI run identifier")
	platform := flags.String("platform", "", "windows or macos")
	version := flags.String("version", "", "candidate MAJOR.MINOR.PATCH")
	output := flags.String("output", "", "new evidence report path")
	_ = flags.Parse(args)
	if *runID == "" || *platform == "" || *version == "" || *output == "" || flags.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: go run ./dev/pilot-acceptance isolated --run-id ID --platform windows|macos --version MAJOR.MINOR.PATCH --output FILE")
		os.Exit(2)
	}
	finished := time.Now().UTC()
	report := pilotacceptance.Isolated(*runID, *platform, *version, finished.Add(-time.Second), finished)
	fatalIf(pilotacceptance.Write(*output, report))
	fmt.Printf("isolated engineering evidence written to %s; corporate-device acceptance remains pending\n", *output)
}

func validate(args []string) {
	flags := flag.NewFlagSet("validate", flag.ExitOnError)
	reportPath := flags.String("report", "", "acceptance report path")
	_ = flags.Parse(args)
	if *reportPath == "" || flags.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: go run ./dev/pilot-acceptance validate --report FILE")
		os.Exit(2)
	}
	report, err := pilotacceptance.Read(*reportPath)
	fatalIf(err)
	fmt.Printf("%s evidence valid: %s/%s (%s)\n", report.Mode, report.Platform, report.CandidateVersion, report.ReadinessClaim)
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: go run ./dev/pilot-acceptance <isolated|validate> [options]")
	os.Exit(2)
}

func fatalIf(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
