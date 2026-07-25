// Command release builds and verifies unsigned Maestro release candidates.
// It is factory tooling and is never shipped in the Maestro base bundle.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	devharness "github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/dev/harness"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/dev/releasepack"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	root, err := devharness.FindRoot(".")
	if err != nil {
		fatal(err)
	}
	switch os.Args[1] {
	case "candidate":
		candidate(root, os.Args[2:])
	case "verify":
		verify(root, os.Args[2:])
	default:
		usage()
	}
}

func candidate(root string, args []string) {
	flags := flag.NewFlagSet("candidate", flag.ExitOnError)
	version := flags.String("version", "", "immutable MAJOR.MINOR.PATCH candidate version")
	channel := flags.String("channel", "canary", "candidate channel")
	output := flags.String("output", "", "new output directory")
	_ = flags.Parse(args)
	if *version == "" || *output == "" || flags.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: go run ./dev/release candidate --version MAJOR.MINOR.PATCH --channel canary --output DIRECTORY")
		os.Exit(2)
	}
	outputPath := absoluteFromRoot(root, *output)
	manifest, err := releasepack.BuildCandidate(context.Background(), releasepack.CandidateOptions{
		Root:    root,
		Output:  outputPath,
		Version: *version,
		Channel: *channel,
	})
	if err != nil {
		fatal(err)
	}
	fmt.Printf("candidate %s (%s) verified at %s\n", manifest.Release, manifest.Channel, outputPath)
	fmt.Println("status: unsigned release candidate; pilot publication unavailable")
}

func verify(root string, args []string) {
	flags := flag.NewFlagSet("verify", flag.ExitOnError)
	directory := flags.String("directory", "", "candidate directory")
	_ = flags.Parse(args)
	if *directory == "" || flags.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: go run ./dev/release verify --directory DIRECTORY")
		os.Exit(2)
	}
	path := absoluteFromRoot(root, *directory)
	if err := releasepack.VerifyCandidate(path); err != nil {
		fatal(err)
	}
	fmt.Printf("unsigned candidate integrity verified at %s\n", path)
}

func absoluteFromRoot(root, value string) string {
	if filepath.IsAbs(value) {
		return filepath.Clean(value)
	}
	return filepath.Join(root, filepath.Clean(value))
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: go run ./dev/release <candidate|verify> [options]")
	os.Exit(2)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
