//go:build maestro_experimental_installed_seed

package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installer"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/processidentity"
)

func runInstalledMaintenanceWake(parent context.Context, args []string, output io.Writer) error {
	s, err := installer.ParseMaintenanceWakeCommand(args, true)
	if err != nil {
		return err
	}
	fixed, err := installer.LoadExperimentalInstalledContext(s.ManagedRoot, AuthorityRegistrySHA256, time.Now)
	if err != nil || fixed.UserHome != s.UserHome || fixed.ExperimentalSeed.Identity().AdmissionSHA256 != s.AdmissionSHA256 {
		return errors.New("native wake installed owner or role differs")
	}
	o := installer.ClientOptions{SeededOperationOptions: fixed, Request: installer.ClientRequest{CarrierID: s.CarrierID, Workspace: s.Workspace}}
	launch, state, err := installer.PrepareMaintenanceWake(parent, o, s.BindingSHA256)
	if err != nil {
		return err
	}
	actual, err := processidentity.Current()
	if err != nil || actual.Path != launch.Executable || !installedCurrentImageMatches(actual.Path, actual.SHA256, Version, s.ManagedRoot, launch.State) || launch.State.Binding.ManifestSHA256 != s.ManifestSHA256 || launch.State.Binding.ClientInputsSHA256 != s.ClientInputsSHA256 {
		return errors.New("native wake must use the exact current admitted server")
	}
	osHome, err := installer.MaintenanceWakeOSHome()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	wakeAllowed := func() error {
		current, err := installer.ReadMaintenanceWake(ctx, o)
		if err != nil || current.ClientRecordSHA != state.ClientRecordSHA || current.BindingSHA256 != state.BindingSHA256 || current.CurrentManifest != state.CurrentManifest {
			return errors.New("native wake current selection changed")
		}
		return state.Binding.VerifyNativeWake(ctx, osHome)
	}
	if err := wakeAllowed(); err != nil {
		return err
	}
	options := product.Options{ExperimentalSeed: fixed.ExperimentalSeed, ExperimentalSeedRoot: fixed.ManagedRoot, ProductRoot: fixed.ManagedRoot, UserHome: fixed.UserHome, WorkspacePath: s.Workspace, InstalledManifestSHA256: s.ManifestSHA256, InstalledClientInputsSHA256: s.ClientInputsSHA256, GuardProcess: true, ManagementCarrierID: s.CarrierID, ManagementInstallerVersion: fixed.Version, OwnerReviewChannel: state.Record.Materialization.OwnerReviewChannel}
	seedDoclingAuthority(&options)
	// Empty declaration preserves the original stored registration authority;
	// fixed native transports are composed by the shared startup owner.
	if err := configureWeeklyRegistration(&options, ""); err != nil {
		return err
	}
	bound, err := product.Open(options)
	if err != nil {
		return err
	}
	report, err := bound.RunMaintenanceWake(ctx, wakeAllowed)
	if err != nil {
		return err
	}
	return json.NewEncoder(output).Encode(map[string]any{"state": "wake_completed", "workspace_id": state.Binding.WorkspaceID, "carrier_id": state.Binding.CarrierID, "current_release": state.CurrentRelease, "due_work": report, "native_scheduler_delivery": "not_attested_by_process_invocation"})
}
