package acquisition

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ingest/docling"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseverify"
)

type signedFixture struct {
	options Options
	recipe  Recipe
	key     ed25519.PrivateKey
}

func fixture(t *testing.T) signedFixture {
	t.Helper()
	root := t.TempDir()
	now := time.Now().UTC().Truncate(time.Second)
	o := Options{ReleaseDirectory: filepath.Join(root, "release"), RecipeDirectory: filepath.Join(root, "recipe"), ServerPath: filepath.Join(root, "server.exe"), RegistryPath: filepath.Join(root, "registry.json"), CacheRoot: filepath.Join(root, "cache")}
	for _, d := range []string{o.ReleaseDirectory, o.RecipeDirectory} {
		if err := os.Mkdir(d, 0700); err != nil {
			t.Fatal(err)
		}
	}
	public, key, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	issuer := releasecontract.Issuer{ID: "maestro-engineering-pilot-" + now.Format("20060102"), KeyID: "pilot-" + now.Format("20060102")}
	registry, _ := releasecontract.CanonicalJSON(map[string]any{"schema_version": 1, "product": "maestro", "authorities": []any{map[string]any{"issuer": issuer.ID, "key_id": issuer.KeyID, "algorithm": "ed25519", "public_key": base64.StdEncoding.EncodeToString(public), "status": "active", "valid_from": now.Add(-time.Minute).Format(time.RFC3339), "valid_until": now.Add(14*24*time.Hour - time.Minute).Format(time.RFC3339)}}})
	writeFixture(t, o.RegistryPath, registry)
	o.RegistrySHA256 = digest(registry)
	server := []byte("public signed server fixture; never executed")
	writeFixture(t, o.ServerPath, server)
	m := releasecontract.Manifest{SchemaVersion: 2, Product: "maestro", Release: "0.1.0", Channel: "canary", Issuer: issuer, Server: releasecontract.ServerComponent{Version: "0.1.0", CompatibleBundle: ">=0.1.0 <0.1.1", Protocol: releasecontract.ServerProtocol, EmbeddedInputsSHA256: digest([]byte("inventory"))}, Delivery: releasecontract.DeliveryComponent{Profile: releasecontract.ModernProfile, InstallerVersion: "0.1.0", InstallerProtocol: releasecontract.InstallerProtocol}, Bundle: releasecontract.BundleComponent{Version: "0.1.0", CompatibleServer: ">=0.1.0 <0.1.1"}, Migrations: []releasecontract.Migration{}, ReleaseNotes: releasecontract.ReleaseNotes{Name: "notes_0.1.0.md", SHA256: digest([]byte("public notes"))}}
	for _, a := range []struct {
		kind, name, os, arch string
		body                 []byte
	}{{"bundle", "base_0.1.0.tar.gz", "any", "any", []byte("bundle")}, {"mcp_server", "server_0.1.0_windows_amd64.exe", "windows", "amd64", server}, {"runtime_pack", "runtime_0.1.0_windows_amd64.tar.gz", "windows", "amd64", []byte("expected assembled archive")}} {
		artifact := releasecontract.Artifact{Kind: a.kind, OS: a.os, Arch: a.arch, Name: a.name, Size: int64(len(a.body)), SHA256: digest(a.body), SignatureRef: a.name + ".sig"}
		if a.kind == "runtime_pack" {
			artifact.Adapter = "docling"
		} else {
			writeFixture(t, filepath.Join(o.ReleaseDirectory, a.name), a.body)
		}
		writeFixture(t, filepath.Join(o.ReleaseDirectory, artifact.SignatureRef), ed25519.Sign(key, a.body))
		m.Artifacts = append(m.Artifacts, artifact)
	}
	m.Delivery.ClientInputsSHA256, _ = m.ClientInputsDigest()
	body, _ := releasecontract.CanonicalJSON(m)
	if _, _, err = releaseverify.VerifyManifest(body, ed25519.Sign(key, body), releaseverify.StaticRegistry{"maestro/" + issuer.ID + "/" + issuer.KeyID: public}); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, filepath.Join(o.ReleaseDirectory, releaseverify.ManifestName), body)
	writeFixture(t, filepath.Join(o.ReleaseDirectory, releaseverify.ManifestSignatureName), ed25519.Sign(key, body))
	writeFixture(t, filepath.Join(o.ReleaseDirectory, "notes_0.1.0.md"), []byte("public notes"))
	d, err := docling.DescribeProfile("windows", "amd64", docling.DocumentsProfile)
	if err != nil {
		t.Fatal(err)
	}
	db, _ := releasecontract.CanonicalJSON(d)
	total := d.Python.Size
	for _, i := range d.Inputs {
		total += i.Size
	}
	f := signedFixture{o, Recipe{SchemaVersion: 1, Purpose: Purpose, Issuer: issuer, ManifestSHA256: digest(body), ServerSHA256: digest(server), OS: "windows", Arch: "amd64", Profile: d.Profile, DescriptorSHA256: digest(db), InputCount: len(d.Inputs) + 1, TotalInputBytes: total, RuntimeArtifact: m.Artifacts[2], IssuedAt: now.Add(-time.Minute).Format(time.RFC3339), ExpiresAt: now.Add(time.Hour).Format(time.RFC3339)}, key}
	f.sign(t)
	return f
}
func writeFixture(t *testing.T, name string, body []byte) {
	t.Helper()
	if err := os.WriteFile(name, body, 0600); err != nil {
		t.Fatal(err)
	}
}
func (f signedFixture) sign(t *testing.T) {
	t.Helper()
	b, _ := releasecontract.CanonicalJSON(f.recipe)
	writeFixture(t, filepath.Join(f.options.RecipeDirectory, RecipeName), b)
	writeFixture(t, filepath.Join(f.options.RecipeDirectory, RecipeSignatureName), ed25519.Sign(f.key, b))
}
func TestReviewAuthenticatesClosedRecipeWithoutEffects(t *testing.T) {
	f := fixture(t)
	p, err := Review(context.Background(), f.options)
	if err != nil {
		t.Fatal(err)
	}
	if p.Recipe.InputCount != 57 || len(p.Inputs) != 57 || p.SHA256 == "" {
		t.Fatalf("wrong closed review: %+v", p)
	}
	if _, err = os.Lstat(f.options.CacheRoot); !os.IsNotExist(err) {
		t.Fatal("review created cache")
	}
}
func TestReviewRejectsSignedRecipeDriftBeforeCacheEffects(t *testing.T) {
	tests := map[string]func(*Recipe){"purpose": func(r *Recipe) { r.Purpose = "another-domain" }, "expired": func(r *Recipe) { r.ExpiresAt = time.Now().Add(-time.Minute).UTC().Format(time.RFC3339) }, "future": func(r *Recipe) { r.IssuedAt = time.Now().Add(time.Hour).UTC().Format(time.RFC3339) }, "long": func(r *Recipe) { r.ExpiresAt = time.Now().Add(15 * 24 * time.Hour).UTC().Format(time.RFC3339) }, "mixed target": func(r *Recipe) { r.OS = "darwin" }, "profile": func(r *Recipe) { r.Profile = docling.LegacyProfile }, "descriptor": func(r *Recipe) { r.DescriptorSHA256 = digest([]byte("other")) }, "count": func(r *Recipe) { r.InputCount-- }, "aggregate": func(r *Recipe) { r.TotalInputBytes++ }, "archive": func(r *Recipe) { r.RuntimeArtifact.Size++ }, "server": func(r *Recipe) { r.ServerSHA256 = digest([]byte("other")) }, "manifest": func(r *Recipe) { r.ManifestSHA256 = digest([]byte("other")) }}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			f := fixture(t)
			mutate(&f.recipe)
			f.sign(t)
			if _, err := Review(context.Background(), f.options); err == nil {
				t.Fatal("signed drift accepted")
			}
			if _, err := os.Lstat(f.options.CacheRoot); !os.IsNotExist(err) {
				t.Fatal("rejected review created cache")
			}
		})
	}
}

func TestAcquisitionAuthorityHasTheSameFiniteEngineeringPilotWindow(t *testing.T) {
	f := fixture(t)
	body, err := os.ReadFile(f.options.RegistryPath)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err = json.Unmarshal(body, &document); err != nil {
		t.Fatal(err)
	}
	document["authorities"].([]any)[0].(map[string]any)["valid_until"] = time.Now().Add(15 * 24 * time.Hour).UTC().Format(time.RFC3339)
	body, _ = releasecontract.CanonicalJSON(document)
	writeFixture(t, f.options.RegistryPath, body)
	f.options.RegistrySHA256 = digest(body)
	if _, err = Review(context.Background(), f.options); err == nil {
		t.Fatal("acquisition accepted an authority outside the fourteen-day engineering pilot contract")
	}
}
