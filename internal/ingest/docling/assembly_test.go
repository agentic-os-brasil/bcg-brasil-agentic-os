package docling_test

import (
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ingest/docling"
	"os"
	"path/filepath"
	"testing"
)

func TestAssemblyRefusesUnauthenticatedRawInterpreterBeforeCreatingOutput(t *testing.T) {
	root := t.TempDir()
	inputs := filepath.Join(root, "inputs")
	if err := os.Mkdir(inputs, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(inputs, docling.PythonArchive), []byte("untrusted archive"), 0600); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(root, "pack")
	if _, err := docling.Assemble(inputs, destination); err == nil {
		t.Fatal("untrusted raw interpreter accepted")
	}
	if _, err := os.Lstat(destination); !os.IsNotExist(err) {
		t.Fatalf("output created before raw input authentication: %v", err)
	}
}
