package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/portableactivation"
)

var Version = "0.0.0-dev"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	managedRoot, dataRoot, err := activationRoots(os.Args[1], os.Args[2:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		usage()
		os.Exit(2)
	}
	receipt, err := portableactivation.Activate(portableactivation.Options{ManagedRoot: managedRoot, DataRoot: dataRoot, ExpectedVersion: Version})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(receipt); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func activationRoots(command string, args []string) (string, string, error) {
	switch command {
	case "portable-install":
		if len(args) != 0 {
			return "", "", fmt.Errorf("portable-install accepts no arguments")
		}
		executable, err := os.Executable()
		if err != nil {
			return "", "", fmt.Errorf("resolve bootstrapper: %w", err)
		}
		executable, err = filepath.EvalSymlinks(executable)
		if err != nil {
			return "", "", fmt.Errorf("resolve bootstrapper: %w", err)
		}
		managedRoot := filepath.Dir(executable)
		return managedRoot, filepath.Join(filepath.Dir(managedRoot), "data"), nil
	case "activate":
		flags := flag.NewFlagSet("activate", flag.ContinueOnError)
		flags.SetOutput(os.Stderr)
		managedRoot := flags.String("managed-root", "", "portable managed root")
		dataRoot := flags.String("data-root", "", "owner-private data root")
		if err := flags.Parse(args); err != nil {
			return "", "", err
		}
		if flags.NArg() != 0 || *managedRoot == "" || *dataRoot == "" {
			return "", "", fmt.Errorf("activate requires --managed-root and --data-root")
		}
		return *managedRoot, *dataRoot, nil
	default:
		return "", "", fmt.Errorf("unknown command %q", command)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: bcgos-bootstrap portable-install")
	fmt.Fprintln(os.Stderr, "   or: bcgos-bootstrap activate --managed-root DIR --data-root DIR")
}
