//go:build maestro_experimental_installed_seed

package main

import (
	"context"
	"errors"
	"flag"
	"io"
	"os"
	"path/filepath"
	"time"

	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installer"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installtx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/processidentity"
)

func experimentalCommandFlags(flags *flag.FlagSet, o *product.Options) func() error {
	digest := flags.String("experimental-seed-sha256", "", "fixed signed engineering installed-seed identity")
	root := flags.String("experimental-installed-root", "", "fixed reviewed engineering installation")
	return func() error {
		if *root == "" || *digest == "" || !filepath.IsAbs(*root) || filepath.Clean(*root) != *root {
			return errors.New("experimental installed binding is required")
		}
		fixed, err := installer.LoadExperimentalInstalledContext(*root, AuthorityRegistrySHA256, time.Now)
		if err != nil || fixed.ExperimentalSeed.Identity().AdmissionSHA256 != *digest {
			return errors.New("experimental signed seed differs")
		}
		if o.UserHome != "" && o.UserHome != fixed.UserHome {
			return errors.New("experimental child owner differs")
		}
		role := fixed.ExperimentalSeed
		current, err := installtx.ReadSourceState(fixed.DataRoot, *root)
		if err != nil {
			return err
		}
		actual, err := processidentity.Current()
		if err != nil {
			return err
		}
		if actual.SHA256 != current.Server.SHA256 || Version != current.Server.Version {
			return errors.New("experimental child is not the exact current server")
		}
		o.ExperimentalSeed, o.ExperimentalSeedRoot = role, *root
		return nil
	}
}

func experimentalEarlyCommand(ctx context.Context, command string, args []string, output io.Writer) (bool, error) {
	if command == "installed-maintenance-wake" {
		return true, runInstalledMaintenanceWake(ctx, args, output)
	}
	if command == "installed-native-hook" {
		return true, runInstalledNativeHook(ctx, args, os.Stdin, output)
	}
	if command == "installed-pilot-prepare-document" {
		return true, runInstalledPilotPrepareDocument(ctx, args, output)
	}
	if command != "installed-pilot-check" {
		return false, nil
	}
	return true, runInstalledPilotCheck(ctx, args, output)
}
