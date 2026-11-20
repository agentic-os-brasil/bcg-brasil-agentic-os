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
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/userlevel"
)

// SeededActivationOptions is process composition. Version and registry pin are
// compiled seed inputs, never decoded from the reviewed plan. Production uses
// the canonical strict native verifier; engineering tests compose that same
// owner in process through VerifyNative, without a command-line trust override.
type SeededActivationOptions struct {
	ExperimentalSeed                       *VerifiedExperimentalSeed
	Self, Version, AuthorityRegistrySHA256 string
	Plan                                   Plan
	ReviewDigest                           string
	Confirmed                              bool
	VerifyNative                           nativeVerifier
}

// ActivateSeeded authenticates the installed seed itself and reconstructs the
// exact confirmed canonical review before the existing transaction may write.
// It introduces no approval service, secret, alternate verifier or transaction.
func ActivateSeeded(ctx context.Context, o SeededActivationOptions) (installtx.State, error) {
	if err := userlevel.EnsureNotElevated(); err != nil {
		return installtx.State{}, err
	}
	p := o.Plan
	if !o.Confirmed || len(o.ReviewDigest) != 64 || p.PlanDigest != o.ReviewDigest || PlanDigest(p) != o.ReviewDigest {
		return installtx.State{}, errors.New("explicit confirmed canonical install review is required")
	}
	if p.Profile != releasecontract.ModernProfile || !activationAdmissionMatches(p, o.ExperimentalSeed) ||
		p.BootstrapperVersion != o.Version || p.RegistrySHA256 != o.AuthorityRegistrySHA256 ||
		p.TargetOS != runtime.GOOS || p.TargetArch != runtime.GOARCH ||
		filepath.Clean(o.Self) != seedPath(p.ManagedRoot, p.TargetOS, p.Profile) {
		return installtx.State{}, errors.New("confirmed install review does not bind this independently seeded native installer")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	if o.ExperimentalSeed != nil {
		if o.VerifyNative != nil {
			return installtx.State{}, errors.New("experimental activation cannot accept native verifier overrides")
		}
		if err := verifyExperimentalActivation(o); err != nil {
			return installtx.State{}, err
		}
	}
	verifyNative := o.VerifyNative
	if verifyNative == nil && o.ExperimentalSeed == nil {
		verifyNative = func(ctx context.Context, path string) error {
			return nativeSignatureCheck(ctx, path, NativeTrustStrict)
		}
	}
	if err := verifyInstallerArtifact(ctx, Options{ExperimentalSeed: o.ExperimentalSeed, VerifyNative: verifyNative, TargetOS: p.TargetOS, TargetArch: p.TargetArch}, o.Self); err != nil {
		return installtx.State{}, fmt.Errorf("native installer self-trust: %w", err)
	}
	if err := validateRegular(o.Self, maxBootstrapperBytes); err != nil {
		return installtx.State{}, err
	}
	selfDigest, err := fileSHA256(o.Self)
	if err != nil || selfDigest != p.BootstrapperSHA256 {
		return installtx.State{}, errors.New("installed native seed differs from the exact confirmed source seed")
	}
	if _, err := releaseverify.LoadPinnedAuthorityRegistry(
		filepath.Join(p.ManagedRoot, "trust", "release-authority-registry.json"), o.AuthorityRegistrySHA256, time.Now,
	); err != nil {
		return installtx.State{}, err
	}
	// Reuse the canonical read-only owner: authenticate the original release,
	// original native source seed/status and exact paths again. A stored digest
	// or confirmation boolean alone cannot replace this recomputation.
	recomputed, verified, err := Prepare(Options{
		UserHome: p.UserHome, ReleaseDir: p.ReleaseDir, Bootstrapper: p.Bootstrapper,
		AuthorityRegistry: p.AuthorityRegistry, ManagedRoot: p.ManagedRoot, DataRoot: p.DataRoot,
		TargetOS: p.TargetOS, TargetArch: p.TargetArch, NativeTrustMode: NativeTrustMode(p.NativeTrustMode), ExperimentalSeed: o.ExperimentalSeed,
		VerifyNative: verifyNative,
	})
	if err != nil {
		return installtx.State{}, err
	}
	if recomputed.PlanDigest != o.ReviewDigest {
		return installtx.State{}, errors.New("confirmed install review changed before native activation")
	}
	options := installtx.PrepareOptions{
		UserHome: p.UserHome, Transition: "install", TargetOS: p.TargetOS, TargetArch: p.TargetArch,
		ManagedRoot: p.ManagedRoot, DataRoot: p.DataRoot,
	}
	plan, err := installtx.Prepare(verified, options)
	if err != nil {
		return installtx.State{}, err
	}
	if err := recheckExperimentalOperation(experimentalOperationOptions(p, o.ExperimentalSeed)); err != nil {
		return installtx.State{}, err
	}
	if err := installtx.Activate(plan, verified, installtx.ActivateOptions{PrepareOptions: options}); err != nil {
		return installtx.State{}, err
	}
	readCurrent := installtx.ReadStateForManagedRoot
	if verified.Manifest.Server.ExecutionContract == releasecontract.SyncedSourceContract {
		readCurrent = installtx.ReadSourceState
	}
	state, err := readCurrent(p.DataRoot, p.ManagedRoot)
	if err != nil {
		return installtx.State{}, err
	}
	if err := installtx.CheckActivatedServer(p.ManagedRoot, state); err != nil {
		return state, err
	}
	return state, recheckExperimentalOperation(experimentalOperationOptions(p, o.ExperimentalSeed))
}
