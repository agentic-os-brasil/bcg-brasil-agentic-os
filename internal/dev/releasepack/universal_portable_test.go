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
		"managed/windows/activate-maestro.cmd", "managed/windows/bcgos-bootstrap.exe",
		"managed/macos/activate-maestro.sh", "managed/macos/bcgos-bootstrap-arm64", "managed/macos/bcgos-bootstrap-amd64",
		"maestro-os/CLAUDE.md", "release/release-manifest.json", "managed/trust/release-authority-registry.json",
	} {
		if _, ok := entries[root+required]; !ok {
			t.Fatalf("missing %s", required)
		}
	}
	orientation := readZipEntry(t, entries[root+"maestro-os/CLAUDE.md"])
	for _, required := range []string{"Windows", "macOS", "activate-maestro.cmd", "activate-maestro.sh", "Peca uma unica confirmacao"} {
		if !strings.Contains(orientation, required) {
			t.Fatalf("orientation missing %q: %s", required, orientation)
		}
	}
	if entries[root+"managed/macos/activate-maestro.sh"].FileInfo().Mode()&0o100 == 0 {
		t.Fatal("macOS activator lost its executable bit")
	}
	macActivation := readZipEntry(t, entries[root+"managed/macos/activate-maestro.sh"])
	for _, required := range []string{"/../..", "uname -m", "bcgos-bootstrap-arm64", "bcgos-bootstrap-amd64", "Library/Application Support/BCGOS", "setup apply --workspace"} {
		if !strings.Contains(macActivation, required) {
			t.Fatalf("macOS activation missing %q: %s", required, macActivation)
		}
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
	arm := filepath.Join(t.TempDir(), "bcgos-bootstrap_0.2.0_darwin_arm64")
	amd := filepath.Join(t.TempDir(), "bcgos-bootstrap_0.2.0_darwin_amd64")
	if err := os.WriteFile(arm, []byte("synthetic-darwin-arm64-bootstrapper"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(amd, []byte("synthetic-darwin-amd64-bootstrapper"), 0o700); err != nil {
		t.Fatal(err)
	}
	return UniversalPortableOptions{
		Version: base.Version, ReleaseDirectory: base.ReleaseDirectory, AuthorityRegistry: base.AuthorityRegistry, AuthorityRegistrySHA256: base.AuthorityRegistrySHA256,
		WindowsBootstrapper: base.Bootstrapper, WindowsBootstrapperSHA256: base.BootstrapperSHA256,
		DarwinARM64Bootstrapper: arm, DarwinARM64BootstrapperSHA256: testDigest([]byte("synthetic-darwin-arm64-bootstrapper")),
		DarwinAMD64Bootstrapper: amd, DarwinAMD64BootstrapperSHA256: testDigest([]byte("synthetic-darwin-amd64-bootstrapper")),
		Output: filepath.Join(t.TempDir(), "Maestro-Portable-0.2.0-universal-local-beta-unsigned.zip"), Clock: base.Clock,
		BootstrapperSeedStatus: func(string) (BootstrapperSeedStatus, error) {
			return BootstrapperSeedStatus{Version: base.Version, AuthorityRegistrySHA256: base.AuthorityRegistrySHA256}, nil
		},
		WindowsSignatureStatus: func(string) (string, error) { return "NotSigned", nil },
	}
}
