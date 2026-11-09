//go:build !windows

package zipmigration

import (
	"os"
	"testing"
)

func migrationDenyRead(t *testing.T, path string, _ bool) func() {
	t.Helper()
	return migrationFixtureMode(t, path, 0)
}

func migrationDenyWrite(t *testing.T, path string) func() {
	t.Helper()
	return migrationFixtureMode(t, path, 0500)
}

func migrationFixtureMode(t *testing.T, path string, mode os.FileMode) func() {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	restored := false
	restore := func() {
		if restored {
			return
		}
		if err := os.Chmod(path, info.Mode().Perm()); err != nil {
			t.Fatal("restore migration fixture permissions", err)
		}
		restored = true
	}
	t.Cleanup(restore)
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
	return restore
}
