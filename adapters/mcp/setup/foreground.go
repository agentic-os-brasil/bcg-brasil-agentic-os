package setup

import (
	"context"
	"errors"

	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp/packaging"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installer"
)

// ObserveForegroundRuntime measures only the current admitted client and its
// dedicated purpose profile. Neither authentication nor a declaration invokes a model.
func ObserveForegroundRuntime(ctx context.Context, o installer.ClientOptions) (product.ForegroundRuntime, error) {
	var out product.ForegroundRuntime
	profile, err := ReviewForegroundProfile(ctx, o)
	if err != nil {
		return out, err
	}
	if !profile.Prepared {
		return out, ErrModelProfilePreparationRequired
	}
	observation, base, err := modelProfileObservation(ctx, o, profile)
	if err != nil {
		return out, err
	}
	loggedIn, err := modelAuthentication(ctx, profile, base)
	if err != nil {
		return out, err
	}
	if !loggedIn {
		return out, ErrModelProfileLoginRequired
	}
	configuration, err := packaging.ObserveWeeklyConfiguration(ctx, profile.ProfileDirectory, profile.WorkingDirectory)
	if err != nil {
		return out, err
	}
	check := func(c context.Context) error {
		current, e := ReviewForegroundProfile(c, o)
		if e != nil || current.Digest != profile.Digest {
			return errors.New("foreground_profile_or_source_changed")
		}
		return packaging.VerifyWeeklyConfiguration(c, configuration)
	}
	if err = check(ctx); err != nil {
		return out, err
	}
	return product.ForegroundRuntime{Executable: observation.Executable, ExecutableSHA256: observation.ExecutableSHA256, RuntimeVersion: observation.RuntimeVersion, ProfileDirectory: profile.ProfileDirectory, WorkingDirectory: profile.WorkingDirectory, ClientRecordSHA256: profile.ClientRecordSHA256, Configuration: configuration, Environment: base, Identity: observation.Declaration, Check: check}, nil
}
