//go:build !maestro_experimental_installed_seed

package main

import "github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installer"

// The ordinary entry has no engineering input, flag, role reader or override.
func installerEntryAdmission(string) (*installer.VerifiedExperimentalSeed, error) { return nil, nil }
