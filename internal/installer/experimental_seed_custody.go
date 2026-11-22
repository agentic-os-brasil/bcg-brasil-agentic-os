package installer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/datahome"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installtx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseverify"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/updateservice"
)

const ExperimentalSeedAdmissionName = "experimental-installed-seed.json"
const experimentalSeedOriginName = "experimental-installed-origin.json"

// LoadExperimentalSeed reads only the fixed public role adjacent to a separately
// constructed experimental entry. It never discovers a key or accepts a trust flag.
func LoadExperimentalSeed(root, registryPin, version string, clock func() time.Time) (*VerifiedExperimentalSeed, error) {
	return LoadExperimentalSeedContext(context.Background(), root, registryPin, version, clock)
}

func LoadExperimentalSeedContext(ctx context.Context, root, registryPin, version string, clock func() time.Time) (*VerifiedExperimentalSeed, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	read := func(name string, limit int64) ([]byte, error) {
		path := filepath.Join(root, "trust", name)
		file, err := scheduler.OpenRegularFile(path)
		if err != nil {
			return nil, err
		}
		defer file.Close()
		if err := scheduler.ValidateSingleLink(file); err != nil {
			return nil, err
		}
		body, err := io.ReadAll(installerReadContext{ctx, io.LimitReader(file, limit+1)})
		if cancelled := ctx.Err(); cancelled != nil {
			return nil, cancelled
		}
		if err != nil || int64(len(body)) > limit {
			return nil, errors.New("bounded experimental public input required")
		}
		return body, nil
	}
	body, err := read(ExperimentalSeedAdmissionName, 128<<10)
	if err != nil {
		return nil, err
	}
	sig, err := read(ExperimentalSeedAdmissionName+".sig", 64)
	if err != nil {
		return nil, err
	}
	registry, err := read("release-authority-registry.json", 1<<20)
	if err != nil {
		return nil, err
	}
	role, err := VerifyExperimentalSeedAdmission(body, sig, registry, registryPin, clock)
	if err != nil {
		return nil, err
	}
	if version != "" && role.Identity().InstallerVersion != version {
		return nil, errors.New("experimental installer version differs from its compiled entry")
	}
	return role, ctx.Err()
}

type experimentalSeedOrigin struct {
	SchemaVersion   int    `json:"schema_version"`
	Profile         string `json:"profile"`
	Evidence        string `json:"evidence"`
	AdmissionSHA256 string `json:"admission_sha256"`
	InstallerSHA256 string `json:"installer_sha256"`
	RegistrySHA256  string `json:"registry_sha256"`
	UserHome        string `json:"user_home"`
	ManagedRoot     string `json:"managed_root"`
	DataRoot        string `json:"data_root"`
	BrainHome       string `json:"brain_home"`
	ReviewSHA256    string `json:"review_sha256"`
}

func expectedExperimentalOrigin(p Plan) (experimentalSeedOrigin, error) {
	if p.ExperimentalSeed == nil || p.NativeTrustMode != string(NativeTrustExperimentalInstalled) || p.ExperimentalBrainHome == "" || !isLowerSHA256(p.PlanDigest) {
		return experimentalSeedOrigin{}, errors.New("exact experimental installation review required")
	}
	i := *p.ExperimentalSeed
	return experimentalSeedOrigin{SchemaVersion: 1, Profile: ExperimentalSeedProfile, Evidence: "engineering_only", AdmissionSHA256: i.AdmissionSHA256, InstallerSHA256: p.BootstrapperSHA256, RegistrySHA256: p.RegistrySHA256, UserHome: p.UserHome, ManagedRoot: p.ManagedRoot, DataRoot: p.DataRoot, BrainHome: p.ExperimentalBrainHome, ReviewSHA256: p.PlanDigest}, nil
}

func stageExperimentalSeed(p Plan, role *VerifiedExperimentalSeed) error {
	if role == nil {
		return nil
	}
	want, err := expectedExperimentalOrigin(p)
	if err != nil {
		return err
	}
	body, sig := role.RecordBytes()
	for name, value := range map[string][]byte{ExperimentalSeedAdmissionName: body, ExperimentalSeedAdmissionName + ".sig": sig} {
		if err := scheduler.WriteNewPrivateFile(filepath.Join(p.ManagedRoot, "trust", name), value); err != nil {
			return err
		}
	}
	b, err := json.Marshal(want)
	if err != nil {
		return err
	}
	return scheduler.WriteNewPrivateFile(filepath.Join(p.ManagedRoot, "trust", experimentalSeedOriginName), b)
}

func readExperimentalOrigin(o SeededOperationOptions) (experimentalSeedOrigin, error) {
	return readExperimentalOriginContext(context.Background(), o)
}

func readExperimentalOriginContext(ctx context.Context, o SeededOperationOptions) (experimentalSeedOrigin, error) {
	var origin experimentalSeedOrigin
	if err := ctx.Err(); err != nil {
		return origin, err
	}
	if o.ExperimentalSeed == nil {
		return origin, errors.New("verified experimental installed-seed role required")
	}
	if err := o.ExperimentalSeed.Revalidate(); err != nil {
		return origin, err
	}
	body, err := scheduler.ReadPrivateStateFile(filepath.Join(o.ManagedRoot, "trust", experimentalSeedOriginName), 16<<10)
	if err != nil {
		return origin, err
	}
	if err := releasecontract.DecodeStrictJSON(body, &origin); err != nil {
		return origin, err
	}
	i := o.ExperimentalSeed.Identity()
	if origin.SchemaVersion != 1 || origin.Profile != ExperimentalSeedProfile || origin.Evidence != "engineering_only" || origin.AdmissionSHA256 != i.AdmissionSHA256 || origin.InstallerSHA256 != i.InstallerSHA256 || origin.RegistrySHA256 != i.RegistrySHA256 || origin.UserHome != o.UserHome || origin.ManagedRoot != o.ManagedRoot || origin.DataRoot != o.DataRoot || !isLowerSHA256(origin.ReviewSHA256) {
		return origin, errors.New("experimental installation origin differs; production or foreign installations cannot be adopted")
	}
	home, err := datahome.PreviewInstallation(datahome.Options{ProductRoot: o.ManagedRoot, UserHome: o.UserHome})
	if err != nil || home.Home != origin.BrainHome {
		return origin, errors.New("experimental installation brain binding changed; preserve original binding")
	}
	return origin, ctx.Err()
}

func experimentalOperationOptions(p Plan, role *VerifiedExperimentalSeed) SeededOperationOptions {
	return SeededOperationOptions{Self: seedPath(p.ManagedRoot, p.TargetOS, p.Profile), Version: p.BootstrapperVersion, AuthorityRegistrySHA256: p.RegistrySHA256, UserHome: p.UserHome, ManagedRoot: p.ManagedRoot, DataRoot: p.DataRoot, ExperimentalSeed: role}
}

// VerifyExperimentalInstalledBinding is a read-only child admission check. The
// opaque role cannot turn a production installation into an experimental one.
func VerifyExperimentalInstalledBinding(o SeededOperationOptions) error {
	return VerifyExperimentalInstalledBindingContext(context.Background(), o)
}

func VerifyExperimentalInstalledBindingContext(ctx context.Context, o SeededOperationOptions) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if o.ExperimentalSeed == nil || filepath.Clean(o.Self) != seedPath(o.ManagedRoot, runtime.GOOS, releasecontract.ModernProfile) {
		return errors.New("fixed experimental installed seed required")
	}
	if err := recheckExperimentalOperationContext(ctx, o); err != nil {
		return err
	}
	state, err := installtx.ReadSourceState(o.DataRoot, o.ManagedRoot)
	if err != nil {
		return err
	}
	if err := o.ExperimentalSeed.VerifyReleaseIdentity(state.Binding.ManifestSHA256, state.Binding.ClientInputsSHA256); err != nil {
		return err
	}
	if state.Source != nil {
		return o.ExperimentalSeed.VerifySource(state.Source.Tuple)
	}
	return ctx.Err()
}

// This is a narrowing final fence, not a replacement current-state lock. It
// performs no writes and production's nil-admission branch remains an identity.
func recheckExperimentalOperation(o SeededOperationOptions) error {
	return recheckExperimentalOperationContext(context.Background(), o)
}

func recheckExperimentalOperationContext(ctx context.Context, o SeededOperationOptions) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if o.ExperimentalSeed == nil {
		return nil
	}
	if err := o.ExperimentalSeed.Revalidate(); err != nil {
		return err
	}
	if err := o.ExperimentalSeed.VerifyInstallerContext(ctx, o.Self, o.Version, runtime.GOOS, runtime.GOARCH); err != nil {
		return err
	}
	if _, err := readExperimentalOriginContext(ctx, o); err != nil {
		return err
	}
	fresh, err := LoadExperimentalSeedContext(ctx, o.ManagedRoot, o.AuthorityRegistrySHA256, o.Version, o.ExperimentalSeed.clock)
	if cancelled := ctx.Err(); cancelled != nil {
		return cancelled
	}
	if err != nil || fresh.Identity() != o.ExperimentalSeed.Identity() {
		return errors.New("installed experimental role changed")
	}
	return ctx.Err()
}

// The existing rollback transaction authenticates the previous release. This
// extra engineering fence only narrows that target to the same finite role.
func verifyExperimentalRollback(o SeededOperationOptions) error {
	if o.ExperimentalSeed == nil {
		return nil
	}
	if err := recheckExperimentalOperation(o); err != nil {
		return err
	}
	state, err := installtx.ReadSourceState(o.DataRoot, o.ManagedRoot)
	if err != nil {
		return err
	}
	if state.Previous == nil {
		return errors.New("no previous finite experimental release available")
	}
	return o.ExperimentalSeed.VerifyReleaseIdentity(state.Previous.Binding.ManifestSHA256, state.Previous.Binding.ClientInputsSHA256)
}

// LoadExperimentalInstalledContext derives participant selections only from the
// existing fixed private origin, never a shared publisher record or tool input.
func LoadExperimentalInstalledContext(root, pin string, clock func() time.Time) (SeededOperationOptions, error) {
	return LoadExperimentalInstalledContextWithContext(context.Background(), root, pin, clock)
}

// LoadExperimentalInstalledContextWithContext preserves the same origin, role
// and binding checks while consuming the original operation's remaining budget.
func LoadExperimentalInstalledContextWithContext(ctx context.Context, root, pin string, clock func() time.Time) (SeededOperationOptions, error) {
	role, err := LoadExperimentalSeedContext(ctx, root, pin, "", clock)
	if err != nil {
		return SeededOperationOptions{}, err
	}
	body, err := scheduler.ReadPrivateStateFile(filepath.Join(root, "trust", experimentalSeedOriginName), 16<<10)
	if err != nil {
		return SeededOperationOptions{}, err
	}
	var origin experimentalSeedOrigin
	if err := releasecontract.DecodeStrictJSON(body, &origin); err != nil {
		return SeededOperationOptions{}, err
	}
	if origin.ManagedRoot != root || !filepath.IsAbs(origin.UserHome) || filepath.Clean(origin.UserHome) != origin.UserHome || origin.DataRoot != filepath.Join(origin.UserHome, ".maestro-install") {
		return SeededOperationOptions{}, errors.New("fixed experimental origin required")
	}
	o := SeededOperationOptions{ExperimentalSeed: role, Self: seedPath(root, runtime.GOOS, releasecontract.ModernProfile), Version: role.Identity().InstallerVersion, AuthorityRegistrySHA256: pin, ManagedRoot: root, UserHome: origin.UserHome, DataRoot: origin.DataRoot}
	if err := VerifyExperimentalInstalledBindingContext(ctx, o); err != nil {
		return SeededOperationOptions{}, err
	}
	return o, ctx.Err()
}

func buildClientsWithSeedFence(ctx context.Context, o ClientOptions, state installtx.State, binding ClientBinding, previous *ClientRecord, build ClientMaterializer) (ClientMaterialization, error) {
	if err := recheckExperimentalOperation(o.SeededOperationOptions); err != nil {
		return ClientMaterialization{}, err
	}
	material, err := build(ctx, state, binding, previous)
	if err != nil {
		return material, err
	}
	if err := recheckExperimentalOperation(o.SeededOperationOptions); err != nil {
		return ClientMaterialization{}, err
	}
	return material, nil
}

func verifyExperimentalPreparation(o Options, p *Plan) error {
	if o.ExperimentalSeed == nil {
		return nil
	}
	if o.NativeTrustMode != NativeTrustExperimentalInstalled || o.VerifyNative != nil || !emptyLocalBetaPins(o.LocalBetaPins) {
		return errors.New("experimental admission cannot masquerade as native strict or accept a verifier override")
	}
	if err := o.ExperimentalSeed.VerifyInstaller(o.Bootstrapper, p.BootstrapperVersion, o.TargetOS, o.TargetArch); err != nil {
		return err
	}
	home, err := datahome.PreviewInstallation(datahome.Options{ProductRoot: o.ManagedRoot, UserHome: o.UserHome})
	if err != nil {
		return err
	}
	i := o.ExperimentalSeed.Identity()
	p.ExperimentalSeed, p.ExperimentalBrainHome = &i, home.Home
	if _, err := os.Lstat(o.DataRoot); err == nil {
		if _, err := readExperimentalOrigin(experimentalOperationOptions(*p, o.ExperimentalSeed)); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func verifyInstallerArtifact(ctx context.Context, o Options, path string) error {
	if o.ExperimentalSeed != nil {
		return o.ExperimentalSeed.VerifyInstaller(path, o.ExperimentalSeed.Identity().InstallerVersion, o.TargetOS, o.TargetArch)
	}
	return o.VerifyNative(ctx, path)
}

type experimentalSeedStatus struct {
	SchemaVersion           int                       `json:"schema_version"`
	Product                 string                    `json:"product"`
	Component               string                    `json:"component"`
	Protocol                string                    `json:"protocol"`
	InstallerVersion        string                    `json:"installer_version"`
	AuthorityRegistrySHA256 string                    `json:"authority_registry_sha256"`
	ExperimentalSeed        *ExperimentalSeedIdentity `json:"experimental_seed"`
}

func readExperimentalSeedStatus(ctx context.Context, path string, run commandRunner) (experimentalSeedStatus, error) {
	bounded, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	body, err := run(bounded, path, "seed-status")
	if err != nil {
		return experimentalSeedStatus{}, fmt.Errorf("read experimental seed status: %w", err)
	}
	if len(body) > 1<<20 {
		return experimentalSeedStatus{}, errors.New("bounded experimental seed status required")
	}
	var out experimentalSeedStatus
	if err := releasecontract.DecodeStrictJSON(body, &out); err != nil {
		return out, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		return out, err
	}
	if len(fields) != 7 || out.SchemaVersion != 3 || out.Product != "maestro" || out.Component != "experimental_installer" || out.Protocol != ExperimentalSeedProtocol || out.ExperimentalSeed == nil {
		return out, errors.New("closed experimental seed status required")
	}
	return out, nil
}

func experimentalStatusMatchesIdentity(status experimentalSeedStatus, identity ExperimentalSeedIdentity) bool {
	return status.SchemaVersion == 3 && status.Product == "maestro" && status.Component == "experimental_installer" && status.Protocol == ExperimentalSeedProtocol && status.InstallerVersion == identity.InstallerVersion && status.AuthorityRegistrySHA256 == identity.RegistrySHA256 && status.ExperimentalSeed != nil && *status.ExperimentalSeed == identity
}

func readReviewedSeedStatus(ctx context.Context, path string, run commandRunner, plan Plan, role *VerifiedExperimentalSeed) error {
	if role == nil {
		status, err := readSeedStatus(ctx, path, run)
		if err != nil {
			return err
		}
		if !seedMatchesPlan(status, plan) {
			return errors.New("installed bootstrapper seed does not match the confirmed release package")
		}
		return nil
	}
	if !activationAdmissionMatches(plan, role) {
		return errors.New("experimental review identity differs")
	}
	status, err := readExperimentalSeedStatus(ctx, path, run)
	if err != nil {
		return err
	}
	if !experimentalStatusMatchesIdentity(status, role.Identity()) {
		return errors.New("experimental installed seed differs")
	}
	return nil
}

func activationAdmissionMatches(plan Plan, role *VerifiedExperimentalSeed) bool {
	if role == nil {
		return plan.ExperimentalSeed == nil && plan.ExperimentalBrainHome == "" && plan.NativeTrustMode == string(NativeTrustStrict)
	}
	return plan.ExperimentalSeed != nil && *plan.ExperimentalSeed == role.Identity() && plan.NativeTrustMode == string(NativeTrustExperimentalInstalled)
}

func verifyExperimentalActivation(o SeededActivationOptions) error {
	p := o.Plan
	if !activationAdmissionMatches(p, o.ExperimentalSeed) {
		return errors.New("experimental activation identity differs from local review")
	}
	if _, err := os.Lstat(p.DataRoot); !errors.Is(err, os.ErrNotExist) {
		return errors.New("experimental first activation cannot adopt existing installation state")
	}
	want, err := expectedExperimentalOrigin(p)
	if err != nil {
		return err
	}
	got, err := readExperimentalOrigin(experimentalOperationOptions(p, o.ExperimentalSeed))
	if err != nil || got != want {
		return errors.New("experimental child origin differs from the exact local review")
	}
	fresh, err := LoadExperimentalSeed(p.ManagedRoot, o.AuthorityRegistrySHA256, o.Version, o.ExperimentalSeed.clock)
	if err != nil || fresh.Identity() != *p.ExperimentalSeed {
		return errors.New("experimental child role differs from staged review")
	}
	return nil
}

func admitExperimentalOperationSeed(ctx context.Context, o SeededOperationOptions) (releaseverify.AuthorityRegistry, error) {
	if o.VerifyNative != nil {
		return releaseverify.AuthorityRegistry{}, errors.New("experimental operation cannot accept a native verifier override")
	}
	if err := VerifyExperimentalInstalledBindingContext(ctx, o); err != nil {
		return releaseverify.AuthorityRegistry{}, err
	}
	status, err := readExperimentalSeedStatus(ctx, o.Self, execCommand)
	if err != nil || !experimentalStatusMatchesIdentity(status, o.ExperimentalSeed.Identity()) {
		return releaseverify.AuthorityRegistry{}, errors.New("actual child lacks the exact experimental seed status")
	}
	return releaseverify.LoadPinnedAuthorityRegistry(filepath.Join(o.ManagedRoot, "trust", "release-authority-registry.json"), o.AuthorityRegistrySHA256, time.Now)
}

// ExperimentalSeedStatus is emitted only by the compile-time experimental entry.
// Production schema 2 consumers cannot accept this distinct evidence contract.
func ExperimentalSeedStatus(role *VerifiedExperimentalSeed) (any, error) {
	if role == nil {
		return nil, errors.New("verified experimental seed required")
	}
	if err := role.Revalidate(); err != nil {
		return nil, err
	}
	i := role.Identity()
	return experimentalSeedStatus{SchemaVersion: 3, Product: "maestro", Component: "experimental_installer", Protocol: ExperimentalSeedProtocol, InstallerVersion: i.InstallerVersion, AuthorityRegistrySHA256: i.RegistrySHA256, ExperimentalSeed: &i}, nil
}

func experimentalSyncedAdmission(role *VerifiedExperimentalSeed) updateservice.SyncedAdmission {
	if role == nil {
		return nil
	}
	return role
}

// ExperimentalChildArgs carries a fixed reviewed installation identity only to
// a separately compiled experimental child. A production parser has no such flags.
func ExperimentalChildArgs(args []string, role *VerifiedExperimentalSeed, managedRoot string) []string {
	if role == nil {
		return args
	}
	return append(args, "--experimental-seed-sha256", role.Identity().AdmissionSHA256, "--experimental-installed-root", managedRoot)
}
