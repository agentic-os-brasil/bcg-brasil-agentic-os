//go:build !maestro_experimental_installed_seed

package main

import (
	"context"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installer"
)

func experimentalMaintenanceWakeCommand(context.Context, string, string, []string, *installer.VerifiedExperimentalSeed) (bool, error) {
	return false, nil
}
