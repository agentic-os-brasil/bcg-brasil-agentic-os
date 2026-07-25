package releasepack

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteBinaryProvenanceCapturesBoundedBuildEvidence(t *testing.T) {
	target := Target{OS: "darwin", Arch: "arm64"}
	binary := filepath.Join(t.TempDir(), binaryName("0.1.0", target))
	if err := os.WriteFile(binary, []byte("native binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	setProvenanceEnvironment(t)
	t.Setenv("GH_TOKEN", "must-not-enter-provenance")
	output, err := WriteBinaryProvenance(binary, "0.1.0", target)
	if err != nil {
		t.Fatalf("WriteBinaryProvenance() error = %v", err)
	}
	body, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "must-not-enter-provenance") {
		t.Fatal("unapproved environment value entered provenance")
	}
	var provenance BinaryProvenance
	if err := json.Unmarshal(body, &provenance); err != nil {
		t.Fatal(err)
	}
	if provenance.SchemaVersion != 2 ||
		provenance.Component != string(NativeCLI) ||
		provenance.BinaryName != filepath.Base(binary) ||
		provenance.BinarySize != int64(len("native binary")) ||
		len(provenance.BinarySHA256) != 64 ||
		!provenance.CGOEnabled ||
		provenance.ImageVersion != "20260725.1" {
		t.Fatalf("unexpected provenance: %#v", provenance)
	}
}

func TestWriteNativeProvenanceCoversBootstrapper(t *testing.T) {
	target := Target{OS: "windows", Arch: "amd64"}
	binary := filepath.Join(t.TempDir(), bootstrapperBinaryName("0.1.0", target))
	if err := os.WriteFile(binary, []byte("stable bootstrapper"), 0o755); err != nil {
		t.Fatal(err)
	}
	setProvenanceEnvironment(t)
	output, err := WriteNativeProvenance(binary, "0.1.0", target, NativeBootstrapper)
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	var provenance BinaryProvenance
	if err := json.Unmarshal(body, &provenance); err != nil {
		t.Fatal(err)
	}
	if provenance.Component != string(NativeBootstrapper) ||
		provenance.BinaryName != filepath.Base(binary) {
		t.Fatalf("unexpected bootstrapper provenance: %#v", provenance)
	}
}

func TestWriteBinaryProvenanceRejectsUntrustedInputs(t *testing.T) {
	target := Target{OS: "windows", Arch: "amd64"}
	binary := filepath.Join(t.TempDir(), binaryName("0.1.0", target))
	if err := os.WriteFile(binary, []byte("native binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	setProvenanceEnvironment(t)
	t.Setenv("GITHUB_SHA", "not-a-commit")
	if _, err := WriteBinaryProvenance(binary, "0.1.0", target); err == nil ||
		!strings.Contains(err.Error(), "GITHUB_SHA") {
		t.Fatalf("malformed commit error = %v", err)
	}
	setProvenanceEnvironment(t)
	if _, err := WriteBinaryProvenance(binary, "0.2.0", target); err == nil ||
		!strings.Contains(err.Error(), "must be named") {
		t.Fatalf("mismatched version error = %v", err)
	}
}

func setProvenanceEnvironment(t *testing.T) {
	t.Helper()
	t.Setenv("GITHUB_SHA", strings.Repeat("a", 40))
	t.Setenv("GITHUB_RUN_ID", "12345")
	t.Setenv("GITHUB_RUN_ATTEMPT", "1")
	t.Setenv("RUNNER_OS", "macOS")
	t.Setenv("RUNNER_ARCH", "ARM64")
	t.Setenv("ImageOS", "macos15")
	t.Setenv("ImageVersion", "20260725.1")
}
