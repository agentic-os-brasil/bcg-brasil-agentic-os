package setup

import (
	"context"
	"errors"

	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp/packaging"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installer"
)

// ObserveForegroundRuntime performs no model call. Local closure admission
// precedes version/auth startup; remote/OS policy remains explicitly unattested.
func ObserveForegroundRuntime(ctx context.Context, o installer.ClientOptions) (product.ForegroundRuntime, error) {
	var out product.ForegroundRuntime
	profile, err := ReviewForegroundProfile(ctx, o)
	if err != nil {
		return out, err
	}
	if !profile.Prepared {
		return out, ErrModelProfilePreparationRequired
	}
	base, err := modelBaseEnvironment(o, profile)
	if err != nil {
		return out, err
	}
	configuration, err := packaging.ObserveWeeklyConfiguration(ctx, profile.ProfileDirectory, profile.WorkingDirectory)
	if err != nil {
		return out, err
	}
	out = product.ForegroundRuntime{Executable: profile.Executable, ExecutableSHA256: profile.ExecutableSHA256, ProfileDirectory: profile.ProfileDirectory, WorkingDirectory: profile.WorkingDirectory, ClientRecordSHA256: profile.ClientRecordSHA256, Configuration: configuration, Environment: base, ManagedPolicyBoundary: packaging.ForegroundManagedPolicyBoundary, ContextBoundary: packaging.ForegroundContextBoundary}
	out.LocalClosureSHA256, err = packaging.ObserveForegroundLocalClosure(ctx, out)
	if err != nil {
		return product.ForegroundRuntime{}, err
	}
	observation, _, err := modelProfileObservation(ctx, o, profile)
	if err != nil {
		return product.ForegroundRuntime{}, err
	}
	loggedIn, err := modelAuthentication(ctx, profile, base)
	if err != nil {
		return product.ForegroundRuntime{}, err
	}
	if !loggedIn {
		return product.ForegroundRuntime{}, ErrModelProfileLoginRequired
	}
	out.RuntimeVersion, out.Identity = observation.RuntimeVersion, observation.Declaration
	measured := out
	check := func(c context.Context) error {
		current, e := ReviewForegroundProfile(c, o)
		if e != nil || current.Digest != profile.Digest {
			return errors.New("foreground_profile_or_source_changed")
		}
		return packaging.VerifyForegroundRuntimeSources(c, measured)
	}
	if err = check(ctx); err != nil {
		return product.ForegroundRuntime{}, err
	}
	out.Check = check
	return out, nil
}
