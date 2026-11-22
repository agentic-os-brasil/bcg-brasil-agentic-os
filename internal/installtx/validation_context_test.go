package installtx

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
)

type cancelInstallDigestWriter struct {
	cancel context.CancelFunc
	bytes  int
}

func (w *cancelInstallDigestWriter) Write(p []byte) (int, error) {
	w.bytes += len(p)
	w.cancel()
	return len(p), nil
}

func TestInstalledValidationHashHonorsMidReadCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	body := bytes.Repeat([]byte("authenticated input"), 1<<14)
	sink := &cancelInstallDigestWriter{cancel: cancel}
	n, err := io.Copy(sink, installReadContext{ctx, bytes.NewReader(body)})
	if !errors.Is(err, context.Canceled) || n <= 0 || n >= int64(len(body)) {
		t.Fatalf("hash continued after cancellation or bypassed Read: n=%d err=%v", n, err)
	}
}

func TestCurrentValidationHonorsAlreadyCancelledCaller(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := ValidateQualifiedLaunchContext(ctx, "not-opened", "not-opened", ReleaseBinding{}, time.Now()); !errors.Is(err, context.Canceled) {
		t.Fatalf("qualified validation ignored caller: %v", err)
	}
	if _, err := ValidateActivatedCurrentContext(ctx, "not-opened", "not-opened", ReleaseBinding{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("activated validation ignored caller: %v", err)
	}
	if err := CheckActivatedServerContext(ctx, "not-opened", State{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("identity validation ignored caller: %v", err)
	}
}

// Test-binary protocol fixture only. The child really executes the bounded
// identity command; no production entrypoint or trust override is introduced.
// Its signed synthetic source tests do not create a NativeHost or seed role.
func init() {
	if len(os.Args) != 2 || os.Args[1] != "identity" {
		return
	}
	response := os.Getenv("MAESTRO_INSTALLTX_TEST_IDENTITY_RESPONSE")
	if response == "" {
		return
	}
	counter := os.Getenv("MAESTRO_INSTALLTX_TEST_IDENTITY_COUNTER")
	file, err := os.OpenFile(counter, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		os.Exit(91)
	}
	if _, err = file.WriteString("identity\n"); err != nil {
		os.Exit(92)
	}
	if err = file.Close(); err != nil {
		os.Exit(93)
	}
	fmt.Print(response)
	os.Exit(0)
}

func qualifiedValidationIdentityFixture(t *testing.T) (string, string, State, string) {
	t.Helper()
	managed, data := t.TempDir(), t.TempDir()
	now := time.Now().UTC().Truncate(time.Second)
	public, private, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	document := map[string]any{"schema_version": 1, "product": "maestro", "authorities": []any{map[string]any{"issuer": "engineering", "key_id": "fixture", "algorithm": "ed25519", "public_key": base64.StdEncoding.EncodeToString(public), "status": "active", "valid_from": now.Add(-2 * time.Hour).Format(time.RFC3339), "valid_until": now.Add(2 * time.Hour).Format(time.RFC3339)}}}
	registry, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(filepath.Join(managed, "trust"), 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(managed, "trust", "release-authority-registry.json"), registry, 0600); err != nil {
		t.Fatal(err)
	}
	resources := []releasecontract.EmbeddedResource{{URI: "bcgos://synthetic/source-validation", Bytes: 1, SHA256: strings.Repeat("a", 64)}}
	embedded, err := releasecontract.EmbeddedDigest(resources)
	if err != nil {
		t.Fatal(err)
	}
	identity := releasecontract.ServerIdentity{SchemaVersion: 2, Product: "maestro", Component: "mcp_server", Protocol: releasecontract.ServerProtocol, Version: "0.2.0", CompatibleBundle: ">=0.0.0 <1.0.0", AuthorityRegistrySHA256: fmt.Sprintf("%x", sha256.Sum256(registry)), Resources: resources, EmbeddedInputsSHA256: embedded, ExecutionContract: releasecontract.SyncedSourceContract}
	response, err := json.Marshal(identity)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("MAESTRO_INSTALLTX_TEST_IDENTITY_RESPONSE", string(response))
	counter := filepath.Join(t.TempDir(), "identity-calls")
	t.Setenv("MAESTRO_INSTALLTX_TEST_IDENTITY_COUNTER", counter)
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	executable, err := os.ReadFile(self)
	if err != nil {
		t.Fatal(err)
	}
	serverPath := filepath.Join(managed, "bin", executableForProfile(runtime.GOOS, releasecontract.ModernProfile))
	if err = os.MkdirAll(filepath.Dir(serverPath), 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(serverPath, executable, 0700); err != nil {
		t.Fatal(err)
	}
	var archive bytes.Buffer
	gzipWriter := gzip.NewWriter(&archive)
	tarWriter := tar.NewWriter(gzipWriter)
	if err = tarWriter.WriteHeader(&tar.Header{Name: "fixture.md", Mode: 0600, Size: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err = tarWriter.Write([]byte("x")); err != nil {
		t.Fatal(err)
	}
	if err = tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err = gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	archivePath := filepath.Join(data, "updates", "bundle.tar.gz")
	if err = os.MkdirAll(filepath.Dir(archivePath), 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(archivePath, archive.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	bundleRoot := filepath.Join(managed, "bundles", "0.2.0")
	if err = os.MkdirAll(bundleRoot, 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(bundleRoot, "fixture.md"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	manifest := releasecontract.Manifest{SchemaVersion: 2, Product: "maestro", Release: "0.2.0", Channel: "canary", Issuer: releasecontract.Issuer{ID: "engineering", KeyID: "fixture"}, Server: releasecontract.ServerComponent{Version: "0.2.0", Protocol: identity.Protocol, ExecutionContract: identity.ExecutionContract, CompatibleBundle: identity.CompatibleBundle, EmbeddedInputsSHA256: embedded}, Bundle: releasecontract.BundleComponent{Version: "0.2.0", CompatibleServer: ">=0.0.0 <1.0.0"}, Delivery: releasecontract.DeliveryComponent{Profile: releasecontract.ModernProfile, InstallerVersion: "0.2.0", InstallerProtocol: releasecontract.InstallerProtocol}, Artifacts: []releasecontract.Artifact{{Kind: "mcp_server", OS: runtime.GOOS, Arch: runtime.GOARCH, Name: "maestro-mcp_0.2.0_" + runtime.GOOS + "_" + runtime.GOARCH, Size: int64(len(executable)), SHA256: fmt.Sprintf("%x", sha256.Sum256(executable)), SignatureRef: "server_0.2.0.sig"}, {Kind: "bundle", OS: "any", Arch: "any", Name: "maestro-base_0.2.0.tar.gz", Size: int64(archive.Len()), SHA256: fmt.Sprintf("%x", sha256.Sum256(archive.Bytes())), SignatureRef: "bundle_0.2.0.sig"}}, Migrations: []releasecontract.Migration{}, ReleaseNotes: releasecontract.ReleaseNotes{Name: "notes_0.2.0.md", SHA256: strings.Repeat("c", 64)}}
	manifest.Delivery.ClientInputsSHA256, err = manifest.ClientInputsDigest()
	if err != nil {
		t.Fatal(err)
	}
	manifestBody, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	binding := ReleaseBinding{ManifestSHA256: fmt.Sprintf("%x", sha256.Sum256(manifestBody)), ClientInputsSHA256: manifest.Delivery.ClientInputsSHA256, ManifestJSON: string(manifestBody), ManifestSignature: base64.StdEncoding.EncodeToString(ed25519.Sign(private, manifestBody)), BundleSHA256: manifest.Artifacts[1].SHA256, BundleSize: int64(archive.Len()), BundleArchive: archivePath}
	generation := releasecontract.Generation{SchemaVersion: 1, Product: "maestro", Protocol: releasecontract.GenerationProtocol, Issuer: manifest.Issuer, Source: releasecontract.SourceTuple{SourceID: "synthetic", Channel: "canary", OS: runtime.GOOS, Arch: runtime.GOARCH}, Sequence: 1, PublishedAt: now.Add(-time.Hour).Format(time.RFC3339), ExpiresAt: now.Add(time.Hour).Format(time.RFC3339), Release: manifest.Release, ManifestSHA256: binding.ManifestSHA256, ClientInputsSHA256: binding.ClientInputsSHA256, Recalls: []releasecontract.ReleaseRecall{}, MinimumReaderContract: releasecontract.SyncedSourceContract}
	body, err := json.Marshal(generation)
	if err != nil {
		t.Fatal(err)
	}
	state := State{SchemaVersion: 4, Profile: releasecontract.ModernProfile, ManagedRoot: managed, Release: manifest.Release, Channel: manifest.Channel, CLIVersion: manifest.Release, BundleVersion: manifest.Bundle.Version, TargetOS: runtime.GOOS, TargetArch: runtime.GOARCH, ActivatedAt: now, Server: serverForManifest(manifest, manifest.Artifacts[0]), Binding: binding, Source: &SourceControl{Locator: filepath.Join(data, "source"), SourceBinding: SourceBinding{Tuple: generation.Source, Sequence: generation.Sequence, GenerationSHA256: fmt.Sprintf("%x", sha256.Sum256(body))}, Body: body, Signature: ed25519.Sign(private, body), Recalls: []releasecontract.ReleaseRecall{}, MinimumReaderContract: generation.MinimumReaderContract, ObservedAt: now}}
	if err = WriteState(data, state); err != nil {
		t.Fatal(err)
	}
	return managed, data, state, counter
}

func TestQualifiedValidationUsesOneActualIdentityObservation(t *testing.T) {
	managed, data, state, counter := qualifiedValidationIdentityFixture(t)
	if _, err := ValidateQualifiedLaunchContext(t.Context(), managed, data, state.Binding, time.Now()); err != nil {
		t.Fatal(err)
	}
	calls, err := os.ReadFile(counter)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(string(calls), "identity\n"); got != 1 {
		t.Fatalf("full validation identity calls=%d, want one actual observation per operation", got)
	}
}

func TestQualifiedValidationRetainsIdentityAndArtifactRefusals(t *testing.T) {
	for _, name := range []string{"version", "protocol", "execution-contract", "bundle-compatibility", "embedded-inventory", "registry-pin", "server-content", "registry-content", "archive-content", "installed-tree", "manifest-pin", "client-input-pin"} {
		t.Run(name, func(t *testing.T) {
			managed, data, state, _ := qualifiedValidationIdentityFixture(t)
			var identity releasecontract.ServerIdentity
			if err := json.Unmarshal([]byte(os.Getenv("MAESTRO_INSTALLTX_TEST_IDENTITY_RESPONSE")), &identity); err != nil {
				t.Fatal(err)
			}
			expected := state.Binding
			flip := func(path string) {
				t.Helper()
				body, err := os.ReadFile(path)
				if err != nil || len(body) == 0 {
					t.Fatalf("fixture read: %v", err)
				}
				body[len(body)-1] ^= 1
				if err = os.WriteFile(path, body, 0700); err != nil {
					t.Fatal(err)
				}
			}
			switch name {
			case "version":
				identity.Version = "0.3.0"
			case "protocol":
				identity.Protocol = "foreign"
			case "execution-contract":
				identity.SchemaVersion = 1
				identity.ExecutionContract = ""
			case "bundle-compatibility":
				identity.CompatibleBundle = ">=0.2.0 <0.3.0"
			case "embedded-inventory":
				identity.Resources[0].Bytes++
				var err error
				identity.EmbeddedInputsSHA256, err = releasecontract.EmbeddedDigest(identity.Resources)
				if err != nil {
					t.Fatal(err)
				}
			case "registry-pin":
				identity.AuthorityRegistrySHA256 = strings.Repeat("f", 64)
			case "server-content":
				flip(filepath.Join(managed, "bin", executableForProfile(runtime.GOOS, releasecontract.ModernProfile)))
			case "registry-content":
				flip(filepath.Join(managed, "trust", "release-authority-registry.json"))
			case "archive-content":
				flip(state.Binding.BundleArchive)
			case "installed-tree":
				flip(filepath.Join(managed, "bundles", state.BundleVersion, "fixture.md"))
			case "manifest-pin":
				expected.ManifestSHA256 = strings.Repeat("f", 64)
			case "client-input-pin":
				expected.ClientInputsSHA256 = strings.Repeat("f", 64)
			}
			response, err := json.Marshal(identity)
			if err != nil {
				t.Fatal(err)
			}
			t.Setenv("MAESTRO_INSTALLTX_TEST_IDENTITY_RESPONSE", string(response))
			if _, err = ValidateQualifiedLaunchContext(t.Context(), managed, data, expected, time.Now()); err == nil {
				t.Fatal("changed actual identity or authenticated artifact admitted")
			}
		})
	}
}

func TestQualifiedValidationDoesNotFreezeTimeAcrossAssets(t *testing.T) {
	managed, data, state, _ := qualifiedValidationIdentityFixture(t)
	var generation releasecontract.Generation
	if err := json.Unmarshal(state.Source.Body, &generation); err != nil {
		t.Fatal(err)
	}
	expires, err := time.Parse(time.RFC3339, generation.ExpiresAt)
	if err != nil {
		t.Fatal(err)
	}
	// The existing public clock argument models the observation at entry. A real
	// identity subprocess and archive/tree walk consume more than one nanosecond;
	// the late fence must advance that anchor rather than reuse its frozen value.
	if _, err = ValidateQualifiedLaunchContext(t.Context(), managed, data, state.Binding, expires.Add(-time.Nanosecond)); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("source expiry during actual asset validation escaped late check: %v", err)
	}
}
