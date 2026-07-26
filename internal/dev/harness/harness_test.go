package harness

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestFindRootWalksUpward(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := FindRoot(nested)
	if err != nil {
		t.Fatalf("FindRoot() error = %v", err)
	}
	if got != root {
		t.Fatalf("FindRoot() = %q, want %q", got, root)
	}
}

func TestChildEnvironmentRemovesGitLocalVariablesOnly(t *testing.T) {
	environment := []string{
		"PATH=/bin",
		"GIT_INDEX_FILE=/tmp/parent.index",
		"GIT_DIR=/repo/.git",
		"GIT_COMMON_DIR=/repo/.git",
		"GIT_TRACE=1",
	}
	got := childEnvironment(environment)
	for _, removed := range []string{
		"GIT_INDEX_FILE=/tmp/parent.index",
		"GIT_DIR=/repo/.git",
		"GIT_COMMON_DIR=/repo/.git",
	} {
		if slices.Contains(got, removed) {
			t.Fatalf("childEnvironment() retained %q: %v", removed, got)
		}
	}
	if !slices.Contains(got, "PATH=/bin") || !slices.Contains(got, "GIT_TRACE=1") {
		t.Fatalf("childEnvironment() unexpectedly removed unrelated variables: %v", got)
	}
}
