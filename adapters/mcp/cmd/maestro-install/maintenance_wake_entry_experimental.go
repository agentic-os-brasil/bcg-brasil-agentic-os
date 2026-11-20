//go:build maestro_experimental_installed_seed

package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installer"
)

func experimentalMaintenanceWakeCommand(ctx context.Context, command, self string, args []string, role *installer.VerifiedExperimentalSeed) (bool, error) {
	if command != "maintenance-wake" {
		return false, nil
	}
	if role == nil {
		return true, errors.New("independently admitted native wake required")
	}
	s, err := installer.ParseMaintenanceWakeCommand(args, false)
	if err != nil {
		return true, err
	}
	o := installer.ClientOptions{SeededOperationOptions: installer.SeededOperationOptions{ExperimentalSeed: role, Self: self, Version: Version, AuthorityRegistrySHA256: AuthorityRegistrySHA256, ManagedRoot: s.ManagedRoot, UserHome: s.UserHome, DataRoot: filepath.Join(s.UserHome, ".maestro-install")}, Request: installer.ClientRequest{CarrierID: s.CarrierID, Workspace: s.Workspace}}
	launch, state, err := installer.PrepareMaintenanceWake(ctx, o, s.BindingSHA256)
	if err != nil {
		return true, err
	}
	osHome, err := installer.MaintenanceWakeOSHome()
	if err != nil {
		return true, err
	}
	if err := state.Binding.VerifyNativeWake(ctx, osHome); err != nil {
		return true, err
	}
	child := exec.CommandContext(ctx, launch.Executable, launch.Args...)
	child.Dir = s.Workspace
	child.Stdout, child.Stderr = os.Stdout, os.Stderr
	return true, child.Run()
}
