package main

import (
	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseverify"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
	"os"
	"path/filepath"
	"time"
)

func seedDoclingAuthority(options *product.Options) {
	options.CLIVersion = "0.1.0"
	if AuthorityRegistrySHA256 == "" {
		return
	}
	executable, err := os.Executable()
	if err != nil {
		return
	}
	registry := filepath.Join(filepath.Dir(filepath.Dir(executable)), "managed", "trust", "release-authority-registry.json")
	if err := scheduler.ValidateDirectory(filepath.Dir(registry)); err != nil {
		return
	}
	if _, err := releaseverify.LoadPinnedAuthorityRegistry(registry, AuthorityRegistrySHA256, time.Now); err != nil {
		return
	}
	options.AuthorityRegistryPath = registry
	options.AuthorityRegistrySHA256 = AuthorityRegistrySHA256
}
