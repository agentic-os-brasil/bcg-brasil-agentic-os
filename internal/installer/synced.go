package installer

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installtx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseverify"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/updateplan"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/updateservice"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/userlevel"
)

// SeededOperationOptions are trusted process composition. Production entrypoints
// supply their compiled version/pin and strict native verifier; no command-line
// trust override is accepted. The engineering verifier seam remains in process.
type SeededOperationOptions struct {
	Self, Version, AuthorityRegistrySHA256 string
	UserHome, ManagedRoot, DataRoot        string
	VerifyNative                           nativeVerifier
}

func admitOperationSeed(ctx context.Context, o SeededOperationOptions) (releaseverify.AuthorityRegistry, error) {
	if err := userlevel.EnsureNotElevated(); err != nil {
		return releaseverify.AuthorityRegistry{}, err
	}
	if o.UserHome == "" || o.DataRoot != filepath.Join(o.UserHome, ".maestro-install") || filepath.Clean(o.Self) != seedPath(o.ManagedRoot, runtime.GOOS, releasecontract.ModernProfile) {
		return releaseverify.AuthorityRegistry{}, errors.New("operation must use independently admitted installed installer and fixed private custody")
	}
	if err := validateModernCustodyRoots(Options{UserHome: o.UserHome, ManagedRoot: o.ManagedRoot, DataRoot: o.DataRoot}); err != nil {
		return releaseverify.AuthorityRegistry{}, err
	}
	if err := validateModernInputFiles([]string{o.Self, filepath.Join(o.ManagedRoot, "trust", "release-authority-registry.json")}); err != nil {
		return releaseverify.AuthorityRegistry{}, err
	}
	verifyNative := o.VerifyNative
	if verifyNative == nil {
		verifyNative = func(ctx context.Context, path string) error {
			return nativeSignatureCheck(ctx, path, NativeTrustStrict)
		}
	}
	if err := validateRegular(o.Self, maxBootstrapperBytes); err != nil {
		return releaseverify.AuthorityRegistry{}, err
	}
	if err := verifyNative(ctx, o.Self); err != nil {
		return releaseverify.AuthorityRegistry{}, fmt.Errorf("native installer self-trust: %w", err)
	}
	status, err := readSeedStatus(ctx, o.Self, execCommand)
	if err != nil {
		return releaseverify.AuthorityRegistry{}, err
	}
	if status.SchemaVersion != 2 || status.Product != "maestro" || status.Component != "installer" || status.Protocol != releasecontract.InstallerProtocol || status.InstallerVersion != o.Version || status.AuthorityRegistrySHA256 != o.AuthorityRegistrySHA256 {
		return releaseverify.AuthorityRegistry{}, errors.New("actual installer seed differs from compiled version/authority")
	}
	return releaseverify.LoadPinnedAuthorityRegistry(filepath.Join(o.ManagedRoot, "trust", "release-authority-registry.json"), o.AuthorityRegistrySHA256, time.Now)
}

type QualifiedLaunchOptions struct {
	SeededOperationOptions
	ExpectedBinding installtx.ReleaseBinding
	Workspace       string
}
type QualifiedLaunch struct {
	Executable string
	Args       []string
	State      installtx.State
}

// PrepareQualifiedLaunch chooses only the active admitted server; callers may
// attach stdio and execute this exact argv. It has no executable/path selection
// input and performs no source refresh, enrollment, repair or activation.
func PrepareQualifiedLaunch(ctx context.Context, o QualifiedLaunchOptions) (QualifiedLaunch, error) {
	if _, err := admitOperationSeed(ctx, o.SeededOperationOptions); err != nil {
		return QualifiedLaunch{}, err
	}
	state, err := installtx.ValidateQualifiedLaunch(o.ManagedRoot, o.DataRoot, o.ExpectedBinding, time.Now())
	if err != nil {
		return QualifiedLaunch{}, err
	}
	if o.ExpectedBinding.ManifestSHA256 == "" || o.ExpectedBinding.ClientInputsSHA256 == "" {
		return QualifiedLaunch{}, errors.New("qualified carrier requires exact immutable installed release binding")
	}
	args := []string{"serve", "--product-root", o.ManagedRoot, "--user-home", o.UserHome, "--installed-manifest", state.Binding.ManifestSHA256, "--installed-client-inputs", state.Binding.ClientInputsSHA256}
	if o.Workspace != "" {
		args = append(args, "--workspace", o.Workspace)
	}
	return QualifiedLaunch{Executable: installedExecutablePath(o.ManagedRoot, state.TargetOS, state.Profile), Args: args, State: state}, nil
}

type SeededUpdateOptions struct {
	SeededOperationOptions
	PlanID string
}

func validateSeededPending(o SeededUpdateOptions) error {
	pending, err := updateservice.LoadPending(o.DataRoot)
	if err != nil {
		return err
	}
	if pending.SchemaVersion != 2 || pending.Plan.SchemaVersion != 3 || pending.Plan.ID != o.PlanID || pending.Plan.InstallerVersion != o.Version {
		return errors.New("source-aware pending update requires the exact modern plan and independently admitted installer")
	}
	return updateplan.Validate(pending.Plan)
}

func ReviewPendingSeeded(ctx context.Context, o SeededUpdateOptions) (updateservice.Pending, error) {
	r, err := admitOperationSeed(ctx, o.SeededOperationOptions)
	if err != nil {
		return updateservice.Pending{}, err
	}
	if err := validateSeededPending(o); err != nil {
		return updateservice.Pending{}, err
	}
	pending, _, err := updateservice.ConfirmPending(o.DataRoot, o.ManagedRoot, o.PlanID, r)
	if err != nil {
		return updateservice.Pending{}, err
	}
	if pending.SchemaVersion != 2 || pending.Plan.InstallerVersion != o.Version {
		return updateservice.Pending{}, errors.New("pending update requires another independently admitted installer")
	}
	return pending, nil
}

// UpdateSeeded reauthenticates signed pending truth, source custody, native seed,
// real candidate identity and prepared bytes through the existing transaction.
func UpdateSeeded(ctx context.Context, o SeededUpdateOptions) (installtx.State, error) {
	r, err := admitOperationSeed(ctx, o.SeededOperationOptions)
	if err != nil {
		return installtx.State{}, err
	}
	if err := validateSeededPending(o); err != nil {
		return installtx.State{}, err
	}
	pending, verified, err := updateservice.ConfirmPending(o.DataRoot, o.ManagedRoot, o.PlanID, r)
	if err != nil {
		return installtx.State{}, err
	}
	if pending.SchemaVersion != 2 || verified.Manifest.Delivery.InstallerVersion != o.Version {
		return installtx.State{}, errors.New("signed update requires a different independently admitted installer version")
	}
	options := installtx.PrepareOptions{UserHome: o.UserHome, Transition: "update", ConfirmationPlanID: pending.Plan.ID, FromRelease: pending.Plan.FromRelease, FromChannel: pending.Plan.FromChannel, FromCLIVersion: pending.Plan.FromCLIVersion, FromBundleVersion: pending.Plan.FromBundleVersion, TargetOS: pending.Plan.TargetOS, TargetArch: pending.Plan.TargetArch, ManagedRoot: o.ManagedRoot, DataRoot: o.DataRoot, Source: pending.Plan.Source, SourceStateSHA256: pending.Plan.FromStateSHA256}
	if err := installtx.Activate(pending.ActivationPlanPath, verified, installtx.ActivateOptions{PrepareOptions: options}); err != nil {
		return installtx.State{}, err
	}
	state, err := installtx.ReadSourceState(o.DataRoot, o.ManagedRoot)
	if err != nil {
		return state, err
	}
	if err := installtx.CheckActivatedServer(o.ManagedRoot, state); err != nil {
		return state, err
	}
	_, err = installtx.ValidateQualifiedLaunch(o.ManagedRoot, o.DataRoot, state.Binding, time.Now())
	return state, err
}

func ReconcileSeeded(ctx context.Context, o SeededUpdateOptions) (installtx.State, bool, error) {
	r, err := admitOperationSeed(ctx, o.SeededOperationOptions)
	if err != nil {
		return installtx.State{}, false, err
	}
	if err := validateSeededPending(o); err != nil {
		return installtx.State{}, false, err
	}
	_, applied, err := updateservice.ReconcilePending(o.DataRoot, o.ManagedRoot, o.PlanID, r)
	state, readErr := installtx.ReadSourceState(o.DataRoot, o.ManagedRoot)
	if err != nil {
		return state, applied, err
	}
	if readErr != nil {
		return state, applied, readErr
	}
	if applied {
		_, err = installtx.ValidateQualifiedLaunch(o.ManagedRoot, o.DataRoot, state.Binding, time.Now())
	}
	return state, applied, err
}

type SeededRollbackOptions struct {
	SeededOperationOptions
	ReviewDigest string
}

func ReviewRollbackSeeded(ctx context.Context, o SeededOperationOptions) (installtx.RollbackReview, error) {
	if _, err := admitOperationSeed(ctx, o); err != nil {
		return installtx.RollbackReview{}, err
	}
	return installtx.ReviewRollback(o.ManagedRoot, o.DataRoot)
}
func RollbackSeeded(ctx context.Context, o SeededRollbackOptions) (installtx.State, error) {
	if _, err := admitOperationSeed(ctx, o.SeededOperationOptions); err != nil {
		return installtx.State{}, err
	}
	review, err := installtx.ReviewRollback(o.ManagedRoot, o.DataRoot)
	if err != nil {
		return installtx.State{}, err
	}
	if review.Digest != o.ReviewDigest {
		return installtx.State{}, errors.New("exact rollback review changed")
	}
	if err := installtx.RollbackQualified(o.ManagedRoot, o.DataRoot, o.ReviewDigest); err != nil {
		return installtx.State{}, err
	}
	state, err := installtx.ValidateQualifiedLaunch(o.ManagedRoot, o.DataRoot, installtx.ReleaseBinding{}, time.Now())
	return state, err
}

type SeededSourceOptions struct {
	SeededOperationOptions
	SourceRoot     string
	ExpectedSource releasecontract.SourceTuple
	ReviewDigest   string
}

func ReviewSourceSeeded(ctx context.Context, o SeededSourceOptions) (updateservice.SyncedReview, error) {
	r, err := admitOperationSeed(ctx, o.SeededOperationOptions)
	if err != nil {
		return updateservice.SyncedReview{}, err
	}
	return updateservice.ReviewSynced(updateservice.SyncedOptions{UserHome: o.UserHome, ManagedRoot: o.ManagedRoot, DataRoot: o.DataRoot, SourceRoot: o.SourceRoot, ExpectedSource: o.ExpectedSource, Registry: r})
}
func CheckSourceSeeded(ctx context.Context, o SeededSourceOptions) (updateservice.CheckResult, error) {
	r, err := admitOperationSeed(ctx, o.SeededOperationOptions)
	if err != nil {
		return updateservice.CheckResult{}, err
	}
	return updateservice.CheckSynced(updateservice.SyncedOptions{UserHome: o.UserHome, ManagedRoot: o.ManagedRoot, DataRoot: o.DataRoot, SourceRoot: o.SourceRoot, ExpectedSource: o.ExpectedSource, Registry: r}, o.ReviewDigest)
}
