package main

import (
	"path/filepath"
	"testing"
)

func TestManagedRootComesOnlyFromInstalledBootstrapperPath(t *testing.T) {
	root := filepath.Join(t.TempDir(), "maestro")
	for name, path := range map[string]string{
		"root seed":        filepath.Join(root, "bcgos-bootstrap"),
		"bootstrap folder": filepath.Join(root, "bootstrap", "bcgos-bootstrap.exe"),
	} {
		t.Run(name, func(t *testing.T) {
			got, err := managedRootFromExecutablePath(path)
			if err != nil {
				t.Fatal(err)
			}
			if got != root {
				t.Fatalf("managed root = %s, want %s", got, root)
			}
		})
	}
	if _, err := managedRootFromExecutablePath(filepath.Join(root, "attacker-bootstrap")); err == nil {
		t.Fatal("managedRootFromExecutablePath() accepted an unprotected executable name")
	}
}
