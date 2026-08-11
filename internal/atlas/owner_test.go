package atlas

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The owner atlas is cross-project by definition: it holds professional
// trajectory, methods and learnings that outlive any single engagement.
// Initialize requires a registered workspace before it will touch anything,
// which is correct for the workspace root and wrong for the owner root, so the
// owner scaffold needs a resolver that depends on the data root alone.

func TestInitializeOwnerCreatesScaffoldWithoutAWorkspace(t *testing.T) {
	dataRoot := filepath.Join(t.TempDir(), "local", "BCGOS")
	if err := os.MkdirAll(dataRoot, 0o700); err != nil {
		t.Fatal(err)
	}

	owner, err := InitializeOwner(dataRoot)
	if err != nil {
		t.Fatalf("initialize owner atlas: %v", err)
	}
	if !owner.Available || owner.State != "available" {
		t.Fatalf("owner pointer = %+v, want an available root", owner)
	}

	for _, relative := range []string{"index.md", "learnings/index.md", "development/index.md", "concepts/index.md"} {
		path := filepath.Join(owner.Path, filepath.FromSlash(relative))
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("owner scaffold is missing %s: %v", relative, err)
		}
	}
}

func TestInitializeOwnerPreservesHandAuthoredContent(t *testing.T) {
	dataRoot := filepath.Join(t.TempDir(), "local", "BCGOS")
	if err := os.MkdirAll(dataRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	owner, err := InitializeOwner(dataRoot)
	if err != nil {
		t.Fatal(err)
	}

	// The owner owns this corpus and may edit it directly. A second bootstrap
	// must not reset what they wrote.
	edited := "# Learnings\n\nProcurement transformations stall at the category-owner layer.\n"
	index := filepath.Join(owner.Path, "learnings", "index.md")
	if err := os.WriteFile(index, []byte(edited), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := InitializeOwner(dataRoot); err != nil {
		t.Fatalf("second owner bootstrap: %v", err)
	}

	body, err := os.ReadFile(index)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != edited {
		t.Fatalf("second bootstrap overwrote a hand-authored page:\n got: %q\nwant: %q", body, edited)
	}
}

func TestInitializeOwnerRequiresADataRoot(t *testing.T) {
	if _, err := InitializeOwner("   "); err == nil {
		t.Fatal("owner bootstrap accepted an empty data root")
	}
}

func TestOwnerPointerReportsSymlinkedRootAsUnsafe(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires privileges on some Windows runners")
	}
	root := t.TempDir()
	dataRoot := filepath.Join(root, "local", "BCGOS")
	if err := os.MkdirAll(filepath.Join(dataRoot, "atlas"), 0o700); err != nil {
		t.Fatal(err)
	}
	external := filepath.Join(root, "external")
	if err := os.MkdirAll(external, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, filepath.Join(dataRoot, "atlas", "owner")); err != nil {
		t.Fatal(err)
	}

	status := Inspect(Options{DataRoot: dataRoot, WorkspacePath: filepath.Join(root, "absent")})
	if status.Owner.Available {
		t.Fatalf("a symlinked owner root reported as available: %+v", status.Owner)
	}
	if status.Owner.State != "unsafe" {
		t.Fatalf("owner state = %q, want %q so the caller can tell it apart from a root that simply does not exist", status.Owner.State, "unsafe")
	}
}

func TestOwnerPointerReportsMissingRootAsMissing(t *testing.T) {
	dataRoot := filepath.Join(t.TempDir(), "local", "BCGOS")
	status := Inspect(Options{DataRoot: dataRoot, WorkspacePath: filepath.Join(dataRoot, "absent")})
	if status.Owner.Available || status.Owner.State != "missing" {
		t.Fatalf("owner pointer = %+v, want a missing root", status.Owner)
	}
	if !strings.HasSuffix(filepath.ToSlash(status.Owner.Path), "atlas/owner") {
		t.Fatalf("owner pointer path = %q, want it to name the owner root", status.Owner.Path)
	}
}
