package main

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/datahome"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ingest/docling/acquisition"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ingest/docling"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseverify"
)

type acquisitionFixture struct {
	options acquisition.Options
	recipe  acquisition.Recipe
	key     ed25519.PrivateKey
}

func signedAcquisitionFixture(t *testing.T) acquisitionFixture {
	t.Helper()
	root := t.TempDir()
	now := time.Now().UTC().Truncate(time.Second)
	o := acquisition.Options{ReleaseDirectory: filepath.Join(root, "release"), RecipeDirectory: filepath.Join(root, "recipe"), ServerPath: filepath.Join(root, "server.exe"), RegistryPath: filepath.Join(root, "registry.json"), CacheRoot: filepath.Join(root, "cache")}
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
	writeAcquisitionFixture(t, o.RegistryPath, registry)
	o.RegistrySHA256 = acquisitionTestDigest(registry)
	server := []byte("public signed server fixture; never executed")
	writeAcquisitionFixture(t, o.ServerPath, server)
	m := releasecontract.Manifest{SchemaVersion: 2, Product: "maestro", Release: "0.1.0", Channel: "canary", Issuer: issuer, Server: releasecontract.ServerComponent{Version: "0.1.0", CompatibleBundle: ">=0.1.0 <0.1.1", Protocol: releasecontract.ServerProtocol, EmbeddedInputsSHA256: acquisitionTestDigest([]byte("inventory"))}, Delivery: releasecontract.DeliveryComponent{Profile: releasecontract.ModernProfile, InstallerVersion: "0.1.0", InstallerProtocol: releasecontract.InstallerProtocol}, Bundle: releasecontract.BundleComponent{Version: "0.1.0", CompatibleServer: ">=0.1.0 <0.1.1"}, Migrations: []releasecontract.Migration{}, ReleaseNotes: releasecontract.ReleaseNotes{Name: "notes_0.1.0.md", SHA256: acquisitionTestDigest([]byte("public notes"))}}
	for _, a := range []struct {
		kind, name, os, arch string
		body                 []byte
	}{{"bundle", "base_0.1.0.tar.gz", "any", "any", []byte("bundle")}, {"mcp_server", "server_0.1.0_windows_amd64.exe", "windows", "amd64", server}, {"runtime_pack", "runtime_0.1.0_windows_amd64.tar.gz", "windows", "amd64", []byte("expected assembled archive")}} {
		artifact := releasecontract.Artifact{Kind: a.kind, OS: a.os, Arch: a.arch, Name: a.name, Size: int64(len(a.body)), SHA256: acquisitionTestDigest(a.body), SignatureRef: a.name + ".sig"}
		if a.kind == "runtime_pack" {
			artifact.Adapter = "docling"
		} else {
			writeAcquisitionFixture(t, filepath.Join(o.ReleaseDirectory, a.name), a.body)
		}
		writeAcquisitionFixture(t, filepath.Join(o.ReleaseDirectory, artifact.SignatureRef), ed25519.Sign(key, a.body))
		m.Artifacts = append(m.Artifacts, artifact)
	}
	m.Delivery.ClientInputsSHA256, _ = m.ClientInputsDigest()
	body, _ := releasecontract.CanonicalJSON(m)
	if _, _, err = releaseverify.VerifyManifest(body, ed25519.Sign(key, body), releaseverify.StaticRegistry{"maestro/" + issuer.ID + "/" + issuer.KeyID: public}); err != nil {
		t.Fatal(err)
	}
	writeAcquisitionFixture(t, filepath.Join(o.ReleaseDirectory, releaseverify.ManifestName), body)
	writeAcquisitionFixture(t, filepath.Join(o.ReleaseDirectory, releaseverify.ManifestSignatureName), ed25519.Sign(key, body))
	writeAcquisitionFixture(t, filepath.Join(o.ReleaseDirectory, "notes_0.1.0.md"), []byte("public notes"))
	d, err := docling.DescribeProfile("windows", "amd64", docling.DocumentsProfile)
	if err != nil {
		t.Fatal(err)
	}
	db, _ := releasecontract.CanonicalJSON(d)
	total := d.Python.Size
	for _, i := range d.Inputs {
		total += i.Size
	}
	f := acquisitionFixture{o, acquisition.Recipe{SchemaVersion: 1, Purpose: acquisition.Purpose, Issuer: issuer, ManifestSHA256: acquisitionTestDigest(body), ServerSHA256: acquisitionTestDigest(server), OS: "windows", Arch: "amd64", Profile: d.Profile, DescriptorSHA256: acquisitionTestDigest(db), InputCount: len(d.Inputs) + 1, TotalInputBytes: total, RuntimeArtifact: m.Artifacts[2], IssuedAt: now.Add(-time.Minute).Format(time.RFC3339), ExpiresAt: now.Add(time.Hour).Format(time.RFC3339)}, key}
	f.sign(t)
	return f
}
func writeAcquisitionFixture(t *testing.T, name string, body []byte) {
	t.Helper()
	if err := os.WriteFile(name, body, 0600); err != nil {
		t.Fatal(err)
	}
}
func (f acquisitionFixture) sign(t *testing.T) {
	t.Helper()
	b, _ := releasecontract.CanonicalJSON(f.recipe)
	writeAcquisitionFixture(t, filepath.Join(f.options.RecipeDirectory, acquisition.RecipeName), b)
	writeAcquisitionFixture(t, filepath.Join(f.options.RecipeDirectory, acquisition.RecipeSignatureName), ed25519.Sign(f.key, b))
}

func acquisitionTestDigest(b []byte) string { return fmt.Sprintf("%x", sha256.Sum256(b)) }
func TestPilotAcquisitionReviewAndConsentPreservePrivateState(t *testing.T) {
	f := signedAcquisitionFixture(t)
	root := filepath.Dir(f.options.ReleaseDirectory)
	release, recipe := filepath.Join(root, "runtime-release"), filepath.Join(root, "runtime-recipe")
	if err := os.Rename(f.options.ReleaseDirectory, release); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(f.options.RecipeDirectory, recipe); err != nil {
		t.Fatal(err)
	}
	o := product.Options{ProductRoot: filepath.Join(root, "product"), UserHome: filepath.Join(root, "owner"), WorkspacePath: filepath.Join(root, "workspace"), CLIVersion: "0.1.0", AuthorityRegistrySHA256: f.options.RegistrySHA256, LookupEnv: func(string) (string, bool) { return "", false }}
	server := filepath.Join(o.ProductRoot, "server", "maestro-mcp.exe")
	o.AuthorityRegistryPath = filepath.Join(o.ProductRoot, "managed", "trust", "release-authority-registry.json")
	for _, d := range []string{filepath.Dir(server), filepath.Dir(o.AuthorityRegistryPath), o.UserHome} {
		if err := os.MkdirAll(d, 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Rename(f.options.ServerPath, server); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(f.options.RegistryPath, o.AuthorityRegistryPath); err != nil {
		t.Fatal(err)
	}
	home, err := datahome.InitializeFresh(datahome.Options{ProductRoot: o.ProductRoot, UserHome: o.UserHome, LookupEnv: o.LookupEnv})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = workspace.Initialize(workspace.Options{WorkspacePath: o.WorkspacePath, DataRoot: filepath.Join(home.Home, "state")}); err != nil {
		t.Fatal(err)
	}
	review, err := reviewPilotAcquisition(context.Background(), o, server, release)
	if err != nil {
		t.Fatal(err)
	}
	_, source, _, _ := runtime.Caller(0)
	sourceRoot := filepath.Clean(filepath.Join(filepath.Dir(source), "..", "..", "..", ".."))
	cmd := exec.Command("go", "run", "./dev/pilot/verify-carrier", root, o.AuthorityRegistryPath, filepath.Join(root, "builder-review-cache"))
	cmd.Dir = sourceRoot
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("actual development carrier verifier: %v %s", err, output)
	}
	if review.WorkspaceID == "" || len(review.Plan.Inputs) != 57 || review.Plan.MaximumAdditionalBytes <= 0 || review.SHA256 == "" {
		t.Fatal("incomplete finite acquisition review")
	}
	path := filepath.Join(root, "full-review.json")
	b, _ := json.Marshal(review)
	writeAcquisitionFixture(t, path, b)
	if _, err = applyPilotAcquisition(context.Background(), o, server, release, path, review.SHA256, false); err == nil {
		t.Fatal("missing consent admitted")
	}
	duplicate := append([]byte("{\"review_sha256\":\"ambiguous\","), b[1:]...)
	writeAcquisitionFixture(t, path, duplicate)
	if _, err = applyPilotAcquisition(context.Background(), o, server, release, path, review.SHA256, true); err == nil {
		t.Fatal("duplicate review field admitted")
	}
	changed := review
	changed.WorkspaceID = "different"
	b, _ = json.Marshal(changed)
	writeAcquisitionFixture(t, path, b)
	if _, err = applyPilotAcquisition(context.Background(), o, server, release, path, review.SHA256, true); err == nil {
		t.Fatal("changed review admitted")
	}
	if _, err = os.Lstat(review.CacheRoot); !os.IsNotExist(err) {
		t.Fatal("review/refusal created cache")
	}
	if _, err = os.Stat(server); err != nil {
		t.Fatal("prepared server lost")
	}
}
