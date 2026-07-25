package updateplan

import (
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installtx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
)

func TestBuildRequiresExplicitConfirmationForUpgrade(t *testing.T) {
	current := installtx.State{SchemaVersion: 1, Release: "0.1.0", CLIVersion: "0.1.0", BundleVersion: "0.1.0", TargetOS: "darwin", TargetArch: "arm64"}
	manifest := updateManifest("0.2.0")
	first, err := Build(current, manifest, "darwin", "arm64")
	if err != nil {
		t.Fatal(err)
	}
	second, err := Build(current, manifest, "darwin", "arm64")
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == "" || first.ID != second.ID || !first.ConfirmationRequired || first.State != "available" {
		t.Fatalf("unexpected update plan: %#v", first)
	}
	if first.CLIArtifact != "bcgos_0.2.0_darwin_arm64" || first.BundleArtifact != "maestro-base_0.2.0.tar.gz" {
		t.Fatalf("unexpected selected artifacts: %#v", first)
	}
}

func TestBuildRejectsDowngradeAndUnsupportedPlatform(t *testing.T) {
	current := installtx.State{SchemaVersion: 1, Release: "0.2.0", CLIVersion: "0.2.0", BundleVersion: "0.2.0"}
	for name, manifest := range map[string]releasecontract.Manifest{
		"downgrade": updateManifest("0.1.0"),
		"same":      updateManifest("0.2.0"),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Build(current, manifest, "darwin", "arm64"); err == nil {
				t.Fatal("Build() accepted a non-upgrade")
			}
		})
	}
	if _, err := Build(current, updateManifest("0.3.0"), "windows", "arm64"); err == nil {
		t.Fatal("Build() accepted a release without the target CLI")
	}
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
