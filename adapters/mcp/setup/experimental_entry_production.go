//go:build !maestro_experimental_installed_seed

package setup

import (
	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installer"
)

func configureExperimentalInstall(o installer.Options, _ Config) installer.Options       { return o }
func configureExperimentalBinding(o product.Options, _ Config, _ string) product.Options { return o }
