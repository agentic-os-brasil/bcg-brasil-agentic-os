//go:build maestro_experimental_installed_seed

package setup

import (
	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installer"
)

func configureExperimentalInstall(o installer.Options, c Config) installer.Options {
	o.ExperimentalSeed = c.ExperimentalSeed
	o.NativeTrustMode = installer.NativeTrustExperimentalInstalled
	return o
}
func configureExperimentalBinding(o product.Options, c Config, root string) product.Options {
	o.ExperimentalSeed = c.ExperimentalSeed
	o.ExperimentalSeedRoot = root
	return o
}
