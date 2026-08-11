package releasepack

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildUniversalPortableProducesOneClaudeDirectedArchive(t *testing.T) {
	options := validUniversalPortableOptions(t)
	result, err := BuildUniversalPortable(options)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "unsigned-controlled-canary" {
		t.Fatalf("result=%#v", result)
	}
	archive, err := zip.OpenReader(result.Output)
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	entries := map[string]*zip.File{}
	for _, entry := range archive.File {
		entries[entry.Name] = entry
	}
	root := "Maestro-Portable-0.2.0-universal/"
	for _, required := range []string{
		"managed/windows/bcgos-bootstrap.exe",
		"managed/macos/arm64/bcgos-bootstrap", "managed/macos/amd64/bcgos-bootstrap",
		"maestro-os/CLAUDE.md", "release/release-manifest.json", "managed/trust/release-authority-registry.json",
	} {
		if _, ok := entries[root+required]; !ok {
			t.Fatalf("missing %s", required)
		}
	}
	orientation := readZipEntry(t, entries[root+"maestro-os/CLAUDE.md"])
	for _, required := range []string{"Windows", "macOS", "portable-activate", "bcgos-bootstrap.exe", "Peca uma unica confirmacao"} {
		if !strings.Contains(orientation, required) {
			t.Fatalf("orientation missing %q: %s", required, orientation)
		}
	}
	for _, forbidden := range []string{"cmd.exe", ".cmd", ".sh", "activate-maestro"} {
		if strings.Contains(orientation, forbidden) {
			t.Fatalf("orientation retained shell activation detail %q: %s", forbidden, orientation)
		}
	}
	for name := range entries {
		if strings.Contains(name, "activate-maestro") || strings.HasSuffix(name, ".cmd") || strings.HasSuffix(name, ".sh") {
			t.Fatalf("portable archive retained a shell activation surface: %s", name)
		}
	}
}

func TestBuildUniversalPortableRejectsBootstrapperWithoutDirectActivation(t *testing.T) {
	options := validUniversalPortableOptions(t)
	if err := os.WriteFile(options.WindowsBootstrapper, []byte("stale-bootstrapper"), 0o700); err != nil {
		t.Fatal(err)
	}
	options.WindowsBootstrapperSHA256 = testDigest([]byte("stale-bootstrapper"))
	if _, err := BuildUniversalPortable(options); err == nil || !strings.Contains(err.Error(), "does not support direct portable activation") {
		t.Fatalf("BuildUniversalPortable() error = %v, want direct activation rejection", err)
	}
}

func readZipEntry(t *testing.T, entry *zip.File) string {
	t.Helper()
	if entry == nil {
		t.Fatal("missing zip entry")
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
	return string(body)
}

func validUniversalPortableOptions(t *testing.T) UniversalPortableOptions {
	t.Helper()
	base := validWindowsPortableOptions(t)
	windowsBody := []byte("synthetic-windows-bootstrapper-" + universalPortableActivationContract)
	if err := os.WriteFile(base.Bootstrapper, windowsBody, 0o700); err != nil {
		t.Fatal(err)
	}
	base.BootstrapperSHA256 = testDigest(windowsBody)
	arm := filepath.Join(t.TempDir(), "bcgos-bootstrap_0.2.0_darwin_arm64")
	amd := filepath.Join(t.TempDir(), "bcgos-bootstrap_0.2.0_darwin_amd64")
	armBody := []byte("synthetic-darwin-arm64-bootstrapper-" + universalPortableActivationContract)
	amdBody := []byte("synthetic-darwin-amd64-bootstrapper-" + universalPortableActivationContract)
	if err := os.WriteFile(arm, armBody, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(amd, amdBody, 0o700); err != nil {
		t.Fatal(err)
	}
	return UniversalPortableOptions{
		Version: base.Version, ReleaseDirectory: base.ReleaseDirectory, AuthorityRegistry: base.AuthorityRegistry, AuthorityRegistrySHA256: base.AuthorityRegistrySHA256,
		WindowsBootstrapper: base.Bootstrapper, WindowsBootstrapperSHA256: base.BootstrapperSHA256,
		DarwinARM64Bootstrapper: arm, DarwinARM64BootstrapperSHA256: testDigest(armBody),
		DarwinAMD64Bootstrapper: amd, DarwinAMD64BootstrapperSHA256: testDigest(amdBody),
		Output: filepath.Join(t.TempDir(), "Maestro-Portable-0.2.0-universal-local-beta-unsigned.zip"), Clock: base.Clock,
		BootstrapperSeedStatus: func(string) (BootstrapperSeedStatus, error) {
			return BootstrapperSeedStatus{Version: base.Version, AuthorityRegistrySHA256: base.AuthorityRegistrySHA256}, nil
		},
		WindowsSignatureStatus: func(string) (string, error) { return "NotSigned", nil },
	}
}
