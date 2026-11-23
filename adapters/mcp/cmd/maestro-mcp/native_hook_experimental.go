//go:build maestro_experimental_installed_seed

package main

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"time"

	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installer"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/processidentity"
)

type installedNativeHookSelection struct {
	Event, BindingSHA256, CarrierID    string
	ProductRoot, UserHome, Workspace   string
	ManifestSHA256, ClientInputsSHA256 string
	AdmissionSHA256, InstalledRoot     string
}

func parseInstalledNativeHook(args []string) (installedNativeHookSelection, error) {
	var s installedNativeHookSelection
	fields := map[string]*string{
		"--event": &s.Event, "--binding-sha256": &s.BindingSHA256,
		"--carrier-id": &s.CarrierID, "--product-root": &s.ProductRoot,
		"--user-home": &s.UserHome, "--workspace": &s.Workspace,
		"--installed-manifest": &s.ManifestSHA256, "--installed-client-inputs": &s.ClientInputsSHA256,
		"--experimental-seed-sha256": &s.AdmissionSHA256, "--experimental-installed-root": &s.InstalledRoot,
	}
	if len(args) != 2*len(fields) {
		return s, errors.New("closed installed native hook selection required")
	}
	for i := 0; i < len(args); i += 2 {
		field, ok := fields[args[i]]
		if !ok || *field != "" || args[i+1] == "" || len(args[i+1]) > 4096 || strings.ContainsAny(args[i+1], "\x00\r\n") {
			return s, errors.New("closed installed native hook selection required")
		}
		*field = args[i+1]
	}
	for _, path := range []string{s.ProductRoot, s.UserHome, s.Workspace, s.InstalledRoot} {
		if !filepath.IsAbs(path) || filepath.Clean(path) != path {
			return s, errors.New("canonical installed native hook paths required")
		}
	}
	if s.InstalledRoot != s.ProductRoot || !pilotWorkspaceID.MatchString(s.CarrierID) {
		return s, errors.New("installed native hook selection differs")
	}
	for _, pin := range []string{s.BindingSHA256, s.ManifestSHA256, s.ClientInputsSHA256, s.AdmissionSHA256} {
		if !pilotKitID.MatchString(pin) {
			return s, errors.New("installed native hook pins required")
		}
	}
	switch s.Event {
	case "SessionStart", "UserPromptSubmit", "PreToolUse", "PostToolUse", "SubagentStart", "SubagentStop", "Stop":
		return s, nil
	}
	return s, errors.New("installed native hook event unavailable")
}

func runInstalledNativeHook(ctx context.Context, args []string, input io.Reader, output io.Writer) error {
	return handleInstalledNativeHook(ctx, args, input, output, withInstalledNativeHookAdmission)
}

// This private seam composes the real parser and handler. Shipping callers
// always pass the immutable production admission function above; test-only
// subprocesses can exercise protocol delivery without manufacturing host proof.
func handleInstalledNativeHook(ctx context.Context, args []string, input io.Reader, output io.Writer, admit func(context.Context, installedNativeHookSelection, func(product.Options) error) error) error {
	s, err := parseInstalledNativeHook(args)
	if err != nil {
		return err
	}
	return admit(ctx, s, func(o product.Options) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		return product.HandleClaudeHook(o, s.Event, input, output)
	})
}

func withInstalledNativeHookAdmission(ctx context.Context, s installedNativeHookSelection, use func(product.Options) error) error {
	fixed, err := installer.LoadExperimentalInstalledContext(s.ProductRoot, AuthorityRegistrySHA256, time.Now)
	if err != nil {
		return err
	}
	if fixed.ManagedRoot != s.ProductRoot || fixed.UserHome != s.UserHome || fixed.ExperimentalSeed.Identity().AdmissionSHA256 != s.AdmissionSHA256 {
		return errors.New("installed native hook owner or admission differs")
	}
	o := installer.ClientOptions{SeededOperationOptions: fixed, Request: installer.ClientRequest{CarrierID: s.CarrierID, Workspace: s.Workspace}}
	// Reuse the original current-record native owner in this child, including
	// its source guard and four-field selection check. Do not execute its argv.
	// Release the installation lock before the handler acquires domain locks;
	// product.HandleClaudeHook independently revalidates the installed source.
	launch, err := installer.PrepareStableNativeHook(ctx, o, s.Event, s.BindingSHA256)
	if err != nil {
		return err
	}
	actual, err := processidentity.Current()
	if err != nil {
		return errors.New("native hook must use the actual current installed server")
	}
	if err := validateInstalledNativeHookCurrent(s, launch, actual, Version); err != nil {
		return err
	}
	options := product.Options{
		ExperimentalSeed: fixed.ExperimentalSeed, ExperimentalSeedRoot: fixed.ManagedRoot,
		ProductRoot: fixed.ManagedRoot, UserHome: fixed.UserHome, WorkspacePath: s.Workspace,
		InstalledManifestSHA256: launch.State.Binding.ManifestSHA256, InstalledClientInputsSHA256: launch.State.Binding.ClientInputsSHA256,
		GuardProcess: true,
	}
	seedDoclingAuthority(&options)
	return use(options)
}

func validateInstalledNativeHookCurrent(s installedNativeHookSelection, launch installer.QualifiedLaunch, actual processidentity.Identity, version string) error {
	if launch.State.Binding.ManifestSHA256 != s.ManifestSHA256 || launch.State.Binding.ClientInputsSHA256 != s.ClientInputsSHA256 {
		return errors.New("installed native hook release differs")
	}
	if actual.Path != launch.Executable || !installedCurrentImageMatches(actual.Path, actual.SHA256, version, s.ProductRoot, launch.State) {
		return errors.New("native hook must use the actual current installed server")
	}
	return nil
}
