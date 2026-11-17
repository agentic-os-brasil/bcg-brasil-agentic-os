//go:build maestro_experimental_installed_seed

package main

import (
	"path/filepath"
	"runtime"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installer"
)

func installerEntryAdmission(self string) (*installer.VerifiedExperimentalSeed, error) {
	role, err := installer.LoadExperimentalSeed(filepath.Dir(self), AuthorityRegistrySHA256, Version, time.Now)
	if err != nil {
		return nil, err
	}
	if err := role.VerifyInstaller(self, Version, runtime.GOOS, runtime.GOARCH); err != nil {
		return nil, err
	}
	return role, nil
}
