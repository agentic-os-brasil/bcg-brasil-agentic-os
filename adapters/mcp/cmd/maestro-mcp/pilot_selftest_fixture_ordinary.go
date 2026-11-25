//go:build !maestro_experimental_installed_seed

package main

import (
	"context"
	"errors"
	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
)

func installedPilotPrepareFixture(context.Context, product.Options, string) error {
	return errors.New("installed_functional_profile_unavailable")
}
