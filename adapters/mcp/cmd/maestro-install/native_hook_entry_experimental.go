//go:build maestro_experimental_installed_seed

package main

import (
	"context"
	"errors"
	"flag"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installer"
)

func experimentalNativeHookCommand(ctx context.Context, command, self string, args []string, role *installer.VerifiedExperimentalSeed) (bool, error) {
	if command != "stable-native-hook" {
		return false, nil
	}
	if role == nil || len(args) != 12 {
		return true, errors.New("closed stable native hook selection required")
	}
	allowed := map[string]bool{"--event": true, "--binding-sha256": true, "--product-root": true, "--user-home": true, "--workspace": true, "--carrier-id": true}
	seen := map[string]bool{}
	for i := 0; i < len(args); i += 2 {
		if !allowed[args[i]] || seen[args[i]] || strings.ContainsAny(args[i+1], "\x00\r\n") {
			return true, errors.New("closed stable native hook selection required")
		}
		seen[args[i]] = true
	}
	f := flag.NewFlagSet(command, flag.ContinueOnError)
	f.SetOutput(io.Discard)
	var event, selection, root, user, workspace, carrier string
	f.StringVar(&event, "event", "", "")
	f.StringVar(&selection, "binding-sha256", "", "")
	f.StringVar(&root, "product-root", "", "")
	f.StringVar(&user, "user-home", "", "")
	f.StringVar(&workspace, "workspace", "", "")
	f.StringVar(&carrier, "carrier-id", "", "")
	if err := f.Parse(args); err != nil || f.NArg() != 0 {
		return true, errors.New("closed stable native hook selection required")
	}
	o := installer.ClientOptions{SeededOperationOptions: installer.SeededOperationOptions{ExperimentalSeed: role, Self: self, Version: Version, AuthorityRegistrySHA256: AuthorityRegistrySHA256, ManagedRoot: root, UserHome: user, DataRoot: filepath.Join(user, ".maestro-install")}, Request: installer.ClientRequest{CarrierID: carrier, Workspace: workspace}}
	launch, err := installer.PrepareStableNativeHook(ctx, o, event, selection)
	if err != nil {
		return true, err
	}
	child := exec.CommandContext(ctx, launch.Executable, launch.Args...)
	child.Stdin, child.Stdout, child.Stderr = os.Stdin, os.Stdout, os.Stderr
	return true, child.Run()
}
