package scheduler_test

import (
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestOpenDirectoryHoldsOriginalWithoutFollowingReplacement(t *testing.T) {
	root := t.TempDir()
	name := filepath.Join(root, "held")
	if err := os.Mkdir(name, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(name, "original"), []byte("yes"), 0644); err != nil {
		t.Fatal(err)
	}
	f, err := scheduler.OpenDirectory(name)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.IsDir() {
		t.Fatal(info, err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0755 {
		t.Fatal("permissions changed")
	}
	outside := filepath.Join(root, "outside")
	if err := os.Mkdir(outside, 0700); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		return
	}
	if err := os.Rename(name, name+"-moved"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, name); err != nil {
		t.Fatal(err)
	}
	entries, err := f.ReadDir(-1)
	if err != nil || len(entries) != 1 || entries[0].Name() != "original" {
		t.Fatalf("held handle redirected: %v %v", entries, err)
	}
	if alias, err := scheduler.OpenDirectory(name); err == nil {
		alias.Close()
		t.Fatal("directory alias admitted")
	}
	if alias, err := scheduler.OpenDirectory(filepath.Join(name, "child")); err == nil {
		alias.Close()
		t.Fatal("ancestor alias admitted")
	}
	if file, err := scheduler.OpenDirectory(filepath.Join(name+"-moved", "original")); err == nil {
		file.Close()
		t.Fatal("regular file admitted")
	}
}
