package releasepack

import (
	"archive/zip"
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBuildWindowsPortableProducesVerifiedClaudeReadyArchive(t *testing.T) {
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
		Candidate: candidate, Output: signed, Registry: registryPath,
		Issuer: "maestro-release", KeyID: "pilot-2026", PrivateKey: privateKey,
		Clock: func() time.Time { return time.Unix(2000, 0).UTC() },
	}); err != nil {
		t.Fatal(err)
	}
	bootstrapperBody := []byte("synthetic-windows-bootstrapper")
	bootstrapper := filepath.Join(t.TempDir(), "bcgos-bootstrap_0.2.0_windows_amd64.exe")
	if err := os.WriteFile(bootstrapper, bootstrapperBody, 0o700); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "Maestro-Portable-0.2.0-windows-amd64-local-beta-unsigned.zip")
	options := WindowsPortableOptions{
		Version: "0.2.0", ReleaseDirectory: signed,
		AuthorityRegistry: registryPath, AuthorityRegistrySHA256: testDigest(registryBody),
		Bootstrapper: bootstrapper, BootstrapperSHA256: testDigest(bootstrapperBody),
		Output: output, Clock: func() time.Time { return time.Unix(2000, 0).UTC() },
		BootstrapperSeedStatus: func(string) (BootstrapperSeedStatus, error) {
			return BootstrapperSeedStatus{Version: "0.2.0", AuthorityRegistrySHA256: testDigest(registryBody)}, nil
		},
		NativeSignatureStatus: func(string) (string, error) { return "NotSigned", nil },
	}
	result, err := BuildWindowsPortable(options)
	if err != nil {
		t.Fatalf("BuildWindowsPortable() error = %v", err)
	}
	if result.Output != output || result.SHA256 == "" || result.Status != "unsigned-controlled-canary" {
		t.Fatalf("unexpected portable result: %#v", result)
	}
	checksum, err := os.ReadFile(result.Checksum)
	if err != nil || string(checksum) != result.SHA256+"  "+filepath.Base(output)+"\n" {
		t.Fatalf("unexpected portable checksum: %q err=%v", checksum, err)
	}
	if provenance, err := os.ReadFile(result.Provenance); err != nil || !bytes.Contains(provenance, []byte(`"distribution_profile": "windows-portable-local-beta"`)) {
		t.Fatalf("portable provenance is missing or invalid: %q err=%v", provenance, err)
	}
	archive, err := zip.OpenReader(output)
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	entries := map[string][]byte{}
	for _, entry := range archive.File {
		if entry.FileInfo().Mode()&os.ModeSymlink != 0 {
			t.Fatalf("portable archive contains a link: %s", entry.Name)
		}
		if entry.FileInfo().IsDir() {
			continue
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
	root := "Maestro-Portable-0.2.0-windows-amd64/"
	for _, required := range []string{
		root + "Activate-Maestro.cmd",
		root + "README-PORTABLE.md",
		root + "portable-provenance.json",
		root + "managed/bcgos-bootstrap.exe",
		root + "managed/trust/release-authority-registry.json",
		root + "release/release-manifest.json",
		root + "release/release-manifest.json.sig",
		root + "workspace/README.md",
	} {
		if _, ok := entries[required]; !ok {
			t.Fatalf("portable archive is missing %s", required)
		}
	}
	activation := string(entries[root+"Activate-Maestro.cmd"])
	for _, required := range []string{
		`%LOCALAPPDATA%\BCGOS`,
		`LOCALAPPDATA is unavailable`,
		`bcgos-bootstrap.exe" install`,
		`setup apply --workspace`,
		`--runtime claude`,
		`--confirm`,
		`adapter verify --runtime claude`,
		`bcgos 0.2.0`,
	} {
		if !strings.Contains(activation, required) {
			t.Fatalf("activation script is missing %q:\n%s", required, activation)
		}
	}
	for name := range entries {
		if strings.Contains(strings.ToLower(name), "wizard") || strings.HasSuffix(strings.ToLower(name), "maestro-installer.exe") {
			t.Fatalf("portable archive retained installer surface %s", name)
		}
	}
	if !bytes.Contains(entries[root+"README-PORTABLE.md"], []byte("open the workspace folder in Claude Desktop")) {
		t.Fatal("portable README does not describe the daily Claude Desktop journey")
	}
	secondOutput := filepath.Join(t.TempDir(), filepath.Base(output))
	options.Output = secondOutput
	second, err := BuildWindowsPortable(options)
	if err != nil {
		t.Fatal(err)
	}
	if second.SHA256 != result.SHA256 {
		t.Fatalf("portable archive is not deterministic: %s != %s", second.SHA256, result.SHA256)
	}
}

func TestBuildWindowsPortableRejectsUnpinnedOrSignedBootstrapper(t *testing.T) {
	for _, test := range []struct {
		name      string
		registry  string
		bootstrap string
		native    string
	}{
		{name: "missing registry pin", bootstrap: strings.Repeat("b", 64), native: "NotSigned"},
		{name: "missing bootstrapper pin", registry: strings.Repeat("a", 64), native: "NotSigned"},
		{name: "unexpected native status", registry: strings.Repeat("a", 64), bootstrap: strings.Repeat("b", 64), native: "Valid"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := BuildWindowsPortable(WindowsPortableOptions{
				Version: "0.2.0", ReleaseDirectory: "missing", AuthorityRegistry: "missing",
				AuthorityRegistrySHA256: test.registry, Bootstrapper: "missing",
				BootstrapperSHA256: test.bootstrap, Output: filepath.Join(t.TempDir(), "portable.zip"),
				NativeSignatureStatus: func(string) (string, error) { return test.native, nil },
			})
			if err == nil {
				t.Fatal("BuildWindowsPortable() accepted an incomplete trust profile")
			}
		})
	}
}

func testDigest(body []byte) string {
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:])
}
