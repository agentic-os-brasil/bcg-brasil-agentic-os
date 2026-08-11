package scheduler

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// The owner atlas needs to rewrite a page it did not create, without ever
// following a link and without silently creating a page that is absent.
// WriteNewPrivateFile cannot do it (it fails closed on an existing leaf) and
// ReadPrivateFile is read-only, so the boundary must expose a replace verb.

// privateTestDir mirrors how real callers reach the boundary: the enclosing
// tree is created by ordinary means first (internal/atlas relies on
// workspace.Initialize for exactly this), and the path is canonicalized before
// the no-follow walk. Skipping the canonicalization leaves a Windows 8.3 short
// name in the path; skipping the MkdirAll leaves the walk with no tree to open.
func privateTestDir(t *testing.T, parts ...string) string {
	t.Helper()
	root, err := CanonicalPrivatePath(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(append([]string{root}, parts...)...)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := EnsurePrivateDirectory(directory); err != nil {
		t.Fatal(err)
	}
	return directory
}

func TestReplacePrivateFileRewritesExistingLeaf(t *testing.T) {
	directory := privateTestDir(t, "atlas", "owner")
	path := filepath.Join(directory, "page.md")
	if err := WriteNewPrivateFile(path, []byte("# Page\n\n## Entries\n")); err != nil {
		t.Fatal(err)
	}

	replaced := []byte("# Page\n\n## Entries\n- first entry\n")
	if err := ReplacePrivateFile(path, replaced); err != nil {
		t.Fatalf("replace existing private leaf: %v", err)
	}

	body, err := ReadPrivateFile(path, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != string(replaced) {
		t.Fatalf("replaced body = %q, want %q", body, replaced)
	}
}

func TestReplacePrivateFileFailsClosedWhenLeafIsAbsent(t *testing.T) {
	directory := privateTestDir(t, "atlas", "owner")
	path := filepath.Join(directory, "absent.md")

	if err := ReplacePrivateFile(path, []byte("created by replace\n")); err == nil {
		t.Fatal("replace created an absent page; it must fail closed so creation stays with WriteNewPrivateFile")
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("failed replace left a leaf behind: %v", err)
	}
}

func TestReplacePrivateFileRejectsSymlinkedLeaf(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix symlink fixture; Windows reparse coverage lives in private_path_windows_test.go")
	}
	outside := t.TempDir()
	directory := privateTestDir(t, "atlas", "owner")
	target := filepath.Join(outside, "escaped.md")
	if err := os.WriteFile(target, []byte("outside\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "page.md")
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}

	if err := ReplacePrivateFile(path, []byte("written through the link\n")); err == nil {
		t.Fatal("replace followed a symlinked leaf")
	}
	body, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "outside\n" {
		t.Fatalf("replace wrote through the link: target body = %q", body)
	}
}

// Directory enumeration is deliberately absent. Proposal 007 bounds collect to
// "named pages or a segment index" and forbids an implicit whole-root dump, so
// a segment is reached through its index page rather than by walking the
// filesystem. secureReadDir additionally hangs on Windows today: walkWindows
// Directory opens directory handles without FILE_SYNCHRONOUS_IO_NONALERT, so a
// synchronous ReadDir on that handle never returns.
