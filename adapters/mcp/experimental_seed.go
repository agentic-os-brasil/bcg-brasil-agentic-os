package product

import (
	"context"
	"errors"
	"path/filepath"
	"runtime"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installer"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installtx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/processidentity"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/updateservice"
)

func (b *Bound) validateExperimentalSeed() error {
	return b.validateExperimentalSeedContext(context.Background())
}

func (b *Bound) validateExperimentalSeedContext(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	role := b.options.ExperimentalSeed
	if role == nil {
		if b.options.ExperimentalSeedRoot != "" {
			return errors.New("experimental_seed_unavailable")
		}
		return nil
	}
	root := b.options.ExperimentalSeedRoot
	if root == "" || !filepath.IsAbs(root) || filepath.Clean(root) != root || b.options.AuthorityRegistrySHA256 != role.Identity().RegistrySHA256 {
		return errors.New("experimental_seed_binding_unavailable")
	}
	name := "maestro-install"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	o := installer.SeededOperationOptions{ExperimentalSeed: role, Self: filepath.Join(root, name), Version: role.Identity().InstallerVersion, AuthorityRegistrySHA256: b.options.AuthorityRegistrySHA256, UserHome: b.options.UserHome, ManagedRoot: root, DataRoot: filepath.Join(b.options.UserHome, ".maestro-install")}
	if err := installer.VerifyExperimentalInstalledBindingContext(ctx, o); err != nil {
		if cancelled := ctx.Err(); cancelled != nil {
			return cancelled
		}
		return errors.New("experimental_seed_binding_unavailable")
	}
	state, err := installtx.ReadSourceState(o.DataRoot, root)
	if err != nil || state.Server.Version != b.options.CLIVersion {
		return errors.New("experimental_server_binding_unavailable")
	}
	if b.options.GuardProcess {
		actual, err := processidentity.Current()
		if err != nil || actual.SHA256 != state.Server.SHA256 {
			return errors.New("experimental_server_identity_changed")
		}
	}
	if err := role.Revalidate(); err != nil {
		return err
	}
	return ctx.Err()
}

func (b *Bound) experimentalSourceAdmission() updateservice.SyncedAdmission {
	if b.options.ExperimentalSeed == nil {
		return nil
	}
	return b.options.ExperimentalSeed
}

// readInstalledSourceState keeps the original state owner and adds only the
// existing opaque engineering admission fence. Classification consumes the
// returned owning snapshot once; these checks never supply state authority.
func (b *Bound) readInstalledSourceState(dataRoot, managedRoot string) (installtx.State, error) {
	return b.readInstalledSourceStateContext(context.Background(), dataRoot, managedRoot)
}

func (b *Bound) readInstalledSourceStateContext(ctx context.Context, dataRoot, managedRoot string) (installtx.State, error) {
	if err := ctx.Err(); err != nil {
		return installtx.State{}, err
	}
	if err := b.validateExperimentalSeedContext(ctx); err != nil {
		return installtx.State{}, err
	}
	state, err := installtx.ReadSourceState(dataRoot, managedRoot)
	if err != nil {
		return state, err
	}
	if err = b.fenceExperimentalStateContext(ctx, state); err != nil {
		return installtx.State{}, err
	}
	return state, ctx.Err()
}
func (b *Bound) fenceExperimentalState(state installtx.State) error {
	return b.fenceExperimentalStateContext(context.Background(), state)
}

func (b *Bound) fenceExperimentalStateContext(ctx context.Context, state installtx.State) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := b.validateExperimentalSeedContext(ctx); err != nil {
		return err
	}
	return b.checkExperimentalStateRoleContext(ctx, state)
}

// Pure membership/lifetime checks, not installed admission. A composed read may
// use these between its full entry and post-qualified experimental fences.
func (b *Bound) checkExperimentalStateRoleContext(ctx context.Context, state installtx.State) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	role := b.options.ExperimentalSeed
	if role == nil {
		return nil
	}
	if err := role.VerifyReleaseIdentity(state.Binding.ManifestSHA256, state.Binding.ClientInputsSHA256); err != nil {
		return err
	}
	if state.Source != nil {
		if err := role.VerifySource(state.Source.Tuple); err != nil {
			return err
		}
	}
	if err := role.Revalidate(); err != nil {
		return err
	}
	return ctx.Err()
}
func (b *Bound) validatedQualifiedState(expected installtx.ReleaseBinding) (installtx.State, error) {
	return b.validatedQualifiedStateContext(context.Background(), expected)
}

func (b *Bound) validatedQualifiedStateContext(ctx context.Context, expected installtx.ReleaseBinding) (installtx.State, error) {
	if err := ctx.Err(); err != nil {
		return installtx.State{}, err
	}
	if err := b.validateExperimentalSeedContext(ctx); err != nil {
		return installtx.State{}, err
	}
	state, err := installtx.ValidateQualifiedLaunchContext(ctx, b.options.ProductRoot, filepath.Join(b.options.UserHome, ".maestro-install"), expected, time.Now())
	if err != nil {
		return state, err
	}
	if err = b.fenceExperimentalStateContext(ctx, state); err != nil {
		return installtx.State{}, err
	}
	return state, ctx.Err()
}
func (b *Bound) validatedActivatedState(expected installtx.ReleaseBinding) (installtx.State, error) {
	return b.validatedActivatedStateContext(context.Background(), expected)
}

func (b *Bound) validatedActivatedStateContext(ctx context.Context, expected installtx.ReleaseBinding) (installtx.State, error) {
	if err := ctx.Err(); err != nil {
		return installtx.State{}, err
	}
	if err := b.validateExperimentalSeedContext(ctx); err != nil {
		return installtx.State{}, err
	}
	state, err := installtx.ValidateActivatedCurrentContext(ctx, b.options.ProductRoot, filepath.Join(b.options.UserHome, ".maestro-install"), expected)
	if err != nil {
		return state, err
	}
	if err = b.fenceExperimentalStateContext(ctx, state); err != nil {
		return installtx.State{}, err
	}
	return state, ctx.Err()
}
func (b *Bound) withExperimentalSeedFence(action func() error) error {
	return b.withExperimentalSeedFenceContext(context.Background(), action)
}

func (b *Bound) withExperimentalSeedFenceContext(ctx context.Context, action func() error) error {
	if err := b.validateExperimentalSeedContext(ctx); err != nil {
		return err
	}
	if err := action(); err != nil {
		return err
	}
	return b.validateExperimentalSeedContext(ctx)
}
