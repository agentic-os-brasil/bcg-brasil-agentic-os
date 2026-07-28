package markitdown

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolvePackReportsUnavailableWhenNotInstalled(t *testing.T) {
	pack, err := ResolvePack(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if pack.State != "unavailable" || len(pack.Command) != 0 {
		t.Fatalf("pack = %+v", pack)
	}
}

func TestResolvePackReturnsOnlyManagedFiles(t *testing.T) {
	root := t.TempDir()
	packRoot := filepath.Join(root, "ingestion", "markitdown")
	if err := os.MkdirAll(filepath.Join(packRoot, "runtime"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{filepath.Join(packRoot, "runtime", "python"), filepath.Join(packRoot, "adapter.py")} {
		if err := os.WriteFile(path, []byte("fixture"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	manifest := `{"schema_version":1,"adapter":"markitdown","version":"0.1.6","python_path":"runtime/python","script_path":"adapter.py"}` + "\n"
	if err := os.WriteFile(filepath.Join(packRoot, "pack.json"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}

	pack, err := ResolvePack(root)
	if err != nil {
		t.Fatal(err)
	}
	if pack.State != "ready" || len(pack.Command) != 3 || pack.Command[2] != "--request-stdin" {
		t.Fatalf("pack = %+v", pack)
	}
	if !strings.HasPrefix(pack.Command[0], packRoot) || !strings.HasPrefix(pack.Command[1], packRoot) {
		t.Fatalf("pack escaped root: %+v", pack.Command)
	}
}

func TestResolvePackRejectsTraversalWithoutLeakingReason(t *testing.T) {
	root := t.TempDir()
	packRoot := filepath.Join(root, "ingestion", "markitdown")
	if err := os.MkdirAll(packRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	manifest := `{"schema_version":1,"adapter":"markitdown","version":"0.1.6","python_path":"../python","script_path":"adapter.py"}` + "\n"
	if err := os.WriteFile(filepath.Join(packRoot, "pack.json"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	pack, err := ResolvePack(root)
	if err != nil {
		t.Fatal(err)
	}
	if pack.State != "unavailable" || strings.Contains(pack.Reason, "../") {
		t.Fatalf("pack = %+v", pack)
	}
}
