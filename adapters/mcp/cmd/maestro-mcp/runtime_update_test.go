package main

import (
	"bytes"
	"context"
	"testing"

	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
)

func TestInstalledRuntimeLocalCommandsCannotGrantUninstalledAuthority(t *testing.T) {
	for _, command := range []string{"runtime-review", "runtime-apply", "maestro_apply", "install"} {
		t.Run(command, func(t *testing.T) {
			var output bytes.Buffer
			if err := runInstalledRuntime(context.Background(), command, product.Options{}, "", "", false, &output); err == nil {
				t.Fatal("uninstalled/unconfirmed command admitted")
			}
		})
	}
}
