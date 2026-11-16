package docling_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ingest/docling"
)

func TestWindowsAssemblyAuthenticatesItsOwnInterpreterBeforePublishing(t *testing.T) {
	root := t.TempDir()
	destination := filepath.Join(root, "pack")
	_, err := docling.AssembleTarget(root, destination, "windows", "amd64")
	if err == nil || !strings.Contains(err.Error(), "cpython-3.12.16+20261026-x86_64-pc-windows-msvc-install_only.tar.gz") {
		t.Fatalf("expected the pinned native Windows interpreter, got %v", err)
	}
	if _, err := os.Lstat(destination); !os.IsNotExist(err) {
		t.Fatalf("output published before authentication: %v", err)
	}
}

func TestAssemblyRejectsUnsupportedTargetBeforeReadingInputs(t *testing.T) {
	_, err := docling.AssembleTarget(t.TempDir(), "unused", "windows", "arm64")
	if err == nil || err.Error() != "platform_unavailable" {
		t.Fatalf("unsupported target: %v", err)
	}
}
