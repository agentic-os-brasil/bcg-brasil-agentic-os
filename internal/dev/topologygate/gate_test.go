package topologygate

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateRejectsRetiredRoleMarker(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("retired role marker: capability_specialist\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Validate(root); err == nil {
		t.Fatal("retired role marker was accepted")
	}
}

func TestValidateAcceptsCanonicalSurfacesWithoutRetiredRole(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "specs"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "specs", "040.md"), []byte("Maestro depth one and Yoda materiality gate\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Validate(root); err != nil {
		t.Fatal(err)
	}
}
