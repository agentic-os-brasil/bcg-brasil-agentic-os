//go:build maestro_experimental_installed_seed

package product

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"runtime"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installer"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/processidentity"
)

// HandleInstalledClaudeHook owns one child invocation. The canonical native
// owner admits current bytes/carrier/selection, inspects the private product
// binding in its active guard, then performs its original late fences. Only
// after that guard is released may the event acquire domain locks. No binding,
// callback, state capability, cached approval or validation toggle escapes.
func HandleInstalledClaudeHook(ctx context.Context, o Options, event, selection, carrier string, in io.Reader, out io.Writer) error {
	b, err := newBound(o)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if o.ExperimentalSeed == nil || !o.GuardProcess || o.ExperimentalSeedRoot != o.ProductRoot {
		return errors.New("hook_binding_unavailable")
	}
	name := "maestro-install"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	owner := installer.ClientOptions{
		SeededOperationOptions: installer.SeededOperationOptions{
			ExperimentalSeed: o.ExperimentalSeed, Self: filepath.Join(o.ProductRoot, name),
			Version:                 o.ExperimentalSeed.Identity().InstallerVersion,
			AuthorityRegistrySHA256: o.AuthorityRegistrySHA256,
			ManagedRoot:             o.ProductRoot, UserHome: o.UserHome,
			DataRoot: filepath.Join(o.UserHome, ".maestro-install"),
		},
		Request: installer.ClientRequest{CarrierID: carrier, Workspace: o.WorkspacePath}, LookupEnv: o.LookupEnv,
	}
	finishAdmission := installer.BeginNativeHookDiagnostic("child_admission")
	_, err = installer.InspectStableNativeHook(ctx, owner, event, selection, func(launch installer.QualifiedLaunch) error {
		// Only this synchronous canonical-owner callback can reach private
		// binding preparation with an already fully admitted current State.
		if err := b.validateExperimentalSeedContext(ctx); err != nil {
			return err
		}
		state := launch.State
		if state.Source == nil || state.Binding.ManifestSHA256 != o.InstalledManifestSHA256 || state.Binding.ClientInputsSHA256 != o.InstalledClientInputsSHA256 {
			return errors.New("installed native hook release differs")
		}
		actual, err := processidentity.Current()
		if err != nil || !installedNativeHookImageMatches(actual, launch, o) {
			return errors.New("native hook must use the actual current installed server")
		}
		if err := b.fenceExperimentalStateContext(ctx, state); err != nil {
			return err
		}
		return b.validateBindingContext(ctx)
	})
	finishAdmission(err)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	finishHandler := installer.BeginNativeHookDiagnostic("handler")
	err = b.handleClaudeHook(event, installedHookInput{ctx, in}, installedHookOutput{ctx, out})
	finishHandler(err)
	return err
}

func installedNativeHookImageMatches(actual processidentity.Identity, launch installer.QualifiedLaunch, o Options) bool {
	state := launch.State
	name := "maestro-mcp"
	if state.TargetOS == "windows" {
		name += ".exe"
	}
	return actual.Path == launch.Executable && filepath.Clean(actual.Path) == filepath.Join(o.ProductRoot, "bin", name) &&
		len(actual.SHA256) == 64 && actual.SHA256 == state.Server.SHA256 && o.CLIVersion != "" && o.CLIVersion == state.Server.Version &&
		state.TargetOS == runtime.GOOS && state.TargetArch == runtime.GOARCH
}

type installedHookInput struct {
	context.Context
	io.Reader
}

func (r installedHookInput) Read(body []byte) (int, error) {
	if err := r.Err(); err != nil {
		return 0, err
	}
	n, err := r.Reader.Read(body)
	if cancelled := r.Err(); cancelled != nil {
		return n, cancelled
	}
	return n, err
}

type installedHookOutput struct {
	context.Context
	io.Writer
}

func (w installedHookOutput) Write(body []byte) (int, error) {
	if err := w.Err(); err != nil {
		return 0, err
	}
	return w.Writer.Write(body)
}
