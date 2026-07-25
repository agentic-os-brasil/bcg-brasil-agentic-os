package releasepack

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeBinaryBuilder struct{}

func (fakeBinaryBuilder) Build(_ context.Context, _ string, output, version string, target Target) error {
	return os.WriteFile(output, []byte(version+" "+target.OS+"/"+target.Arch+"\n"), 0o755)
}

func TestBuildAndVerifyCandidateProducesClosedReleaseSet(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "bundles/base/runtime/capabilities.json", "{}\n")
	writeFile(t, root, "schemas/runtime-capabilities.schema.json", "{}\n")
	allowlist := `{"schema_version":1,"files":[` +
		`{"source":"schemas/runtime-capabilities.schema.json","path":"schemas/runtime-capabilities.schema.json"},` +
		`{"source":"bundles/base/runtime/capabilities.json","path":"runtime/capabilities.json"}]}`
	writeFile(t, root, "bundles/base/distribution.json", allowlist)
	output := filepath.Join(t.TempDir(), "candidate")

	manifest, err := BuildCandidate(context.Background(), CandidateOptions{
		Root:      root,
		Output:    output,
		Version:   "0.1.0",
		Channel:   "canary",
		Builder:   fakeBinaryBuilder{},
		Allowlist: "bundles/base/distribution.json",
	})
	if err != nil {
		t.Fatalf("BuildCandidate() error = %v", err)
	}
	if len(manifest.Artifacts) != 4 {
		t.Fatalf("artifact count = %d, want 4", len(manifest.Artifacts))
	}
	if err := VerifyCandidate(output); err != nil {
		t.Fatalf("VerifyCandidate() error = %v", err)
	}
	artifactPath := filepath.Join(output, manifest.Artifacts[0].Name)
	info, err := os.Stat(artifactPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(artifactPath, []byte(strings.Repeat("x", int(info.Size()))), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := VerifyCandidate(output); err == nil || !strings.Contains(err.Error(), "digest") {
		t.Fatalf("VerifyCandidate(tampered) error = %v", err)
	}
}

func TestBuildCandidateRejectsDirtyOutputAndInvalidVersion(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "bundles/base/runtime/capabilities.json", "{}\n")
	writeFile(t, root, "bundles/base/distribution.json", `{"schema_version":1,"files":[{"source":"bundles/base/runtime/capabilities.json","path":"runtime/capabilities.json"}]}`)
	output := filepath.Join(t.TempDir(), "candidate")
	if err := os.MkdirAll(output, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(output, "stale"), []byte("stale"), 0o600); err != nil {
		t.Fatal(err)
	}
	for name, version := range map[string]string{"dirty output": "0.1.0", "invalid version": "latest"} {
		t.Run(name, func(t *testing.T) {
			candidateOutput := output
			if name == "invalid version" {
				candidateOutput = filepath.Join(t.TempDir(), "clean")
			}
			if _, err := BuildCandidate(context.Background(), CandidateOptions{
				Root: root, Output: candidateOutput, Version: version, Channel: "canary",
				Builder: fakeBinaryBuilder{},
			}); err == nil {
				t.Fatal("BuildCandidate() accepted invalid input")
			}
		})
	}
}

func TestBuildCandidateAssemblesExactPrebuiltNativeBinaries(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "bundles/base/runtime/capabilities.json", "{}\n")
	writeFile(t, root, "bundles/base/distribution.json", `{"schema_version":1,"files":[{"source":"bundles/base/runtime/capabilities.json","path":"runtime/capabilities.json"}]}`)
	prebuilt := t.TempDir()
	for _, target := range candidateTargets {
		name := binaryName("0.1.0", target)
		writeFile(t, prebuilt, name, target.OS+"/"+target.Arch+"\n")
	}
	output := filepath.Join(t.TempDir(), "candidate")
	manifest, err := BuildCandidate(context.Background(), CandidateOptions{
		Root: root, Output: output, Version: "0.1.0", Channel: "canary", Prebuilt: prebuilt,
	})
	if err != nil {
		t.Fatalf("BuildCandidate(prebuilt) error = %v", err)
	}
	for _, target := range candidateTargets {
		name := binaryName("0.1.0", target)
		body, err := os.ReadFile(filepath.Join(output, name))
		if err != nil {
			t.Fatal(err)
		}
		if string(body) != target.OS+"/"+target.Arch+"\n" {
			t.Fatalf("assembled %s = %q", name, body)
		}
	}
	if len(manifest.Artifacts) != 4 {
		t.Fatalf("artifact count = %d, want 4", len(manifest.Artifacts))
	}
}

func TestBuildCandidateRejectsSymlinkedPrebuiltBinary(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "bundles/base/runtime/capabilities.json", "{}\n")
	writeFile(t, root, "bundles/base/distribution.json", `{"schema_version":1,"files":[{"source":"bundles/base/runtime/capabilities.json","path":"runtime/capabilities.json"}]}`)
	prebuilt := t.TempDir()
	realBinary := filepath.Join(prebuilt, "real")
	if err := os.WriteFile(realBinary, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	name := binaryName("0.1.0", candidateTargets[0])
	if err := os.Symlink(realBinary, filepath.Join(prebuilt, name)); err != nil {
		t.Fatal(err)
	}
	_, err := BuildCandidate(context.Background(), CandidateOptions{
		Root: root, Output: filepath.Join(t.TempDir(), "candidate"), Version: "0.1.0",
		Channel: "canary", Prebuilt: prebuilt,
	})
	if err == nil || !strings.Contains(err.Error(), "regular file") {
		t.Fatalf("BuildCandidate(symlink) error = %v", err)
	}
}

func TestBuildNativeBinaryValidatesTargetAndOutputName(t *testing.T) {
	root := t.TempDir()
	target := Target{OS: "darwin", Arch: "arm64"}
	output := filepath.Join(t.TempDir(), binaryName("0.1.0", target))
	if err := BuildNativeBinary(context.Background(), root, output, "0.1.0", target, fakeBinaryBuilder{}); err != nil {
		t.Fatalf("BuildNativeBinary() error = %v", err)
	}
	if _, err := os.Stat(output); err != nil {
		t.Fatal(err)
	}
	if err := BuildNativeBinary(
		context.Background(), root, filepath.Join(t.TempDir(), "wrong-name"), "0.1.0", target, fakeBinaryBuilder{},
	); err == nil || !strings.Contains(err.Error(), "must be named") {
		t.Fatalf("BuildNativeBinary(wrong name) error = %v", err)
	}
	if err := BuildNativeBinary(
		context.Background(), root, filepath.Join(t.TempDir(), "bcgos_0.1.0_linux_amd64"), "0.1.0",
		Target{OS: "linux", Arch: "amd64"}, fakeBinaryBuilder{},
	); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("BuildNativeBinary(unsupported) error = %v", err)
	}
}
