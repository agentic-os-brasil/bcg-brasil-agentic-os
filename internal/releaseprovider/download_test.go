package releaseprovider

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseverify"
)

type memoryAssetFetcher map[string][]byte

func (fetcher memoryAssetFetcher) FetchAsset(_ context.Context, asset Asset, destination string) error {
	body, ok := fetcher[asset.Name]
	if !ok {
		return os.ErrNotExist
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return err
	}
	return os.WriteFile(destination, body, 0o600)
}

func TestDownloadVerifiedFetchesOnlyManifestAuthorizedAssets(t *testing.T) {
	release, fetcher, registry := providerReleaseFixture(t)
	output := filepath.Join(t.TempDir(), "verified")
	verified, err := DownloadVerified(context.Background(), fetcher, release, output, registry)
	if err != nil {
		t.Fatalf("DownloadVerified() error = %v", err)
	}
	if verified.Manifest.Release != "0.2.0" {
		t.Fatalf("release = %s, want 0.2.0", verified.Manifest.Release)
	}
	if _, err := os.Stat(filepath.Join(output, "provider-extra.txt")); !os.IsNotExist(err) {
		t.Fatal("provider-only extra asset entered the verified release directory")
	}
}

func TestDownloadVerifiedRejectsMissingManifestAsset(t *testing.T) {
	release, fetcher, registry := providerReleaseFixture(t)
	for index, asset := range release.Assets {
		if asset.Name == ManifestSignatureName {
			release.Assets = append(release.Assets[:index], release.Assets[index+1:]...)
			break
		}
	}
	if _, err := DownloadVerified(context.Background(), fetcher, release, filepath.Join(t.TempDir(), "verified"), registry); err == nil {
		t.Fatal("DownloadVerified() accepted missing manifest signature")
	}
}

func providerReleaseFixture(t *testing.T) (Release, memoryAssetFetcher, releaseverify.StaticRegistry) {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	cli := []byte("cli 0.2.0")
	bundle := []byte("bundle 0.2.0")
	notes := []byte("# 0.2.0\n")
	manifest := releasecontract.Manifest{
		SchemaVersion: 1, Product: "maestro", Release: "0.2.0", Channel: "canary",
		Issuer: releasecontract.Issuer{ID: "maestro-release", KeyID: "pilot-2026"},
		CLI:    releasecontract.CLIComponent{Version: "0.2.0", CompatibleBundle: ">=0.2.0 <0.2.1"},
		Bundle: releasecontract.BundleComponent{Version: "0.2.0", CompatibleCLI: ">=0.2.0 <0.2.1"},
		Artifacts: []releasecontract.Artifact{
			{Kind: "cli", OS: "darwin", Arch: "arm64", Name: "bcgos_0.2.0_darwin_arm64", Size: int64(len(cli)), SHA256: testDigest(cli), SignatureRef: "bcgos_0.2.0_darwin_arm64.sig"},
			{Kind: "bundle", OS: "any", Arch: "any", Name: "maestro-base_0.2.0.tar.gz", Size: int64(len(bundle)), SHA256: testDigest(bundle), SignatureRef: "maestro-base_0.2.0.tar.gz.sig"},
		},
		Migrations:   []releasecontract.Migration{},
		ReleaseNotes: releasecontract.ReleaseNotes{Name: "release-notes-0.2.0.md", SHA256: testDigest(notes)},
	}
	manifestBody, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	manifestBody = append(manifestBody, '\n')
	fetcher := memoryAssetFetcher{
		ManifestName:                    manifestBody,
		ManifestSignatureName:           ed25519.Sign(privateKey, manifestBody),
		"bcgos_0.2.0_darwin_arm64":      cli,
		"bcgos_0.2.0_darwin_arm64.sig":  ed25519.Sign(privateKey, cli),
		"maestro-base_0.2.0.tar.gz":     bundle,
		"maestro-base_0.2.0.tar.gz.sig": ed25519.Sign(privateKey, bundle),
		"release-notes-0.2.0.md":        notes,
		"provider-extra.txt":            []byte("must not download"),
	}
	var assets []Asset
	for name, body := range fetcher {
		assets = append(assets, Asset{ID: int64(len(assets) + 1), Name: name, APIURL: "https://api.github.example/assets/" + name, Size: int64(len(body))})
	}
	return Release{ID: 42, TagName: "maestro-v0.2.0", Assets: assets}, fetcher,
		releaseverify.StaticRegistry{"maestro/maestro-release/pilot-2026": publicKey}
}

func testDigest(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}
