package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installtx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseverify"
)

func TestManagedRootComesOnlyFromInstalledBootstrapperPath(t *testing.T) {
	root := filepath.Join(t.TempDir(), "maestro")
	for name, path := range map[string]string{
		"root seed":            filepath.Join(root, "bcgos-bootstrap"),
		"bootstrap folder":     filepath.Join(root, "bootstrap", "bcgos-bootstrap.exe"),
		"portable Windows":     filepath.Join(root, "windows", "bcgos-bootstrap.exe"),
		"portable macOS arm":   filepath.Join(root, "macos", "arm64", "bcgos-bootstrap"),
		"portable macOS Intel": filepath.Join(root, "macos", "amd64", "bcgos-bootstrap"),
	} {
		t.Run(name, func(t *testing.T) {
			got, err := managedRootFromExecutablePath(path)
			if err != nil {
				t.Fatal(err)
			}
			if got != root {
				t.Fatalf("managed root = %s, want %s", got, root)
			}
		})
	}
	if _, err := managedRootFromExecutablePath(filepath.Join(root, "attacker-bootstrap")); err == nil {
		t.Fatal("managedRootFromExecutablePath() accepted an unprotected executable name")
	}
	if _, err := managedRootFromExecutablePath(
		filepath.Join(string(filepath.Separator), "bcgos-bootstrap"),
	); err == nil {
		t.Fatal("managedRootFromExecutablePath() accepted the filesystem root")
	}
}

func TestSeedStatusExposesOnlyPublicTrustBinding(t *testing.T) {
	var output bytes.Buffer
	digest := strings.Repeat("a", 64)
	if err := writeSeedStatus(&output, "0.2.0", digest); err != nil {
		t.Fatal(err)
	}
	var status seedStatus
	if err := json.Unmarshal(output.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if status.SchemaVersion != 1 ||
		status.Product != "maestro" ||
		status.BootstrapperVersion != "0.2.0" ||
		status.AuthorityRegistrySHA256 != digest {
		t.Fatalf("unexpected seed status: %#v", status)
	}
}

func TestPortableInstallContractIsExplicit(t *testing.T) {
	if portableInstallContract != "maestro-portable-install-v2" {
		t.Fatalf("unexpected portable install contract: %q", portableInstallContract)
	}
}

func TestPortableInstallFailureIsUserFacing(t *testing.T) {
	if strings.Contains(strings.ToLower(portableInstallFailureMessage), "cmd") || strings.Contains(portableInstallFailureMessage, "--") {
		t.Fatalf("portable install failure leaks command detail: %q", portableInstallFailureMessage)
	}
}

func TestFirstInstallCreatesItsOwnVerifiedActivationPlan(t *testing.T) {
	releaseDirectory := t.TempDir()
	cliBody := []byte("bcgos 0.1.0")
	cliName := "bcgos_0.1.0_darwin_arm64"
	if err := os.WriteFile(filepath.Join(releaseDirectory, cliName), cliBody, 0o755); err != nil {
		t.Fatal(err)
	}
	bundleName := "maestro-base_0.1.0.tar.gz"
	bundlePath := filepath.Join(releaseDirectory, bundleName)
	writeBootstrapTestBundle(t, bundlePath)
	bundleBody, err := os.ReadFile(bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	verified := releaseverify.VerifiedRelease{
		Directory: releaseDirectory,
		Manifest: releasecontract.Manifest{
			Release: "0.1.0", Channel: "canary",
			CLI:    releasecontract.CLIComponent{Version: "0.1.0"},
			Bundle: releasecontract.BundleComponent{Version: "0.1.0"},
			Artifacts: []releasecontract.Artifact{
				{
					Kind: "cli", OS: "darwin", Arch: "arm64", Name: cliName,
					Size: int64(len(cliBody)), SHA256: bootstrapTestDigest(cliBody),
				},
				{
					Kind: "bundle", OS: "any", Arch: "any", Name: bundleName,
					Size: int64(len(bundleBody)), SHA256: bootstrapTestDigest(bundleBody),
				},
			},
		},
		ManifestSHA256: strings.Repeat("a", 64),
	}
	managedRoot := filepath.Join(t.TempDir(), "managed")
	dataRoot := filepath.Join(t.TempDir(), "data")
	checker := func(path, version string) error {
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !strings.Contains(string(body), version) {
			return os.ErrInvalid
		}
		return nil
	}
	if err := firstInstall(
		verified,
		managedRoot,
		dataRoot,
		"darwin",
		"arm64",
		checker,
	); err != nil {
		t.Fatalf("firstInstall() error = %v", err)
	}
	state, err := installtx.ReadStateForManagedRoot(dataRoot, managedRoot)
	if err != nil {
		t.Fatal(err)
	}
	if state.Release != "0.1.0" || state.CLIVersion != "0.1.0" {
		t.Fatalf("unexpected installed state: %#v", state)
	}
	if err := firstInstall(
		verified,
		managedRoot,
		dataRoot,
		"darwin",
		"arm64",
		checker,
	); err == nil {
		t.Fatal("firstInstall() accepted an existing managed installation")
	}
}

func writeBootstrapTestBundle(t *testing.T, path string) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	gzipWriter := gzip.NewWriter(file)
	tarWriter := tar.NewWriter(gzipWriter)
	body := []byte("managed\n")
	if err := tarWriter.WriteHeader(&tar.Header{
		Name: "runtime/capabilities.json", Mode: 0o644, Size: int64(len(body)),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := tarWriter.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func bootstrapTestDigest(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}
