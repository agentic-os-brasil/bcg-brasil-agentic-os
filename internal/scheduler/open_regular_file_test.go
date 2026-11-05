package scheduler_test

import (
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenRegularFileReadsOrdinaryFileWithoutChangingPermissions(t *testing.T) {
	dir := t.TempDir()
	name := filepath.Join(dir, "source.docx")
	if err := os.WriteFile(name, []byte("source"), 0644); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(name)
	if err != nil {
		t.Fatal(err)
	}
	f, err := scheduler.OpenRegularFile(name)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || info.Mode() != before.Mode() {
		t.Fatalf("mode changed: %v %v", info, err)
	}
	alias := filepath.Join(dir, "alias")
	if err := os.Symlink(name, alias); err != nil {
		t.Skip(err)
	}
	if f, err := scheduler.OpenRegularFile(alias); err == nil {
		f.Close()
		t.Fatal("alias admitted")
	}
	if f, err := scheduler.OpenRegularFile(dir); err == nil {
		f.Close()
		t.Fatal("directory admitted")
	}
}
