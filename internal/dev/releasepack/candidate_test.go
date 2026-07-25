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
