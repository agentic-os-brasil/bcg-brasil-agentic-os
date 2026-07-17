package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/DScardini91/bcg-brasil-agentic-os/internal/dev/decisionlog"
	devharness "github.com/DScardini91/bcg-brasil-agentic-os/internal/dev/harness"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	root, err := devharness.FindRoot(".")
	if err != nil {
		fatal(err)
	}
	switch os.Args[1] {
	case "validate":
		validateCommand(root, os.Args[2:])
	case "decision":
		decisionCommand(root, os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
}

func validateCommand(root string, args []string) {
	flags := flag.NewFlagSet("validate", flag.ExitOnError)
	full := flags.Bool("full", false, "run the complete development and CI gate")
	_ = flags.Parse(args)
	if err := devharness.Validate(root, *full, os.Stdout); err != nil {
		fatal(err)
	}
}

func decisionCommand(root string, args []string) {
	if len(args) < 1 {
		decisionUsage()
	}
	path := filepath.Join(root, "docs", "decisions", "decision-log.md")
	entries, err := decisionlog.ParseFile(path)
	if err != nil {
		fatal(err)
	}
	switch args[0] {
	case "check":
		if len(args) != 1 {
			decisionUsage()
		}
		fmt.Printf("decision log valid: %d entries\n", len(entries))
	case "available":
		if len(args) != 2 {
			decisionUsage()
		}
		if err := decisionlog.Available(entries, args[1]); err != nil {
			fatal(err)
		}
		fmt.Printf("decision code available: %s\n", args[1])
	default:
		decisionUsage()
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: go run ./dev/harness <validate|decision> [options]")
}

func decisionUsage() {
	fmt.Fprintln(os.Stderr, "usage: go run ./dev/harness decision <check|available CODE>")
	os.Exit(2)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
