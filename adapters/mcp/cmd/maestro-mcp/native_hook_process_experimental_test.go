//go:build maestro_experimental_installed_seed

package main

import (
	"context"
	"fmt"
	"os"

	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
)

// Present only in a go-test executable. This controlled delivery fixture uses
// the actual closed parser and product.HandleClaudeHook. It deliberately
// does NOT admit an installation, publish NativeEnforcement or qualify a host.
// No shipping flag, environment switch or mutable admission override exists.
func init() {
	if len(os.Args) < 2 || (os.Args[1] != "installed-native-hook" && os.Args[1] != "hook") {
		return
	}
	var err error
	if os.Args[1] == "hook" {
		err = run() // Preserve the actual generic packet-only command guard.
	} else {
		err = handleInstalledNativeHook(context.Background(), os.Args[2:], os.Stdin, os.Stdout,
			func(_ context.Context, s installedNativeHookSelection, use func(product.Options) error) error {
				return use(product.Options{ProductRoot: s.ProductRoot, UserHome: s.UserHome, WorkspacePath: s.Workspace})
			})
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "controlled offline native hook delivery:", err)
		os.Exit(1)
	}
	os.Exit(0)
}
