// Command bcgos-bootstrap is the stable activation and rollback helper.
// It is intentionally separate from the self-updating bcgos executable.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installtx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/processwait"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseverify"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/updateservice"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/userlevel"
)

var Version = "0.0.0-dev"
var AuthorityRegistrySHA256 = ""

const portableInstallContract = "maestro-portable-install-v2"
const portableInstallFailureMessage = "A preparação do Maestro não pôde ser concluída neste computador."

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	switch os.Args[1] {
	case "install":
		install(os.Args[2:])
	case "portable-install":
		portableInstall(os.Args[2:])
	case "capabilities":
		fmt.Println(portableInstallContract)
	case "activate":
		activate(os.Args[2:])
	case "rollback":
		rollback(os.Args[2:])
	case "version":
		fmt.Println("bcgos-bootstrap " + Version)
	case "seed-status":
		fatalIf(writeSeedStatus(os.Stdout, Version, AuthorityRegistrySHA256))
	default:
		usage()
	}
}

type seedStatus struct {
	SchemaVersion           int    `json:"schema_version"`
	Product                 string `json:"product"`
	BootstrapperVersion     string `json:"bootstrapper_version"`
	AuthorityRegistrySHA256 string `json:"authority_registry_sha256"`
}

func writeSeedStatus(out io.Writer, version, registrySHA256 string) error {
	return json.NewEncoder(out).Encode(seedStatus{
		SchemaVersion: 1, Product: "maestro",
		BootstrapperVersion: version, AuthorityRegistrySHA256: registrySHA256,
	})
}

func install(args []string) {
	flags := flag.NewFlagSet("install", flag.ExitOnError)
	verifiedDirectory := flags.String("verified-directory", "", "signed release directory")
	dataRoot := flags.String("data-root", "", "owner-data root")
	_ = flags.Parse(args)
	if *verifiedDirectory == "" || *dataRoot == "" || flags.NArg() != 0 {
		fmt.Fprintln(
			os.Stderr,
			"usage: bcgos-bootstrap install --verified-directory DIR --data-root DIR",
		)
		os.Exit(2)
	}
	managedRoot, err := managedRootForBootstrap()
	fatalIf(err)
	registry, err := releaseverify.LoadPinnedAuthorityRegistry(
		filepath.Join(managedRoot, "trust", "release-authority-registry.json"),
		AuthorityRegistrySHA256,
		time.Now,
	)
	fatalIf(err)
	verified, err := releaseverify.VerifyDirectory(*verifiedDirectory, registry)
	fatalIf(err)
	fatalIf(firstInstall(verified, managedRoot, *dataRoot, runtime.GOOS, runtime.GOARCH, nil))
	fmt.Println("Maestro installation complete")
}

func firstInstall(
	verified releaseverify.VerifiedRelease,
	managedRoot, dataRoot, targetOS, targetArch string,
	checkCLI func(path, version string) error,
) error {
	if err := userlevel.EnsureNotElevated(); err != nil {
		return err
	}
	options := installtx.PrepareOptions{
		Transition: "install", TargetOS: targetOS, TargetArch: targetArch,
		ManagedRoot: managedRoot, DataRoot: dataRoot,
	}
	planPath, err := installtx.Prepare(verified, options)
	if err != nil {
		return err
	}
	return installtx.Activate(planPath, verified, installtx.ActivateOptions{
		PrepareOptions: options,
		CheckCLI:       checkCLI,
	})
}

func activate(args []string) {
	fatalIf(userlevel.EnsureNotElevated())
	flags := flag.NewFlagSet("activate", flag.ExitOnError)
	planID := flags.String("plan-id", "", "exact durable update confirmation plan")
	dataRoot := flags.String("data-root", "", "owner-data root")
	waitPID := flags.Int("wait-pid", 0, "CLI process to wait for before activation")
	timeout := flags.Duration("wait-timeout", 2*time.Minute, "maximum process-exit wait")
	_ = flags.Parse(args)
	if *planID == "" || *dataRoot == "" || flags.NArg() != 0 {
		fmt.Fprintln(
			os.Stderr,
			"usage: bcgos-bootstrap activate --plan-id ID --data-root DIR [--wait-pid PID]",
		)
		os.Exit(2)
	}
	managedRoot, err := managedRootForBootstrap()
	fatalIf(err)
	if *waitPID > 0 {
		fatalIf(processwait.UntilExit(*waitPID, *timeout))
	}
	registryPath := filepath.Join(managedRoot, "trust", "release-authority-registry.json")
	registry, err := releaseverify.LoadPinnedAuthorityRegistry(
		registryPath,
		AuthorityRegistrySHA256,
		time.Now,
	)
	fatalIf(err)
	_, reconciled, err := updateservice.ReconcilePending(
		*dataRoot,
		managedRoot,
		*planID,
		registry,
	)
	fatalIf(err)
	if reconciled {
		fatalIf(updateservice.RemovePending(*dataRoot, *planID))
		fmt.Println("Maestro activation complete")
		return
	}
	pending, verified, err := updateservice.ConfirmPending(*dataRoot, managedRoot, *planID, registry)
	fatalIf(err)
	fatalIf(installtx.Activate(pending.ActivationPlanPath, verified, installtx.ActivateOptions{
		PrepareOptions: installtx.PrepareOptions{
			Transition:         "update",
			ConfirmationPlanID: pending.Plan.ID,
			FromRelease:        pending.Plan.FromRelease,
			FromChannel:        pending.Plan.FromChannel,
			FromCLIVersion:     pending.Plan.FromCLIVersion,
			FromBundleVersion:  pending.Plan.FromBundleVersion,
			TargetOS:           pending.Plan.TargetOS,
			TargetArch:         pending.Plan.TargetArch,
			ManagedRoot:        managedRoot,
			DataRoot:           *dataRoot,
		},
	}))
	fatalIf(updateservice.RemovePending(*dataRoot, *planID))
	fmt.Println("Maestro activation complete")
}

func rollback(args []string) {
	fatalIf(userlevel.EnsureNotElevated())
	flags := flag.NewFlagSet("rollback", flag.ExitOnError)
	dataRoot := flags.String("data-root", "", "owner-data root")
	_ = flags.Parse(args)
	if *dataRoot == "" || flags.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: bcgos-bootstrap rollback --data-root DIR")
		os.Exit(2)
	}
	managedRoot, err := managedRootForBootstrap()
	fatalIf(err)
	fatalIf(installtx.Rollback(managedRoot, *dataRoot, nil))
	fmt.Println("Maestro rollback complete")
}

// portableInstall activates only the verified managed core of a portable
// Maestro archive. The runtime invokes the installed CLI afterwards for
// workspace setup and adapter verification, preserving that boundary from the
// stable bootstrapper.
func portableInstall(args []string) {
	flags := flag.NewFlagSet("portable-install", flag.ExitOnError)
	_ = flags.Parse(args)
	if flags.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: bcgos-bootstrap portable-install")
		os.Exit(2)
	}
	managedRoot, err := managedRootForBootstrap()
	fatalPortableInstall(err)
	portableRoot, err := portableRootFromManagedRoot(managedRoot)
	fatalPortableInstall(err)
	dataRoot, err := portableDataRoot()
	fatalPortableInstall(err)
	registry, err := releaseverify.LoadPinnedAuthorityRegistry(
		filepath.Join(managedRoot, "trust", "release-authority-registry.json"),
		AuthorityRegistrySHA256,
		time.Now,
	)
	fatalPortableInstall(err)
	verified, err := releaseverify.VerifyDirectory(filepath.Join(portableRoot, "release"), registry)
	fatalPortableInstall(err)
	cliName := "bcgos"
	if runtime.GOOS == "windows" {
		cliName += ".exe"
	}
	cliPath := filepath.Join(managedRoot, "bin", cliName)
	if _, err := os.Stat(cliPath); os.IsNotExist(err) {
		fatalPortableInstall(firstInstall(verified, managedRoot, dataRoot, runtime.GOOS, runtime.GOARCH, nil))
	} else {
		fatalPortableInstall(err)
	}
	fmt.Println("Maestro core is ready")
}

func fatalPortableInstall(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, portableInstallFailureMessage)
		os.Exit(1)
	}
}

func portableRootFromManagedRoot(managedRoot string) (string, error) {
	root := filepath.Dir(filepath.Clean(managedRoot))
	if filepath.Dir(root) == root {
		return "", errors.New("portable package root cannot be a filesystem root")
	}
	for _, required := range []string{"release", "maestro-os"} {
		info, err := os.Stat(filepath.Join(root, required))
		if err != nil || !info.IsDir() {
			return "", errors.New("portable Maestro package is incomplete")
		}
	}
	return root, nil
}

func portableDataRoot() (string, error) {
	if runtime.GOOS == "windows" {
		if local := os.Getenv("LOCALAPPDATA"); local != "" {
			return filepath.Join(local, "BCGOS"), nil
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if runtime.GOOS == "darwin" {
		return filepath.Join(home, "Library", "Application Support", "BCGOS"), nil
	}
	return filepath.Join(home, ".local", "share", "BCGOS"), nil
}

func managedRootForBootstrap() (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", err
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return "", err
	}
	return managedRootFromExecutablePath(executable)
}

func managedRootFromExecutablePath(executable string) (string, error) {
	name := filepath.Base(executable)
	if name != "bcgos-bootstrap" && name != "bcgos-bootstrap.exe" {
		return "", errors.New("bootstrapper executable does not have its protected installed name")
	}
	root := filepath.Dir(executable)
	if filepath.Base(root) == "bootstrap" {
		root = filepath.Dir(root)
	}
	if platformRoot := filepath.Base(root); platformRoot == "windows" || platformRoot == "macos" {
		root = filepath.Dir(root)
	} else if architectureRoot := filepath.Base(root); (architectureRoot == "arm64" || architectureRoot == "amd64") && filepath.Base(filepath.Dir(root)) == "macos" {
		root = filepath.Dir(filepath.Dir(root))
	}
	if filepath.Dir(filepath.Clean(root)) == filepath.Clean(root) {
		return "", errors.New("bootstrapper managed root cannot be a filesystem root")
	}
	return filepath.Clean(root), nil
}

func usage() {
	fmt.Fprintln(
		os.Stderr,
		"usage: bcgos-bootstrap <install|portable-install|activate|rollback|version|seed-status|capabilities> [options]",
	)
	os.Exit(2)
}

func fatalIf(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
