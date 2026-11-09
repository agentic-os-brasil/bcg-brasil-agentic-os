package releasepack

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestLegacySeededBuilderStillRefusesRemovedCLI(t *testing.T) {
	root := t.TempDir()
	output := filepath.Join(root, "removed-cli")
	err := (GoSeededComponentBuilder{}).Build(context.Background(), root, output, "0.1.0", Target{OS: runtime.GOOS, Arch: runtime.GOARCH}, NativeCLI, "")
	if err == nil || !strings.Contains(err.Error(), "no CLI binary target") {
		t.Fatalf("historical CLI gained a modern alias: %v", err)
	}
	if _, err := os.Lstat(output); !os.IsNotExist(err) {
		t.Fatal("removed CLI builder wrote output")
	}
}

func TestModernCandidateRejectsWrongRunnerBeforeOutput(t *testing.T) {
	root := t.TempDir()
	output := filepath.Join(root, "candidate")
	_, err := BuildModernCandidate(context.Background(), ModernOptions{Root: root, Output: output, AuthorityRegistry: filepath.Join(root, "authority.json"), Version: "0.1.42", InstallerVersion: "0.3.7", Channel: "canary", Target: Target{OS: "unsupported", Arch: runtime.GOARCH}})
	if err == nil || !strings.Contains(err.Error(), "supported runner") {
		t.Fatalf("unsupported target was selected: %v", err)
	}
	if _, err := os.Lstat(output); !os.IsNotExist(err) {
		t.Fatal("wrong runner created output")
	}
}

func TestRootLicenseExceptionRemainsExactAndNonexecutable(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "LICENSE.md")
	if err := os.WriteFile(path, []byte("public license notice"), 0755); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := BuildBundle(root, Allowlist{SchemaVersion: 1, Files: []BundleFile{{Source: "LICENSE.md", Path: "LICENSE.md", Role: "executable"}}}, &output); err == nil || !strings.Contains(err.Error(), "nonexecutable") {
		t.Fatalf("executable root license accepted: %v", err)
	}
	if err := BuildBundle(root, Allowlist{SchemaVersion: 1, Files: []BundleFile{{Source: "other/LICENSE.md", Path: "LICENSE.md"}}}, &output); err == nil {
		t.Fatal("root license exception broadened to another source")
	}
}
