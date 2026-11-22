//go:build maestro_experimental_installed_seed

package main

import (
	"context"
	"errors"
	"flag"
	"io"
	"testing"

	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
)

func TestExperimentalMaintenanceBinderChecksCancellationBeforeAdmission(t *testing.T) {
	flags := flag.NewFlagSet("synthetic", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var options product.Options
	bind := experimentalCommandFlags(flags, &options)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := bind(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("experimental admission dropped caller cancellation: %v", err)
	}
	if options.ExperimentalSeed != nil || options.ExperimentalSeedRoot != "" {
		t.Fatal("canceled binder adopted an installed identity")
	}
}
