package updateservice

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installtx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseprovider"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseverify"
)

type fakeProvider struct {
	releases []releaseprovider.Release
}

func (provider fakeProvider) ListReleases(context.Context) ([]releaseprovider.Release, error) {
	return append([]releaseprovider.Release(nil), provider.releases...), nil
}

func (fakeProvider) FetchAsset(context.Context, releaseprovider.Asset, string) error {
	return errors.New("unexpected asset fetch")
}

func TestCheckBindsLatestProviderReleaseAndManifestDigest(t *testing.T) {
	provider := fakeProvider{releases: []releaseprovider.Release{
		{ID: 41, TagName: "maestro-v0.2.0"},
		{ID: 42, TagName: "maestro-v0.3.0", Prerelease: true},
		{ID: 99, TagName: "unrelated-v9.0.0"},
	}}
	download := func(
		_ context.Context,
		_ releaseprovider.AssetFetcher,
		release releaseprovider.Release,
		destination string,
		_ releaseverify.KeyRegistry,
	) (releaseverify.VerifiedRelease, error) {
		if release.ID != 42 {
			t.Fatalf("selected release ID = %d, want 42", release.ID)
		}
		if err := os.MkdirAll(destination, 0o700); err != nil {
			return releaseverify.VerifiedRelease{}, err
		}
		body := []byte("authenticated manifest bytes\n")
		if err := os.WriteFile(filepath.Join(destination, releaseverify.ManifestName), body, 0o600); err != nil {
			return releaseverify.VerifiedRelease{}, err
		}
		return releaseverify.VerifiedRelease{
			Directory: destination, Manifest: updateManifest("0.3.0"),
			ManifestSHA256: strings.Repeat("a", 64),
		}, nil
	}
	result, err := Check(context.Background(), CheckOptions{
		Current:  installtx.State{SchemaVersion: 1, Release: "0.1.0"},
		TargetOS: "darwin", TargetArch: "arm64", StagingRoot: t.TempDir(),
		Provider: provider, Registry: releaseverify.StaticRegistry{}, Download: download,
	})
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if result.Plan.ProviderReleaseID != 42 ||
		result.Plan.Provider != "github" ||
		result.Plan.ManifestSHA256 != strings.Repeat("a", 64) ||
		result.Plan.ToRelease != "0.3.0" ||
		result.Plan.ID == "" {
		t.Fatalf("unexpected bound plan: %#v", result.Plan)
	}
}

func TestCheckRejectsDuplicateOrMismatchedProviderIdentity(t *testing.T) {
	for name, releases := range map[string][]releaseprovider.Release{
		"duplicate": {
			{ID: 41, TagName: "maestro-v0.2.0"},
			{ID: 42, TagName: "maestro-v0.2.0"},
		},
		"malformed": {{ID: 42, TagName: "maestro-vlatest"}},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Check(context.Background(), CheckOptions{
				Current:  installtx.State{Release: "0.1.0"},
				TargetOS: "darwin", TargetArch: "arm64", StagingRoot: t.TempDir(),
				Provider: fakeProvider{releases: releases}, Registry: releaseverify.StaticRegistry{},
			})
			if err == nil {
				t.Fatal("Check() accepted ambiguous provider identity")
			}
		})
	}

	stagingRoot := t.TempDir()
	mismatchOptions := CheckOptions{
		Current:  installtx.State{Release: "0.1.0"},
		TargetOS: "darwin", TargetArch: "arm64", StagingRoot: stagingRoot,
		Provider: fakeProvider{releases: []releaseprovider.Release{{ID: 42, TagName: "maestro-v0.2.0"}}},
		Registry: releaseverify.StaticRegistry{},
		Download: func(
			_ context.Context,
			_ releaseprovider.AssetFetcher,
			_ releaseprovider.Release,
			destination string,
			_ releaseverify.KeyRegistry,
		) (releaseverify.VerifiedRelease, error) {
			if err := os.MkdirAll(destination, 0o700); err != nil {
				return releaseverify.VerifiedRelease{}, err
			}
			if err := os.WriteFile(filepath.Join(destination, releaseverify.ManifestName), []byte("manifest"), 0o600); err != nil {
				return releaseverify.VerifiedRelease{}, err
			}
			return releaseverify.VerifiedRelease{
				Directory: destination, Manifest: updateManifest("0.3.0"),
				ManifestSHA256: strings.Repeat("b", 64),
			}, nil
		},
	}
	for attempt := 1; attempt <= 2; attempt++ {
		_, err := Check(context.Background(), mismatchOptions)
		if err == nil || !strings.Contains(err.Error(), "does not match") {
			t.Fatalf("tag mismatch attempt %d error = %v", attempt, err)
		}
		if _, statErr := os.Stat(filepath.Join(stagingRoot, "github-release-42")); !os.IsNotExist(statErr) {
			t.Fatalf("attempt %d left a poisoned provisional destination: %v", attempt, statErr)
		}
	}
}

func TestCheckRemovesProvisionalDownloadWhenPlanCannotSupportTarget(t *testing.T) {
	stagingRoot := t.TempDir()
	_, err := Check(context.Background(), CheckOptions{
		Current:  installtx.State{Release: "0.1.0"},
		TargetOS: "windows", TargetArch: "arm64", StagingRoot: stagingRoot,
		Provider: fakeProvider{releases: []releaseprovider.Release{{ID: 42, TagName: "maestro-v0.3.0"}}},
		Registry: releaseverify.StaticRegistry{},
		Download: func(
			_ context.Context,
			_ releaseprovider.AssetFetcher,
			_ releaseprovider.Release,
			destination string,
			_ releaseverify.KeyRegistry,
		) (releaseverify.VerifiedRelease, error) {
			if err := os.MkdirAll(destination, 0o700); err != nil {
				return releaseverify.VerifiedRelease{}, err
			}
			return releaseverify.VerifiedRelease{
				Directory: destination, Manifest: updateManifest("0.3.0"),
				ManifestSHA256: strings.Repeat("c", 64),
			}, nil
		},
	})
	if err == nil || !strings.Contains(err.Error(), "does not support") {
		t.Fatalf("unsupported target error = %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(stagingRoot, "github-release-42")); !os.IsNotExist(statErr) {
		t.Fatalf("plan failure left a poisoned provisional destination: %v", statErr)
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
