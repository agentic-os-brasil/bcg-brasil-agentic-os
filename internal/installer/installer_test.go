package installer

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultPathsKeepManagedAndOwnerDataSeparate(t *testing.T) {
	tests := []struct {
		platform, home, local string
		managed, data         string
	}{
		{"windows", `C:\Users\pilot`, `C:\Users\pilot\AppData\Local`, `C:\Users\pilot\AppData\Local\Maestro`, `C:\Users\pilot\AppData\Local\BCGOS`},
		{"darwin", "/Users/pilot", "", "/Users/pilot/Library/Application Support/Maestro", "/Users/pilot/Library/Application Support/BCGOS"},
	}
	for _, test := range tests {
		t.Run(test.platform, func(t *testing.T) {
			paths, err := DefaultPaths(test.platform, test.home, test.local)
			if err != nil {
				t.Fatal(err)
			}
			if paths.ManagedRoot != test.managed || paths.DataRoot != test.data {
				t.Fatalf("paths = %#v, want managed=%q data=%q", paths, test.managed, test.data)
			}
		})
	}
}

func TestPrepareRejectsBootstrapperNameDriftBeforeNativeCheck(t *testing.T) {
	if err := validateBootstrapperName("wrong-name", "0.1.0", "darwin", "arm64"); err == nil || !strings.Contains(err.Error(), "bootstrapper must be named") {
		t.Fatalf("validateBootstrapperName error = %v, want name rejection", err)
	}
}

func TestReadSeedStatusRejectsTrailingJSON(t *testing.T) {
	_, err := readSeedStatus(context.Background(), "ignored", func(context.Context, string, ...string) ([]byte, error) {
		return []byte(`{"schema_version":1,"product":"maestro","bootstrapper_version":"0.1.0","authority_registry_sha256":"` + strings.Repeat("a", 64) + `"} {}`), nil
	})
	if err == nil || !strings.Contains(err.Error(), "multiple JSON values") {
		t.Fatalf("readSeedStatus error = %v, want trailing JSON rejection", err)
	}
}

func TestFileSHA256(t *testing.T) {
	path := filepath.Join(t.TempDir(), "value")
	writeInstallerFile(t, path, []byte("maestro"))
	digest, err := fileSHA256(path)
	if err != nil {
		t.Fatal(err)
	}
	expected := sha256.Sum256([]byte("maestro"))
	if digest != hex.EncodeToString(expected[:]) {
		t.Fatalf("digest = %s", digest)
	}
}

func TestInstallDelegatesOnlyAfterFullSeedBinding(t *testing.T) {
	root := t.TempDir()
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	publicKey := privateKey.Public().(ed25519.PublicKey)
	registryBody := []byte(fmt.Sprintf(`{"schema_version":1,"product":"maestro","authorities":[{"issuer":"release","key_id":"key-1","algorithm":"ed25519","public_key":"%s","status":"active","valid_from":"2026-01-01T00:00:00Z","valid_until":"2030-01-01T00:00:00Z"}]}`,
		base64.StdEncoding.EncodeToString(publicKey)))
	registryPath := filepath.Join(root, "registry.json")
	writeInstallerFile(t, registryPath, registryBody)
	registryDigest, err := fileSHA256(registryPath)
	if err != nil {
		t.Fatal(err)
	}
	releaseDir := filepath.Join(root, "release")
	writeSignedReleaseFixture(t, releaseDir, privateKey)
	bootstrapper := filepath.Join(root, "bcgos-bootstrap_0.1.0_darwin_arm64")
	writeInstallerFile(t, bootstrapper, []byte("native signed bootstrapper"))
	managedRoot := filepath.Join(root, "Maestro")
	dataRoot := filepath.Join(root, "BCGOS")
	run := func(_ context.Context, path string, args ...string) ([]byte, error) {
		if strings.Contains(filepath.Base(path), "bcgos-bootstrap") && len(args) == 1 && args[0] == "seed-status" {
			return []byte(fmt.Sprintf(`{"schema_version":1,"product":"maestro","bootstrapper_version":"0.1.0","authority_registry_sha256":"%s"}`,
				registryDigest)), nil
		}
		if strings.Contains(filepath.Base(path), "bcgos-bootstrap") && len(args) > 0 && args[0] == "install" {
			if err := os.MkdirAll(filepath.Join(managedRoot, "bin"), 0o700); err != nil {
				return nil, err
			}
			if err := os.WriteFile(filepath.Join(managedRoot, "bin", "bcgos"), []byte("bcgos 0.1.0"), 0o700); err != nil {
				return nil, err
			}
			return []byte("Maestro installation complete"), nil
		}
		if filepath.Base(path) == "bcgos" && len(args) == 1 && args[0] == "version" {
			return []byte("bcgos 0.1.0\n"), nil
		}
		return nil, fmt.Errorf("unexpected command %s %v", path, args)
	}
	result, err := Install(context.Background(), Options{
		ReleaseDir: releaseDir, Bootstrapper: bootstrapper, AuthorityRegistry: registryPath,
		ManagedRoot: managedRoot, DataRoot: dataRoot, TargetOS: "darwin", TargetArch: "arm64",
		VerifyNative: func(context.Context, string) error { return nil }, Run: run,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Release != "0.1.0" || result.CLIPath != filepath.Join(managedRoot, "bin", "bcgos") {
		t.Fatalf("unexpected install result: %#v", result)
	}
	if _, err := os.Stat(filepath.Join(managedRoot, "trust", "release-authority-registry.json")); err != nil {
		t.Fatal(err)
	}
}

func writeSignedReleaseFixture(t *testing.T, directory string, privateKey ed25519.PrivateKey) {
	t.Helper()
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	artifacts := []struct {
		kind, osName, arch, name string
		body                     []byte
	}{
		{"cli", "darwin", "arm64", "bcgos_0.1.0_darwin_arm64", []byte("bcgos 0.1.0")},
		{"bundle", "any", "any", "maestro-base_0.1.0.tar.gz", []byte("base bundle")},
	}
	notes := []byte("# Maestro 0.1.0\n")
	manifestArtifacts := make([]map[string]any, 0, len(artifacts))
	for _, artifact := range artifacts {
		digest := sha256.Sum256(artifact.body)
		manifestArtifacts = append(manifestArtifacts, map[string]any{
			"kind": artifact.kind, "os": artifact.osName, "arch": artifact.arch,
			"name": artifact.name, "size": len(artifact.body), "sha256": hex.EncodeToString(digest[:]),
			"signature_ref": artifact.name + ".sig",
		})
	}
	noteDigest := sha256.Sum256(notes)
	manifest := map[string]any{
		"schema_version": 1, "product": "maestro", "release": "0.1.0", "channel": "canary",
		"issuer":    map[string]string{"id": "release", "key_id": "key-1"},
		"cli":       map[string]string{"version": "0.1.0", "compatible_bundle": ">=0.1.0 <0.2.0"},
		"bundle":    map[string]string{"version": "0.1.0", "compatible_cli": ">=0.1.0 <0.2.0"},
		"artifacts": manifestArtifacts, "migrations": []any{},
		"release_notes": map[string]any{"name": "release-notes-0.1.0.md", "sha256": hex.EncodeToString(noteDigest[:])},
	}
	manifestBody, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	writeInstallerFile(t, filepath.Join(directory, "release-manifest.json"), manifestBody)
	writeInstallerFile(t, filepath.Join(directory, "release-manifest.json.sig"), ed25519.Sign(privateKey, manifestBody))
	writeInstallerFile(t, filepath.Join(directory, "release-notes-0.1.0.md"), notes)
	for _, artifact := range artifacts {
		writeInstallerFile(t, filepath.Join(directory, artifact.name), artifact.body)
		writeInstallerFile(t, filepath.Join(directory, artifact.name+".sig"), ed25519.Sign(privateKey, artifact.body))
	}
}

func writeInstallerFile(t *testing.T, path string, body []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
}
