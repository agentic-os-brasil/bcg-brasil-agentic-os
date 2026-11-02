package docling_test

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	goruntime "runtime"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ingest/docling"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
)

// This fixture establishes refusal only. Successful conversion uses the complete
// official pinned pack in TestAuthenticatedRealDocling, never this archive.
func TestRuntimeRefusesSignedMalformedArchiveAndChangedAuthority(t *testing.T) {
	if goruntime.GOOS != "darwin" || goruntime.GOARCH != "arm64" {
		t.Skip("current admitted runtime platform")
	}
	root := t.TempDir()
	base := filepath.Join(root, "ingestion", "docling")
	if err := os.MkdirAll(base, 0700); err != nil {
		t.Fatal(err)
	}
	public, private, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	registry, _ := json.Marshal(map[string]any{"schema_version": 1, "product": "maestro", "authorities": []any{map[string]any{"issuer": "engineering-fixture", "key_id": "ephemeral", "algorithm": "ed25519", "public_key": base64.StdEncoding.EncodeToString(public), "status": "active", "valid_from": "2020-01-01T00:00:00Z", "valid_until": "2100-01-01T00:00:00Z"}}})
	registryPath := filepath.Join(root, "authority.json")
	put(t, registryPath, registry)
	archive := []byte("authenticated but malformed archive")
	manifest := releasecontract.Manifest{SchemaVersion: 1, Product: "maestro", Release: "0.1.0", Channel: "canary", Issuer: releasecontract.Issuer{ID: "engineering-fixture", KeyID: "ephemeral"}, CLI: releasecontract.CLIComponent{Version: "0.1.0", CompatibleBundle: ">=0.1.0 <0.2.0"}, Bundle: releasecontract.BundleComponent{Version: "0.1.0", CompatibleCLI: ">=0.1.0 <0.2.0"}, Migrations: []releasecontract.Migration{}, ReleaseNotes: releasecontract.ReleaseNotes{Name: "notes-0.1.0.md", SHA256: sum([]byte("fixture"))}, Artifacts: []releasecontract.Artifact{{Kind: "bundle", OS: "any", Arch: "any", Name: "bundle-0.1.0.tar.gz", Size: 1, SHA256: sum([]byte("b")), SignatureRef: "bundle-0.1.0.tar.gz.sig"}, {Kind: "runtime_pack", OS: "darwin", Arch: "arm64", Name: "docling.tar.gz", Size: int64(len(archive)), SHA256: sum(archive), SignatureRef: "docling.tar.gz.sig"}}}
	body, _ := json.Marshal(manifest)
	put(t, filepath.Join(base, "release-manifest.json"), body)
	put(t, filepath.Join(base, "release-manifest.json.sig"), ed25519.Sign(private, body))
	put(t, filepath.Join(base, "docling.tar.gz"), archive)
	put(t, filepath.Join(base, "docling.tar.gz.sig"), ed25519.Sign(private, archive))
	runtime := docling.New(docling.Options{DataRoot: root, AuthorityRegistryPath: registryPath, AuthorityRegistrySHA256: sum(registry), CLIVersion: "0.1.0"})
	if got := runtime.Status(context.Background()); got.State != "unavailable" || got.Reason != "pack_untrusted" {
		t.Fatalf("malformed signed archive admitted: %+v", got)
	}
	put(t, registryPath, append(registry, ' '))
	if got := runtime.Status(context.Background()); got.State != "unavailable" || got.Reason != "authority_unavailable" {
		t.Fatalf("changed registry admitted: %+v", got)
	}
}
