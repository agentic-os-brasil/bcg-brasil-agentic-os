//go:build !windows

package ownerctx

import (
	"os"
	"testing"
)

func ownerPublicRegistry(t *testing.T, path string) func() {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	return func() {
		if err := os.Chmod(path, info.Mode().Perm()); err != nil {
			t.Fatal(err)
		}
	}
}

func ownerDirectoryAlias(t *testing.T, alias, target string) {
	t.Helper()
	if err := os.Symlink(target, alias); err != nil {
		t.Fatal(err)
	}
}

func ownerSecuritySnapshot(t *testing.T, path string) string { t.Helper(); return "" }
