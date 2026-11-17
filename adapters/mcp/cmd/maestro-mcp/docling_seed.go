package main

import (
	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/processidentity"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseverify"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
	"path/filepath"
	"time"
)

func seedDoclingAuthority(options *product.Options) {
	options.CLIVersion = Version
	if AuthorityRegistrySHA256 == "" {
		return
	}
	executable, err := processidentity.Current()
	if err != nil {
		return
	}
	directory := filepath.Dir(executable.Path)
	root := filepath.Dir(directory)
	var registry string
	switch filepath.Base(directory) {
	case "bin":
		registry = filepath.Join(root, "trust", "release-authority-registry.json")
	case "scripts", "server":
		registry = filepath.Join(root, "managed", "trust", "release-authority-registry.json")
	default:
		return
	}
	if err := scheduler.ValidateDirectory(filepath.Dir(registry)); err != nil {
		return
	}
	if _, err := releaseverify.LoadPinnedAuthorityRegistry(registry, AuthorityRegistrySHA256, time.Now); err != nil {
		return
	}
	options.AuthorityRegistryPath = registry
	options.AuthorityRegistrySHA256 = AuthorityRegistrySHA256
}
