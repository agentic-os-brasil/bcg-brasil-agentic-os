// Command release builds and verifies unsigned Maestro release candidates.
// It is factory tooling and is never shipped in the Maestro base bundle.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	devharness "github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/dev/harness"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/dev/releasepack"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/dev/releasereadiness"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installerbundle"
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
	case "icons":
		icons(root, os.Args[2:])
	case "seeded-binaries":
		seededBinaries(root, os.Args[2:])
	case "candidate":
		candidate(root, os.Args[2:])
	case "provenance":
		provenance(root, os.Args[2:])
	case "sign":
		sign(root, os.Args[2:])
	case "verify":
		verify(root, os.Args[2:])
	case "verify-signed":
		verifySigned(root, os.Args[2:])
	case "readiness":
		readiness(root, os.Args[2:])
	case "self-contained":
		selfContained(root, os.Args[2:])
	default:
		usage()
	}
}

func selfContained(root string, args []string) {
	flags := flag.NewFlagSet("self-contained", flag.ExitOnError)
	base := flags.String("base", "", "outer Windows wrapper executable")
	source := flags.String("source", "", "complete validated installer package directory")
	output := flags.String("output", "", "new self-contained executable path")
	_ = flags.Parse(args)
	if *base == "" || *source == "" || *output == "" || flags.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: go run ./dev/release self-contained --base FILE --source DIRECTORY --output FILE")
		os.Exit(2)
	}
	basePath := absoluteFromRoot(root, *base)
	sourcePath := absoluteFromRoot(root, *source)
	outputPath := absoluteFromRoot(root, *output)
	outputParent := filepath.Dir(outputPath)
	temporary, err := os.MkdirTemp(outputParent, ".maestro-installer-payload-")
	if err != nil {
		fatal(err)
	}
	defer os.RemoveAll(temporary)
	payloadPath := filepath.Join(temporary, "payload.tar.gz")
	payloadFile, err := os.OpenFile(payloadPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		fatal(err)
	}
	_, packErr := installerbundle.PackDirectory(sourcePath, payloadFile)
	closeErr := payloadFile.Close()
	if packErr != nil {
		fatal(packErr)
	}
	if closeErr != nil {
		fatal(closeErr)
	}
	payloadInfo, err := installerbundle.AppendPayload(basePath, payloadPath, outputPath)
	if err != nil {
		fatal(err)
	}
	if err := json.NewEncoder(os.Stdout).Encode(map[string]any{
		"output":  outputPath,
		"payload": payloadInfo,
		"status":  "unsigned-candidate",
	}); err != nil {
		fatal(err)
	}
}

func readiness(root string, args []string) {
	flags := flag.NewFlagSet("readiness", flag.ExitOnError)
	provider := flags.String("provider-config", "", "explicit public provider configuration path")
	registry := flags.String("authority-registry", "", "explicit public authority registry path")
	registryDigest := flags.String("authority-registry-sha256", "", "exact lowercase SHA-256 pin for the authority registry")
	candidate := flags.String("candidate", "", "explicit unsigned candidate directory")
	_ = flags.Parse(args)
	if flags.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: go run ./dev/release readiness [--provider-config FILE] [--authority-registry FILE --authority-registry-sha256 SHA256] [--candidate DIRECTORY]")
		os.Exit(2)
	}
	report := releasereadiness.Evaluate(releasereadiness.Options{
		Root:                    root,
		ProviderConfig:          optionalAbsoluteFromRoot(root, *provider),
		AuthorityRegistry:       optionalAbsoluteFromRoot(root, *registry),
		AuthorityRegistrySHA256: *registryDigest,
		Candidate:               optionalAbsoluteFromRoot(root, *candidate),
	})
	if err := json.NewEncoder(os.Stdout).Encode(report); err != nil {
		fatal(err)
	}
	for _, check := range report.Checks {
		if check.State == releasereadiness.StateBlocked {
			os.Exit(1)
		}
	}
	for _, check := range report.Checks {
		if check.State == releasereadiness.StateUnavailable || check.State == releasereadiness.StateNotEvaluated {
			os.Exit(3)
		}
	}
}

func provenance(root string, args []string) {
	flags := flag.NewFlagSet("provenance", flag.ExitOnError)
	version := flags.String("version", "", "immutable MAJOR.MINOR.PATCH candidate version")
	osName := flags.String("os", "", "native target operating system")
	arch := flags.String("arch", "", "native target architecture")
	binary := flags.String("binary", "", "native binary path")
	component := flags.String("component", "cli", "native component: cli or bootstrapper")
	_ = flags.Parse(args)
	if *version == "" || *osName == "" || *arch == "" || *binary == "" || flags.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: go run ./dev/release provenance --version MAJOR.MINOR.PATCH --os OS --arch ARCH --binary FILE")
		os.Exit(2)
	}
	output, err := releasepack.WriteNativeProvenance(
		absoluteFromRoot(root, *binary),
		*version,
		releasepack.Target{OS: *osName, Arch: *arch},
		releasepack.NativeComponent(*component),
	)
	if err != nil {
		fatal(err)
	}
	fmt.Printf("native build provenance written at %s\n", output)
}

func seededBinaries(root string, args []string) {
	flags := flag.NewFlagSet("seeded-binaries", flag.ExitOnError)
	version := flags.String("version", "", "immutable MAJOR.MINOR.PATCH release version")
	osName := flags.String("os", "", "native target operating system")
	arch := flags.String("arch", "", "native target architecture")
	output := flags.String("output", "", "new native output directory")
	provider := flags.String("provider-config", "", "approved public provider configuration")
	publicationRepo := flags.String("publication-repository", "", "exact OWNER/REPOSITORY publication target")
	registry := flags.String("authority-registry", "", "approved public authority registry")
	_ = flags.Parse(args)
	if *version == "" || *osName == "" || *arch == "" || *output == "" ||
		*provider == "" || *publicationRepo == "" || *registry == "" || flags.NArg() != 0 {
		fmt.Fprintln(
			os.Stderr,
			"usage: go run ./dev/release seeded-binaries --version MAJOR.MINOR.PATCH --os OS --arch ARCH --provider-config FILE --publication-repository OWNER/REPOSITORY --authority-registry FILE --output DIRECTORY",
		)
		os.Exit(2)
	}
	artifacts, err := releasepack.BuildSeededNativeBinaries(
		context.Background(),
		releasepack.SeededBuildOptions{
			Root: root, Output: absoluteFromRoot(root, *output), Version: *version,
			Target:            releasepack.Target{OS: *osName, Arch: *arch},
			ProviderConfig:    absoluteFromRoot(root, *provider),
			PublicationRepo:   *publicationRepo,
			AuthorityRegistry: absoluteFromRoot(root, *registry),
		},
	)
	if err != nil {
		fatal(err)
	}
	fmt.Printf(
		"seeded native CLI and bootstrapper built with authority registry %s at %s\n",
		artifacts.AuthorityRegistrySHA256,
		filepath.Dir(artifacts.CLI),
	)
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

func sign(root string, args []string) {
	flags := flag.NewFlagSet("sign", flag.ExitOnError)
	candidate := flags.String("candidate", "", "closed unsigned candidate directory")
	output := flags.String("output", "", "new signed release directory")
	registry := flags.String("authority-registry", "", "approved public authority registry")
	issuer := flags.String("issuer", "", "approved Maestro release issuer")
	keyID := flags.String("key-id", "", "approved active signing key ID")
	_ = flags.Parse(args)
	if *candidate == "" || *output == "" || *registry == "" ||
		*issuer == "" || *keyID == "" || flags.NArg() != 0 {
		fmt.Fprintln(
			os.Stderr,
			"usage: go run ./dev/release sign --candidate DIR --output DIR --authority-registry FILE --issuer ID --key-id ID < base64-seed",
		)
		os.Exit(2)
	}
	privateKey, err := releasepack.ParseSigningSeed(os.Stdin)
	if err != nil {
		fatal(err)
	}
	manifest, err := releasepack.SignCandidate(releasepack.SignCandidateOptions{
		Candidate: absoluteFromRoot(root, *candidate),
		Output:    absoluteFromRoot(root, *output),
		Registry:  absoluteFromRoot(root, *registry),
		Issuer:    *issuer, KeyID: *keyID, PrivateKey: privateKey,
		Clock: time.Now,
	})
	if err != nil {
		fatal(err)
	}
	fmt.Printf("signed Maestro %s release verified at %s\n", manifest.Release, absoluteFromRoot(root, *output))
}

func verifySigned(root string, args []string) {
	flags := flag.NewFlagSet("verify-signed", flag.ExitOnError)
	directory := flags.String("directory", "", "signed release directory")
	registry := flags.String("authority-registry", "", "approved public authority registry")
	_ = flags.Parse(args)
	if *directory == "" || *registry == "" || flags.NArg() != 0 {
		fmt.Fprintln(
			os.Stderr,
			"usage: go run ./dev/release verify-signed --directory DIR --authority-registry FILE",
		)
		os.Exit(2)
	}
	if err := releasepack.VerifySignedRelease(
		absoluteFromRoot(root, *directory),
		absoluteFromRoot(root, *registry),
		time.Now,
	); err != nil {
		fatal(err)
	}
	fmt.Printf("signed release verified at %s\n", absoluteFromRoot(root, *directory))
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
	fmt.Fprintln(
		os.Stderr,
		"usage: go run ./dev/release <binary|icons|seeded-binaries|candidate|provenance|sign|verify|verify-signed|readiness|self-contained> [options]",
	)
	os.Exit(2)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
