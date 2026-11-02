//go:build darwin || linux

package scheduler

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestRenamePrivateFileExclusivePublishesOneLinkAndPreservesCollision(t *testing.T) {
	path := privateTestDir(t, "publication")
	directory, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer directory.Close()
	if err := WriteNewPrivateFile(filepath.Join(path, "staged"), []byte("first publication")); err != nil {
		t.Fatal(err)
	}
	if err := RenamePrivateFileExclusive(directory, "staged", "published"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(path, "staged")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("exclusive publication left its staging alias: %v", err)
	}
	var stat unix.Stat_t
	if err := unix.Stat(filepath.Join(path, "published"), &stat); err != nil || stat.Nlink != 1 {
		t.Fatalf("published inode is not single-link: nlink=%d err=%v", stat.Nlink, err)
	}
	if err := WriteNewPrivateFile(filepath.Join(path, "second"), []byte("must remain staged")); err != nil {
		t.Fatal(err)
	}
	if err := RenamePrivateFileExclusive(directory, "second", "published"); !errors.Is(err, os.ErrExist) {
		t.Fatalf("collision was not refused: %v", err)
	}
	for name, expected := range map[string]string{"published": "first publication", "second": "must remain staged"} {
		body, err := ReadPrivateFile(filepath.Join(path, name), 128)
		if err != nil || string(body) != expected {
			t.Fatalf("collision lost %s: %q %v", name, body, err)
		}
	}
}

func TestRenamePrivateFileExclusiveUsesHeldDirectoryAndRejectsPaths(t *testing.T) {
	path := privateTestDir(t, "publication")
	directory, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer directory.Close()
	if err := WriteNewPrivateFile(filepath.Join(path, "staged"), []byte("held directory")); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"", ".", "..", "../outside", "/outside", "nested/leaf", "nul\x00leaf"} {
		if err := RenamePrivateFileExclusive(directory, "staged", name); err == nil {
			t.Fatalf("accepted target path %q", name)
		}
		if err := RenamePrivateFileExclusive(directory, name, "published"); err == nil {
			t.Fatalf("accepted source path %q", name)
		}
	}
	if err := RenamePrivateFileExclusive(nil, "staged", "published"); err == nil {
		t.Fatal("accepted missing directory descriptor")
	}
	if err := os.Rename(path, path+".held"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := WriteNewPrivateFile(filepath.Join(path, "published"), []byte("replacement directory")); err != nil {
		t.Fatal(err)
	}
	if err := RenamePrivateFileExclusive(directory, "staged", "published"); err != nil {
		t.Fatal(err)
	}
	for name, expected := range map[string]string{filepath.Join(path+".held", "published"): "held directory", filepath.Join(path, "published"): "replacement directory"} {
		body, err := ReadPrivateFile(name, 128)
		if err != nil || string(body) != expected {
			t.Fatalf("descriptor custody changed: %q %v", body, err)
		}
	}
}
