package releasepack

import (
	"archive/zip"
	"bytes"
	"crypto/ed25519"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestBuildMacOSPortableProducesVerifiedClaudeReadyArchive(t *testing.T) {
	options := validMacOSPortableOptions(t)
	result, err := BuildMacOSPortable(options)
	if err != nil {
		t.Fatalf("BuildMacOSPortable() error = %v", err)
	}
	if result.Status != "unsigned-controlled-canary" || result.SHA256 == "" {
		t.Fatalf("unexpected portable result: %#v", result)
	}
	archive, err := zip.OpenReader(result.Output)
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	entries := map[string][]byte{}
	for _, entry := range archive.File {
		if entry.FileInfo().IsDir() {
			continue
		}
		if entry.FileInfo().Mode()&os.ModeSymlink != 0 {
			t.Fatalf("portable archive contains a link: %s", entry.Name)
		}
		reader, err := entry.Open()
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(reader)
		_ = reader.Close()
		if err != nil {
			t.Fatal(err)
		}
		entries[entry.Name] = body
	}
	root := "Maestro-Portable-0.2.0-macos-arm64/"
	for _, required := range []string{
		"README-PORTABLE.md", "portable-provenance.json", "managed/bcgos-bootstrap",
		"managed/trust/release-authority-registry.json", "release/release-manifest.json",
		"release/release-manifest.json.sig", "maestro-os/CLAUDE.md", "maestro-os/README.md",
	} {
		if _, ok := entries[root+required]; !ok {
			t.Fatalf("portable archive is missing %s", required)
		}
	}
	for name := range entries {
		if strings.Contains(name, "/managed/windows/") || strings.Contains(name, "/managed/macos/") ||
			(strings.HasSuffix(name, ".exe") && !strings.Contains(name, "/release/")) {
			t.Fatalf("macOS portable retained foreign-target bootstrap surface %s", name)
		}
	}
	orientation := string(entries[root+"maestro-os/CLAUDE.md"])
	for _, required := range []string{
		"macOS Apple Silicon", "Peca uma unica confirmacao curta", "../managed/bcgos-bootstrap portable-install",
		"../managed/bin/bcgos", "Nao peca para a pessoa digitar ou executar comandos", "maestro-onboarding",
	} {
		if !strings.Contains(orientation, required) {
			t.Fatalf("portable CLAUDE.md is missing %q:\n%s", required, orientation)
		}
	}
	if strings.Contains(orientation, "portable-activate") {
		t.Fatalf("macOS orientation retained an obsolete or terminal-directed flow: %s", orientation)
	}
	if !bytes.Contains(entries[root+"portable-provenance.json"], []byte(`"bootstrapper_codesign_status": "AdHoc"`)) {
		t.Fatalf("macOS provenance is missing ad-hoc codesign status: %s", entries[root+"portable-provenance.json"])
	}
	if !bytes.Contains(entries[root+"portable-provenance.json"], []byte(`"cli_codesign_status": "AdHoc"`)) {
		t.Fatalf("macOS provenance is missing CLI ad-hoc codesign status: %s", entries[root+"portable-provenance.json"])
	}
	secondOutput := filepath.Join(t.TempDir(), filepath.Base(result.Output))
	options.Output = secondOutput
	second, err := BuildMacOSPortable(options)
	if err != nil {
		t.Fatal(err)
	}
	if second.SHA256 != result.SHA256 {
		t.Fatalf("portable archive is not deterministic: %s != %s", second.SHA256, result.SHA256)
	}
}

func TestBuildMacOSPortableRejectsNonAdHocBootstrapper(t *testing.T) {
	options := validMacOSPortableOptions(t)
	options.StructuralSignature = func(path string) (string, error) {
		if path == options.Bootstrapper {
			return "Signed", nil
		}
		return "CodeSignaturePresent", nil
	}
	if _, err := BuildMacOSPortable(options); err == nil || !strings.Contains(err.Error(), "must carry an ad-hoc") {
		t.Fatalf("BuildMacOSPortable() error = %v, want ad-hoc rejection", err)
	}
}

func TestBuildMacOSPortableRejectsNonAdHocCLI(t *testing.T) {
	options := validMacOSPortableOptions(t)
	options.StructuralSignature = func(path string) (string, error) {
		if path == filepath.Join(options.ReleaseDirectory, "bcgos_0.2.0_darwin_arm64") {
			return "NoCodeSignature", nil
		}
		return "CodeSignaturePresent", nil
	}
	if _, err := BuildMacOSPortable(options); err == nil || !strings.Contains(err.Error(), "CLI must carry an ad-hoc") {
		t.Fatalf("BuildMacOSPortable() error = %v, want CLI ad-hoc rejection", err)
	}
}

func TestBuildMacOSPortableRejectsBootstrapperWithoutPortableInstall(t *testing.T) {
	options := validMacOSPortableOptions(t)
	if err := os.WriteFile(options.Bootstrapper, []byte("stale-bootstrapper"), 0o700); err != nil {
		t.Fatal(err)
	}
	options.BootstrapperSHA256 = testDigest([]byte("stale-bootstrapper"))
	if _, err := BuildMacOSPortable(options); err == nil || !strings.Contains(err.Error(), "does not support portable core installation") {
		t.Fatalf("BuildMacOSPortable() error = %v, want portable-install rejection", err)
	}
}

func validMacOSPortableOptions(t *testing.T) MacOSPortableOptions {
	t.Helper()
	candidate := unsignedCandidateFixture(t)
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	registryBody := authorityRegistryForKey(t, publicKey)
	registryPath := filepath.Join(t.TempDir(), "registry.json")
	if err := os.WriteFile(registryPath, registryBody, 0o600); err != nil {
		t.Fatal(err)
	}
	signed := filepath.Join(t.TempDir(), "signed")
	if _, err := SignCandidate(SignCandidateOptions{
		Candidate: candidate, Output: signed, Registry: registryPath, Issuer: "maestro-release", KeyID: "pilot-2026", PrivateKey: privateKey,
		Clock: func() time.Time { return time.Unix(2000, 0).UTC() },
	}); err != nil {
		t.Fatal(err)
	}
	body := []byte("synthetic-macos-bootstrapper-" + portableInstallContract)
	bootstrapper := filepath.Join(t.TempDir(), "bcgos-bootstrap_0.2.0_darwin_arm64")
	if err := os.WriteFile(bootstrapper, body, 0o700); err != nil {
		t.Fatal(err)
	}
	options := MacOSPortableOptions{
		Version: "0.2.0", ReleaseDirectory: signed, AuthorityRegistry: registryPath, AuthorityRegistrySHA256: testDigest(registryBody),
		Bootstrapper: bootstrapper, BootstrapperSHA256: testDigest(body),
		Output: filepath.Join(t.TempDir(), "Maestro-Portable-0.2.0-macos-arm64-local-beta-unsigned.zip"), Clock: func() time.Time { return time.Unix(2000, 0).UTC() },
		BootstrapperSeedStatus: func(string) (BootstrapperSeedStatus, error) {
			return BootstrapperSeedStatus{Version: "0.2.0", AuthorityRegistrySHA256: testDigest(registryBody)}, nil
		},
		StructuralSignature: func(string) (string, error) { return "CodeSignaturePresent", nil },
	}
	if runtime.GOOS == "darwin" {
		options.NativeSignature = func(string) (string, error) { return "AdHoc", nil }
	}
	return options
}
