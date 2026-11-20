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

// This public assembly check uses only explicit pinned public archives; it never
// launches Windows Python or establishes native qualification.
func TestWindowsDocumentsAssemblyDefersUnusedAsyncCLI(t *testing.T) {
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
	assembled, err := docling.AssembleProfile(inputs, filepath.Join(root, "pack"), "windows", "amd64", docling.DocumentsProfile)
	if err != nil {
		t.Fatal(err)
	}
	installed := filepath.Join(filepath.Dir(assembled.ArchivePath), "installed", assembled.ArchiveSHA256)
	provenance, err := os.ReadFile(filepath.Join(installed, "compatibility", "pydantic-settings-lazy-asyncio-v1.json"))
	if err != nil {
		t.Fatalf("explicit compatibility provenance missing: %v", err)
	}
	if !bytes.Contains(provenance, []byte("8d523e5c129b097686bde019bd6109bff53f4c8c3f63f1ddf9933ffa42e5ab4f")) {
		t.Fatal("compatibility provenance lost original wheel identity")
	}
	body, err := os.ReadFile(filepath.Join(installed, "site-packages", "pydantic_settings", "main.py"))
	if err != nil || sum(body) != "ba6767448d4f690e8a87440910800231f50da10703cd457f4e0613a5adde3336" {
		t.Fatalf("real settings source did not receive exact bounded import deferral: %v", err)
	}
	record, err := os.ReadFile(filepath.Join(installed, "site-packages", "pydantic_settings-2.16.2.dist-info", "RECORD"))
	if err != nil || sum(record) != "da6a8b905d49193fd75fcd05ff8f882ece1f084e0e818d98cf1901c56136d82d" {
		t.Fatalf("installed RECORD does not describe transformed source: %v", err)
	}
}
