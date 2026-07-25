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
	"strings"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
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
	command.Env = append(filteredEnvironment(os.Environ(), "GOOS", "GOARCH", "CGO_ENABLED"),
		"GOOS="+target.OS,
		"GOARCH="+target.Arch,
		"CGO_ENABLED=0",
	)
	outputBytes, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("build bcgos for %s/%s: %w: %s", target.OS, target.Arch, err, strings.TrimSpace(string(outputBytes)))
	}
	return nil
}

type CandidateOptions struct {
	Root      string
	Output    string
	Version   string
	Channel   string
	Allowlist string
	Builder   BinaryBuilder
}

func BuildCandidate(ctx context.Context, options CandidateOptions) (releasecontract.Manifest, error) {
	if options.Builder == nil {
		options.Builder = GoBinaryBuilder{}
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
