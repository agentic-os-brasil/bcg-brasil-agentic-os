package scheduler

import (
	"os"
	"path/filepath"
	"testing"
)

// EnsurePrivateDirectory is the only way product code creates a private tree,
// and internal/atlas relies on it to bring <dataRoot>/atlas/owner into
// existence: two components that do not exist yet. The walk must therefore be
// able to create a component, not merely open one that is already there.
func TestEnsurePrivateDirectoryCreatesMissingComponents(t *testing.T) {
	root, err := CanonicalPrivatePath(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(root, "atlas", "owner")

	if err := EnsurePrivateDirectory(directory); err != nil {
		t.Fatalf("create nested private directory: %v", err)
	}

	info, err := os.Lstat(directory)
	if err != nil {
		t.Fatalf("nested private directory was not created: %v", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("created path is not a plain directory: %v", info.Mode())
	}

	// Repeating the call must open what is already there rather than fail.
	if err := EnsurePrivateDirectory(directory); err != nil {
		t.Fatalf("reopen existing private directory: %v", err)
	}
}

func TestEnsurePrivateDirectoryCreatesDeeperSegmentTree(t *testing.T) {
	root, err := CanonicalPrivatePath(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(root, "atlas", "owner", "development", "retros")

	if err := EnsurePrivateDirectory(directory); err != nil {
		t.Fatalf("create deep private directory: %v", err)
	}
	if err := ValidatePrivateDirectory(directory); err != nil {
		t.Fatalf("validate created private directory: %v", err)
	}
}
