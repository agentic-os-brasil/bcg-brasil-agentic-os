package releaseprovider_test

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseprovider"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseverify"
)

func syncedSnapshotFixture(t *testing.T) (string, releaseverify.VerifiedGeneration, releaseverify.KeyRegistry) {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(base, "synced")
	key := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{73}, ed25519.SeedSize))
	registry := releaseverify.StaticRegistry{"maestro/synced-test/fixture": key.Public().(ed25519.PublicKey)}
	digest := func(b []byte) string { return fmt.Sprintf("%x", sha256.Sum256(b)) }
	notes := []byte("synthetic release notes")
	m := releasecontract.Manifest{SchemaVersion: 2, Product: "maestro", Release: "0.1.0", Channel: "canary", Issuer: releasecontract.Issuer{ID: "synced-test", KeyID: "fixture"}, Server: releasecontract.ServerComponent{Version: "0.1.0", CompatibleBundle: ">=0.1.0 <0.2.0", Protocol: releasecontract.ServerProtocol, EmbeddedInputsSHA256: strings.Repeat("a", 64), ExecutionContract: releasecontract.SyncedSourceContract}, Bundle: releasecontract.BundleComponent{Version: "0.1.0", CompatibleServer: ">=0.1.0 <0.2.0"}, Delivery: releasecontract.DeliveryComponent{Profile: releasecontract.ModernProfile, InstallerVersion: "0.1.0", InstallerProtocol: releasecontract.InstallerProtocol}, Artifacts: []releasecontract.Artifact{}, Migrations: []releasecontract.Migration{}, ReleaseNotes: releasecontract.ReleaseNotes{Name: "notes-0.1.0.md", SHA256: digest(notes)}}
	bodies := map[string][]byte{m.ReleaseNotes.Name: notes}
	for _, part := range []struct{ kind, os, arch, name, body string }{{"mcp_server", runtime.GOOS, runtime.GOARCH, "server-0.1.0", "synthetic source server bytes"}, {"bundle", "any", "any", "bundle-0.1.0.zip", "synthetic source bundle bytes"}} {
		body := []byte(part.body)
		m.Artifacts = append(m.Artifacts, releasecontract.Artifact{Kind: part.kind, OS: part.os, Arch: part.arch, Name: part.name, Size: int64(len(body)), SHA256: digest(body), SignatureRef: part.name + ".sig"})
		bodies[part.name], bodies[part.name+".sig"] = body, ed25519.Sign(key, body)
	}
	m.Delivery.ClientInputsSHA256, err = m.ClientInputsDigest()
	if err != nil {
		t.Fatal(err)
	}
	if err = m.Validate(); err != nil {
		t.Fatal(err)
	}
	manifest, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	bodies[releaseprovider.ManifestName], bodies[releaseprovider.ManifestSignatureName] = manifest, ed25519.Sign(key, manifest)
	g := releasecontract.Generation{SchemaVersion: 1, Product: "maestro", Protocol: releasecontract.GenerationProtocol, Issuer: m.Issuer, Source: releasecontract.SourceTuple{SourceID: "synthetic-team", Channel: m.Channel, OS: runtime.GOOS, Arch: runtime.GOARCH}, Sequence: 1, PublishedAt: "2026-11-17T00:00:00Z", ExpiresAt: "2026-11-24T00:00:00Z", Release: m.Release, ManifestSHA256: digest(manifest), ClientInputsSHA256: m.Delivery.ClientInputsSHA256, Recalls: []releasecontract.ReleaseRecall{}, MinimumReaderContract: releasecontract.SyncedSourceContract}
	generation, err := json.Marshal(g)
	if err != nil {
		t.Fatal(err)
	}
	verified, err := releaseverify.VerifyGeneration(generation, ed25519.Sign(key, generation), registry)
	if err != nil {
		t.Fatal(err)
	}
	write := func(path string, body []byte) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, body, 0600); err != nil {
			t.Fatal(err)
		}
	}
	for name, body := range bodies {
		write(filepath.Join(releaseprovider.SyncedReleaseDirectory(root, g), name), body)
	}
	write(filepath.Join(root, releaseprovider.CurrentGenerationName), generation)
	write(filepath.Join(root, releaseprovider.CurrentGenerationSignatureName), verified.Signature)
	immutable := filepath.Join(root, "generations", g.GenerationDirectory(verified.SHA256))
	write(filepath.Join(immutable, releaseprovider.GenerationName), generation)
	write(filepath.Join(immutable, releaseprovider.GenerationSignatureName), verified.Signature)
	return root, verified, registry
}

func TestSyncedSnapshotKeepsPrivateOwnerAndAcceptedBytes(t *testing.T) {
	for _, mode := range []string{"complete", "changed-payload", "partial-signature", "missing-payload", "source-overlap", "occupied-destination"} {
		t.Run(mode, func(t *testing.T) {
			root, generation, registry := syncedSnapshotFixture(t)
			private := filepath.Dir(root)
			retained := filepath.Join(private, "owner-brain.md")
			accepted := filepath.Join(private, "accepted-server")
			for _, path := range []string{retained, accepted} {
				if err := os.WriteFile(path, []byte("KEEP EXACT PRIVATE BYTES"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			destination := filepath.Join(private, "new-private-snapshot")
			release := releaseprovider.SyncedReleaseDirectory(root, generation.Generation)
			switch mode {
			case "changed-payload":
				if err := os.WriteFile(filepath.Join(release, "server-0.1.0"), []byte("changed after review"), 0600); err != nil {
					t.Fatal(err)
				}
			case "partial-signature":
				if err := os.WriteFile(filepath.Join(root, releaseprovider.CurrentGenerationSignatureName), []byte("partial"), 0600); err != nil {
					t.Fatal(err)
				}
			case "missing-payload":
				if err := os.Remove(filepath.Join(release, "bundle-0.1.0.zip")); err != nil {
					t.Fatal(err)
				}
			case "source-overlap":
				destination = filepath.Join(root, "snapshot")
			case "occupied-destination":
				if err := os.Mkdir(destination, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(destination, "existing.md"), []byte("KEEP EXACT PRIVATE BYTES"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			snapshot, err := releaseprovider.SnapshotSyncedRelease(root, destination, generation, registry)
			if mode == "complete" {
				if err != nil || snapshot.ManifestSHA256 != generation.Generation.ManifestSHA256 {
					t.Fatalf("complete verified snapshot failed: %v", err)
				}
				if err := os.WriteFile(filepath.Join(release, "server-0.1.0"), []byte("later source mutation"), 0600); err != nil {
					t.Fatal(err)
				}
				body, err := os.ReadFile(filepath.Join(destination, "server-0.1.0"))
				if err != nil || string(body) != "synthetic source server bytes" {
					t.Fatal("private copy followed later source mutation", err)
				}
			} else if err == nil {
				t.Fatal("incomplete/unsafe source admitted")
			}
			for _, path := range []string{retained, accepted} {
				body, err := os.ReadFile(path)
				if err != nil || string(body) != "KEEP EXACT PRIVATE BYTES" {
					t.Fatal("source ingress changed private owner/accepted state", err)
				}
			}
			if mode == "occupied-destination" {
				body, err := os.ReadFile(filepath.Join(destination, "existing.md"))
				if err != nil || string(body) != "KEEP EXACT PRIVATE BYTES" {
					t.Fatal("existing destination changed", err)
				}
			} else if mode != "complete" {
				if _, err := os.Stat(destination); !os.IsNotExist(err) {
					t.Fatal("failed ingress published destination", err)
				}
			}
		})
	}
}
