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
	"path/filepath"
	"runtime"
	"strings"
	"time"

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
		base := filepath.Join(home, "Library", "Application Support")
		return Paths{ManagedRoot: filepath.Join(base, "Maestro"), DataRoot: dataRoot}, nil
	default:
		return Paths{}, fmt.Errorf("installer does not support %q", platform)
	}
}

type nativeVerifier func(context.Context, string) error
type commandRunner func(context.Context, string, ...string) ([]byte, error)

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
	BootstrapperVersion string `json:"bootstrapper_version"`
	PlanDigest          string `json:"plan_digest"`
}

// Result is the durable handoff summary returned after bootstrapper success.
type Result struct {
	Plan
	CLIPath string `json:"cli_path"`
	Output  string `json:"bootstrapper_output"`
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
	if err := options.VerifyNative(context.Background(), options.Bootstrapper); err != nil {
		return Plan{}, releaseverify.VerifiedRelease{}, fmt.Errorf("native bootstrapper trust check: %w", err)
	}
	status, err := readSeedStatus(context.Background(), options.Bootstrapper, options.Run)
	if err != nil {
		return Plan{}, releaseverify.VerifiedRelease{}, err
	}
	registryDigest, err := fileSHA256(options.AuthorityRegistry)
	if err != nil {
		return Plan{}, releaseverify.VerifiedRelease{}, err
	}
	if status.SchemaVersion != 1 || status.Product != "maestro" ||
		status.BootstrapperVersion != verified.Manifest.Release ||
		status.AuthorityRegistrySHA256 != registryDigest {
		return Plan{}, releaseverify.VerifiedRelease{}, errors.New("bootstrapper seed does not bind this release and authority registry")
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
		BootstrapperVersion: status.BootstrapperVersion,
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
	if err := ensureFreshManagedRoot(plan.ManagedRoot); err != nil {
		return Result{}, err
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
	if err := options.VerifyNative(ctx, installedBootstrapper); err != nil {
		cleanup()
		return Result{}, fmt.Errorf("installed bootstrapper trust check: %w", err)
	}
	status, err := readSeedStatus(ctx, installedBootstrapper, runCommand(options.Run))
	if err != nil || status.AuthorityRegistrySHA256 != plan.RegistrySHA256 || status.BootstrapperVersion != plan.Release {
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
		cleanup()
		if err == nil {
			err = errors.New("installed CLI reported an unexpected version")
		}
		return Result{}, fmt.Errorf("installed CLI self-check failed: %w", err)
	}
	return Result{Plan: plan, CLIPath: cliPath, Output: strings.TrimSpace(string(output))}, nil
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
		options.VerifyNative = nativeSignatureCheck
	}
	return options
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

func nativeSignatureCheck(ctx context.Context, path string) error {
	switch runtime.GOOS {
	case "darwin":
		if _, err := exec.CommandContext(ctx, "codesign", "--verify", "--strict", "--verbose=2", path).CombinedOutput(); err != nil {
			return errors.New("codesign rejected the bootstrapper")
		}
		return nil
	case "windows":
		command := fmt.Sprintf("$s = Get-AuthenticodeSignature -LiteralPath '%s'; if ($s.Status -ne 'Valid') { exit 1 }", strings.ReplaceAll(path, "'", "''"))
		if _, err := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", command).CombinedOutput(); err != nil {
			return errors.New("Authenticode rejected the bootstrapper")
		}
		return nil
	default:
		return errors.New("native signature verification is unavailable on this platform")
	}
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

func within(root, candidate string) bool {
	relative, err := filepath.Rel(filepath.Clean(root), filepath.Clean(candidate))
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
