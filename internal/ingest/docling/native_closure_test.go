package docling_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ingest/docling"
)

func TestWindowsAssemblyIncludesAuthenticatedMSVCPClosure(t *testing.T) {
	inputs := os.Getenv("MAESTRO_WINDOWS_DOCLING_ASSEMBLY_INPUTS")
	if inputs == "" {
		t.Skip("explicit official raw Windows inputs required for assembly-only proof")
	}
	root := t.TempDir()
	t.Cleanup(func() {
		_ = filepath.WalkDir(root, func(name string, item os.DirEntry, err error) error {
			if err == nil && item.IsDir() {
				return os.Chmod(name, 0700)
			}
			return err
		})
	})
	assembled, err := docling.AssembleTarget(inputs, filepath.Join(root, "pack"), "windows", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	installed := filepath.Join(filepath.Dir(assembled.ArchivePath), "installed", assembled.ArchiveSHA256)
	alias, err := os.ReadFile(filepath.Join(installed, "python", "msvcp140.dll"))
	if err != nil {
		t.Fatalf("authenticated app-local MSVCP closure missing: %v", err)
	}
	source, err := os.ReadFile(filepath.Join(installed, "site-packages", "numpy.libs", "msvcp140-a4c2229bdc2a2a630acdc095b4d86008.dll"))
	if err != nil {
		t.Fatal(err)
	}
	if len(alias) != 575056 || sum(alias) != "a4c2229bdc2a2a630acdc095b4d86008e5c3e3bc7773174354f3da4f5beb9cde" || !bytes.Equal(alias, source) {
		t.Fatal("app-local MSVCP was not the exact authenticated upstream bytes")
	}
}
