package markitdown

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolvePackReportsUnavailableWhenNotInstalled(t *testing.T) {
	pack, err := ResolvePack(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if pack.State != "unavailable" || len(pack.Command) != 0 {
		t.Fatalf("pack = %+v", pack)
	}
}

func TestResolvePackNeverExecutesWithoutApprovedVerifier(t *testing.T) {
	root := t.TempDir()
	packRoot := filepath.Join(root, "ingestion", "markitdown")
	if err := os.MkdirAll(filepath.Join(packRoot, "runtime"), 0o700); err != nil {
		t.Fatal(err)
	}
	pythonPath := filepath.Join(packRoot, "runtime", "python")
	scriptPath := filepath.Join(packRoot, "adapter.py")
	if err := os.WriteFile(pythonPath, []byte("python"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(scriptPath, []byte("script"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(packRoot, "pack.json"), []byte(packManifest(t, pythonPath, scriptPath)), 0o600); err != nil {
		t.Fatal(err)
	}

	pack, err := ResolvePack(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if pack.State != "unavailable" || !strings.Contains(pack.Reason, "verifier") {
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
	manifest := packManifest(t, filepath.Join(packRoot, "runtime", "python"), filepath.Join(packRoot, "adapter.py"))
	if err := os.WriteFile(filepath.Join(packRoot, "pack.json"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}

	pack, err := ResolvePack(root, func([]byte) error { return nil })
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

func TestResolvePackRejectsTamperedManagedFile(t *testing.T) {
	root := t.TempDir()
	packRoot := filepath.Join(root, "ingestion", "markitdown")
	if err := os.MkdirAll(filepath.Join(packRoot, "runtime"), 0o700); err != nil {
		t.Fatal(err)
	}
	pythonPath := filepath.Join(packRoot, "runtime", "python")
	scriptPath := filepath.Join(packRoot, "adapter.py")
	if err := os.WriteFile(pythonPath, []byte("python"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(scriptPath, []byte("script"), 0o700); err != nil {
		t.Fatal(err)
	}
	manifest := packManifest(t, pythonPath, scriptPath)
	if err := os.WriteFile(filepath.Join(packRoot, "pack.json"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(scriptPath, []byte("tampered"), 0o700); err != nil {
		t.Fatal(err)
	}

	pack, err := ResolvePack(root, func([]byte) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if pack.State != "unavailable" || !strings.Contains(pack.Reason, "verification failed") {
		t.Fatalf("pack = %+v", pack)
	}
}

func TestResolvePackRejectsTraversalWithoutLeakingReason(t *testing.T) {
	root := t.TempDir()
	packRoot := filepath.Join(root, "ingestion", "markitdown")
	if err := os.MkdirAll(packRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	manifest := `{"schema_version":1,"adapter":"markitdown","version":"0.1.6","python_path":"../python","script_path":"adapter.py","python_sha256":"0000000000000000000000000000000000000000000000000000000000000000","script_sha256":"0000000000000000000000000000000000000000000000000000000000000000","provenance":"bcgos-managed-installer"}` + "\n"
	if err := os.WriteFile(filepath.Join(packRoot, "pack.json"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	pack, err := ResolvePack(root, func([]byte) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if pack.State != "unavailable" || strings.Contains(pack.Reason, "../") {
		t.Fatalf("pack = %+v", pack)
	}
}

func packManifest(t *testing.T, pythonPath, scriptPath string) string {
	t.Helper()
	return `{"schema_version":1,"adapter":"markitdown","version":"0.1.6","python_path":"runtime/python","script_path":"adapter.py","python_sha256":"` + fileDigest(t, pythonPath) + `","script_sha256":"` + fileDigest(t, scriptPath) + `","provenance":"bcgos-managed-installer"}` + "\n"
}

func fileDigest(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}
