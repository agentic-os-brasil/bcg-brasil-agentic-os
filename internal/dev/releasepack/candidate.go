package releasepack

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/rolemigration"
)

const (
	ManifestName = "release-manifest.json"
)

type Target struct {
	OS   string
	Arch string
}

var candidateTargets = []Target{
	{OS: "windows", Arch: "amd64"},
	{OS: "darwin", Arch: "amd64"},
	{OS: "darwin", Arch: "arm64"},
}

type BinaryBuilder interface {
	Build(context.Context, string, string, string, Target) error
}

type GoBinaryBuilder struct{}

func (GoBinaryBuilder) Build(ctx context.Context, root, output, version string, target Target) error {
	environment := append(filteredEnvironment(os.Environ(), "GOOS", "GOARCH", "CGO_ENABLED"),
		"GOOS="+target.OS,
		"GOARCH="+target.Arch,
		"CGO_ENABLED=0",
	)
	return runGoBuild(ctx, root, output, version, target, environment)
}

type NativeGoBinaryBuilder struct{}

func (NativeGoBinaryBuilder) Build(ctx context.Context, root, output, version string, target Target) error {
	if runtime.GOOS != target.OS || runtime.GOARCH != target.Arch {
		return fmt.Errorf(
			"native release build target %s/%s does not match runner %s/%s",
			target.OS, target.Arch, runtime.GOOS, runtime.GOARCH,
		)
	}
	cgo := "0"
	if target.OS == "darwin" {
		cgo = "1"
	}
	environment := append(filteredEnvironment(os.Environ(), "GOOS", "GOARCH", "CGO_ENABLED"),
		"CGO_ENABLED="+cgo,
	)
	return runGoBuild(ctx, root, output, version, target, environment)
}

type PrebuiltBinaryBuilder struct {
	Directory string
}

func (builder PrebuiltBinaryBuilder) Build(_ context.Context, _ string, output, version string, target Target) error {
	source := filepath.Join(builder.Directory, binaryName(version, target))
	info, err := os.Lstat(source)
	if err != nil {
		return fmt.Errorf("load prebuilt binary for %s/%s: %w", target.OS, target.Arch, err)
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > 1<<30 {
		return fmt.Errorf("prebuilt binary for %s/%s is not a bounded regular file", target.OS, target.Arch)
	}
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	destination, err := os.OpenFile(output, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	written, copyErr := io.Copy(destination, io.LimitReader(input, 1<<30+1))
	closeErr := destination.Close()
	if copyErr != nil {
		return copyErr
	}
	if written != info.Size() || written > 1<<30 {
		return fmt.Errorf("prebuilt binary size changed while copying %s/%s", target.OS, target.Arch)
	}
	return closeErr
}

func runGoBuild(
	ctx context.Context,
	root, output, version string,
	target Target,
	environment []string,
) error {
	command := exec.CommandContext(
		ctx,
		"go", "build",
		"-mod=readonly",
		"-buildvcs=false",
		"-trimpath",
		"-ldflags", "-s -w -X github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/cli.Version="+version,
		"-o", output,
		"./cmd/bcgos",
	)
	command.Dir = root
	command.Env = environment
	outputBytes, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("build bcgos for %s/%s: %w: %s", target.OS, target.Arch, err, strings.TrimSpace(string(outputBytes)))
	}
	return nil
}

type CandidateOptions struct {
	Root       string
	Output     string
	Version    string
	Channel    string
	Allowlist  string
	Prebuilt   string
	Builder    BinaryBuilder
	AdHocMacOS bool
}

func BuildCandidate(ctx context.Context, options CandidateOptions) (releasecontract.Manifest, error) {
	if options.Builder == nil {
		if options.Prebuilt != "" {
			options.Builder = PrebuiltBinaryBuilder{Directory: options.Prebuilt}
		} else {
			options.Builder = GoBinaryBuilder{}
		}
	}
	if options.Root == "" || options.Output == "" {
		return releasecontract.Manifest{}, errors.New("candidate root and output are required")
	}
	compatibility := compatibleRange(options.Version)
	if _, err := releasecontract.ParseVersionRange(compatibility); err != nil {
		return releasecontract.Manifest{}, fmt.Errorf("invalid candidate version %q: %w", options.Version, err)
	}
	if options.Channel != "canary" && options.Channel != "beta" && options.Channel != "stable" {
		return releasecontract.Manifest{}, fmt.Errorf("invalid candidate channel %q", options.Channel)
	}
	if _, err := os.Stat(options.Output); err == nil {
		return releasecontract.Manifest{}, fmt.Errorf("candidate output already exists: %s", options.Output)
	} else if !errors.Is(err, os.ErrNotExist) {
		return releasecontract.Manifest{}, err
	}
	if options.Allowlist == "" {
		options.Allowlist = "bundles/base/distribution.json"
	}
	parent := filepath.Dir(options.Output)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return releasecontract.Manifest{}, err
	}
	staging, err := os.MkdirTemp(parent, ".maestro-candidate-")
	if err != nil {
		return releasecontract.Manifest{}, err
	}
	defer os.RemoveAll(staging)

	allowlist, err := LoadAllowlist(filepath.Join(options.Root, filepath.FromSlash(options.Allowlist)))
	if err != nil {
		return releasecontract.Manifest{}, fmt.Errorf("load distribution allowlist: %w", err)
	}
	manifest := releasecontract.Manifest{
		SchemaVersion: 1,
		Product:       "maestro",
		Release:       options.Version,
		Channel:       options.Channel,
		Issuer: releasecontract.Issuer{
			ID:    "maestro-release-candidate",
			KeyID: "candidate-unavailable",
		},
		CLI: releasecontract.CLIComponent{
			Version:          options.Version,
			CompatibleBundle: compatibility,
		},
		Bundle: releasecontract.BundleComponent{
			Version:       options.Version,
			CompatibleCLI: compatibility,
		},
		Migrations: []releasecontract.Migration{},
	}

	for _, target := range candidateTargets {
		name := binaryName(options.Version, target)
		path := filepath.Join(staging, name)
		if err := options.Builder.Build(ctx, options.Root, path, options.Version, target); err != nil {
			return releasecontract.Manifest{}, err
		}
		if options.AdHocMacOS && target.OS == "darwin" {
			if runtime.GOOS != "darwin" {
				return releasecontract.Manifest{}, errors.New("macOS ad-hoc candidate signing requires a macOS host")
			}
			if output, err := exec.Command("/usr/bin/codesign", "--force", "--sign", "-", path).CombinedOutput(); err != nil {
				return releasecontract.Manifest{}, fmt.Errorf("ad-hoc sign candidate %s: %w: %s", target.Arch, err, strings.TrimSpace(string(output)))
			}
		}
		artifact, err := inspectArtifact("cli", target.OS, target.Arch, path)
		if err != nil {
			return releasecontract.Manifest{}, err
		}
		manifest.Artifacts = append(manifest.Artifacts, artifact)
	}

	bundleName := "maestro-base_" + options.Version + ".tar.gz"
	bundlePath := filepath.Join(staging, bundleName)
	bundleFile, err := os.OpenFile(bundlePath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return releasecontract.Manifest{}, err
	}
	if err := BuildBundle(options.Root, allowlist, bundleFile); err != nil {
		bundleFile.Close()
		return releasecontract.Manifest{}, err
	}
	if err := bundleFile.Close(); err != nil {
		return releasecontract.Manifest{}, err
	}
	bundleArtifact, err := inspectArtifact("bundle", "any", "any", bundlePath)
	if err != nil {
		return releasecontract.Manifest{}, err
	}
	manifest.Artifacts = append(manifest.Artifacts, bundleArtifact)
	if expired, _ := rolemigration.IsExpired(options.Version); expired {
		catalogInfo, catalogSHA256, err := inspectFile(filepath.Join(options.Root, "bundles", "base", "agents", "catalog.json"))
		if err != nil || catalogInfo.Size() == 0 {
			return releasecontract.Manifest{}, fmt.Errorf("inspect role catalog identity: %w", err)
		}
		policyInfo, policySHA256, err := inspectFile(filepath.Join(options.Root, "bundles", "base", "skills", "agent-skill-policy.json"))
		if err != nil || policyInfo.Size() == 0 {
			return releasecontract.Manifest{}, fmt.Errorf("inspect role policy identity: %w", err)
		}
		manifest.Migrations = append(manifest.Migrations, releasecontract.Migration{
			ID: rolemigration.MigrationID, Component: "bundle", From: rolemigration.SourceRange,
			To: options.Version, Required: true,
			FromRole: rolemigration.LegacyRole, ToRole: rolemigration.CanonicalRole,
			AliasExpiresAfter: rolemigration.AliasExpiresAfter,
			BundleSHA256:      bundleArtifact.SHA256, CatalogSHA256: catalogSHA256, PolicySHA256: policySHA256,
		})
	}

	notesName := "release-notes-" + options.Version + ".md"
	notesBody := []byte(fmt.Sprintf(
		"# Maestro %s release candidate\n\nChannel: `%s`\n\nThis output is deterministic and unsigned. It is not a pilot release until production manifest/artifact signatures, native platform signing, authenticated publication and clean-device acceptance exist.\n",
		options.Version,
		options.Channel,
	))
	if err := os.WriteFile(filepath.Join(staging, notesName), notesBody, 0o644); err != nil {
		return releasecontract.Manifest{}, err
	}
	manifest.ReleaseNotes = releasecontract.ReleaseNotes{Name: notesName, SHA256: SHA256(notesBody)}
	if err := manifest.Validate(); err != nil {
		return releasecontract.Manifest{}, fmt.Errorf("validate candidate manifest: %w", err)
	}
	manifestBody, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return releasecontract.Manifest{}, err
	}
	manifestBody = append(manifestBody, '\n')
	if err := os.WriteFile(filepath.Join(staging, ManifestName), manifestBody, 0o644); err != nil {
		return releasecontract.Manifest{}, err
	}
	if err := VerifyCandidate(staging); err != nil {
		return releasecontract.Manifest{}, fmt.Errorf("verify staged candidate: %w", err)
	}
	if err := os.Rename(staging, options.Output); err != nil {
		return releasecontract.Manifest{}, fmt.Errorf("activate candidate output: %w", err)
	}
	return manifest, nil
}

func BuildNativeBinary(
	ctx context.Context,
	root, output, version string,
	target Target,
	builder BinaryBuilder,
) error {
	if root == "" || output == "" {
		return errors.New("native binary root and output are required")
	}
	if !supportedCandidateTarget(target) {
		return fmt.Errorf("unsupported native release target %s/%s", target.OS, target.Arch)
	}
	if _, err := releasecontract.ParseVersionRange(compatibleRange(version)); err != nil {
		return fmt.Errorf("invalid native binary version %q: %w", version, err)
	}
	if filepath.Base(output) != binaryName(version, target) {
		return fmt.Errorf("native binary output must be named %s", binaryName(version, target))
	}
	if _, err := os.Stat(output); err == nil {
		return fmt.Errorf("native binary output already exists: %s", output)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		return err
	}
	if builder == nil {
		builder = NativeGoBinaryBuilder{}
	}
	return builder.Build(ctx, root, output, version, target)
}

func VerifyCandidate(directory string) error {
	manifestFile, err := os.Open(filepath.Join(directory, ManifestName))
	if err != nil {
		return err
	}
	manifest, err := releasecontract.Parse(manifestFile)
	closeErr := manifestFile.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	expected := map[string]bool{ManifestName: true, manifest.ReleaseNotes.Name: true}
	for _, artifact := range manifest.Artifacts {
		expected[artifact.Name] = true
		path := filepath.Join(directory, artifact.Name)
		info, digest, err := inspectFile(path)
		if err != nil {
			return err
		}
		if info.Size() != artifact.Size {
			return fmt.Errorf("artifact size mismatch for %s", artifact.Name)
		}
		if digest != artifact.SHA256 {
			return fmt.Errorf("artifact digest mismatch for %s", artifact.Name)
		}
	}
	_, notesDigest, err := inspectFile(filepath.Join(directory, manifest.ReleaseNotes.Name))
	if err != nil {
		return err
	}
	if notesDigest != manifest.ReleaseNotes.SHA256 {
		return errors.New("release notes digest mismatch")
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || !expected[entry.Name()] {
			return fmt.Errorf("unexpected candidate entry %s", entry.Name())
		}
	}
	if len(entries) != len(expected) {
		return errors.New("candidate release set is incomplete")
	}
	return nil
}

func inspectArtifact(kind, osName, arch, path string) (releasecontract.Artifact, error) {
	info, digest, err := inspectFile(path)
	if err != nil {
		return releasecontract.Artifact{}, err
	}
	name := filepath.Base(path)
	return releasecontract.Artifact{
		Kind:         kind,
		OS:           osName,
		Arch:         arch,
		Name:         name,
		Size:         info.Size(),
		SHA256:       digest,
		SignatureRef: name + ".sig",
	}, nil
}

func inspectFile(path string) (os.FileInfo, string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, "", err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, "", err
	}
	if !info.Mode().IsRegular() {
		return nil, "", fmt.Errorf("candidate entry must be a regular file: %s", path)
	}
	body, err := io.ReadAll(file)
	if err != nil {
		return nil, "", err
	}
	return info, SHA256(body), nil
}

func binaryName(version string, target Target) string {
	name := "bcgos_" + version + "_" + target.OS + "_" + target.Arch
	if target.OS == "windows" {
		name += ".exe"
	}
	return name
}

func supportedCandidateTarget(target Target) bool {
	for _, candidate := range candidateTargets {
		if candidate == target {
			return true
		}
	}
	return false
}

func compatibleRange(version string) string {
	parts := strings.Split(version, ".")
	if len(parts) != 3 {
		return ""
	}
	return ">=" + version + " <" + parts[0] + "." + parts[1] + "." + nextPatch(parts[2])
}

func nextPatch(value string) string {
	var patch uint64
	for _, character := range value {
		if character < '0' || character > '9' {
			return ""
		}
		patch = patch*10 + uint64(character-'0')
	}
	return fmt.Sprint(patch + 1)
}

func filteredEnvironment(environment []string, names ...string) []string {
	blocked := map[string]bool{}
	for _, name := range names {
		blocked[name] = true
	}
	filtered := make([]string, 0, len(environment))
	for _, entry := range environment {
		name, _, _ := strings.Cut(entry, "=")
		if !blocked[name] {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}
