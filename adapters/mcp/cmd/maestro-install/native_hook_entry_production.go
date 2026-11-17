//go:build !maestro_experimental_installed_seed

package main

import (
	"context"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installer"
)

// Production neither recognizes this engineering command nor parses its flags.
func experimentalNativeHookCommand(context.Context, string, string, []string, *installer.VerifiedExperimentalSeed) (bool, error) {
	return false, nil
}
