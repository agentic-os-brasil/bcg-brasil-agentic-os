package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"

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
	default:
		usage()
		os.Exit(2)
	}
}

func skillsIndexCommand(root string, args []string) {
	if len(args) != 0 {
		fmt.Fprintln(os.Stderr, "usage: go run ./dev/harness skills-index")
		os.Exit(2)
	}
	fatalIf(skillsindex.Write(filepath.Join(root, "bundles", "base", "skills")))
	fmt.Println("skills index regenerated")
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
	fmt.Fprintln(os.Stderr, "usage: go run ./dev/harness <validate|decision|doctor|setup|recover|guard|claude|skills-index> [options]")
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
