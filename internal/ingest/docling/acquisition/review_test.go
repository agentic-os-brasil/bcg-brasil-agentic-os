package acquisition_test

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ingest/docling/acquisition"
)

func TestReviewRejectsUntrustedManifestBeforeCacheEffects(t *testing.T) {
	root := t.TempDir()
	release := filepath.Join(root, "release")
	if err := os.Mkdir(release, 0700); err != nil {
		t.Fatal(err)
	}
	public, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	issuer := "maestro-engineering-pilot-" + now.Format("20060102")
	keyID := "pilot-" + now.Format("20060102")
	registry, _ := json.Marshal(map[string]any{"schema_version": 1, "product": "maestro", "authorities": []any{map[string]any{"issuer": issuer, "key_id": keyID, "algorithm": "ed25519", "public_key": base64.StdEncoding.EncodeToString(public), "status": "active", "valid_from": now.Add(-time.Minute).Format(time.RFC3339), "valid_until": now.Add(14 * 24 * time.Hour).Format(time.RFC3339)}}})
	registryPath := filepath.Join(root, "public-registry.json")
	if err = os.WriteFile(registryPath, registry, 0600); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]any{"issuer": map[string]any{"id": issuer, "key_id": keyID}})
	if err = os.WriteFile(filepath.Join(release, "release-manifest.json"), body, 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(release, "release-manifest.json.sig"), make([]byte, 64), 0600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(registry)
	cache := filepath.Join(root, "private-cache")
	_, err = acquisition.Review(context.Background(), acquisition.Options{ReleaseDirectory: release, RecipeDirectory: filepath.Join(root, "metadata"), ServerPath: filepath.Join(root, "server.exe"), RegistryPath: registryPath, RegistrySHA256: hex.EncodeToString(sum[:]), CacheRoot: cache})
	if err == nil || err.Error() != "acquisition_manifest_untrusted" {
		t.Fatalf("untrusted manifest did not fail first: %v", err)
	}
	if _, err = os.Lstat(cache); !os.IsNotExist(err) {
		t.Fatalf("review created private cache: %v", err)
	}
}
