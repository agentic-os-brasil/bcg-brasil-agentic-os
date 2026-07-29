package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/atlas"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/capabilitybundle"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/dev/decisionlog"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/dev/gitguard"
	devharness "github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/dev/harness"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/skillsindex"
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
	case "doctor":
		fatalIf(gitguard.Doctor(root, os.Stdout))
	case "setup":
		fatalIf(gitguard.Setup(root, os.Stdout))
	case "recover":
		fatalIf(gitguard.Recover(root, os.Stdout))
	case "guard":
		guardCommand(root, os.Args[2:])
	case "claude":
		claudeCommand(root, os.Args[2:])
	case "skills-index":
		skillsIndexCommand(root, os.Args[2:])
	case "wiki":
		wikiCommand(root, os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
}

func wikiCommand(root string, args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: go run ./dev/harness wiki <reconcile|validate|verify> [--allowlist path] [--output path]")
		os.Exit(2)
	}
	flags := flag.NewFlagSet("wiki", flag.ExitOnError)
	allowlist := flags.String("allowlist", filepath.Join(root, "dev", "wiki", "managed-allowlist.json"), "managed source allowlist")
	output := flags.String("output", filepath.Join(root, "bundles", "base", "atlas", "managed"), "managed OKF output directory")
	_ = flags.Parse(args[1:])
	switch args[0] {
	case "reconcile":
		report, err := atlas.ReconcileManaged(root, *allowlist, *output)
		fatalIf(err)
		fmt.Printf("managed wiki reconciled: %d concepts, fingerprint %s\n", report.Concepts, report.Fingerprint)
	case "validate":
		fatalIf(atlas.ValidateManagedBundle(*output))
		fmt.Println("managed wiki bundle valid")
	case "verify":
		fatalIf(atlas.VerifyManagedUpToDate(root, *allowlist, *output))
		fmt.Println("managed wiki bundle current")
	default:
		fmt.Fprintln(os.Stderr, "usage: go run ./dev/harness wiki <reconcile|validate|verify> [--allowlist path] [--output path]")
		os.Exit(2)
	}
}

func skillsIndexCommand(root string, args []string) {
	if len(args) != 0 {
		fmt.Fprintln(os.Stderr, "usage: go run ./dev/harness skills-index")
		os.Exit(2)
	}
	catalog, err := capabilitybundle.LoadFile(filepath.Join(root, "bundles", "catalog", "catalog.json"))
	if err != nil {
		fatal(err)
	}
	for _, bundle := range catalog.Bundles {
		skillsRoot := filepath.Join(root, filepath.FromSlash(filepath.Dir(bundle.CatalogPointer)))
		fatalIf(skillsindex.Write(skillsRoot))
	}
	fmt.Println("skills indexes regenerated")
}

func guardCommand(root string, args []string) {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: go run ./dev/harness guard <pre-commit|pre-push>")
		os.Exit(2)
	}
	var err error
	switch args[0] {
	case "pre-commit":
		err = gitguard.PreCommit(root, os.Stdout)
	case "pre-push":
		err = gitguard.PrePush(root, bufio.NewScanner(os.Stdin), os.Stdout)
	default:
		fmt.Fprintln(os.Stderr, "usage: go run ./dev/harness guard <pre-commit|pre-push>")
		os.Exit(2)
	}
	fatalIf(err)
}

func claudeCommand(root string, args []string) {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: go run ./dev/harness claude <session-start|pre-tool|post-tool>")
		os.Exit(2)
	}
	code, err := gitguard.ClaudeHook(root, args[0], os.Stdin, os.Stdout)
	if err != nil {
		fatal(err)
	}
	os.Exit(code)
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
	fmt.Fprintln(os.Stderr, "usage: go run ./dev/harness <validate|decision|doctor|setup|recover|guard|claude|skills-index|wiki> [options]")
}

func fatalIf(err error) {
	if err != nil {
		fatal(err)
	}
}

func decisionUsage() {
	fmt.Fprintln(os.Stderr, "usage: go run ./dev/harness decision <check|available CODE>")
	os.Exit(2)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
