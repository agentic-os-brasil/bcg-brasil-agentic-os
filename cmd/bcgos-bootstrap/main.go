// Command bcgos-bootstrap is the stable activation and rollback helper.
// It is intentionally separate from the self-updating bcgos executable.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installtx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/processwait"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseverify"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/updateservice"
)

var Version = "0.0.0-dev"
var AuthorityRegistrySHA256 = ""

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	switch os.Args[1] {
	case "install":
		install(os.Args[2:])
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

func install(args []string) {
	flags := flag.NewFlagSet("install", flag.ExitOnError)
	planPath := flags.String("plan", "", "verified first-install activation plan")
	verifiedDirectory := flags.String("verified-directory", "", "signed release directory")
	dataRoot := flags.String("data-root", "", "owner-data root")
	_ = flags.Parse(args)
	if *planPath == "" || *verifiedDirectory == "" || *dataRoot == "" || flags.NArg() != 0 {
		fmt.Fprintln(
			os.Stderr,
			"usage: bcgos-bootstrap install --plan FILE --verified-directory DIR --data-root DIR",
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
	fatalIf(installtx.Activate(*planPath, verified, installtx.ActivateOptions{
		PrepareOptions: installtx.PrepareOptions{
			Transition: "install", TargetOS: runtime.GOOS, TargetArch: runtime.GOARCH,
			ManagedRoot: managedRoot, DataRoot: *dataRoot,
		},
	}))
	fmt.Println("Maestro installation complete")
}

func activate(args []string) {
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
	return filepath.Clean(root), nil
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: bcgos-bootstrap <install|activate|rollback|version> [options]")
	os.Exit(2)
}

func fatalIf(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
