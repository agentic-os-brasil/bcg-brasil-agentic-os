package updateservice

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installtx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseprovider"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseverify"
)

func availabilityFixture(t *testing.T) (SyncedOptions, releasecontract.Generation, func(releasecontract.Generation)) {
	t.Helper()
	root := t.TempDir()
	owner, managed, source := filepath.Join(root, "owner"), filepath.Join(root, "product"), filepath.Join(root, "synced")
	now := time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	registry := releaseverify.StaticRegistry{"maestro/engineering/fixture": pub}
	g := releasecontract.Generation{SchemaVersion: 1, Product: "maestro", Protocol: releasecontract.GenerationProtocol,
		Issuer: releasecontract.Issuer{ID: "engineering", KeyID: "fixture"}, Source: releasecontract.SourceTuple{SourceID: "fixture", Channel: "canary", OS: "darwin", Arch: "arm64"}, Sequence: 5,
		PublishedAt: now.Add(-time.Hour).Format(time.RFC3339), ExpiresAt: now.Add(time.Hour).Format(time.RFC3339), Release: "0.2.0",
		ManifestSHA256: strings.Repeat("a", 64), ClientInputsSHA256: strings.Repeat("b", 64), Recalls: []releasecontract.ReleaseRecall{}, MinimumReaderContract: releasecontract.SyncedSourceContract}
	write := func(g releasecontract.Generation) {
		t.Helper()
		body, err := json.Marshal(g)
		if err != nil {
			t.Fatal(err)
		}
		sum := fmt.Sprintf("%x", sha256.Sum256(body))
		dir := filepath.Join(source, "generations", g.GenerationDirectory(sum))
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		for path, bytes := range map[string][]byte{
			filepath.Join(source, releaseprovider.CurrentGenerationName):          body,
			filepath.Join(source, releaseprovider.CurrentGenerationSignatureName): ed25519.Sign(key, body),
			filepath.Join(dir, releaseprovider.GenerationName):                    body,
			filepath.Join(dir, releaseprovider.GenerationSignatureName):           ed25519.Sign(key, body),
		} {
			if err := os.WriteFile(path, bytes, 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	write(g)
	body, _ := json.Marshal(g)
	data := filepath.Join(owner, ".maestro-install")
	s := installtx.State{SchemaVersion: 4, Profile: releasecontract.ModernProfile, ManagedRoot: managed,
		Release: "0.2.0", Channel: "canary", CLIVersion: "0.2.0", BundleVersion: "0.2.0", TargetOS: "darwin", TargetArch: "arm64", ActivatedAt: now,
		Server:  installtx.ExecutableIdentity{Version: "0.2.0", Protocol: releasecontract.ServerProtocol, ExecutionContract: releasecontract.SyncedSourceContract, CompatibleBundle: ">=0.2.0 <0.3.0", CompatibleServer: ">=0.2.0 <0.3.0", EmbeddedInputsSHA256: strings.Repeat("a", 64), ArtifactName: "server", SHA256: strings.Repeat("b", 64), Size: 1},
		Binding: installtx.ReleaseBinding{ManifestSHA256: g.ManifestSHA256, ClientInputsSHA256: g.ClientInputsSHA256, ManifestJSON: "{}", ManifestSignature: strings.Repeat("a", 88), BundleSHA256: strings.Repeat("c", 64), BundleSize: 1, BundleArchive: filepath.Join(data, "updates", "fixture.tar.gz")},
		Source:  &installtx.SourceControl{Locator: source, SourceBinding: installtx.SourceBinding{Tuple: g.Source, Sequence: g.Sequence, GenerationSHA256: fmt.Sprintf("%x", sha256.Sum256(body))}, Body: body, Signature: ed25519.Sign(key, body), Recalls: []releasecontract.ReleaseRecall{}, MinimumReaderContract: g.MinimumReaderContract, ObservedAt: now}}
	if err := os.MkdirAll(managed, 0700); err != nil {
		t.Fatal(err)
	}
	if err := installtx.WriteState(data, s); err != nil {
		t.Fatal(err)
	}
	return SyncedOptions{UserHome: owner, ManagedRoot: managed, DataRoot: data, SourceRoot: source, ExpectedSource: g.Source, Registry: registry, Clock: func() time.Time { return now }}, g, write
}

func availabilityTree(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	if err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out[path] = fmt.Sprintf("%x", sha256.Sum256(b))
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestSyncedAvailabilityAuthenticatesWithoutCustodyOrStaging(t *testing.T) {
	for _, tc := range []struct {
		name, outcome string
		change        func(*releasecontract.Generation)
	}{
		{"current", "current_signed_metadata", func(*releasecontract.Generation) {}},
		{"newer", "update_available_metadata_only", func(g *releasecontract.Generation) {
			g.Sequence++
			g.Release = "0.2.1"
			g.ManifestSHA256 = strings.Repeat("c", 64)
		}},
		{"expired", "source_expired", func(g *releasecontract.Generation) {
			g.Sequence++
			g.PublishedAt = "2026-11-16T00:00:00Z"
			g.ExpiresAt = "2026-11-17T11:00:00Z"
		}},
		{"replay", "source_replayed_or_changed", func(g *releasecontract.Generation) { g.Sequence-- }},
		{"equivocation", "source_replayed_or_changed", func(g *releasecontract.Generation) { g.ManifestSHA256 = strings.Repeat("c", 64) }},
		{"recall", "installed_release_recalled", func(g *releasecontract.Generation) {
			g.Sequence++
			g.Recalls = []releasecontract.ReleaseRecall{{Release: g.Release, ManifestSHA256: g.ManifestSHA256}}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o, g, write := availabilityFixture(t)
			tc.change(&g)
			write(g)
			before := availabilityTree(t, filepath.Dir(o.UserHome))
			got, err := ObserveSyncedAvailability(o)
			if err != nil || got.Outcome != tc.outcome || got.PayloadState != "not_checked" {
				t.Fatalf("observation %+v %v", got, err)
			}
			if !reflect.DeepEqual(before, availabilityTree(t, filepath.Dir(o.UserHome))) {
				t.Fatal("observation mutated source, custody, staging or owner files")
			}
		})
	}
}

func TestSyncedAvailabilityOfflineAndUnenrolledAreNotCurrent(t *testing.T) {
	o, _, _ := availabilityFixture(t)
	if err := os.Remove(filepath.Join(o.SourceRoot, releaseprovider.CurrentGenerationName)); err != nil {
		t.Fatal(err)
	}
	got, err := ObserveSyncedAvailability(o)
	if err != nil || got.Outcome != "source_unavailable" {
		t.Fatalf("offline %+v %v", got, err)
	}
	o.SourceRoot = filepath.Join(filepath.Dir(o.SourceRoot), "not-enrolled")
	if _, err := ObserveSyncedAvailability(o); err == nil {
		t.Fatal("observation selected an unenrolled source")
	}
}
