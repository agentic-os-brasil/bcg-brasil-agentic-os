// Package installer owns the signed, user-space first-install handoff.
//
// The visual wizard is deliberately not a trust root. This package verifies
// the exact release directory, checks the bootstrapper seed binding and then
// delegates activation to the independently seeded bcgos-bootstrap process.
package installer

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	pathpkg "path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installtx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseverify"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
)

const maxBootstrapperBytes = 1 << 30

// Paths are the two user-level roots used by a first installation. They must
// remain siblings: managed code is never placed inside owner data.
type Paths struct {
	ManagedRoot string `json:"managed_root"`
	DataRoot    string `json:"data_root"`
}

// DefaultPaths returns the normal no-admin locations for a supported pilot OS.
func DefaultPaths(platform, home, localAppData string) (Paths, error) {
	dataRoot, err := workspace.DefaultDataRoot(platform, home, localAppData, "")
	if err != nil {
		return Paths{}, err
	}
	switch platform {
	case "windows":
		base := strings.TrimRight(localAppData, `\\/`)
		return Paths{ManagedRoot: base + `\Maestro`, DataRoot: dataRoot}, nil
	case "darwin":
		base := pathpkg.Join(home, "Library", "Application Support")
		return Paths{ManagedRoot: pathpkg.Join(base, "Maestro"), DataRoot: dataRoot}, nil
	default:
		return Paths{}, fmt.Errorf("installer does not support %q", platform)
	}
}

type nativeVerifier func(context.Context, string) error
type commandRunner func(context.Context, string, ...string) ([]byte, error)

// NativeTrustMode controls the platform-signature policy applied to the
// bootstrapper. The zero value is normalized to NativeTrustStrict. The local
// beta exception is intentionally Windows-only and remains bound to exact
// package identities and digests through LocalBetaPins.
type NativeTrustMode string

const (
	NativeTrustStrict NativeTrustMode = "strict"
	// NativeTrustCanarySimple keeps the signed release-manifest verification but
	// deliberately omits platform certificate and factory-pin gates for the
	// controlled Windows Canary installer.
	NativeTrustCanarySimple     NativeTrustMode = "canary-simple"
	NativeTrustWindowsLocalBeta NativeTrustMode = "windows-local-beta"
)

// LocalBetaPins are public, package-specific trust inputs compiled into a
// controlled local-beta installer bridge. They are not credentials. All four
// values are mandatory in Windows local-beta mode and forbidden in strict mode.
type LocalBetaPins struct {
	AuthorityRegistrySHA256 string
	BootstrapperSHA256      string
	Issuer                  string
	KeyID                   string
}

// Options controls a signed first install. ReleaseDir and Bootstrapper must
// come from the same immutable release package. AuthorityRegistry is the
// public registry that the seeded bootstrapper is compiled to pin.
type Options struct {
	ReleaseDir         string
	Bootstrapper       string
	AuthorityRegistry  string
	ManagedRoot        string
	DataRoot           string
	TargetOS           string
	TargetArch         string
	Clock              func() time.Time
	VerifyNative       nativeVerifier
	Run                commandRunner
	ExpectedPlanDigest string
	NativeTrustMode    NativeTrustMode
	LocalBetaPins      LocalBetaPins
}

// Plan is the result shown by the wizard before it allows the one install
// confirmation. No filesystem mutation happens while producing a Plan.
type Plan struct {
	Release             string `json:"release"`
	Channel             string `json:"channel"`
	TargetOS            string `json:"target_os"`
	TargetArch          string `json:"target_arch"`
	ManagedRoot         string `json:"managed_root"`
	DataRoot            string `json:"data_root"`
	ReleaseDir          string `json:"release_dir"`
	Bootstrapper        string `json:"bootstrapper"`
	AuthorityRegistry   string `json:"authority_registry"`
	ManifestSHA256      string `json:"manifest_sha256"`
	RegistrySHA256      string `json:"registry_sha256"`
	BootstrapperSHA256  string `json:"bootstrapper_sha256"`
	BootstrapperVersion string `json:"bootstrapper_version"`
	ReleaseIssuer       string `json:"release_issuer"`
	ReleaseKeyID        string `json:"release_key_id"`
	NativeTrustMode     string `json:"native_trust_mode"`
	PlanDigest          string `json:"plan_digest"`
}

// Result is the durable handoff summary returned after bootstrapper success.
type Result struct {
	Plan
	CLIPath     string    `json:"cli_path"`
	Output      string    `json:"bootstrapper_output"`
	Disposition string    `json:"disposition"`
	Recovery    *Recovery `json:"recovery,omitempty"`
}

// Recovery identifies preserved installer-owned material from an incomplete
// first install. It never contains workspace or owner content.
type Recovery struct {
	PlanDigest         string `json:"plan_digest"`
	ManagedRootBackup  string `json:"managed_root_backup,omitempty"`
	InstallStateBackup string `json:"install_state_backup,omitempty"`
}

type managedRootPreparation struct {
	Existing *Result
	Recovery *Recovery
}

type recoveryRecord struct {
	SchemaVersion      int    `json:"schema_version"`
	PlanDigest         string `json:"plan_digest"`
	Reason             string `json:"reason"`
	ManagedRoot        string `json:"managed_root"`
	ManagedRootBackup  string `json:"managed_root_backup"`
	InstallState       string `json:"install_state"`
	InstallStateBackup string `json:"install_state_backup"`
	Status             string `json:"status"`
}

type seedStatus struct {
	SchemaVersion           int    `json:"schema_version"`
	Product                 string `json:"product"`
	BootstrapperVersion     string `json:"bootstrapper_version"`
	AuthorityRegistrySHA256 string `json:"authority_registry_sha256"`
}

// Prepare validates every input and authenticates the release before any
// managed-root write. It is safe to call from a UI preview endpoint.
func Prepare(options Options) (Plan, releaseverify.VerifiedRelease, error) {
	options = withDefaults(options)
	if options.ReleaseDir == "" || options.Bootstrapper == "" || options.AuthorityRegistry == "" {
		return Plan{}, releaseverify.VerifiedRelease{}, errors.New("release directory, bootstrapper and authority registry are required")
	}
	if options.TargetOS != "windows" && options.TargetOS != "darwin" {
		return Plan{}, releaseverify.VerifiedRelease{}, fmt.Errorf("unsupported installer target %q", options.TargetOS)
	}
	if options.TargetArch != "amd64" && options.TargetArch != "arm64" {
		return Plan{}, releaseverify.VerifiedRelease{}, fmt.Errorf("unsupported installer architecture %q", options.TargetArch)
	}
	if options.ManagedRoot == "" || options.DataRoot == "" {
		return Plan{}, releaseverify.VerifiedRelease{}, errors.New("managed and owner-data roots are required")
	}
	for name, value := range map[string]string{
		"release directory":  options.ReleaseDir,
		"bootstrapper":       options.Bootstrapper,
		"authority registry": options.AuthorityRegistry,
		"managed root":       options.ManagedRoot,
		"owner-data root":    options.DataRoot,
	} {
		absolute, err := filepath.Abs(value)
		if err != nil {
			return Plan{}, releaseverify.VerifiedRelease{}, fmt.Errorf("normalize %s: %w", name, err)
		}
		switch name {
		case "release directory":
			options.ReleaseDir = filepath.Clean(absolute)
		case "bootstrapper":
			options.Bootstrapper = filepath.Clean(absolute)
		case "authority registry":
			options.AuthorityRegistry = filepath.Clean(absolute)
		case "managed root":
			options.ManagedRoot = filepath.Clean(absolute)
		case "owner-data root":
			options.DataRoot = filepath.Clean(absolute)
		}
	}
	managedRoot, err := canonicalInstallRoot(options.ManagedRoot, "managed root")
	if err != nil {
		return Plan{}, releaseverify.VerifiedRelease{}, err
	}
	dataRoot, err := canonicalInstallRoot(options.DataRoot, "owner-data root")
	if err != nil {
		return Plan{}, releaseverify.VerifiedRelease{}, err
	}
	options.ManagedRoot = managedRoot
	options.DataRoot = dataRoot
	if within(options.ManagedRoot, options.DataRoot) || within(options.DataRoot, options.ManagedRoot) {
		return Plan{}, releaseverify.VerifiedRelease{}, errors.New("managed and owner-data roots must be separate")
	}
	registry, err := releaseverify.LoadAuthorityRegistry(options.AuthorityRegistry, options.Clock)
	if err != nil {
		return Plan{}, releaseverify.VerifiedRelease{}, fmt.Errorf("load authority registry: %w", err)
	}
	verified, err := releaseverify.VerifyDirectory(options.ReleaseDir, registry)
	if err != nil {
		return Plan{}, releaseverify.VerifiedRelease{}, fmt.Errorf("verify signed release: %w", err)
	}
	if err := validateBootstrapperName(options.Bootstrapper, verified.Manifest.Release, options.TargetOS, options.TargetArch); err != nil {
		return Plan{}, releaseverify.VerifiedRelease{}, err
	}
	if err := validateRegular(options.Bootstrapper, maxBootstrapperBytes); err != nil {
		return Plan{}, releaseverify.VerifiedRelease{}, fmt.Errorf("validate bootstrapper: %w", err)
	}
	if err := validateRegular(options.AuthorityRegistry, 1<<20); err != nil {
		return Plan{}, releaseverify.VerifiedRelease{}, fmt.Errorf("validate authority registry: %w", err)
	}
	registryDigest, err := fileSHA256(options.AuthorityRegistry)
	if err != nil {
		return Plan{}, releaseverify.VerifiedRelease{}, err
	}
	bootstrapperDigest, err := fileSHA256(options.Bootstrapper)
	if err != nil {
		return Plan{}, releaseverify.VerifiedRelease{}, err
	}
	if err := validateNativeTrustPolicy(options, verified, registryDigest, bootstrapperDigest); err != nil {
		return Plan{}, releaseverify.VerifiedRelease{}, err
	}
	skipNativeTrust := options.NativeTrustMode == NativeTrustCanarySimple && options.TargetOS == "windows" && verified.Manifest.Channel == "canary"
	if !skipNativeTrust {
		if err := options.VerifyNative(context.Background(), options.Bootstrapper); err != nil {
			return Plan{}, releaseverify.VerifiedRelease{}, fmt.Errorf("native bootstrapper trust check: %w", err)
		}
	}
	bootstrapperVersion := verified.Manifest.Release
	if options.NativeTrustMode == NativeTrustStrict {
		status, seedErr := readSeedStatus(context.Background(), options.Bootstrapper, options.Run)
		if seedErr != nil {
			return Plan{}, releaseverify.VerifiedRelease{}, seedErr
		}
		if !seedStatusMatches(status, verified.Manifest.Release, registryDigest) {
			return Plan{}, releaseverify.VerifiedRelease{}, errors.New("bootstrapper seed does not bind this release and authority registry")
		}
		bootstrapperVersion = status.BootstrapperVersion
	}
	plan := Plan{
		Release:             verified.Manifest.Release,
		Channel:             verified.Manifest.Channel,
		TargetOS:            options.TargetOS,
		TargetArch:          options.TargetArch,
		ManagedRoot:         filepath.Clean(options.ManagedRoot),
		DataRoot:            filepath.Clean(options.DataRoot),
		ReleaseDir:          filepath.Clean(options.ReleaseDir),
		Bootstrapper:        filepath.Clean(options.Bootstrapper),
		AuthorityRegistry:   filepath.Clean(options.AuthorityRegistry),
		ManifestSHA256:      verified.ManifestSHA256,
		RegistrySHA256:      registryDigest,
		BootstrapperSHA256:  bootstrapperDigest,
		BootstrapperVersion: bootstrapperVersion,
		ReleaseIssuer:       verified.Manifest.Issuer.ID,
		ReleaseKeyID:        verified.Manifest.Issuer.KeyID,
		NativeTrustMode:     string(options.NativeTrustMode),
	}
	plan.PlanDigest = PlanDigest(plan)
	return plan, verified, nil
}

// Install performs a first install after Prepare's complete verification. It
// refuses to replace an existing managed installation and removes only the
// newly-created managed root if the bootstrapper fails before activation.
func Install(ctx context.Context, options Options) (Result, error) {
	options = withDefaults(options)
	plan, _, err := Prepare(options)
	if err != nil {
		return Result{}, err
	}
	if options.ExpectedPlanDigest != "" && options.ExpectedPlanDigest != plan.PlanDigest {
		return Result{}, errors.New("verified release changed since confirmation")
	}
	preparation, err := prepareManagedRoot(ctx, plan, options)
	if err != nil {
		return Result{}, err
	}
	if preparation.Existing != nil {
		return *preparation.Existing, nil
	}
	created := false
	cleanup := func() {
		if created {
			_ = os.RemoveAll(plan.ManagedRoot)
		}
	}
	if err := os.MkdirAll(filepath.Join(plan.ManagedRoot, "trust"), 0o700); err != nil {
		return Result{}, err
	}
	created = true
	if err := copyRegular(plan.AuthorityRegistry, filepath.Join(plan.ManagedRoot, "trust", "release-authority-registry.json"), 0o600); err != nil {
		cleanup()
		return Result{}, err
	}
	bootstrapperName := "bcgos-bootstrap"
	if plan.TargetOS == "windows" {
		bootstrapperName += ".exe"
	}
	installedBootstrapper := filepath.Join(plan.ManagedRoot, bootstrapperName)
	if err := copyRegular(plan.Bootstrapper, installedBootstrapper, 0o700); err != nil {
		cleanup()
		return Result{}, err
	}
	installedRegistry := filepath.Join(plan.ManagedRoot, "trust", "release-authority-registry.json")
	installedDigest, err := fileSHA256(installedRegistry)
	if err != nil || installedDigest != plan.RegistrySHA256 {
		cleanup()
		if err == nil {
			err = errors.New("installed authority registry changed during staging")
		}
		return Result{}, err
	}
	installedBootstrapperDigest, err := fileSHA256(installedBootstrapper)
	if err != nil || installedBootstrapperDigest != plan.BootstrapperSHA256 {
		cleanup()
		if err == nil {
			err = errors.New("installed bootstrapper changed during staging")
		}
		return Result{}, err
	}
	if err := options.VerifyNative(ctx, installedBootstrapper); err != nil {
		cleanup()
		return Result{}, fmt.Errorf("installed bootstrapper trust check: %w", err)
	}
	status, err := readSeedStatus(ctx, installedBootstrapper, runCommand(options.Run))
	if err != nil || !seedStatusMatches(status, plan.Release, plan.RegistrySHA256) {
		cleanup()
		if err == nil {
			err = errors.New("installed bootstrapper seed binding changed during staging")
		}
		return Result{}, err
	}
	run := options.Run
	if run == nil {
		run = execCommand
	}
	output, err := run(ctx, installedBootstrapper, "install", "--verified-directory", plan.ReleaseDir, "--data-root", plan.DataRoot)
	if err != nil {
		if stateExists(plan.DataRoot) {
			recovery, recoveryErr := quarantineInterruptedInstall(plan, "bootstrapper_failed_after_state", true)
			if recoveryErr != nil {
				return Result{}, fmt.Errorf("bootstrapper installation failed after durable state; installation preserved at %s for recovery: %w: %s (automatic quarantine failed: %v)", plan.ManagedRoot, err, strings.TrimSpace(string(output)), recoveryErr)
			}
			return Result{}, fmt.Errorf("bootstrapper installation failed after durable state; incomplete installation quarantined at %s: %w: %s", recovery.ManagedRootBackup, err, strings.TrimSpace(string(output)))
		}
		cleanup()
		return Result{}, fmt.Errorf("bootstrapper installation failed: %w: %s", err, strings.TrimSpace(string(output)))
	}
	cliName := "bcgos"
	if plan.TargetOS == "windows" {
		cliName += ".exe"
	}
	cliPath := filepath.Join(plan.ManagedRoot, "bin", cliName)
	versionOutput, err := run(ctx, cliPath, "version")
	if err != nil || strings.TrimSpace(string(versionOutput)) != "bcgos "+plan.Release {
		if err == nil {
			err = errors.New("installed CLI reported an unexpected version")
		}
		recovery, recoveryErr := quarantineInterruptedInstall(plan, "post_activation_diagnostic_failed", true)
		if recoveryErr != nil {
			return Result{}, fmt.Errorf("installed CLI final diagnostic failed after activation; coherent installation preserved at %s and was not reported ready: %w (automatic quarantine failed: %v)", plan.ManagedRoot, err, recoveryErr)
		}
		return Result{}, fmt.Errorf("installed CLI final diagnostic failed after activation; incomplete installation quarantined at %s and is safe to reinstall: %w", recovery.ManagedRootBackup, err)
	}
	if err := validateCommittedInstallation(plan); err != nil {
		recovery, recoveryErr := quarantineInterruptedInstall(plan, "post_activation_state_incomplete", true)
		if recoveryErr != nil {
			return Result{}, fmt.Errorf("bootstrapper returned success but the committed installation is incomplete; preserved at %s and not reported ready: %w (automatic quarantine failed: %v)", plan.ManagedRoot, err, recoveryErr)
		}
		return Result{}, fmt.Errorf("bootstrapper returned success but the committed installation is incomplete; quarantined at %s and safe to reinstall: %w", recovery.ManagedRootBackup, err)
	}
	disposition := "installed"
	if preparation.Recovery != nil {
		disposition = "recovered_and_installed"
	}
	return Result{
		Plan: plan, CLIPath: cliPath, Output: strings.TrimSpace(string(output)),
		Disposition: disposition, Recovery: preparation.Recovery,
	}, nil
}

func prepareManagedRoot(ctx context.Context, plan Plan, options Options) (managedRootPreparation, error) {
	run := options.Run
	rootInfo, rootErr := os.Lstat(plan.ManagedRoot)
	rootExists := rootErr == nil
	if rootErr != nil && !errors.Is(rootErr, os.ErrNotExist) {
		return managedRootPreparation{}, fmt.Errorf("inspect managed root: %w", rootErr)
	}
	if rootExists && !rootInfo.IsDir() {
		return managedRootPreparation{}, errors.New("managed root already exists and is not a directory")
	}
	statePath := installStatePath(plan.DataRoot)
	if err := ensureNoSymlinkComponents(plan.DataRoot, statePath); err != nil {
		return managedRootPreparation{}, fmt.Errorf("install state path is unsafe; preserved without replacement: %w", err)
	}
	stateInfo, stateErr := os.Lstat(statePath)
	statePresent := stateErr == nil
	if stateErr != nil && !errors.Is(stateErr, os.ErrNotExist) {
		return managedRootPreparation{}, fmt.Errorf("inspect install state: %w", stateErr)
	}
	if statePresent && !stateInfo.Mode().IsRegular() {
		return managedRootPreparation{}, errors.New("existing install state must be a regular file; refusing automatic recovery")
	}

	var state installtx.State
	if statePresent {
		var err error
		state, err = installtx.ReadStateForManagedRoot(plan.DataRoot, plan.ManagedRoot)
		if err != nil {
			return managedRootPreparation{}, fmt.Errorf("existing install state is invalid; preserved without replacement: %w", err)
		}
	}
	if !rootExists && !statePresent {
		return managedRootPreparation{}, nil
	}
	if !rootExists && statePresent {
		recovery, err := quarantineInterruptedInstall(plan, "orphan_install_state", false)
		if err != nil {
			return managedRootPreparation{}, err
		}
		return managedRootPreparation{Recovery: recovery}, nil
	}

	entries, err := os.ReadDir(plan.ManagedRoot)
	if err != nil {
		return managedRootPreparation{}, fmt.Errorf("inspect managed root: %w", err)
	}
	if len(entries) == 0 && !statePresent {
		return managedRootPreparation{}, nil
	}
	if statePresent {
		cliPath := installedCLIPath(plan.ManagedRoot, state.TargetOS)
		if cliPath != "" && installationStructureComplete(plan.ManagedRoot, state) == nil {
			output, checkErr := run(ctx, cliPath, "version")
			if checkErr == nil && strings.TrimSpace(string(output)) == "bcgos "+state.CLIVersion {
				if state.Release != plan.Release || state.TargetOS != plan.TargetOS || state.TargetArch != plan.TargetArch {
					return managedRootPreparation{}, fmt.Errorf("a healthy Maestro %s installation already exists at %s; use the signed update flow instead of first install", state.Release, plan.ManagedRoot)
				}
				if err := verifyExistingInstallTrust(ctx, plan, options); err != nil {
					recovery, recoveryErr := quarantineInterruptedInstall(plan, "existing_install_trust_failed", true)
					if recoveryErr != nil {
						return managedRootPreparation{}, fmt.Errorf("existing installation trust check failed and was preserved without replacement: %w (automatic quarantine failed: %v)", err, recoveryErr)
					}
					return managedRootPreparation{Recovery: recovery}, nil
				}
				return managedRootPreparation{Existing: &Result{
					Plan: plan, CLIPath: cliPath,
					Output: "existing healthy installation preserved", Disposition: "already_installed",
				}}, nil
			}
		}
		recovery, err := quarantineInterruptedInstall(plan, "stale_bound_installation", true)
		if err != nil {
			return managedRootPreparation{}, err
		}
		return managedRootPreparation{Recovery: recovery}, nil
	}
	for _, entry := range entries {
		if !installerOwnedTopLevel(entry.Name(), plan.TargetOS) {
			return managedRootPreparation{}, fmt.Errorf("managed root contains unrecognized entry %q; preserved without replacement", entry.Name())
		}
	}
	recovery, err := quarantineInterruptedInstall(plan, "interrupted_installer_owned_root", true)
	if err != nil {
		return managedRootPreparation{}, err
	}
	return managedRootPreparation{Recovery: recovery}, nil
}

func verifyExistingInstallTrust(ctx context.Context, plan Plan, options Options) error {
	bootstrapper := installedBootstrapperPath(plan.ManagedRoot, plan.TargetOS)
	bootstrapperDigest, err := fileSHA256(bootstrapper)
	if err != nil {
		return fmt.Errorf("hash installed bootstrapper: %w", err)
	}
	if bootstrapperDigest != plan.BootstrapperSHA256 {
		return errors.New("installed bootstrapper does not match the confirmed release package")
	}
	if err := options.VerifyNative(ctx, bootstrapper); err != nil {
		return fmt.Errorf("installed bootstrapper trust check: %w", err)
	}
	registryPath := filepath.Join(plan.ManagedRoot, "trust", "release-authority-registry.json")
	digest, err := fileSHA256(registryPath)
	if err != nil {
		return fmt.Errorf("hash installed authority registry: %w", err)
	}
	if digest != plan.RegistrySHA256 {
		return errors.New("installed authority registry does not match the confirmed release package")
	}
	status, err := readSeedStatus(ctx, bootstrapper, options.Run)
	if err != nil {
		return err
	}
	if !seedStatusMatches(status, plan.Release, plan.RegistrySHA256) {
		return errors.New("installed bootstrapper seed does not match the confirmed release package")
	}
	return nil
}

func validateCommittedInstallation(plan Plan) error {
	state, err := installtx.ReadStateForManagedRoot(plan.DataRoot, plan.ManagedRoot)
	if err != nil {
		return fmt.Errorf("read committed install state: %w", err)
	}
	if state.Release != plan.Release || state.TargetOS != plan.TargetOS || state.TargetArch != plan.TargetArch {
		return errors.New("committed install state does not match the confirmed release target")
	}
	return installationStructureComplete(plan.ManagedRoot, state)
}

func installationStructureComplete(managedRoot string, state installtx.State) error {
	cliPath := installedCLIPath(managedRoot, state.TargetOS)
	if cliPath == "" {
		return errors.New("installed CLI is missing or unsafe")
	}
	bootstrapper := installedBootstrapperPath(managedRoot, state.TargetOS)
	for label, path := range map[string]string{
		"bootstrapper":       bootstrapper,
		"authority registry": filepath.Join(managedRoot, "trust", "release-authority-registry.json"),
	} {
		if err := ensureNoSymlinkComponents(managedRoot, path); err != nil {
			return fmt.Errorf("%s path is unsafe: %w", label, err)
		}
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() {
			return fmt.Errorf("%s is missing or not a regular file", label)
		}
	}
	bundlePath := filepath.Join(managedRoot, "bundles", state.BundleVersion)
	if err := ensureNoSymlinkComponents(managedRoot, bundlePath); err != nil {
		return fmt.Errorf("bundle path is unsafe: %w", err)
	}
	info, err := os.Lstat(bundlePath)
	if err != nil || !info.IsDir() {
		return errors.New("installed bundle is missing or not a directory")
	}
	return nil
}

func installedBootstrapperPath(managedRoot, targetOS string) string {
	name := "bcgos-bootstrap"
	if targetOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(managedRoot, name)
}

func installedCLIPath(managedRoot, targetOS string) string {
	name := "bcgos"
	if targetOS == "windows" {
		name += ".exe"
	}
	path := filepath.Join(managedRoot, "bin", name)
	if err := ensureNoSymlinkComponents(managedRoot, path); err != nil {
		return ""
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return ""
	}
	return path
}

func ensureNoSymlinkComponents(root, target string) error {
	root = filepath.Clean(root)
	target = filepath.Clean(target)
	relative, err := filepath.Rel(root, target)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return errors.New("path escapes its trusted root")
	}
	current := root
	for _, component := range strings.Split(relative, string(filepath.Separator)) {
		if component == "." || component == "" {
			continue
		}
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("path contains a symlink or reparse point")
		}
	}
	return nil
}

func installerOwnedTopLevel(name, targetOS string) bool {
	allowed := map[string]bool{
		"trust": true, "bin": true, "bundles": true, "recovery": true,
		".activation.lock": true, "bcgos-bootstrap": true,
	}
	if targetOS == "windows" {
		allowed["bcgos-bootstrap.exe"] = true
	}
	return allowed[name]
}

func stateExists(dataRoot string) bool {
	info, err := os.Lstat(installStatePath(dataRoot))
	return err == nil && info.Mode().IsRegular()
}

func installStatePath(dataRoot string) string {
	return filepath.Join(dataRoot, "config", "install-state.json")
}

func quarantineInterruptedInstall(plan Plan, reason string, includeManagedRoot bool) (*Recovery, error) {
	if len(plan.PlanDigest) < 12 {
		return nil, errors.New("cannot quarantine an installation without an exact plan digest")
	}
	prefix := plan.PlanDigest[:12]
	recoveryDir := filepath.Join(plan.DataRoot, "recovery", "installer", plan.PlanDigest)
	statePath := installStatePath(plan.DataRoot)
	stateBackup := filepath.Join(recoveryDir, "install-state.json")
	rootBackup := plan.ManagedRoot + ".interrupted-" + prefix
	recordPath := filepath.Join(recoveryDir, "recovery.json")
	if err := ensureNoSymlinkComponents(plan.DataRoot, statePath); err != nil {
		return nil, fmt.Errorf("install state path is unsafe: %w", err)
	}
	if err := ensureNoSymlinkComponents(plan.DataRoot, recoveryDir); err != nil {
		return nil, fmt.Errorf("installer recovery path is unsafe: %w", err)
	}
	record := recoveryRecord{
		SchemaVersion: 1, PlanDigest: plan.PlanDigest, Reason: reason,
		ManagedRoot: plan.ManagedRoot, ManagedRootBackup: rootBackup,
		InstallState: statePath, InstallStateBackup: stateBackup, Status: "prepared",
	}
	if err := os.MkdirAll(recoveryDir, 0o700); err != nil {
		return nil, fmt.Errorf("create installer recovery directory: %w", err)
	}
	if err := prepareRecoveryRecord(recordPath, record); err != nil {
		return nil, err
	}
	if err := moveRegularIfPresent(statePath, stateBackup); err != nil {
		return nil, fmt.Errorf("preserve interrupted install state: %w", err)
	}
	if includeManagedRoot {
		if err := moveDirectoryIfPresent(plan.ManagedRoot, rootBackup); err != nil {
			return nil, fmt.Errorf("preserve interrupted managed root: %w", err)
		}
	}
	record.Status = "quarantined"
	if err := writeRecoveryRecord(recordPath, record); err != nil {
		return nil, fmt.Errorf("complete installer recovery record: %w", err)
	}
	result := &Recovery{PlanDigest: plan.PlanDigest, InstallStateBackup: stateBackup}
	if includeManagedRoot {
		result.ManagedRootBackup = rootBackup
	}
	return result, nil
}

func prepareRecoveryRecord(path string, expected recoveryRecord) error {
	body, err := json.MarshalIndent(expected, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err == nil {
		if _, writeErr := file.Write(body); writeErr != nil {
			file.Close()
			_ = os.Remove(path)
			return writeErr
		}
		return file.Close()
	}
	if !errors.Is(err, os.ErrExist) {
		return fmt.Errorf("create installer recovery record: %w", err)
	}
	var existing recoveryRecord
	if readErr := readStrictJSON(path, &existing); readErr != nil {
		return fmt.Errorf("read existing installer recovery record: %w", readErr)
	}
	expected.Status = existing.Status
	if existing != expected || (existing.Status != "prepared" && existing.Status != "quarantined") {
		return errors.New("existing installer recovery record does not match this recovery")
	}
	if existing.Status == "quarantined" {
		return errors.New("this installer plan already has a completed recovery; refusing to overwrite it")
	}
	return nil
}

func writeRecoveryRecord(path string, record recoveryRecord) error {
	body, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, append(body, '\n'), 0o600); err != nil {
		return err
	}
	if err := os.Rename(temporary, path); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	return nil
}

func readStrictJSON(path string, target any) error {
	body, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("recovery record contains multiple JSON values")
	}
	return nil
}

func moveRegularIfPresent(source, target string) error {
	info, err := os.Lstat(source)
	if errors.Is(err, os.ErrNotExist) {
		targetInfo, targetErr := os.Lstat(target)
		if targetErr == nil && targetInfo.Mode().IsRegular() {
			return nil
		}
		if errors.Is(targetErr, os.ErrNotExist) {
			return nil
		}
		return targetErr
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("source is not a regular file")
	}
	if _, err := os.Lstat(target); err == nil {
		return errors.New("recovery target already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.Rename(source, target)
}

func moveDirectoryIfPresent(source, target string) error {
	info, err := os.Lstat(source)
	if errors.Is(err, os.ErrNotExist) {
		targetInfo, targetErr := os.Lstat(target)
		if targetErr == nil && targetInfo.IsDir() {
			return nil
		}
		if errors.Is(targetErr, os.ErrNotExist) {
			return nil
		}
		return targetErr
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("source is not a directory")
	}
	if _, err := os.Lstat(target); err == nil {
		return errors.New("recovery target already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.Rename(source, target)
}

// PlanDigest returns the stable digest bound to one verify/install handoff.
// The digest field itself is blanked before encoding so the value is not
// recursive. Struct field order is stable and the plan contains no maps.
func PlanDigest(plan Plan) string {
	plan.PlanDigest = ""
	body, err := json.Marshal(plan)
	if err != nil {
		return ""
	}
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:])
}

func runCommand(run commandRunner) commandRunner {
	if run != nil {
		return run
	}
	return execCommand
}

func withDefaults(options Options) Options {
	if options.NativeTrustMode == "" {
		options.NativeTrustMode = NativeTrustStrict
	}
	if options.Clock == nil {
		options.Clock = time.Now
	}
	if options.TargetOS == "" {
		options.TargetOS = runtime.GOOS
	}
	if options.TargetArch == "" {
		options.TargetArch = runtime.GOARCH
	}
	if options.Run == nil {
		options.Run = execCommand
	}
	if options.VerifyNative == nil {
		mode := options.NativeTrustMode
		options.VerifyNative = func(ctx context.Context, path string) error {
			return nativeSignatureCheck(ctx, path, mode)
		}
	}
	return options
}

func validateNativeTrustPolicy(options Options, verified releaseverify.VerifiedRelease, registryDigest, bootstrapperDigest string) error {
	switch options.NativeTrustMode {
	case NativeTrustStrict:
		if !emptyLocalBetaPins(options.LocalBetaPins) {
			return errors.New("local-beta pins are forbidden in strict native trust mode")
		}
		return nil
	case NativeTrustCanarySimple:
		if options.TargetOS != "windows" {
			return errors.New("canary-simple native trust mode requires a Windows target")
		}
		if verified.Manifest.Channel != "canary" {
			return errors.New("canary-simple native trust mode requires the canary channel")
		}
		if !emptyLocalBetaPins(options.LocalBetaPins) {
			return errors.New("canary-simple trust mode must not carry local-beta pins")
		}
		return nil
	case NativeTrustWindowsLocalBeta:
		pins := options.LocalBetaPins
		if options.TargetOS != "windows" {
			return errors.New("windows local-beta native trust mode requires a Windows target")
		}
		if verified.Manifest.Channel != "canary" {
			return errors.New("windows local-beta native trust mode requires the canary channel")
		}
		if !isLowerSHA256(pins.AuthorityRegistrySHA256) || !isLowerSHA256(pins.BootstrapperSHA256) ||
			pins.Issuer == "" || pins.KeyID == "" {
			return errors.New("windows local-beta native trust mode requires complete canonical package pins")
		}
		if !hasBetaAuthorityMarker(pins.Issuer) || !hasBetaAuthorityMarker(pins.KeyID) {
			return errors.New("windows local-beta authority issuer and key ID must be explicitly beta or test-only")
		}
		if verified.Manifest.Issuer.ID != pins.Issuer || verified.Manifest.Issuer.KeyID != pins.KeyID {
			return errors.New("signed release issuer does not match the pinned local-beta authority")
		}
		if registryDigest != pins.AuthorityRegistrySHA256 {
			return errors.New("authority registry does not match the pinned local-beta package")
		}
		if bootstrapperDigest != pins.BootstrapperSHA256 {
			return errors.New("bootstrapper does not match the pinned local-beta package")
		}
		return nil
	default:
		return fmt.Errorf("unsupported native trust mode %q", options.NativeTrustMode)
	}
}

func emptyLocalBetaPins(pins LocalBetaPins) bool {
	return pins.AuthorityRegistrySHA256 == "" && pins.BootstrapperSHA256 == "" && pins.Issuer == "" && pins.KeyID == ""
}

func isLowerSHA256(value string) bool {
	if len(value) != sha256.Size*2 || strings.ToLower(value) != value {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size
}

func hasBetaAuthorityMarker(value string) bool {
	for _, token := range strings.FieldsFunc(strings.ToLower(value), func(character rune) bool {
		return character == '.' || character == '_' || character == '-'
	}) {
		if token == "beta" || token == "test" || token == "testonly" || token == "localbeta" {
			return true
		}
	}
	return false
}

func seedStatusMatches(status seedStatus, release, registryDigest string) bool {
	return status.SchemaVersion == 1 && status.Product == "maestro" &&
		status.BootstrapperVersion == release && status.AuthorityRegistrySHA256 == registryDigest
}

func readSeedStatus(ctx context.Context, path string, run commandRunner) (seedStatus, error) {
	body, err := run(ctx, path, "seed-status")
	if err != nil {
		return seedStatus{}, fmt.Errorf("read bootstrapper seed status: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var status seedStatus
	if err := decoder.Decode(&status); err != nil {
		return seedStatus{}, fmt.Errorf("decode bootstrapper seed status: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return seedStatus{}, errors.New("bootstrapper seed status contains multiple JSON values")
	}
	return status, nil
}

func nativeSignatureCheck(ctx context.Context, path string, mode NativeTrustMode) error {
	switch runtime.GOOS {
	case "darwin":
		if mode != NativeTrustStrict {
			return errors.New("local-beta native trust exception is unavailable on macOS")
		}
		if _, err := exec.CommandContext(ctx, "codesign", "--verify", "--strict", "--verbose=2", path).CombinedOutput(); err != nil {
			return errors.New("codesign rejected the bootstrapper")
		}
		return nil
	case "windows":
		command := fmt.Sprintf("$ErrorActionPreference = 'Stop'; $s = Get-AuthenticodeSignature -LiteralPath '%s'; [Console]::Out.Write([string]$s.Status)", strings.ReplaceAll(path, "'", "''"))
		output, err := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", command).CombinedOutput()
		if err != nil {
			return errors.New("Authenticode status could not be established for the bootstrapper")
		}
		return validateAuthenticodeStatus(output, mode)
	default:
		return errors.New("native signature verification is unavailable on this platform")
	}
}

func validateAuthenticodeStatus(output []byte, mode NativeTrustMode) error {
	status := strings.TrimSpace(string(output))
	if status == "" || strings.ContainsAny(status, "\r\n") {
		return errors.New("Authenticode returned an invalid bootstrapper status")
	}
	if status == "Valid" && mode == NativeTrustStrict {
		return nil
	}
	if status == "NotSigned" && mode == NativeTrustWindowsLocalBeta {
		return nil
	}
	return fmt.Errorf("Authenticode rejected the bootstrapper with status %s", status)
}

func execCommand(ctx context.Context, path string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, path, args...).CombinedOutput()
}

func validateBootstrapperName(path, version, platform, arch string) error {
	name := "bcgos-bootstrap_" + version + "_" + platform + "_" + arch
	if platform == "windows" {
		name += ".exe"
	}
	if filepath.Base(path) != name {
		return fmt.Errorf("bootstrapper must be named %s", name)
	}
	return nil
}

func validateRegular(path string, maximum int64) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maximum {
		return errors.New("file must be a bounded regular file")
	}
	return nil
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func copyRegular(source, target string, mode os.FileMode) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return err
	}
	output, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, input)
	closeErr := output.Close()
	if copyErr != nil {
		_ = os.Remove(target)
		return copyErr
	}
	return closeErr
}

func ensureFreshManagedRoot(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("managed root already exists and is not a directory")
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return err
	}
	if len(entries) != 0 {
		return errors.New("managed root already exists; refusing to replace it")
	}
	return nil
}

// canonicalInstallRoot resolves existing ancestors before the installer uses
// a root. This makes the physical separation check authoritative even when a
// caller supplies an alias such as a symlinked parent or Windows junction.
// A root that is itself a symlink/reparse point is rejected so the destination
// shown in the plan cannot silently redirect after confirmation.
func canonicalInstallRoot(path, label string) (string, error) {
	absolute, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", fmt.Errorf("normalize %s: %w", label, err)
	}
	current := absolute
	var suffix []string
	for {
		info, statErr := os.Lstat(current)
		if statErr == nil {
			resolved, resolveErr := filepath.EvalSymlinks(current)
			if resolveErr != nil {
				return "", fmt.Errorf("resolve %s: %w", label, resolveErr)
			}
			if len(suffix) == 0 {
				if info.Mode()&os.ModeSymlink != 0 {
					return "", fmt.Errorf("%s must not be a symlink or reparse point", label)
				}
				parent := filepath.Dir(current)
				if parent != current {
					resolvedParent, parentErr := filepath.EvalSymlinks(parent)
					if parentErr != nil {
						return "", fmt.Errorf("resolve %s parent: %w", label, parentErr)
					}
					expected := filepath.Join(resolvedParent, filepath.Base(current))
					if !samePhysicalPath(resolved, expected) {
						return "", fmt.Errorf("%s must not be a symlink or reparse point", label)
					}
				}
			}
			for index := len(suffix) - 1; index >= 0; index-- {
				resolved = filepath.Join(resolved, suffix[index])
			}
			return filepath.Clean(resolved), nil
		}
		if !errors.Is(statErr, os.ErrNotExist) {
			return "", fmt.Errorf("inspect %s: %w", label, statErr)
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", fmt.Errorf("resolve %s: no existing parent", label)
		}
		suffix = append(suffix, filepath.Base(current))
		current = parent
	}
}

func samePhysicalPath(left, right string) bool {
	left = filepath.Clean(left)
	right = filepath.Clean(right)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(left, right)
	}
	return left == right
}

func within(root, candidate string) bool {
	relative, err := filepath.Rel(filepath.Clean(root), filepath.Clean(candidate))
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
