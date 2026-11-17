//go:build !maestro_experimental_installed_seed

package main

import (
	"context"
	"flag"
	"io"

	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
)

func experimentalCommandFlags(*flag.FlagSet, *product.Options) func() error {
	return func() error { return nil }
}
func experimentalEarlyCommand(context.Context, string, []string, io.Writer) (bool, error) {
	return false, nil
}
