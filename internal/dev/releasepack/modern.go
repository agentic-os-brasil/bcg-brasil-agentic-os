package releasepack

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseverify"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/rolemigration"
)

type ModernOptions struct {
	Root, Output, Version, InstallerVersion, Channel, AuthorityRegistry string
	Target                                                              Target
	DoclingPack                                                         string
	Clock                                                               func() time.Time
}
type ModernCandidate struct {
	Directory         string                   `json:"directory"`
	Installer         string                   `json:"installer"`
	AuthorityRegistry string                   `json:"authority_registry"`
	Manifest          releasecontract.Manifest `json:"manifest"`
}

// BuildModernCandidate builds both actual native entrypoints in the MCP module.
// There is intentionally no caller-supplied binary builder or prebuilt shortcut.
// The independently admitted seed remains outside the closed release directory.
func BuildModernCandidate(ctx context.Context, o ModernOptions) (ModernCandidate, error) {
	if o.Root == "" || o.Output == "" || o.AuthorityRegistry == "" {
		return ModernCandidate{}, errors.New("modern factory paths are required")
	}
	if (o.Target.OS != "darwin" && o.Target.OS != "windows") || o.Target.OS != runtime.GOOS || o.Target.Arch != runtime.GOARCH {
		return ModernCandidate{}, errors.New("modern native target must match the supported runner")
	}
	for _, version := range []string{o.Version, o.InstallerVersion} {
		if _, err := releasecontract.ParseVersionRange(compatibleRange(version)); err != nil {
			return ModernCandidate{}, err
		}
	}
	if o.Channel != "canary" && o.Channel != "beta" && o.Channel != "stable" {
		return ModernCandidate{}, errors.New("unsupported modern release channel")
	}
	if o.Clock == nil {
		o.Clock = time.Now
	}
	registry, pin, err := readSeedInput(o.AuthorityRegistry, 1<<20)
	if err != nil {
		return ModernCandidate{}, err
	}
	if _, err = releaseverify.ParseAuthorityRegistry(bytes.NewReader(registry), o.Clock); err != nil {
		return ModernCandidate{}, err
	}
	if _, err = os.Lstat(o.Output); !errors.Is(err, os.ErrNotExist) {
		return ModernCandidate{}, errors.New("modern output must be new")
	}
	if err = os.MkdirAll(filepath.Dir(o.Output), 0755); err != nil {
		return ModernCandidate{}, err
	}
	stage, err := os.MkdirTemp(filepath.Dir(o.Output), ".maestro-modern-")
	if err != nil {
		return ModernCandidate{}, err
	}
	defer os.RemoveAll(stage)
	release := filepath.Join(stage, "candidate")
	seed := filepath.Join(stage, "seed")
	for _, directory := range []string{release, filepath.Join(seed, "trust")} {
		if err = os.MkdirAll(directory, 0700); err != nil {
			return ModernCandidate{}, err
		}
	}
	serverName := modernBinaryName("maestro-mcp", o.Version, o.Target)
	installerName := modernBinaryName("maestro-install", o.InstallerVersion, o.Target)
	for _, build := range []struct{ command, version, output string }{{"maestro-mcp", o.Version, filepath.Join(release, serverName)}, {"maestro-install", o.InstallerVersion, filepath.Join(seed, installerName)}} {
		ldflags := strings.Join([]string{"-s", "-w", "-X", "main.Version=" + build.version, "-X", "main.AuthorityRegistrySHA256=" + pin}, " ")
		if build.command == "maestro-mcp" {
			ldflags += " -X 'main.CompatibleBundle=" + compatibleRange(o.Version) + "'"
		}
		command := exec.CommandContext(ctx, "go", "build", "-mod=readonly", "-buildvcs=false", "-trimpath", "-ldflags", ldflags, "-o", build.output, "./cmd/"+build.command)
		command.Dir = filepath.Join(o.Root, "adapters", "mcp")
		command.Env = append(filteredEnvironment(os.Environ(), "GOOS", "GOARCH", "CGO_ENABLED"), "CGO_ENABLED=0")
		if body, buildErr := command.CombinedOutput(); buildErr != nil {
			return ModernCandidate{}, fmt.Errorf("build real %s: %w: %s", build.command, buildErr, body)
		}
	}
	if err = os.WriteFile(filepath.Join(seed, "trust", "release-authority-registry.json"), registry, 0600); err != nil {
		return ModernCandidate{}, err
	}
	identity, err := releasecontract.ReadServerIdentity(filepath.Join(release, serverName))
	if err != nil {
		return ModernCandidate{}, err
	}
	if identity.Version != o.Version || identity.CompatibleBundle != compatibleRange(o.Version) || identity.AuthorityRegistrySHA256 != pin {
		return ModernCandidate{}, errors.New("built server does not match factory inputs")
	}
	server, err := inspectArtifact("mcp_server", o.Target.OS, o.Target.Arch, filepath.Join(release, serverName))
	if err != nil {
		return ModernCandidate{}, err
	}
	allowlist, err := LoadAllowlist(filepath.Join(o.Root, "bundles", "base", "distribution.json"))
	if err != nil {
		return ModernCandidate{}, err
	}
	bundleName := "maestro-base_" + o.Version + ".tar.gz"
	file, err := os.OpenFile(filepath.Join(release, bundleName), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return ModernCandidate{}, err
	}
	buildErr := BuildBundle(o.Root, allowlist, file)
	closeErr := file.Close()
	if buildErr != nil {
		return ModernCandidate{}, buildErr
	}
	if closeErr != nil {
		return ModernCandidate{}, closeErr
	}
	bundle, err := inspectArtifact("bundle", "any", "any", filepath.Join(release, bundleName))
	if err != nil {
		return ModernCandidate{}, err
	}
	m := releasecontract.Manifest{SchemaVersion: 2, Product: "maestro", Release: o.Version, Channel: o.Channel, Issuer: releasecontract.Issuer{ID: "maestro-release-candidate", KeyID: "candidate-unavailable"}, Server: releasecontract.ServerComponent{Version: o.Version, CompatibleBundle: compatibleRange(o.Version), Protocol: releasecontract.ServerProtocol, EmbeddedInputsSHA256: identity.EmbeddedInputsSHA256}, Bundle: releasecontract.BundleComponent{Version: o.Version, CompatibleServer: compatibleRange(o.Version)}, Delivery: releasecontract.DeliveryComponent{Profile: releasecontract.ModernProfile, InstallerVersion: o.InstallerVersion, InstallerProtocol: releasecontract.InstallerProtocol}, Artifacts: []releasecontract.Artifact{bundle, server}, Migrations: []releasecontract.Migration{}}
	if o.DoclingPack != "" {
		if o.Target.OS != "darwin" || o.Target.Arch != "arm64" {
			return ModernCandidate{}, errors.New("Docling has no qualified pack for the selected target")
		}
		name := "docling-runtime_" + o.Version + "_" + o.Target.OS + "_" + o.Target.Arch + ".tar.gz"
		if err = stageRegularFile(o.DoclingPack, filepath.Join(release, name), 0644); err != nil {
			return ModernCandidate{}, err
		}
		artifact, err := inspectArtifact("runtime_pack", o.Target.OS, o.Target.Arch, filepath.Join(release, name))
		if err != nil {
			return ModernCandidate{}, err
		}
		artifact.Adapter = "docling"
		m.Artifacts = append(m.Artifacts, artifact)
	}
	if expired, _ := rolemigration.IsExpired(o.Version); expired {
		_, catalog, err := inspectFile(filepath.Join(o.Root, "bundles/base/agents/catalog.json"))
		if err != nil {
			return ModernCandidate{}, err
		}
		_, policy, err := inspectFile(filepath.Join(o.Root, "bundles/base/skills/agent-skill-policy.json"))
		if err != nil {
			return ModernCandidate{}, err
		}
		m.Migrations = append(m.Migrations, releasecontract.Migration{ID: rolemigration.MigrationID, Component: "bundle", From: rolemigration.SourceRange, To: o.Version, Required: true, FromRole: rolemigration.LegacyRole, ToRole: rolemigration.CanonicalRole, AliasExpiresAfter: rolemigration.AliasExpiresAfter, BundleSHA256: bundle.SHA256, CatalogSHA256: catalog, PolicySHA256: policy})
	}
	notes := []byte("# Maestro " + o.Version + " engineering candidate\n\nUnsigned factory output. Production signing, independent seed admission and native/client/device acceptance remain separate.\n")
	m.ReleaseNotes = releasecontract.ReleaseNotes{Name: "release-notes-" + o.Version + ".md", SHA256: SHA256(notes)}
	m.Delivery.ClientInputsSHA256, err = m.ClientInputsDigest()
	if err != nil {
		return ModernCandidate{}, err
	}
	if err = m.Validate(); err != nil {
		return ModernCandidate{}, err
	}
	body, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return ModernCandidate{}, err
	}
	if err = os.WriteFile(filepath.Join(release, ManifestName), append(body, '\n'), 0644); err != nil {
		return ModernCandidate{}, err
	}
	if err = os.WriteFile(filepath.Join(release, m.ReleaseNotes.Name), notes, 0644); err != nil {
		return ModernCandidate{}, err
	}
	if err = VerifyCandidate(release); err != nil {
		return ModernCandidate{}, err
	}
	if err = os.Rename(stage, o.Output); err != nil {
		return ModernCandidate{}, err
	}
	return ModernCandidate{Directory: filepath.Join(o.Output, "candidate"), Installer: filepath.Join(o.Output, "seed", installerName), AuthorityRegistry: filepath.Join(o.Output, "seed", "trust", "release-authority-registry.json"), Manifest: m}, nil
}

func modernBinaryName(component, version string, target Target) string {
	name := component + "_" + version + "_" + target.OS + "_" + target.Arch
	if target.OS == "windows" {
		name += ".exe"
	}
	return name
}
