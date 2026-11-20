package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installer"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseprovider"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseverify"
)

func TestFactoryRejectsUnauthenticatedBeforeCreatingOutput(t *testing.T) {
	root := developerFixture(t)
	bad := filepath.Join(root, "untrusted.json")
	if err := os.WriteFile(bad, []byte(`{"schema_version":1}`), 0600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(root, "kit")
	err := buildKit(factoryOptions{Output: out, Installer: bad, Role: bad, Signature: bad, Registry: bad, RegistrySHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", InitialRelease: root, Source: root}, time.Now)
	if err == nil {
		t.Fatal("unverified role must not produce a participant kit")
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatal("invalid admission created output")
	}
}

func TestFactoryAssemblesOnlyOnMatchingIdentityProbeHost(t *testing.T) {
	root := developerFixture(t)
	options, record, _, registry, now := factoryFixture(t, root)
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		if err := buildKit(options, func() time.Time { return now }); err == nil || !strings.Contains(err.Error(), "identity probe requires matching target host") {
			t.Fatalf("cross-target identity probe did not refuse explicitly: %v", err)
		}
		if _, err := os.Lstat(options.Output); !os.IsNotExist(err) {
			t.Fatal("cross-target refusal created output")
		}
		return
	}
	if err := buildKit(options, func() time.Time { return now }); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(options.Output, "DISTRIBUTION.json"))
	if err != nil {
		t.Fatal(err)
	}
	var d distribution
	if err = json.Unmarshal(body, &d); err != nil {
		t.Fatal(err)
	}
	if d.Evidence != "engineering_only" || d.Profile != installer.ExperimentalSeedProfile || d.Identity.Source != record.Source || len(d.ServerHashes) != 2 {
		t.Fatal("wrong public profile/source")
	}
	for _, file := range d.Files {
		if !strings.HasPrefix(file.Path, "bootstrap/") && !strings.HasPrefix(file.Path, "initial-release/") && file.Path != "test-documents/maestro-pilot.docx" {
			t.Fatal("mutable source/private state in immutable bootstrap inventory")
		}
		copied, err := os.ReadFile(filepath.Join(options.Output, filepath.FromSlash(file.Path)))
		if err != nil {
			t.Fatal(err)
		}
		if sum(copied) != file.SHA256 || int64(len(copied)) != file.Size {
			t.Fatalf("copy mismatch %s", file.Path)
		}
	}
	doc, err := os.ReadFile(filepath.Join(options.Output, "test-documents", "maestro-pilot.docx"))
	if err != nil || sum(doc) != "6bd282a12170035e8af5352f2de5a21f4fe4bc5c7919283482a26703c206bf81" {
		t.Fatalf("public synthetic conversion fixture missing or changed: %v", err)
	}
	launcher, err := os.ReadFile(filepath.Join(options.Output, "Maestro.command"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(launcher), sum(body)) || strings.Contains(string(launcher), "@@DISTRIBUTION_SHA256@@") {
		t.Fatal("launcher did not pin the exact public inventory")
	}
	for _, path := range []string{"product", "participant.json", "owner", "workspace"} {
		if _, err := os.Lstat(filepath.Join(options.Output, path)); !os.IsNotExist(err) {
			t.Fatalf("factory created participant state %s", path)
		}
	}
	pubRegistry, err := releaseverify.ParseAuthorityRegistry(strings.NewReader(string(registry)), func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	if _, err = releaseprovider.ReadSyncedMetadata(filepath.Join(options.Output, "source"), record.Source, pubRegistry); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Lstat(filepath.Join(root, "trust")); !os.IsNotExist(err) {
		t.Fatal("factory changed the original installer context")
	}
}

func TestFactoryRejectsInvalidPublicInputsBeforeCreatingOutput(t *testing.T) {
	root := developerFixture(t)
	options, record, key, _, now := factoryFixture(t, root)
	// Portable input refusals happen before the native-host identity probe.
	for _, kind := range []string{"installer", "role", "initial", "foreign-source", "expired"} {
		t.Run(kind, func(t *testing.T) {
			bad := options
			bad.Output = filepath.Join(root, "refused-"+kind)
			clock := func() time.Time { return now }
			switch kind {
			case "installer":
				bad.Installer = writeFixture(t, root, "wrong-installer", []byte("do not execute"))
			case "role":
				bad.Signature = writeFixture(t, root, "bad-role.sig", make([]byte, 64))
			case "initial":
				bad.InitialRelease = filepath.Join(root, "unknown-release")
			case "foreign-source":
				foreign := record
				foreign.Source.SourceID = "other-team"
				b, _ := json.Marshal(foreign)
				bad.Role = writeFixture(t, root, "foreign-role.json", b)
				bad.Signature = writeFixture(t, root, "foreign-role.sig", ed25519.Sign(key, b))
			case "expired":
				clock = func() time.Time { return now.Add(48 * time.Hour) }
			}
			if err := buildKit(bad, clock); err == nil {
				t.Fatal("invalid input accepted")
			}
			if _, err := os.Lstat(bad.Output); !os.IsNotExist(err) {
				t.Fatal("invalid input created an output")
			}
		})
	}
	t.Run("ordinary-installer-is-not-experimental", func(t *testing.T) {
		production := filepath.Join(root, "production-installer")
		repo, err := filepath.Abs("../../..")
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(goTool(t), "build", "-mod=readonly", "-trimpath", "-ldflags", "-X main.Version=0.1.51 -X main.AuthorityRegistrySHA256="+record.RegistrySHA256, "-o", production, "./cmd/maestro-install")
		cmd.Dir = filepath.Join(repo, "adapters/mcp")
		cmd.Env = append(os.Environ(), "GOOS=darwin", "GOARCH=arm64")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("ordinary source compile only: %s %v", out, err)
		}
		b, err := os.ReadFile(production)
		if err != nil {
			t.Fatal(err)
		}
		ordinaryRole := record
		ordinaryRole.InstallerSHA256 = sum(b)
		body, err := json.Marshal(ordinaryRole)
		if err != nil {
			t.Fatal(err)
		}
		bad := options
		bad.Output = filepath.Join(root, "refused-ordinary")
		bad.Installer = production
		bad.Role = writeFixture(t, root, "ordinary-role.json", body)
		bad.Signature = writeFixture(t, root, "ordinary-role.sig", ed25519.Sign(key, body))
		if err = buildKit(bad, func() time.Time { return now }); err == nil || !strings.Contains(err.Error(), "build tag") {
			t.Fatalf("ordinary entry accepted/misclassified: %v", err)
		}
		if _, err := os.Lstat(bad.Output); !os.IsNotExist(err) {
			t.Fatal("ordinary entry created a kit")
		}
	})
}

func TestFactoryRejectsCompiledInstallerIdentityMismatch(t *testing.T) {
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		t.Skip("genuine trimmed installer identity probe requires its Darwin arm64 host")
	}
	root := developerFixture(t)
	options, record, key, _, now := factoryFixture(t, root)
	repo, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct{ name, version, pin string }{
		{"wrong-pin", "0.1.51", strings.Repeat("e", 64)},
		{"wrong-version", "0.1.52", record.RegistrySHA256},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(root, test.name+"-installer")
			command := exec.Command(goTool(t), "build", "-mod=readonly", "-trimpath", "-tags", "maestro_experimental_installed_seed", "-ldflags", "-X main.Version="+test.version+" -X main.AuthorityRegistrySHA256="+test.pin, "-o", path, "./cmd/maestro-install")
			command.Dir = filepath.Join(repo, "adapters/mcp")
			command.Env = append(os.Environ(), "GOOS=darwin", "GOARCH=arm64")
			if output, err := command.CombinedOutput(); err != nil {
				t.Fatalf("compile actual mismatched installer: %s %v", output, err)
			}
			binary, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			changed := record
			changed.InstallerSHA256 = sum(binary)
			body, err := json.Marshal(changed)
			if err != nil {
				t.Fatal(err)
			}
			bad := options
			bad.Output, bad.Installer = filepath.Join(root, "refused-"+test.name), path
			bad.Role = writeFixture(t, root, test.name+"-role.json", body)
			bad.Signature = writeFixture(t, root, test.name+"-role.sig", ed25519.Sign(key, body))
			if err = buildKit(bad, func() time.Time { return now }); err == nil || !strings.Contains(err.Error(), "staged installer identity probe failed") {
				t.Fatalf("actual compiled identity mismatch was accepted or masked: %v", err)
			}
			staged := filepath.Join(bad.Output, "bootstrap", "trust", installer.ExperimentalSeedAdmissionName)
			retained, err := os.ReadFile(staged)
			if err != nil || sum(retained) != sum(body) {
				t.Fatalf("failed authenticated bootstrap was not retained: %v", err)
			}
			if _, err := os.Lstat(filepath.Join(bad.Output, "DISTRIBUTION.json")); !os.IsNotExist(err) {
				t.Fatal("failed compiled identity probe published a distribution")
			}
			if _, err := os.Lstat(filepath.Join(root, "trust")); !os.IsNotExist(err) {
				t.Fatal("probe repaired the original installer context")
			}
		})
	}
}

func TestParticipantTemplatesParseAndKeepTheLegacyFactoryBranch(t *testing.T) {
	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	if bash, err := posixBash(); err == nil {
		for _, path := range []string{"dev/pilot/build-kit.sh", "dev/pilot/build-installed-seed-kit.sh", "dev/pilot/templates/installed-seed/Maestro.command"} {
			cmd := exec.Command(bash, "-n", filepath.Join(root, path))
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("shell syntax %s: %s %v", path, out, err)
			}
		}
		cmd := exec.Command(bash, filepath.Join(root, "dev/pilot/build-kit.sh"), filepath.Join(developerFixture(t), "unused"), "--unknown-profile")
		if out, err := cmd.CombinedOutput(); err == nil || !strings.Contains(string(out), "[--lifecycle-context]") {
			t.Fatalf("legacy unknown-profile refusal changed: %s %v", out, err)
		}
	} else {
		t.Log("Bash unavailable; shell syntax and legacy shell dispatch were not executed on this host")
	}
	if parser, err := powerShellTool(); err == nil {
		// Parse only: never dot-source or execute the participant template.
		cmd := exec.Command(parser, "-NoLogo", "-NoProfile", "-Command", `$tokens=$null;$errors=$null;[System.Management.Automation.Language.Parser]::ParseFile($env:MAESTRO_PARSE_TEMPLATE,[ref]$tokens,[ref]$errors)|Out-Null;if($errors.Count){$errors|ForEach-Object{Write-Error $_.Message};exit 1}`)
		cmd.Env = append(os.Environ(), "MAESTRO_PARSE_TEMPLATE="+filepath.Join(root, "dev/pilot/templates/installed-seed/Maestro.ps1"))
		if dev := os.Getenv("GOCACHE"); strings.Contains(dev, "/Developer/") {
			toolRoot := filepath.Join(strings.SplitN(dev, "/Developer/", 2)[0], "Developer", "ToolData", "powershell-maestro")
			cmd.Env = append(cmd.Env, "XDG_CACHE_HOME="+filepath.Join(toolRoot, "cache"), "XDG_CONFIG_HOME="+filepath.Join(toolRoot, "config"), "XDG_DATA_HOME="+filepath.Join(toolRoot, "data"))
		}
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("PowerShell parse only: %s %v", out, err)
		}
	} else {
		t.Log("PowerShell parser unavailable; Windows syntax/native execution remain unverified")
	}
}

func posixBash() (string, error) {
	if runtime.GOOS == "windows" {
		return "", fmt.Errorf("POSIX shell parser not selected on Windows")
	}
	return exec.LookPath("bash")
}

func powerShellTool() (string, error) {
	if path, err := exec.LookPath("pwsh"); err == nil {
		return path, nil
	}
	if runtime.GOOS == "windows" {
		return exec.LookPath("powershell")
	}
	return "", fmt.Errorf("PowerShell parser unavailable")
}

func factoryFixture(t *testing.T, root string) (factoryOptions, installer.ExperimentalSeedRecord, ed25519.PrivateKey, []byte, time.Time) {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Second)
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	issuer := "maestro-engineering-pilot-" + now.Format("20060102")
	keyID := "pilot-" + now.Format("20060102")
	registry := []byte(fmt.Sprintf(`{"schema_version":1,"product":"maestro","authorities":[{"issuer":%q,"key_id":%q,"algorithm":"ed25519","public_key":%q,"status":"active","valid_from":%q,"valid_until":%q}]}`, issuer, keyID, base64.StdEncoding.EncodeToString(pub), now.Add(-time.Hour).Format(time.RFC3339), now.Add(24*time.Hour).Format(time.RFC3339)))
	pin := sum(registry)
	installerPath := filepath.Join(root, "maestro-install")
	repo, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(goTool(t), "build", "-mod=readonly", "-trimpath", "-tags", "maestro_experimental_installed_seed", "-ldflags", "-X main.Version=0.1.51 -X main.AuthorityRegistrySHA256="+pin, "-o", installerPath, "./cmd/maestro-install")
	cmd.Dir = filepath.Join(repo, "adapters/mcp")
	cmd.Env = append(os.Environ(), "GOOS=darwin", "GOARCH=arm64")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("trusted source compile only: %s %v", out, err)
	}
	installerBody, err := os.ReadFile(installerPath)
	if err != nil {
		t.Fatal(err)
	}
	record := installer.ExperimentalSeedRecord{SchemaVersion: 1, Product: "maestro", Purpose: installer.ExperimentalSeedPurpose, Protocol: installer.ExperimentalSeedProtocol, Profile: installer.ExperimentalSeedProfile, RegistrySHA256: pin, InstallerVersion: "0.1.51", InstallerSHA256: sum(installerBody), TargetOS: "darwin", TargetArch: "arm64", Source: releasecontract.SourceTuple{SourceID: "test-team", Channel: "canary", OS: "darwin", Arch: "arm64"}, Issuer: issuer, KeyID: keyID, NotBefore: now.Add(-time.Minute).Format(time.RFC3339), ExpiresAt: now.Add(time.Hour).Format(time.RFC3339)}
	initial := filepath.Join(root, "initial")
	if err = os.Mkdir(initial, 0700); err != nil {
		t.Fatal(err)
	}
	m := releasecontract.Manifest{SchemaVersion: 2, Product: "maestro", Release: "0.1.51", Channel: "canary", Issuer: releasecontract.Issuer{ID: issuer, KeyID: keyID}, Server: releasecontract.ServerComponent{Version: "0.1.51", Protocol: releasecontract.ServerProtocol, ExecutionContract: releasecontract.SyncedSourceContract, CompatibleBundle: ">=0.1.0 <0.2.0", EmbeddedInputsSHA256: strings.Repeat("a", 64)}, Bundle: releasecontract.BundleComponent{Version: "0.1.51", CompatibleServer: ">=0.1.0 <0.2.0"}, Delivery: releasecontract.DeliveryComponent{Profile: releasecontract.ModernProfile, InstallerVersion: "0.1.51", InstallerProtocol: releasecontract.InstallerProtocol}, Migrations: []releasecontract.Migration{}}
	for _, a := range []struct{ kind, adapter, os, arch, name string }{{"mcp_server", "", "darwin", "arm64", "maestro-mcp_0.1.51_darwin_arm64"}, {"bundle", "", "any", "any", "maestro-base_0.1.51.tar.gz"}, {"runtime_pack", "docling", "darwin", "arm64", "maestro-runtime-docling_0.1.51_darwin_arm64.tar.gz"}} {
		b := []byte("SYNTHETIC INERT " + a.name)
		writeFixture(t, initial, a.name, b)
		writeFixture(t, initial, a.name+".sig", ed25519.Sign(key, b))
		m.Artifacts = append(m.Artifacts, releasecontract.Artifact{Kind: a.kind, Adapter: a.adapter, OS: a.os, Arch: a.arch, Name: a.name, SHA256: sum(b), Size: int64(len(b)), SignatureRef: a.name + ".sig"})
	}
	notes := []byte("synthetic public release notes")
	m.ReleaseNotes = releasecontract.ReleaseNotes{Name: "notes_0.1.51.md", SHA256: sum(notes)}
	writeFixture(t, initial, m.ReleaseNotes.Name, notes)
	m.Delivery.ClientInputsSHA256, err = m.ClientInputsDigest()
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	writeFixture(t, initial, releaseverify.ManifestName, manifest)
	writeFixture(t, initial, releaseverify.ManifestSignatureName, ed25519.Sign(key, manifest))
	record.Releases = []installer.ExperimentalSeedRelease{{Release: m.Release, ManifestSHA256: sum(manifest), ClientInputsSHA256: m.Delivery.ClientInputsSHA256, ServerSHA256: m.Artifacts[0].SHA256, BundleSHA256: m.Artifacts[1].SHA256, RuntimeSHA256: m.Artifacts[2].SHA256}, {Release: "0.1.52", ManifestSHA256: strings.Repeat("b", 64), ClientInputsSHA256: strings.Repeat("c", 64), ServerSHA256: strings.Repeat("d", 64), BundleSHA256: strings.Repeat("e", 64), RuntimeSHA256: strings.Repeat("f", 64)}}
	body, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "synced")
	releaseDest := filepath.Join(source, "releases", m.Release, "darwin-arm64")
	if err = os.MkdirAll(releaseDest, 0700); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(initial)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		b, err := os.ReadFile(filepath.Join(initial, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		writeFixture(t, releaseDest, e.Name(), b)
	}
	g := releasecontract.Generation{SchemaVersion: 1, Product: "maestro", Protocol: releasecontract.GenerationProtocol, Issuer: m.Issuer, Source: record.Source, Sequence: 1, PublishedAt: now.Add(-time.Minute).Format(time.RFC3339), ExpiresAt: now.Add(time.Hour).Format(time.RFC3339), Release: m.Release, ManifestSHA256: sum(manifest), ClientInputsSHA256: m.Delivery.ClientInputsSHA256, Recalls: []releasecontract.ReleaseRecall{}, MinimumReaderContract: releasecontract.SyncedSourceContract}
	gb, err := json.Marshal(g)
	if err != nil {
		t.Fatal(err)
	}
	immutable := filepath.Join(source, "generations", g.GenerationDirectory(sum(gb)))
	if err = os.MkdirAll(immutable, 0700); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, source, "current.json", gb)
	writeFixture(t, source, "current.json.sig", ed25519.Sign(key, gb))
	writeFixture(t, immutable, "generation.json", gb)
	writeFixture(t, immutable, "generation.json.sig", ed25519.Sign(key, gb))
	return factoryOptions{Output: filepath.Join(root, "kit"), Installer: installerPath, Role: writeFixture(t, root, "role.json", body), Signature: writeFixture(t, root, "role.json.sig", ed25519.Sign(key, body)), Registry: writeFixture(t, root, "registry.json", registry), RegistrySHA256: pin, InitialRelease: initial, Source: source, Templates: filepath.Join(repo, "dev/pilot/templates/installed-seed")}, record, key, registry, now
}
func writeFixture(t *testing.T, root, name string, body []byte) string {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.WriteFile(path, body, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

// Normal CI temporary roots need not contain Developer. Preserve the real
// factory guard by making its fixture a canonical private Developer subtree.
func developerFixture(t *testing.T) string {
	t.Helper()
	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(parent, "Developer")
	if err = os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	return root
}

func goTool(t *testing.T) string {
	t.Helper()
	if path, err := exec.LookPath("go"); err == nil {
		return path
	}
	name := "go"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	path := filepath.Join(runtime.GOROOT(), "bin", name)
	if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
		return path
	}
	t.Fatal("Go tool unavailable; trusted source build cannot be checked")
	return ""
}
