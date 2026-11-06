package product_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp/setup"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/dev/releasepack"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installer"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseverify"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/updateservice"
)

const contentFixtureVersion = "0.1.0"

func contentFileSHA(t *testing.T, path string) string {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	hash := sha256.New()
	if _, err = io.Copy(hash, file); err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("%x", hash.Sum(nil))
}

type contentManagedEntry struct {
	Mode os.FileMode
	Body string
}

// Keep directories and exact file bytes/modes, including the managed root itself.
func contentManagedTree(t *testing.T, root string) map[string]contentManagedEntry {
	t.Helper()
	tree := map[string]contentManagedEntry{}
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.IsDir() && !info.Mode().IsRegular() {
			return errors.New("managed snapshot requires only directories and regular files")
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		item := contentManagedEntry{Mode: info.Mode()}
		if !info.IsDir() {
			body, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			item.Body = string(body)
		}
		tree[rel] = item
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return tree
}

func contentFixtureRoot(t *testing.T) string {
	t.Helper()
	root := physicalTempDir(t)
	t.Cleanup(func() {
		_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
			if err == nil && entry.IsDir() {
				_ = os.Chmod(path, 0700)
			}
			return err
		})
	})
	return root
}

// Authenticate the historical authority/pack independently before wrapping the
// unchanged archive in a fresh engineering release. Retained inputs stay intact.
func retainedContentPack(t *testing.T, retained, seed string) (string, string) {
	t.Helper()
	authorityPath := filepath.Join(retained, "authority.json")
	authority := readContentFile(t, authorityPath)
	if fmt.Sprintf("%x", sha256.Sum256(authority)) != seed {
		t.Fatal("independent authority seed mismatch")
	}
	registry, err := releaseverify.ParseAuthorityRegistry(bytes.NewReader(authority), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(retained, "state/ingestion/docling")
	manifestPath := filepath.Join(base, "release-manifest.json")
	manifest, key, err := releaseverify.VerifyManifest(readContentFile(t, manifestPath), readContentFile(t, manifestPath+".sig"), registry)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.SchemaVersion != 1 {
		t.Fatal("historical Docling fixture is not the retained v1 release")
	}
	for _, artifact := range manifest.Artifacts {
		if artifact.Kind != "runtime_pack" || artifact.OS != runtime.GOOS || artifact.Arch != runtime.GOARCH {
			continue
		}
		pack := filepath.Join(base, artifact.Name)
		body := readContentFile(t, pack)
		packSHA := fmt.Sprintf("%x", sha256.Sum256(body))
		if packSHA != "e7cdd0f0f1506878ce3a9390f54798f6a51a288651e8e73a4b836dd67f43333a" || packSHA != artifact.SHA256 || int64(len(body)) != artifact.Size || !ed25519.Verify(key, body, readContentFile(t, filepath.Join(base, artifact.SignatureRef))) {
			t.Fatal("historical signed Docling archive differs from retained root attestation")
		}
		inputs := []string{authorityPath, manifestPath, manifestPath + ".sig", pack, filepath.Join(base, artifact.SignatureRef), filepath.Join(base, "installed", packSHA, "pack.json"), filepath.Join(retained, "workspace/sample.docx")}
		before := map[string]string{}
		for _, path := range inputs {
			before[path] = contentFileSHA(t, path)
		}
		t.Cleanup(func() {
			for path, want := range before {
				if contentFileSHA(t, path) != want {
					t.Errorf("historical fixture input changed: %s", filepath.Base(path))
				}
			}
		})
		t.Logf("independently authenticated retained Docling archive sha256=%s; fresh engineering release preserves these exact bytes", packSHA)
		return pack, packSHA
	}
	t.Fatal("fixture has no concrete signed Docling runtime")
	return "", ""
}

type qualifiedContentRelease struct {
	factory                      releasepack.ModernCandidate
	release, registrySHA, source string
	registry                     releaseverify.KeyRegistry
}

func newQualifiedContentRelease(t *testing.T, pack string) qualifiedContentRelease {
	t.Helper()
	if runtime.GOOS != "darwin" && runtime.GOOS != "windows" {
		t.Skip("actual supported native factory runner required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	root := contentFixtureRoot(t)
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	registryBody, err := json.Marshal(map[string]any{"schema_version": 1, "product": "maestro", "authorities": []any{map[string]any{"issuer": "content-engineering-fixture", "key_id": "ephemeral", "algorithm": "ed25519", "public_key": base64.StdEncoding.EncodeToString(public), "status": "active", "valid_from": "2020-01-01T00:00:00Z", "valid_until": "2100-01-01T00:00:00Z"}}})
	if err != nil {
		t.Fatal(err)
	}
	registryPath := filepath.Join(root, "authority.json")
	putContentFile(t, registryPath, registryBody)
	source, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	factory, err := releasepack.BuildModernCandidate(ctx, releasepack.ModernOptions{Root: source, Output: filepath.Join(root, "factory"), Version: contentFixtureVersion, InstallerVersion: "0.3.7", Channel: "canary", AuthorityRegistry: registryPath, Target: releasepack.Target{OS: runtime.GOOS, Arch: runtime.GOARCH}, DoclingPack: pack})
	if err != nil {
		t.Fatal(err)
	}
	release := filepath.Join(root, "signed-release")
	manifest, err := releasepack.SignCandidate(releasepack.SignCandidateOptions{Candidate: factory.Directory, Output: release, Registry: registryPath, Issuer: "content-engineering-fixture", KeyID: "ephemeral", PrivateKey: private})
	if err != nil {
		t.Fatal(err)
	}
	registry, err := releaseverify.LoadAuthorityRegistry(registryPath, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	verified, err := releaseverify.VerifyDirectory(release, registry)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	generation := releasecontract.Generation{SchemaVersion: 1, Product: "maestro", Protocol: releasecontract.GenerationProtocol, Issuer: manifest.Issuer, Source: releasecontract.SourceTuple{SourceID: "content-qualified-fixture", Channel: manifest.Channel, OS: runtime.GOOS, Arch: runtime.GOARCH}, Sequence: 1, PublishedAt: now.Add(-time.Minute).Format(time.RFC3339), ExpiresAt: now.Add(7 * 24 * time.Hour).Format(time.RFC3339), Release: manifest.Release, ManifestSHA256: verified.ManifestSHA256, ClientInputsSHA256: manifest.Delivery.ClientInputsSHA256, Recalls: []releasecontract.ReleaseRecall{}, MinimumReaderContract: releasecontract.SyncedSourceContract}
	syncRoot := filepath.Join(root, "synced-source")
	if _, err = releasepack.PublishGeneration(releasepack.PublishGenerationOptions{Root: syncRoot, ReleaseDirectory: release, Registry: registryPath, Generation: generation, PrivateKey: private}); err != nil {
		t.Fatal(err)
	}
	return qualifiedContentRelease{factory: factory, release: release, registrySHA: fmt.Sprintf("%x", sha256.Sum256(registryBody)), source: syncRoot, registry: registry}
}

func (r qualifiedContentRelease) install(t *testing.T) ownerFixture {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	root := contentFixtureRoot(t)
	user, workspace := filepath.Join(root, "owner"), filepath.Join(root, "workspace")
	for _, path := range []string{user, workspace} {
		if err := os.Mkdir(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
	seedSHA := contentFileSHA(t, r.factory.Installer)
	// Match setup's canonical factory test: this attests only exact engineering
	// seed bytes, never a production native signature or client qualification.
	config := setup.Config{Installer: r.factory.Installer, AuthorityRegistry: r.factory.AuthorityRegistry, LookupEnv: func(string) (string, bool) { return "", false }, VerifyNative: func(_ context.Context, path string) error {
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if fmt.Sprintf("%x", sha256.Sum256(body)) != seedSHA {
			return errors.New("engineering native verifier: seed bytes changed")
		}
		return nil
	}}
	config.Run = func(ctx context.Context, self string, args ...string) ([]byte, error) {
		if len(args) == 0 || args[0] != "install" {
			return exec.CommandContext(ctx, self, args...).CombinedOutput()
		}
		flags := flag.NewFlagSet("content-engineering-inner-composition", flag.ContinueOnError)
		body := flags.String("confirmed-plan", "", "")
		digest := flags.String("review-digest", "", "")
		confirmed := flags.Bool("confirm", false, "")
		if err := flags.Parse(args[1:]); err != nil {
			return nil, err
		}
		var plan installer.Plan
		if err := releasecontract.DecodeStrictJSON([]byte(*body), &plan); err != nil {
			return nil, err
		}
		state, err := installer.ActivateSeeded(ctx, installer.SeededActivationOptions{Self: self, Version: "0.3.7", AuthorityRegistrySHA256: r.registrySHA, Plan: plan, ReviewDigest: *digest, Confirmed: *confirmed, VerifyNative: config.VerifyNative})
		if err != nil {
			return nil, err
		}
		return json.Marshal(map[string]string{"state": "activated", "profile": state.Profile, "native_admission": "controlled_in_process_engineering_fixture"})
	}
	request := setup.Request{ReleaseDirectory: r.release, ManagedRoot: filepath.Join(root, "managed"), UserHome: user, Workspace: workspace, CodePacket: filepath.Join(root, "code"), DesktopPacket: filepath.Join(root, "desktop.mcpb")}
	review, err := setup.ReviewInstallation(request, config)
	if err != nil {
		t.Fatal(err)
	}
	result, err := setup.Apply(ctx, request, config, review.Digest, true)
	if err != nil || result.State != "ready" {
		t.Fatalf("genuine signed content setup: state=%s error=%v", result.State, err)
	}
	return ownerFixture{root: filepath.Join(result.Home, "brain"), binary: result.Installation.CLIPath, options: product.Options{ProductRoot: request.ManagedRoot, UserHome: user, WorkspacePath: workspace, LookupEnv: config.LookupEnv, CLIVersion: contentFixtureVersion, AuthorityRegistryPath: filepath.Join(request.ManagedRoot, "trust", "release-authority-registry.json"), AuthorityRegistrySHA256: r.registrySHA}}
}

func (r qualifiedContentRelease) enroll(t *testing.T, f ownerFixture) {
	t.Helper()
	options := updateservice.SyncedOptions{UserHome: f.options.UserHome, ManagedRoot: f.options.ProductRoot, DataRoot: filepath.Join(f.options.UserHome, ".maestro-install"), SourceRoot: r.source, ExpectedSource: releasecontract.SourceTuple{SourceID: "content-qualified-fixture", Channel: "canary", OS: runtime.GOOS, Arch: runtime.GOARCH}, Registry: r.registry}
	review, err := updateservice.ReviewSynced(options)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = updateservice.CheckSynced(options, review.Digest); !errors.Is(err, updateservice.ErrCurrent) {
		t.Fatalf("exact signed source enrollment: %v", err)
	}
}

func TestContentCompiledQualifiedStartup(t *testing.T) {
	release := newQualifiedContentRelease(t, "")
	f := release.install(t)
	uninstalled := f
	uninstalled.options.UserHome = physicalTempDir(t)
	for _, refused := range []ownerFixture{uninstalled, f} {
		if session, err := openContent(t, refused, nil, ""); err == nil {
			session.Close()
			t.Fatal("versioned compiled server accepted missing installation/source enrollment")
		} else if !strings.Contains(err.Error(), "startup exit=1 stderr=maestro: signed source recovery required;") {
			t.Fatalf("compiled startup lost bounded source-refusal/exit evidence: %v", err)
		}
	}
	release.enroll(t, f)
	managedBefore := contentManagedTree(t, f.options.ProductRoot)
	if len(managedBefore) < 2 {
		t.Fatal("qualified managed installation is empty")
	}
	t.Logf("engineering signed server version=%s registry_pin=%s installed_sha256=%s; exact signed source enrolled", contentFixtureVersion, release.registrySHA, contentFileSHA(t, f.binary))
	packet := filepath.Join(filepath.Dir(f.options.ProductRoot), "code", "scripts", filepath.Base(f.binary))
	if contentFileSHA(t, packet) != contentFileSHA(t, f.binary) {
		t.Fatal("packet bytes differ from authenticated installed server")
	}
	for i, binary := range []string{"", f.binary, packet} {
		current := f
		current.binary = binary
		for range 2 {
			session := connectContent(t, current, nil, "")
			status := contentCall(t, session, "maestro_query", map[string]any{"operation": "content.status"}, false)
			if status["state"] != "unavailable" {
				t.Fatalf("pack-free qualified startup invented converter availability: %v", status)
			}
			session.Close()
		}
		t.Logf("qualified content.status startup/restart passed carrier=%s", []string{"sdk", "installed", "code_packet"}[i])
	}
	if !reflect.DeepEqual(managedBefore, contentManagedTree(t, f.options.ProductRoot)) {
		t.Fatal("qualified startup/restart changed managed entries, bytes or modes")
	}
	t.Logf("genuine nonempty managed installation preserved all %d relative entries, exact file bytes and modes", len(managedBefore))
}

func TestContentManagedTreeDetectsMutation(t *testing.T) {
	for _, kind := range []string{"byte", "mode", "entry"} {
		t.Run(kind, func(t *testing.T) {
			root := physicalTempDir(t)
			path := filepath.Join(root, "installed-input")
			putContentFile(t, path, []byte("original managed bytes"))
			before := contentManagedTree(t, root)
			var err error
			switch kind {
			case "byte":
				err = os.WriteFile(path, []byte("Original managed bytes"), 0600)
			case "mode":
				err = os.Chmod(path, 0400)
			case "entry":
				err = os.Mkdir(filepath.Join(root, "unexpected-empty-directory"), 0700)
			}
			if err != nil {
				t.Fatal(err)
			}
			if reflect.DeepEqual(before, contentManagedTree(t, root)) {
				t.Fatalf("managed snapshot accepted changed %s", kind)
			}
		})
	}
}

func TestContentStartupStderrBoundedAndSanitized(t *testing.T) {
	var stderr contentStartupStderr
	private := strings.Repeat("private-startup-token", 1000)
	if n, err := stderr.Write([]byte(private)); err != nil || n != len(private) || len(stderr.body) != 4096 {
		t.Fatalf("startup stderr is not bounded/drained: n=%d retained=%d error=%v", n, len(stderr.body), err)
	}
	if diagnostic := stderr.diagnostic(); !strings.HasPrefix(diagnostic, "<redacted 4096 retained bytes sha256=") || strings.Contains(diagnostic, "private-startup-token") {
		t.Fatalf("startup stderr leaked unrecognized bytes: %s", diagnostic)
	}
}
