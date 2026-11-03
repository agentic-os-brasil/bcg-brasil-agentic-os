package releasepack

import (
	"bytes"
	"crypto/ed25519"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseprovider"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseverify"
)

// This is a transport/signature fixture, not native execution qualification.
func signedGenerationFixture(t *testing.T) (PublishGenerationOptions, releaseverify.AuthorityRegistry) {
	t.Helper()
	base := t.TempDir()
	key := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{37}, 32))
	registryPath := filepath.Join(base, "registry.json")
	if err := os.WriteFile(registryPath, authorityRegistryForKey(t, key.Public().(ed25519.PublicKey)), 0600); err != nil {
		t.Fatal(err)
	}
	clock := func() time.Time { return time.Unix(2000, 0).UTC() }
	registry, err := releaseverify.LoadAuthorityRegistry(registryPath, clock)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(base, "signed")
	if err = os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	m := releasecontract.Manifest{SchemaVersion: 2, Product: "maestro", Release: "0.1.0", Channel: "canary", Issuer: releasecontract.Issuer{ID: "maestro-release", KeyID: "pilot-2026"}, Server: releasecontract.ServerComponent{Version: "0.1.0", CompatibleBundle: ">=0.1.0 <0.2.0", Protocol: releasecontract.ServerProtocol, EmbeddedInputsSHA256: strings.Repeat("a", 64), ExecutionContract: releasecontract.SyncedSourceContract}, Bundle: releasecontract.BundleComponent{Version: "0.1.0", CompatibleServer: ">=0.1.0 <0.2.0"}, Delivery: releasecontract.DeliveryComponent{Profile: releasecontract.ModernProfile, InstallerVersion: "0.1.0", InstallerProtocol: releasecontract.InstallerProtocol}, Artifacts: []releasecontract.Artifact{}, Migrations: []releasecontract.Migration{}, ReleaseNotes: releasecontract.ReleaseNotes{Name: "notes-0.1.0.md", SHA256: SHA256([]byte("notes"))}}
	for _, part := range []struct{ kind, os, arch, name, body string }{{"mcp_server", "darwin", "arm64", "server-0.1.0", "signed synthetic server bytes"}, {"bundle", "any", "any", "bundle-0.1.0.zip", "signed synthetic bundle bytes"}} {
		body := []byte(part.body)
		a := releasecontract.Artifact{Kind: part.kind, OS: part.os, Arch: part.arch, Name: part.name, Size: int64(len(body)), SHA256: SHA256(body), SignatureRef: part.name + ".sig"}
		m.Artifacts = append(m.Artifacts, a)
		if err = os.WriteFile(filepath.Join(dir, a.Name), body, 0600); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(dir, a.SignatureRef), ed25519.Sign(key, body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	m.Delivery.ClientInputsSHA256, err = m.ClientInputsDigest()
	if err != nil {
		t.Fatal(err)
	}
	if err = m.Validate(); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(m)
	for name, data := range map[string][]byte{releaseverify.ManifestName: body, releaseverify.ManifestSignatureName: ed25519.Sign(key, body), m.ReleaseNotes.Name: []byte("notes")} {
		if err = os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	g := releasecontract.Generation{SchemaVersion: 1, Product: "maestro", Protocol: releasecontract.GenerationProtocol, Issuer: m.Issuer, Source: releasecontract.SourceTuple{SourceID: "engineering-team", Channel: "canary", OS: "darwin", Arch: "arm64"}, Sequence: 10, PublishedAt: clock().Format(time.RFC3339), ExpiresAt: clock().Add(7 * 24 * time.Hour).Format(time.RFC3339), Release: m.Release, ManifestSHA256: SHA256(body), ClientInputsSHA256: m.Delivery.ClientInputsSHA256, Recalls: []releasecontract.ReleaseRecall{}, MinimumReaderContract: releasecontract.SyncedSourceContract}
	return PublishGenerationOptions{Root: filepath.Join(base, "synced"), ReleaseDirectory: dir, Registry: registryPath, Generation: g, PrivateKey: key, Clock: clock}, registry
}

func TestSignedGenerationPublishReviewSnapshotAndIdenticalRetry(t *testing.T) {
	o, registry := signedGenerationFixture(t)
	v, err := PublishGeneration(o)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = PublishGeneration(o); err != nil {
		t.Fatalf("identical retry: %v", err)
	}
	m, err := releaseprovider.ReadSyncedManifest(o.Root, v, registry)
	if err != nil || m.Release != "0.1.0" {
		t.Fatalf("review: %v", err)
	}
	destination := filepath.Join(t.TempDir(), "private")
	snapshot, err := releaseprovider.SnapshotSyncedRelease(o.Root, destination, v, registry)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Directory != destination || snapshot.ManifestSHA256 != v.Generation.ManifestSHA256 {
		t.Fatal("snapshot binding differs")
	}
	if err = os.WriteFile(filepath.Join(o.Root, "releases", "0.1.0", "darwin-arm64", "server-0.1.0"), []byte("mutation"), 0600); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(destination, "server-0.1.0"))
	if err != nil || string(body) != "signed synthetic server bytes" {
		t.Fatal("private snapshot followed live synced mutation")
	}
}

func TestSignedGenerationRefusesReplayEquivocationSplitAndAliases(t *testing.T) {
	for _, mode := range []string{"replay", "equivocation", "split", "symlink", "hardlink", "wrong-target", "missing-payload"} {
		t.Run(mode, func(t *testing.T) {
			o, registry := signedGenerationFixture(t)
			v, err := PublishGeneration(o)
			if err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "replay":
				o.Generation.Sequence--
				if _, err = PublishGeneration(o); err == nil {
					t.Fatal("replay accepted")
				}
			case "equivocation":
				o.Generation.ExpiresAt = o.Clock().Add(8 * 24 * time.Hour).Format(time.RFC3339)
				if _, err = PublishGeneration(o); err == nil {
					t.Fatal("equivocation accepted")
				}
			case "split":
				if err = os.WriteFile(filepath.Join(o.Root, releaseprovider.CurrentGenerationSignatureName), make([]byte, ed25519.SignatureSize), 0600); err != nil {
					t.Fatal(err)
				}
				if _, err = releaseprovider.ReadSyncedMetadata(o.Root, o.Generation.Source, registry); err == nil {
					t.Fatal("split signature accepted")
				}
			case "symlink", "hardlink":
				path := filepath.Join(o.Root, releaseprovider.CurrentGenerationName)
				original := filepath.Join(t.TempDir(), "original")
				body, _ := os.ReadFile(path)
				if err = os.WriteFile(original, body, 0600); err != nil {
					t.Fatal(err)
				}
				os.Remove(path)
				if mode == "symlink" {
					err = os.Symlink(original, path)
				} else {
					err = os.Link(original, path)
				}
				if err != nil {
					t.Skipf("link unavailable: %v", err)
				}
				if _, err = releaseprovider.ReadSyncedMetadata(o.Root, o.Generation.Source, registry); err == nil {
					t.Fatal("aliased generation accepted")
				}
			case "wrong-target":
				tuple := o.Generation.Source
				tuple.OS = "windows"
				if _, err = releaseprovider.ReadSyncedMetadata(o.Root, tuple, registry); err == nil {
					t.Fatal("wrong target accepted")
				}
			case "missing-payload":
				if err = os.Remove(filepath.Join(releaseprovider.SyncedReleaseDirectory(o.Root, o.Generation), "server-0.1.0")); err != nil {
					t.Fatal(err)
				}
				if _, err = releaseprovider.ReadSyncedMetadata(o.Root, o.Generation.Source, registry); err != nil {
					t.Fatalf("metadata denial hidden by missing payload: %v", err)
				}
				if _, err = releaseprovider.SnapshotSyncedRelease(o.Root, filepath.Join(t.TempDir(), "private"), v, registry); err == nil {
					t.Fatal("partial release copied")
				}
			}
		})
	}
}

func TestSignedGenerationExpiredHigherRecallReadableWithoutPayload(t *testing.T) {
	o, registry := signedGenerationFixture(t)
	if _, err := PublishGeneration(o); err != nil {
		t.Fatal(err)
	}
	g := o.Generation
	g.Sequence = 11
	g.Recalls = []releasecontract.ReleaseRecall{{Release: g.Release, ManifestSHA256: g.ManifestSHA256}}
	// The record is within the actual key window but already expired at the
	// trusted observation clock. It must remain readable without its payload.
	g.PublishedAt = "1970-01-01T00:00:00Z"
	g.ExpiresAt = "1970-01-01T00:20:00Z"
	body, _ := releasecontract.CanonicalJSON(g)
	sig := ed25519.Sign(o.PrivateKey, body)
	dir := filepath.Join(o.Root, "generations", g.GenerationDirectory(SHA256(body)))
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	for path, data := range map[string][]byte{filepath.Join(dir, releaseprovider.GenerationName): body, filepath.Join(dir, releaseprovider.GenerationSignatureName): sig, filepath.Join(o.Root, releaseprovider.CurrentGenerationName): body, filepath.Join(o.Root, releaseprovider.CurrentGenerationSignatureName): sig} {
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.RemoveAll(releaseprovider.SyncedReleaseDirectory(o.Root, g)); err != nil {
		t.Fatal(err)
	}
	v, err := releaseprovider.ReadSyncedMetadata(o.Root, g.Source, registry)
	if err != nil {
		t.Fatalf("expired authenticated denial hidden: %v", err)
	}
	if v.Generation.Sequence != 11 || len(v.Generation.Recalls) != 1 {
		t.Fatal("higher denial not retained in verified observation")
	}
	if err = v.Generation.CheckTime(o.Clock(), time.Time{}); err == nil {
		t.Fatal("expired higher generation grants freshness")
	}
	if _, err = releaseprovider.SnapshotSyncedRelease(o.Root, filepath.Join(t.TempDir(), "private"), v, registry); err == nil {
		t.Fatal("missing expired candidate copied")
	}
}

func TestSignedGenerationPublisherRefusesLifetimeBeyondKeyAndPartialImmutablePointer(t *testing.T) {
	o, registry := signedGenerationFixture(t)
	o.Clock = func() time.Time { return time.Date(2099, 12, 30, 0, 0, 0, 0, time.UTC) }
	o.Generation.PublishedAt = o.Clock().Format(time.RFC3339)
	o.Generation.ExpiresAt = o.Clock().Add(7 * 24 * time.Hour).Format(time.RFC3339)
	if _, err := PublishGeneration(o); err == nil {
		t.Fatal("publication beyond key validity accepted")
	}
	o, registry = signedGenerationFixture(t)
	v, err := PublishGeneration(o)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.RemoveAll(filepath.Join(o.Root, "generations", v.Generation.GenerationDirectory(v.SHA256))); err != nil {
		t.Fatal(err)
	}
	if _, err = releaseprovider.ReadSyncedMetadata(o.Root, o.Generation.Source, registry); err == nil {
		t.Fatal("pointer before immutable generation accepted")
	}
}

func TestSignedGenerationPublisherSerializesSameSequenceEquivocation(t *testing.T) {
	o, registry := signedGenerationFixture(t)
	other := o
	other.Generation.ExpiresAt = o.Clock().Add(8 * 24 * time.Hour).Format(time.RFC3339)
	results := make(chan error, 2)
	go func() { _, err := PublishGeneration(o); results <- err }()
	go func() { _, err := PublishGeneration(other); results <- err }()
	successes := 0
	for range 2 {
		if <-results == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("concurrent equivocation produced %d successful publications", successes)
	}
	v, err := releaseprovider.ReadSyncedMetadata(o.Root, o.Generation.Source, registry)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(o.Root, "generations"))
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "10-") {
			count++
			if entry.Name() != v.Generation.GenerationDirectory(v.SHA256) {
				t.Fatal("current pointer differs from sole immutable generation")
			}
		}
	}
	if count != 1 {
		t.Fatalf("concurrent same-sequence publication left %d immutable generations", count)
	}
}
