//go:build !windows

package execution

import (
	"os"
	"path/filepath"
	"testing"
)

func confinementDirectoryAlias(t *testing.T, alias, target string) {
	t.Helper()
	if err := os.Symlink(target, alias); err != nil {
		t.Fatal(err)
	}
}

func confinementPublicDirectory(t *testing.T, path string) {
	t.Helper()
	if err := os.Chmod(path, 0755); err != nil {
		t.Fatal(err)
	}
}

func schemaLeafAlias(t *testing.T, alias, directory string) {
	t.Helper()
	if err := os.Symlink(filepath.Join(directory, "schema.json"), alias); err != nil {
		t.Fatal(err)
	}
}
