//go:build !windows

package zipmigration_test

import (
	"os"
	"syscall"
	"testing"
)

func externalPlatformAccount(t *testing.T) { t.Helper() }

func externalDirectoryAlias(t *testing.T, alias, target string) {
	t.Helper()
	if err := os.Symlink(target, alias); err != nil {
		t.Fatal(err)
	}
}

func externalPublicControl(t *testing.T, profile, name string) func() {
	t.Helper()
	if err := os.Chmod(profile, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(name, 0777); err != nil {
		t.Fatal(err)
	}
	return func() {
		t.Helper()
		info, err := os.Stat(name)
		if err != nil || info.Mode().Perm() != 0777 {
			t.Fatal("unsafe control was repaired", err)
		}
	}
}

func externalPlatformIdentity(t *testing.T, path string) [3]uint64 {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	native, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		t.Fatal("actual directory identity unavailable")
	}
	return [3]uint64{uint64(native.Dev), uint64(native.Ino) >> 32, uint64(native.Ino) & 0xffffffff}
}
