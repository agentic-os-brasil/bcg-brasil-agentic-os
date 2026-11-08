package product_test

import (
	"archive/zip"
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
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp/packaging"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp/setup"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/dev/releasepack"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installer"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installtx"
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
		if packSHA != artifact.SHA256 || int64(len(body)) != artifact.Size || !ed25519.Verify(key, body, readContentFile(t, filepath.Join(base, artifact.SignatureRef))) {
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

type contentPacketEntry struct {
	body []byte
	mode os.FileMode
}

func contentExtensionEntries(packet packaging.Options, canonicalRoot string) (map[string]contentPacketEntry, error) {
	server, err := os.ReadFile(packet.Server)
	if err != nil {
		return nil, err
	}
	registry, err := os.ReadFile(packet.Binding.AuthorityRegistryPath)
	if err != nil {
		return nil, err
	}
	if fmt.Sprintf("%x", sha256.Sum256(registry)) != packet.Binding.AuthorityRegistrySHA256 {
		return nil, errors.New("registry pin changed")
	}
	serverPath := "server/" + filepath.Base(packet.Server)
	expectedModes := map[string]os.FileMode{"manifest.json": 0600, serverPath: 0700, "managed/trust/release-authority-registry.json": 0600, "maestro-packet.json": 0600}
	archive, err := zip.OpenReader(packet.Destination)
	if err != nil {
		return nil, err
	}
	defer archive.Close()
	if len(archive.File) != len(expectedModes) {
		return nil, errors.New("unexpected Desktop entry count")
	}
	entries := map[string]contentPacketEntry{}
	for _, entry := range archive.File {
		mode, known := expectedModes[entry.Name]
		if _, duplicate := entries[entry.Name]; !known || duplicate || entry.Mode() != mode {
			return nil, fmt.Errorf("unexpected Desktop entry %s", entry.Name)
		}
		maximum := int64(1 << 20)
		if entry.Name == serverPath {
			maximum = int64(len(server))
		}
		if entry.Name == "managed/trust/release-authority-registry.json" {
			maximum = int64(len(registry))
		}
		if entry.UncompressedSize64 > uint64(maximum) {
			return nil, errors.New("Desktop entry exceeds expected size")
		}
		reader, err := entry.Open()
		if err != nil {
			return nil, err
		}
		body, err := io.ReadAll(io.LimitReader(reader, maximum+1))
		closeErr := reader.Close()
		if err != nil || closeErr != nil || int64(len(body)) > maximum {
			return nil, errors.New("invalid Desktop entry bytes")
		}
		entries[entry.Name] = contentPacketEntry{body: body, mode: entry.Mode()}
	}
	if !bytes.Equal(entries[serverPath].body, server) || !bytes.Equal(entries["managed/trust/release-authority-registry.json"].body, registry) {
		return nil, errors.New("Desktop executable or public registry changed")
	}
	state, err := installtx.ReadStateForManagedRoot(filepath.Join(packet.Binding.UserHome, ".maestro-install"), packet.Binding.ProductRoot)
	if err != nil {
		return nil, err
	}
	platform := runtime.GOOS
	if platform == "windows" {
		platform = "win32"
	}
	installer := "maestro-install"
	if platform == "win32" {
		installer += ".exe"
	}
	expectedManifest := map[string]any{
		"manifest_version": "0.3", "name": "maestro-product", "version": packet.Version,
		"description": "Local participant Maestro execution workflow", "author": map[string]any{"name": "Agentic OS Brasil"},
		"server": map[string]any{"type": "binary", "entry_point": serverPath, "mcp_config": map[string]any{
			"command": filepath.Join(packet.Binding.ProductRoot, installer),
			"args":    []any{"launch", "--product-root", packet.Binding.ProductRoot, "--user-home", packet.Binding.UserHome, "--workspace", packet.Binding.WorkspacePath, "--installed-manifest", state.Binding.ManifestSHA256, "--installed-client-inputs", state.Binding.ClientInputsSHA256},
		}},
		"compatibility": map[string]any{"platforms": []any{platform}},
	}
	var manifest map[string]any
	if err = releasecontract.DecodeStrictJSON(entries["manifest.json"].body, &manifest); err != nil || !reflect.DeepEqual(manifest, expectedManifest) {
		return nil, errors.New("Desktop manifest binding changed")
	}
	var inventory, canonical map[string]json.RawMessage
	if err = releasecontract.DecodeStrictJSON(entries["maestro-packet.json"].body, &inventory); err != nil {
		return nil, err
	}
	canonicalBody, err := os.ReadFile(filepath.Join(canonicalRoot, "maestro-packet.json"))
	if err != nil {
		return nil, err
	}
	if err = releasecontract.DecodeStrictJSON(canonicalBody, &canonical); err != nil {
		return nil, err
	}
	var files []struct {
		Path   string `json:"path"`
		SHA256 string `json:"sha256"`
		Mode   uint32 `json:"mode"`
	}
	if err = releasecontract.DecodeStrictJSON(inventory["files"], &files); err != nil || len(files) != 3 {
		return nil, errors.New("Desktop inventory must cover exact payload")
	}
	seen := map[string]bool{}
	for _, file := range files {
		entry, exists := entries[file.Path]
		if !exists || file.Path == "maestro-packet.json" || seen[file.Path] || file.Mode != uint32(entry.mode) || file.SHA256 != fmt.Sprintf("%x", sha256.Sum256(entry.body)) {
			return nil, errors.New("Desktop inventory payload mismatch")
		}
		seen[file.Path] = true
	}
	identity, err := releasecontract.ReadServerIdentity(packet.Server)
	if err != nil {
		return nil, err
	}
	var resources []releasecontract.EmbeddedResource
	if err = releasecontract.DecodeStrictJSON(inventory["embedded_resources"], &resources); err != nil || !reflect.DeepEqual(resources, identity.Resources) || identity.Version != packet.Version || identity.AuthorityRegistrySHA256 != packet.Binding.AuthorityRegistrySHA256 {
		return nil, errors.New("Desktop resource identity changed")
	}
	// Immutable sections match the admitted Code carrier; only Desktop's unavailable-native reason differs.
	var native packaging.NativeCapability
	if err = releasecontract.DecodeStrictJSON(inventory["native"], &native); err != nil || native.State != "unavailable" || native.Reason != "desktop_extension_carries_mcp_resources_only" {
		return nil, errors.New("Desktop native boundary changed")
	}
	native.Reason = "native_host_not_qualified"
	inventory["native"], err = json.Marshal(native)
	if err != nil {
		return nil, err
	}
	delete(inventory, "files")
	delete(canonical, "files")
	actualCommon, _ := json.Marshal(inventory)
	expectedCommon, _ := json.Marshal(canonical)
	var actualValue, expectedValue any
	if json.Unmarshal(actualCommon, &actualValue) != nil || json.Unmarshal(expectedCommon, &expectedValue) != nil || !reflect.DeepEqual(actualValue, expectedValue) || string(inventory["mode"]) != `"mcp_only"` {
		return nil, errors.New("Desktop canonical metadata changed")
	}
	return entries, nil
}

func TestContentDesktopPacketOracle(t *testing.T) {
	release := newQualifiedContentRelease(t, "")
	f := release.install(t)
	const private = "PRIVATE_CONTENT_PACKET_SENTINEL"
	putContentFile(t, filepath.Join(f.root, "private-packet-sentinel.md"), []byte(private))
	putContentFile(t, filepath.Join(f.options.WorkspacePath, "private-client.txt"), []byte(private))
	ownerBefore, managedBefore := treeSnapshot(t, f.root), contentManagedTree(t, f.options.ProductRoot)
	packet := packaging.Options{Destination: filepath.Join(filepath.Dir(f.options.ProductRoot), "oracle.mcpb"), Server: f.binary, Version: contentFixtureVersion, Binding: f.options}
	if err := packaging.BuildExtension(packet); err != nil {
		t.Fatal(err)
	}
	canonicalRoot := filepath.Join(filepath.Dir(f.options.ProductRoot), "code")
	entries, err := contentExtensionEntries(packet, canonicalRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 4 {
		t.Fatalf("expected exact four-entry Desktop carrier, got %d", len(entries))
	}
	for name, entry := range entries {
		if bytes.Contains(entry.body, []byte(private)) {
			t.Fatalf("private state copied into %s", name)
		}
	}
	editJSON := func(files map[string]contentPacketEntry, name string, edit func(map[string]any)) {
		t.Helper()
		var value map[string]any
		if err := json.Unmarshal(files[name].body, &value); err != nil {
			t.Fatal(err)
		}
		edit(value)
		body, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		entry := files[name]
		entry.body = body
		files[name] = entry
	}
	serverPath := "server/" + filepath.Base(f.binary)
	for _, tc := range []struct {
		name   string
		mutate func(map[string]contentPacketEntry)
	}{
		{"unknown_private_file", func(files map[string]contentPacketEntry) {
			files["owner/private.json"] = contentPacketEntry{body: []byte(private), mode: 0600}
		}},
		{"unknown_json_replaces_metadata", func(files map[string]contentPacketEntry) {
			delete(files, "maestro-packet.json")
			files["other.json"] = contentPacketEntry{body: []byte(`{}`), mode: 0600}
		}},
		{"missing_metadata", func(files map[string]contentPacketEntry) { delete(files, "maestro-packet.json") }},
		{"duplicate_entry", func(map[string]contentPacketEntry) {}},
		{"changed_mode", func(files map[string]contentPacketEntry) {
			e := files["maestro-packet.json"]
			e.mode = 0700
			files["maestro-packet.json"] = e
		}},
		{"changed_server", func(files map[string]contentPacketEntry) {
			e := files[serverPath]
			e.body[0] ^= 1
			files[serverPath] = e
		}},
		{"changed_registry", func(files map[string]contentPacketEntry) {
			e := files["managed/trust/release-authority-registry.json"]
			e.body[0] ^= 1
			files["managed/trust/release-authority-registry.json"] = e
		}},
		{"changed_binding", func(files map[string]contentPacketEntry) {
			editJSON(files, "manifest.json", func(m map[string]any) {
				m["server"].(map[string]any)["mcp_config"].(map[string]any)["args"].([]any)[6] = "/other-workspace"
			})
			// A self-consistent archive hash cannot authorize a different workspace.
			editJSON(files, "maestro-packet.json", func(m map[string]any) {
				for _, raw := range m["files"].([]any) {
					f := raw.(map[string]any)
					if f["path"] == "manifest.json" {
						f["sha256"] = fmt.Sprintf("%x", sha256.Sum256(files["manifest.json"].body))
					}
				}
			})
		}},
		{"inventory_hash", func(files map[string]contentPacketEntry) {
			editJSON(files, "maestro-packet.json", func(m map[string]any) { m["files"].([]any)[0].(map[string]any)["sha256"] = strings.Repeat("0", 64) })
		}},
		{"inventory_extra_entry", func(files map[string]contentPacketEntry) {
			editJSON(files, "maestro-packet.json", func(m map[string]any) {
				m["files"] = append(m["files"].([]any), map[string]any{"path": "owner/private.json", "sha256": strings.Repeat("0", 64), "mode": 384})
			})
		}},
		{"resource_identity", func(files map[string]contentPacketEntry) {
			editJSON(files, "maestro-packet.json", func(m map[string]any) {
				m["embedded_resources"].([]any)[0].(map[string]any)["sha256"] = strings.Repeat("0", 64)
			})
		}},
		{"noncanonical_metadata", func(files map[string]contentPacketEntry) {
			editJSON(files, "maestro-packet.json", func(m map[string]any) { m["private"] = private })
		}},
		{"native_promotion", func(files map[string]contentPacketEntry) {
			editJSON(files, "maestro-packet.json", func(m map[string]any) { m["native"].(map[string]any)["state"] = "qualified" })
		}},
		{"wrong_target", func(files map[string]contentPacketEntry) {
			editJSON(files, "maestro-packet.json", func(m map[string]any) { m["target"].(map[string]any)["arch"] = "other" })
		}},
		{"duplicate_json_key", func(files map[string]contentPacketEntry) {
			e := files["maestro-packet.json"]
			e.body = append([]byte(`{"mode":"mcp_only",`), e.body[1:]...)
			files["maestro-packet.json"] = e
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			files := map[string]contentPacketEntry{}
			for name, entry := range entries {
				entry.body = append([]byte(nil), entry.body...)
				files[name] = entry
			}
			tc.mutate(files)
			var body bytes.Buffer
			writer := zip.NewWriter(&body)
			for name, entry := range files {
				if tc.name == "duplicate_entry" && name == "maestro-packet.json" {
					name = "manifest.json"
				}
				header := &zip.FileHeader{Name: name, Method: zip.Deflate}
				header.SetMode(entry.mode)
				w, err := writer.CreateHeader(header)
				if err != nil {
					t.Fatal(err)
				}
				if _, err = w.Write(entry.body); err != nil {
					t.Fatal(err)
				}
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
			mutated := packet
			mutated.Destination = filepath.Join(physicalTempDir(t), "changed.mcpb")
			putContentFile(t, mutated.Destination, body.Bytes())
			if _, err := contentExtensionEntries(mutated, canonicalRoot); err == nil {
				t.Fatal("tampered or noncanonical packet accepted")
			}
		})
	}
	if !reflect.DeepEqual(ownerBefore, treeSnapshot(t, f.root)) || !reflect.DeepEqual(managedBefore, contentManagedTree(t, f.options.ProductRoot)) || string(readContentFile(t, filepath.Join(f.options.WorkspacePath, "private-client.txt"))) != private {
		t.Fatal("archive validation changed private or managed state")
	}
}
