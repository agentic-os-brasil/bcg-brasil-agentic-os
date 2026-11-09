// maestro-install is the independently seeded stable native installer.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp/setup"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installer"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseverify"
)

var Version string
var AuthorityRegistrySHA256 string

func main() {
	if err := run(); err != nil {
		writeFailure(os.Stderr, err)
		os.Exit(1)
	}
}

func writeFailure(output io.Writer, err error) {
	fmt.Fprintln(output, "maestro-install: operation did not complete; inspect reported setup state and review exact local setup before retrying:", err)
}

func run() error {
	if len(os.Args) < 2 {
		return errors.New("command required")
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	root := filepath.Dir(executable)
	registryPath := filepath.Join(root, "trust", "release-authority-registry.json")
	if os.Args[1] == "client-capabilities" {
		if len(os.Args) != 2 || Version == "" || AuthorityRegistrySHA256 == "" {
			return errors.New("installer capabilities unavailable")
		}
		return json.NewEncoder(os.Stdout).Encode(installer.StableClientCapabilities("installer", Version, AuthorityRegistrySHA256))
	}
	if os.Args[1] == "seed-status" {
		if len(os.Args) != 2 || Version == "" || AuthorityRegistrySHA256 == "" {
			return errors.New("installer seed unavailable")
		}
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"schema_version": 2, "product": "maestro", "component": "installer", "protocol": releasecontract.InstallerProtocol, "installer_version": Version, "authority_registry_sha256": AuthorityRegistrySHA256})
	}
	_, err = releaseverify.LoadPinnedAuthorityRegistry(registryPath, AuthorityRegistrySHA256, time.Now)
	if err != nil {
		return err
	}
	flags := flag.NewFlagSet("maestro-install", flag.ContinueOnError)
	planJSON := flags.String("confirmed-plan", "", "exact canonical activation plan from the reviewed handoff")
	digest := flags.String("review-digest", "", "exact setup review digest")
	confirm := flags.Bool("confirm", false, "explicit local confirmation")
	var launch lifecycleFlags
	flags.StringVar(&launch.ManagedRoot, "product-root", "", "fixed installed managed directory")
	flags.StringVar(&launch.UserHome, "user-home", "", "fixed installation owner directory")
	flags.StringVar(&launch.Workspace, "workspace", "", "fixed admitted workspace directory")
	flags.StringVar(&launch.ManifestSHA256, "installed-manifest", "", "fixed carrier signed manifest digest")
	flags.StringVar(&launch.ClientInputsSHA256, "installed-client-inputs", "", "fixed carrier immutable client digest")
	flags.StringVar(&launch.PlanID, "plan-id", "", "exact reviewed pending plan identifier")
	flags.StringVar(&launch.CarrierID, "carrier-id", "", "fixed admitted stable carrier ID")
	flags.StringVar(&launch.PackageSHA256, "package-sha256", "", "exact Desktop package shown during native installation")
	flags.StringVar(&launch.NativeContractSHA256, "native-contract-sha256", "", "exact Desktop native-facing contract")
	flags.StringVar(&launch.ClientCodeRoot, "client-code-root", "", "fixed generated operator launcher Code destination")
	flags.StringVar(&launch.ClientDesktopPath, "client-desktop-path", "", "fixed generated operator launcher Desktop destination")
	flags.StringVar(&launch.ClientCodeExecutable, "client-code-executable", "", "fixed generated operator launcher Code executable")
	flags.StringVar(&launch.ClientScope, "client-scope", "", "fixed generated operator launcher Code scope")
	if err = flags.Parse(os.Args[2:]); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	if handled, err := runClients(context.Background(), os.Args[1], executable, launch, *digest, *confirm); handled {
		return err
	}
	if handled, err := runLifecycle(context.Background(), os.Args[1], executable, registryPath, launch, *digest, *confirm); handled {
		return err
	}
	if os.Args[1] == "install" {
		if len(*planJSON) > 1<<20 {
			return errors.New("bounded confirmed plan required")
		}
		var plan installer.Plan
		if err := releasecontract.DecodeStrictJSON([]byte(*planJSON), &plan); err != nil {
			return err
		}
		state, err := installer.ActivateSeeded(context.Background(), installer.SeededActivationOptions{Self: executable, Version: Version, AuthorityRegistrySHA256: AuthorityRegistrySHA256, Plan: plan, ReviewDigest: *digest, Confirmed: *confirm})
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(map[string]string{"state": "activated", "profile": state.Profile})
	}

	if os.Args[1] != "review" && os.Args[1] != "apply" {
		return errors.New("unsupported command")
	}
	body, err := io.ReadAll(io.LimitReader(os.Stdin, 1<<20+1))
	if err != nil || len(body) > 1<<20 {
		return errors.New("bounded setup request required")
	}
	request, err := setup.DecodeRequest(body)
	if err != nil {
		return err
	}
	config := setup.Config{Installer: executable, AuthorityRegistry: registryPath}
	if os.Args[1] == "review" {
		review, err := setup.ReviewInstallation(request, config)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(review)
	}
	result, err := setup.Apply(context.Background(), request, config, *digest, *confirm)
	if encodeErr := json.NewEncoder(os.Stdout).Encode(result); encodeErr != nil {
		return encodeErr
	}
	return err
}
