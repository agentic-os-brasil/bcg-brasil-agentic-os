package releasepack

import (
	"archive/zip"
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBuildWindowsPortableProducesVerifiedClaudeReadyArchive(t *testing.T) {
	options := validWindowsPortableOptions(t)
	output := options.Output
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
		root + "README-PORTABLE.md",
		root + "portable-provenance.json",
		root + "managed/activate-maestro.cmd",
		root + "managed/bcgos-bootstrap.exe",
		root + "managed/trust/release-authority-registry.json",
		root + "release/release-manifest.json",
		root + "release/release-manifest.json.sig",
		root + "workspace/CLAUDE.md",
		root + "workspace/README.md",
	} {
		if _, ok := entries[required]; !ok {
			t.Fatalf("portable archive is missing %s", required)
		}
	}
	if _, exists := entries[root+"Activate-Maestro.cmd"]; exists {
		t.Fatal("portable archive exposes a user-facing activation command")
	}
	activation := string(entries[root+"managed/activate-maestro.cmd"])
	for _, required := range []string{
		`for %%I in ("%~dp0..") do set "ROOT=%%~fI"`,
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
	orientation := string(entries[root+"workspace/CLAUDE.md"])
	for _, required := range []string{
		"Nao peca para a pessoa digitar ou executar comandos",
		"Peca uma unica confirmacao curta",
		`cmd.exe /d /c "..\managed\activate-maestro.cmd"`,
		"releia este CLAUDE.md",
		"maestro-onboarding",
		"nao repita o onboarding",
		"permissao nativa do Claude Code",
	} {
		if !strings.Contains(orientation, required) {
			t.Fatalf("portable CLAUDE.md is missing %q:\n%s", required, orientation)
		}
	}
	confirmation := strings.Index(orientation, "Peca uma unica confirmacao curta")
	activationCommand := strings.Index(orientation, `cmd.exe /d /c "..\managed\activate-maestro.cmd"`)
	if confirmation < 0 || activationCommand < 0 || confirmation > activationCommand {
		t.Fatal("portable CLAUDE.md must require confirmation before internal activation")
	}
	for name := range entries {
		if strings.Contains(strings.ToLower(name), "wizard") || strings.HasSuffix(strings.ToLower(name), "maestro-installer.exe") {
			t.Fatalf("portable archive retained installer surface %s", name)
		}
	}
	readme := string(entries[root+"README-PORTABLE.md"])
	for _, required := range []string{"Abra a pasta `workspace` no Claude Code", "Envie uma mensagem", "nao execute arquivos `.cmd`"} {
		if !strings.Contains(readme, required) {
			t.Fatalf("portable README does not describe the prompt-first Claude Code journey; missing %q:\n%s", required, readme)
		}
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

func validWindowsPortableOptions(t *testing.T) WindowsPortableOptions {
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
	return WindowsPortableOptions{
		Version: "0.2.0", ReleaseDirectory: signed,
		AuthorityRegistry: registryPath, AuthorityRegistrySHA256: testDigest(registryBody),
		Bootstrapper: bootstrapper, BootstrapperSHA256: testDigest(bootstrapperBody),
		Output: filepath.Join(t.TempDir(), "Maestro-Portable-0.2.0-windows-amd64-local-beta-unsigned.zip"),
		Clock:  func() time.Time { return time.Unix(2000, 0).UTC() },
		BootstrapperSeedStatus: func(string) (BootstrapperSeedStatus, error) {
			return BootstrapperSeedStatus{Version: "0.2.0", AuthorityRegistrySHA256: testDigest(registryBody)}, nil
		},
		NativeSignatureStatus: func(string) (string, error) { return "NotSigned", nil },
	}
}

func TestBuildWindowsPortableRejectsTrustBoundaryViolations(t *testing.T) {
	for _, test := range []struct {
		name    string
		mutate  func(*WindowsPortableOptions)
		wantErr string
	}{
		{
			name: "registry digest drift",
			mutate: func(options *WindowsPortableOptions) {
				options.AuthorityRegistrySHA256 = strings.Repeat("0", 64)
			},
			wantErr: "portable authority registry does not match its approved pin",
		},
		{
			name: "bootstrapper digest drift",
			mutate: func(options *WindowsPortableOptions) {
				options.BootstrapperSHA256 = strings.Repeat("0", 64)
			},
			wantErr: "portable bootstrapper does not match its approved pin",
		},
		{
			name: "signed bootstrapper",
			mutate: func(options *WindowsPortableOptions) {
				options.NativeSignatureStatus = func(string) (string, error) { return "Valid", nil }
			},
			wantErr: "portable local-beta bootstrapper Authenticode status must be exactly NotSigned; got Valid",
		},
		{
			name: "signature verifier failure",
			mutate: func(options *WindowsPortableOptions) {
				options.NativeSignatureStatus = func(string) (string, error) {
					return "", errors.New("signature probe failed")
				}
			},
			wantErr: "inspect portable bootstrapper Authenticode: signature probe failed",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			options := validWindowsPortableOptions(t)
			test.mutate(&options)
			_, err := BuildWindowsPortable(options)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("BuildWindowsPortable() error = %v, want %q", err, test.wantErr)
			}
		})
	}
}

func TestParseBootstrapperSeedStatusRejectsTrailingJSON(t *testing.T) {
	valid := `{"schema_version":1,"product":"maestro","bootstrapper_version":"0.2.0","authority_registry_sha256":"` + strings.Repeat("a", 64) + `"}`
	if _, err := parseBootstrapperSeedStatus([]byte(valid + "\n{}\n")); err == nil ||
		!strings.Contains(err.Error(), "bootstrapper seed status contains multiple JSON values") {
		t.Fatalf("parseBootstrapperSeedStatus() error = %v, want trailing JSON rejection", err)
	}
}

func testDigest(body []byte) string {
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:])
}
