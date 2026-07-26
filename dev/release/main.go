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
	case "binary":
		binary(root, os.Args[2:])
	case "candidate":
		candidate(root, os.Args[2:])
	case "provenance":
		provenance(root, os.Args[2:])
	case "verify":
		verify(root, os.Args[2:])
	default:
		usage()
	}
}

func provenance(root string, args []string) {
	flags := flag.NewFlagSet("provenance", flag.ExitOnError)
	version := flags.String("version", "", "immutable MAJOR.MINOR.PATCH candidate version")
	osName := flags.String("os", "", "native target operating system")
	arch := flags.String("arch", "", "native target architecture")
	binary := flags.String("binary", "", "native binary path")
	_ = flags.Parse(args)
	if *version == "" || *osName == "" || *arch == "" || *binary == "" || flags.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: go run ./dev/release provenance --version MAJOR.MINOR.PATCH --os OS --arch ARCH --binary FILE")
		os.Exit(2)
	}
	output, err := releasepack.WriteBinaryProvenance(
		absoluteFromRoot(root, *binary),
		*version,
		releasepack.Target{OS: *osName, Arch: *arch},
	)
	if err != nil {
		fatal(err)
	}
	fmt.Printf("native build provenance written at %s\n", output)
}

func binary(root string, args []string) {
	flags := flag.NewFlagSet("binary", flag.ExitOnError)
	version := flags.String("version", "", "immutable MAJOR.MINOR.PATCH candidate version")
	osName := flags.String("os", "", "native target operating system")
	arch := flags.String("arch", "", "native target architecture")
	output := flags.String("output", "", "new native binary path")
	_ = flags.Parse(args)
	if *version == "" || *osName == "" || *arch == "" || *output == "" || flags.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: go run ./dev/release binary --version MAJOR.MINOR.PATCH --os OS --arch ARCH --output FILE")
		os.Exit(2)
	}
	outputPath := absoluteFromRoot(root, *output)
	target := releasepack.Target{OS: *osName, Arch: *arch}
	if err := releasepack.BuildNativeBinary(
		context.Background(), root, outputPath, *version, target, nil,
	); err != nil {
		fatal(err)
	}
	fmt.Printf("native candidate binary %s/%s built at %s\n", target.OS, target.Arch, outputPath)
}

func candidate(root string, args []string) {
	flags := flag.NewFlagSet("candidate", flag.ExitOnError)
	version := flags.String("version", "", "immutable MAJOR.MINOR.PATCH candidate version")
	channel := flags.String("channel", "canary", "candidate channel")
	output := flags.String("output", "", "new output directory")
	prebuilt := flags.String("prebuilt", "", "directory containing exact native candidate binaries")
	_ = flags.Parse(args)
	if *version == "" || *output == "" || flags.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: go run ./dev/release candidate --version MAJOR.MINOR.PATCH --channel canary --output DIRECTORY")
		os.Exit(2)
	}
	outputPath := absoluteFromRoot(root, *output)
	manifest, err := releasepack.BuildCandidate(context.Background(), releasepack.CandidateOptions{
		Root:     root,
		Output:   outputPath,
		Version:  *version,
		Channel:  *channel,
		Prebuilt: optionalAbsoluteFromRoot(root, *prebuilt),
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

func optionalAbsoluteFromRoot(root, value string) string {
	if value == "" {
		return ""
	}
	return absoluteFromRoot(root, value)
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: go run ./dev/release <binary|candidate|provenance|verify> [options]")
	os.Exit(2)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
