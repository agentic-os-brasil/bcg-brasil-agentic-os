package updateplan

import (
	"strings"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installtx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
)

func TestBuildRequiresExplicitConfirmationForUpgrade(t *testing.T) {
	current := installtx.State{SchemaVersion: 1, Release: "0.1.0", CLIVersion: "0.1.0", BundleVersion: "0.1.0", TargetOS: "darwin", TargetArch: "arm64"}
	manifest := updateManifest("0.2.0")
	source := SourceBinding{Provider: "github", ProviderReleaseID: 42, ManifestSHA256: strings.Repeat("a", 64)}
	first, err := Build(current, manifest, "darwin", "arm64", source)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Build(current, manifest, "darwin", "arm64", source)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == "" || first.ID != second.ID || !first.ConfirmationRequired || first.State != "available" {
		t.Fatalf("unexpected update plan: %#v", first)
	}
	if first.CLIArtifact != "bcgos_0.2.0_darwin_arm64" || first.BundleArtifact != "maestro-base_0.2.0.tar.gz" {
		t.Fatalf("unexpected selected artifacts: %#v", first)
	}
	if first.Provider != "github" || first.ProviderReleaseID != 42 || first.ManifestSHA256 != strings.Repeat("a", 64) {
		t.Fatalf("source binding was not preserved: %#v", first)
	}
}

func TestBuildRejectsDowngradeAndUnsupportedPlatform(t *testing.T) {
	current := installtx.State{SchemaVersion: 1, Release: "0.2.0", CLIVersion: "0.2.0", BundleVersion: "0.2.0"}
	for name, manifest := range map[string]releasecontract.Manifest{
		"downgrade": updateManifest("0.1.0"),
		"same":      updateManifest("0.2.0"),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Build(current, manifest, "darwin", "arm64", testSourceBinding()); err == nil {
				t.Fatal("Build() accepted a non-upgrade")
			}
		})
	}
	if _, err := Build(current, updateManifest("0.3.0"), "windows", "arm64", testSourceBinding()); err == nil {
		t.Fatal("Build() accepted a release without the target CLI")
	}
}

func TestBuildRejectsUnboundProviderSource(t *testing.T) {
	current := installtx.State{SchemaVersion: 1, Release: "0.1.0"}
	for name, source := range map[string]SourceBinding{
		"provider":   {Provider: "other", ProviderReleaseID: 42, ManifestSHA256: strings.Repeat("a", 64)},
		"release id": {Provider: "github", ProviderReleaseID: 0, ManifestSHA256: strings.Repeat("a", 64)},
		"digest":     {Provider: "github", ProviderReleaseID: 42, ManifestSHA256: "invalid"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Build(current, updateManifest("0.2.0"), "darwin", "arm64", source); err == nil {
				t.Fatal("Build() accepted an unbound source")
			}
		})
	}
}

func testSourceBinding() SourceBinding {
	return SourceBinding{Provider: "github", ProviderReleaseID: 42, ManifestSHA256: strings.Repeat("a", 64)}
}

func updateManifest(version string) releasecontract.Manifest {
	return releasecontract.Manifest{
		SchemaVersion: 1, Product: "maestro", Release: version, Channel: "canary",
		CLI:    releasecontract.CLIComponent{Version: version},
		Bundle: releasecontract.BundleComponent{Version: version},
		Artifacts: []releasecontract.Artifact{
			{Kind: "cli", OS: "darwin", Arch: "arm64", Name: "bcgos_" + version + "_darwin_arm64"},
			{Kind: "bundle", OS: "any", Arch: "any", Name: "maestro-base_" + version + ".tar.gz"},
		},
	}
}
