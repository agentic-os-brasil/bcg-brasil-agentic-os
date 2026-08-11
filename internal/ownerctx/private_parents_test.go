package ownerctx

import (
	"os"
	"path/filepath"
	"testing"
)

// validatePrivateParents guards every owner-private write. The walk that
// collects the path components must terminate on every platform: on Windows
// the filesystem root is a volume whose parent is itself, so a guard that only
// stops at "." or the separator is never reached and the caller hangs before
// it can write anything.
func TestValidatePrivateParentsTerminatesOnANestedPath(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "BCGOS", "owner", "self")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}

	if err := validatePrivateParents(directory); err != nil {
		t.Fatalf("validate private parents: %v", err)
	}
}

func TestAtomicPrivateWriteCompletesOnANestedPath(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "BCGOS", "owner")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "facet.md")

	if err := atomicPrivateWrite(path, []byte("## Current\n\nowner truth\n")); err != nil {
		t.Fatalf("atomic private write: %v", err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "## Current\n\nowner truth\n" {
		t.Fatalf("written body = %q", body)
	}
}
