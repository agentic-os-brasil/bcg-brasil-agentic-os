package product

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
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/datahome"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installer"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installtx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
)

// Only the test executable implements this synthetic identity response. The
// original qualified validator still executes and checks the real child image.
func init() {
	if len(os.Args) != 2 || os.Args[1] != "identity" {
		return
	}
	response := os.Getenv("MAESTRO_PRODUCT_TEST_SEED_IDENTITY")
	if response == "" {
		return
	}
	file, err := os.OpenFile(os.Getenv("MAESTRO_PRODUCT_TEST_SEED_IDENTITIES"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
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

type productSeedFixture struct {
	options         Options
	state           installtx.State
	now, expires    time.Time
	onClock         func()
	identityCounter string
}

func newProductSeedFixture(t *testing.T) *productSeedFixture {
	return newProductSeedFixtureWithSource(t, true)
}

func newProductSeedFixtureWithSource(t *testing.T, enrolled bool) *productSeedFixture {
	t.Helper()
	if runtime.GOOS != "darwin" && runtime.GOOS != "windows" {
		t.Skip("the installed experimental role admits only Mac and Windows")
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	prepareLocalReviewFixtureRoot(t, root)
	user, managed, work := filepath.Join(root, "owner"), filepath.Join(root, "product"), filepath.Join(root, "workspace")
	f := &productSeedFixture{now: time.Now().UTC().Truncate(time.Second)}
	f.expires = f.now.Add(time.Hour)
	write := func(path string, body []byte, mode os.FileMode) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, body, mode); err != nil {
			t.Fatal(err)
		}
	}
	digest := func(body []byte) string { return fmt.Sprintf("%x", sha256.Sum256(body)) }
	encode := func(value any) []byte {
		t.Helper()
		body, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		return body
	}
	public, private, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	registry := encode(map[string]any{"schema_version": 1, "product": "maestro", "authorities": []any{map[string]any{"issuer": "maestro-engineering-pilot-20261122", "key_id": "pilot-20261122", "algorithm": "ed25519", "public_key": base64.StdEncoding.EncodeToString(public), "status": "active", "valid_from": f.now.Add(-time.Hour).Format(time.RFC3339), "valid_until": f.now.Add(2 * time.Hour).Format(time.RFC3339)}}})
	write(filepath.Join(managed, "trust", "release-authority-registry.json"), registry, 0600)
	resources := []releasecontract.EmbeddedResource{{URI: "bcgos://synthetic/product-validation", Bytes: 1, SHA256: strings.Repeat("a", 64)}}
	embedded, err := releasecontract.EmbeddedDigest(resources)
	if err != nil {
		t.Fatal(err)
	}
	identity := releasecontract.ServerIdentity{SchemaVersion: 2, Product: "maestro", Component: "mcp_server", Protocol: releasecontract.ServerProtocol, Version: "0.2.0", CompatibleBundle: ">=0.0.0 <1.0.0", AuthorityRegistrySHA256: digest(registry), Resources: resources, EmbeddedInputsSHA256: embedded, ExecutionContract: releasecontract.SyncedSourceContract}
	f.identityCounter = filepath.Join(root, "identity-calls")
	t.Setenv("MAESTRO_PRODUCT_TEST_SEED_IDENTITY", string(encode(identity)))
	t.Setenv("MAESTRO_PRODUCT_TEST_SEED_IDENTITIES", f.identityCounter)
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	executable, err := os.ReadFile(self)
	if err != nil {
		t.Fatal(err)
	}
	serverName, installerName := "maestro-mcp", "maestro-install"
	if runtime.GOOS == "windows" {
		serverName += ".exe"
		installerName += ".exe"
	}
	write(filepath.Join(managed, "bin", serverName), executable, 0700)
	installerBody := []byte("synthetic installed installer bytes; never executed")
	write(filepath.Join(managed, installerName), installerBody, 0700)
	var archive bytes.Buffer
	gz := gzip.NewWriter(&archive)
	tw := tar.NewWriter(gz)
	if err = tw.WriteHeader(&tar.Header{Name: "fixture.md", Mode: 0600, Size: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err = tw.Write([]byte("x")); err != nil {
		t.Fatal(err)
	}
	if err = tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err = gz.Close(); err != nil {
		t.Fatal(err)
	}
	data := filepath.Join(user, ".maestro-install")
	archivePath := filepath.Join(data, "updates", "fixture.tar.gz")
	write(archivePath, archive.Bytes(), 0600)
	write(filepath.Join(managed, "bundles", "0.2.0", "fixture.md"), []byte("x"), 0600)
	manifest := releasecontract.Manifest{SchemaVersion: 2, Product: "maestro", Release: "0.2.0", Channel: "canary", Issuer: releasecontract.Issuer{ID: "maestro-engineering-pilot-20261122", KeyID: "pilot-20261122"}, Server: releasecontract.ServerComponent{Version: identity.Version, Protocol: identity.Protocol, ExecutionContract: identity.ExecutionContract, CompatibleBundle: identity.CompatibleBundle, EmbeddedInputsSHA256: embedded}, Bundle: releasecontract.BundleComponent{Version: "0.2.0", CompatibleServer: ">=0.0.0 <1.0.0"}, Delivery: releasecontract.DeliveryComponent{Profile: releasecontract.ModernProfile, InstallerVersion: "0.2.0", InstallerProtocol: releasecontract.InstallerProtocol}, Artifacts: []releasecontract.Artifact{{Kind: "mcp_server", OS: runtime.GOOS, Arch: runtime.GOARCH, Name: "maestro-mcp_0.2.0_" + runtime.GOOS + "_" + runtime.GOARCH, Size: int64(len(executable)), SHA256: digest(executable), SignatureRef: "server_0.2.0.sig"}, {Kind: "bundle", OS: "any", Arch: "any", Name: "maestro-base_0.2.0.tar.gz", Size: int64(archive.Len()), SHA256: digest(archive.Bytes()), SignatureRef: "bundle_0.2.0.sig"}}, Migrations: []releasecontract.Migration{}, ReleaseNotes: releasecontract.ReleaseNotes{Name: "notes_0.2.0.md", SHA256: strings.Repeat("c", 64)}}
	manifest.Delivery.ClientInputsSHA256, err = manifest.ClientInputsDigest()
	if err != nil {
		t.Fatal(err)
	}
	manifestBody := encode(manifest)
	binding := installtx.ReleaseBinding{ManifestSHA256: digest(manifestBody), ClientInputsSHA256: manifest.Delivery.ClientInputsSHA256, ManifestJSON: string(manifestBody), ManifestSignature: base64.StdEncoding.EncodeToString(ed25519.Sign(private, manifestBody)), BundleSHA256: manifest.Artifacts[1].SHA256, BundleSize: int64(archive.Len()), BundleArchive: archivePath}
	generation := releasecontract.Generation{SchemaVersion: 1, Product: "maestro", Protocol: releasecontract.GenerationProtocol, Issuer: manifest.Issuer, Source: releasecontract.SourceTuple{SourceID: "synthetic", Channel: "canary", OS: runtime.GOOS, Arch: runtime.GOARCH}, Sequence: 1, PublishedAt: f.now.Add(-time.Hour).Format(time.RFC3339), ExpiresAt: f.expires.Format(time.RFC3339), Release: manifest.Release, ManifestSHA256: binding.ManifestSHA256, ClientInputsSHA256: binding.ClientInputsSHA256, Recalls: []releasecontract.ReleaseRecall{}, MinimumReaderContract: releasecontract.SyncedSourceContract}
	body := encode(generation)
	f.state = installtx.State{SchemaVersion: 4, Profile: releasecontract.ModernProfile, ManagedRoot: managed, Release: manifest.Release, Channel: manifest.Channel, CLIVersion: manifest.Release, BundleVersion: manifest.Bundle.Version, TargetOS: runtime.GOOS, TargetArch: runtime.GOARCH, ActivatedAt: f.now, Server: installtx.ExecutableIdentity{ExecutionContract: identity.ExecutionContract, Version: identity.Version, Protocol: identity.Protocol, CompatibleBundle: identity.CompatibleBundle, CompatibleServer: manifest.Bundle.CompatibleServer, EmbeddedInputsSHA256: embedded, ArtifactName: manifest.Artifacts[0].Name, SHA256: manifest.Artifacts[0].SHA256, Size: int64(len(executable))}, Binding: binding, Source: &installtx.SourceControl{Locator: filepath.Join(data, "source"), SourceBinding: installtx.SourceBinding{Tuple: generation.Source, Sequence: generation.Sequence, GenerationSHA256: digest(body)}, Body: body, Signature: ed25519.Sign(private, body), Recalls: []releasecontract.ReleaseRecall{}, MinimumReaderContract: generation.MinimumReaderContract, ObservedAt: f.now}}
	if !enrolled {
		f.state.Source = nil // A fresh installation, never a rewind of enrolled custody.
	}
	if err = installtx.WriteState(data, f.state); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MAESTRO_HOME", filepath.Join(root, "brain"))
	home, err := datahome.InitializeFresh(datahome.Options{ProductRoot: managed, UserHome: user})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = workspace.Initialize(workspace.Options{WorkspacePath: work, DataRoot: filepath.Join(home.Home, "state")}); err != nil {
		t.Fatal(err)
	}
	role := installer.ExperimentalSeedRecord{SchemaVersion: 1, Product: "maestro", Purpose: installer.ExperimentalSeedPurpose, Protocol: installer.ExperimentalSeedProtocol, Profile: installer.ExperimentalSeedProfile, RegistrySHA256: digest(registry), InstallerVersion: "0.2.0", InstallerSHA256: digest(installerBody), TargetOS: runtime.GOOS, TargetArch: runtime.GOARCH, Source: generation.Source, Issuer: manifest.Issuer.ID, KeyID: manifest.Issuer.KeyID, NotBefore: f.now.Add(-time.Hour).Format(time.RFC3339), ExpiresAt: f.expires.Format(time.RFC3339), Releases: []installer.ExperimentalSeedRelease{{Release: "0.2.0", ManifestSHA256: binding.ManifestSHA256, ClientInputsSHA256: binding.ClientInputsSHA256, ServerSHA256: manifest.Artifacts[0].SHA256, BundleSHA256: manifest.Artifacts[1].SHA256, RuntimeSHA256: strings.Repeat("d", 64)}, {Release: "0.2.1", ManifestSHA256: strings.Repeat("1", 64), ClientInputsSHA256: strings.Repeat("2", 64), ServerSHA256: strings.Repeat("3", 64), BundleSHA256: strings.Repeat("4", 64), RuntimeSHA256: strings.Repeat("5", 64)}}}
	roleBody := encode(role)
	signature := ed25519.Sign(private, roleBody)
	// The original role clock can advance or mutate real fixture inputs after
	// the identity child acknowledges its external operation. No private call
	// names or verifier substitutes participate in admission.
	clock := func() time.Time {
		if f.onClock != nil {
			f.onClock()
		}
		return f.now
	}
	verified, err := installer.VerifyExperimentalSeedAdmission(roleBody, signature, registry, role.RegistrySHA256, clock)
	if err != nil {
		t.Fatal(err)
	}
	write(filepath.Join(managed, "trust", installer.ExperimentalSeedAdmissionName), roleBody, 0600)
	write(filepath.Join(managed, "trust", installer.ExperimentalSeedAdmissionName+".sig"), signature, 0600)
	i := verified.Identity()
	origin := map[string]any{"schema_version": 1, "profile": installer.ExperimentalSeedProfile, "evidence": "engineering_only", "admission_sha256": i.AdmissionSHA256, "installer_sha256": i.InstallerSHA256, "registry_sha256": i.RegistrySHA256, "user_home": user, "managed_root": managed, "data_root": data, "brain_home": home.Home, "review_sha256": strings.Repeat("6", 64)}
	write(filepath.Join(managed, "trust", "experimental-installed-origin.json"), encode(origin), 0600)
	f.options = Options{ProductRoot: managed, UserHome: user, WorkspacePath: work, ExperimentalSeed: verified, ExperimentalSeedRoot: managed, AuthorityRegistrySHA256: role.RegistrySHA256, CLIVersion: manifest.Release, InstalledManifestSHA256: binding.ManifestSHA256, InstalledClientInputsSHA256: binding.ClientInputsSHA256}
	if !enrolled {
		f.options.InstalledManifestSHA256, f.options.InstalledClientInputsSHA256 = "", ""
	}
	return f
}

func TestProductValidationPreEnrollmentRetainsPostReadFence(t *testing.T) {
	for _, kind := range []string{"unchanged", "installer", "origin"} {
		t.Run(kind, func(t *testing.T) {
			f := newProductSeedFixtureWithSource(t, false)
			b, err := Open(f.options)
			if err != nil {
				t.Fatal("valid pre-enrollment installation refused", err)
			}
			changed := false
			read := func(dataRoot, managedRoot string) (installtx.State, error) {
				state, err := installtx.ReadSourceState(dataRoot, managedRoot)
				if err != nil {
					return state, err
				}
				if kind != "unchanged" {
					changed = true
					name := "maestro-install"
					if runtime.GOOS == "windows" {
						name += ".exe"
					}
					path := filepath.Join(f.options.ProductRoot, name)
					body := []byte("changed during pre-enrollment classification")
					if kind == "origin" {
						path = filepath.Join(f.options.ProductRoot, "trust", "experimental-installed-origin.json")
						original, err := os.ReadFile(path)
						if err != nil {
							t.Fatal(err)
						}
						var origin map[string]any
						if err := json.Unmarshal(original, &origin); err != nil {
							t.Fatal(err)
						}
						origin["brain_home"] = filepath.Join(f.options.UserHome, "other-brain")
						body, err = json.Marshal(origin)
						if err != nil {
							t.Fatal(err)
						}
					}
					if err := os.WriteFile(path, body, 0600); err != nil {
						t.Fatal(err)
					}
				}
				return state, nil
			}
			// Same existing typed-reader seam used by source classification. The
			// real full entry fence precedes it, exactly as in validateContext.
			if err := b.validateExperimentalSeedContext(t.Context()); err != nil {
				t.Fatal(err)
			}
			err = b.validateInstalledSourceAfterExperimentalCheckContext(t.Context(), read)
			if kind == "unchanged" {
				if err != nil {
					t.Fatal("unchanged pre-enrollment source classification refused", err)
				}
			} else if !changed || err == nil {
				t.Fatalf("classification-time change admitted or not exercised: changed=%t err=%v", changed, err)
			}
			if _, err := os.Lstat(f.identityCounter); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("pre-enrollment incorrectly entered qualified source execution", err)
			}
		})
	}
}

func TestProductValidationDeliversPromptContextThroughIndependentAdmission(t *testing.T) {
	f := newProductSeedFixture(t)
	b, err := Open(f.options)
	if err != nil {
		t.Fatal("valid original installed fixture refused", err)
	}
	calls, err := os.ReadFile(f.identityCounter)
	if err != nil || strings.Count(string(calls), "identity\n") != 1 {
		t.Fatalf("independent full qualified validation changed: %q %v", calls, err)
	}
	input := fmt.Sprintf(`{"session_id":"bounded-installer-work","prompt_id":"first-turn","cwd":%q,"hook_event_name":"UserPromptSubmit"}`, f.options.WorkspacePath)
	var output bytes.Buffer
	if err := b.HandleClaudeHook("UserPromptSubmit", strings.NewReader(input), &output); err != nil {
		t.Fatal("real prompt hook refused", err)
	}
	calls, err = os.ReadFile(f.identityCounter)
	if err != nil || strings.Count(string(calls), "identity\n") != 3 {
		t.Fatalf("Open/Handle/lifecycle full qualified validations were collapsed: %q %v", calls, err)
	}
	if !json.Valid(output.Bytes()) || !strings.Contains(output.String(), `\"event\":\"context_inject\"`) {
		t.Fatalf("ordinary hook context missing: %s", output.String())
	}
}

func TestProductValidationRechecksExperimentalInputsAfterQualifiedWork(t *testing.T) {
	for _, stage := range []string{"entry", "after-qualified"} {
		for _, kind := range []string{"installer", "origin", "role", "registry", "server-version", "role-expiry", "cancellation"} {
			t.Run(stage+"/"+kind, func(t *testing.T) {
				f := newProductSeedFixture(t)
				if _, err := Open(f.options); err != nil {
					t.Fatal("unchanged fixture must pass before changing its inputs", err)
				}
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				changed := false
				change := func() {
					changed = true
					if kind == "role-expiry" {
						f.now = f.expires // Exclusive expiry, reached during real work.
						return
					}
					if kind == "cancellation" {
						cancel()
						return
					}
					if kind == "server-version" {
						state := f.state
						state.Server.Version, state.CLIVersion = "0.2.1", "0.2.1"
						data := filepath.Join(f.options.UserHome, ".maestro-install")
						if err := installtx.WriteState(data, state); err != nil {
							t.Fatal(err)
						}
						if _, err := installtx.ReadSourceState(data, f.options.ProductRoot); err != nil {
							t.Fatal("changed state must retain canonical shape", err)
						}
						return
					}
					name := map[string]string{"installer": "../maestro-install", "origin": "experimental-installed-origin.json", "role": installer.ExperimentalSeedAdmissionName, "registry": "release-authority-registry.json"}[kind]
					if runtime.GOOS == "windows" && kind == "installer" {
						name += ".exe"
					}
					path := filepath.Join(f.options.ProductRoot, "trust", name)
					body, err := os.ReadFile(path)
					if err != nil || len(body) == 0 {
						t.Fatal("missing actual fixture input", err)
					}
					if kind == "origin" {
						var origin map[string]any
						if err := json.Unmarshal(body, &origin); err != nil {
							t.Fatal(err)
						}
						origin["brain_home"] = filepath.Join(f.options.UserHome, "other-brain")
						body, err = json.Marshal(origin)
						if err != nil {
							t.Fatal(err)
						}
					} else {
						body[len(body)-1] ^= 1
					}
					if err := os.WriteFile(path, body, 0600); err != nil {
						t.Fatal(err)
					}
				}
				if stage == "entry" {
					change()
				} else {
					f.onClock = func() {
						calls, err := os.ReadFile(f.identityCounter)
						if err != nil {
							t.Fatal(err)
						}
						// One acknowledgement belongs to the positive Open above.
						// The next is the actual current identity child for this call.
						if strings.Count(string(calls), "identity\n") < 2 {
							return
						}
						f.onClock = nil
						change()
					}
				}
				if _, err := OpenContext(ctx, f.options); err == nil {
					t.Fatal("changed experimental input admitted")
				} else if kind == "cancellation" && !errors.Is(err, context.Canceled) {
					t.Fatal("cancellation was hidden", err)
				}
				if !changed {
					t.Fatal("negative fixture did not reach its requested change")
				}
				calls, err := os.ReadFile(f.identityCounter)
				wantIdentities := 1 // The unchanged positive precondition above.
				if stage == "after-qualified" {
					wantIdentities++
				}
				if err != nil || strings.Count(string(calls), "identity\n") != wantIdentities {
					t.Fatalf("change did not occur on its intended side of real qualified work: %q %v", calls, err)
				}
			})
		}
	}
}

func TestIndependentInstalledSourceReaderRetainsFreshExperimentalAdmission(t *testing.T) {
	f := newProductSeedFixture(t)
	b, err := Open(f.options)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.validateInstalledSourceContext(t.Context()); err != nil {
		t.Fatal(err)
	}
	name := "maestro-install"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	if err := os.WriteFile(filepath.Join(f.options.ProductRoot, name), []byte("changed installer after Open"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := b.validateInstalledSourceContext(t.Context()); err == nil {
		t.Fatal("independent source reader accepted replaced installer after Open")
	}
	if err := b.validateContext(t.Context()); err == nil {
		t.Fatal("a later complete validation reused an earlier admission")
	}
}
