//go:build darwin || linux

package setup

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installer"
)

// Run the real copy under the launcher's umask in a separate test process;
// another parallel test's file creation must never observe this mask.
func TestQualifiedClientCopyPreservesExactModesUnderPrivateUmask(t *testing.T) {
	const child = "MAESTRO_TEST_QUALIFIED_COPY_UMASK_CHILD"
	if os.Getenv(child) != "1" {
		executable, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(executable, "-test.run=^TestQualifiedClientCopyPreservesExactModesUnderPrivateUmask$", "-test.v")
		cmd.Env = append(os.Environ(), child+"=1")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("isolated private-umask copy: %s %v", out, err)
		}
		return
	}
	previous := syscall.Umask(0077)
	defer syscall.Umask(previous)
	root := t.TempDir()
	source := filepath.Join(root, "qualified-source")
	scripts := filepath.Join(source, "scripts")
	for _, path := range []string{source, scripts} {
		if err := os.Mkdir(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
	asset := filepath.Join(source, "skill.md")
	binary := filepath.Join(scripts, "maestro-mcp")
	for path, fixture := range map[string]struct {
		body string
		mode os.FileMode
	}{
		asset:  {"synthetic instruction asset", 0644},
		binary: {"synthetic nonexecuted binary", 0755},
	} {
		if err := os.WriteFile(path, []byte(fixture.body), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, fixture.mode); err != nil {
			t.Fatal(err)
		}
	}
	expected, err := installer.ClientTreeDigest(source)
	if err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(root, "candidate")
	if err := copyQualifiedClientTree(source, destination, expected); err != nil {
		t.Fatalf("exact qualified copy under umask0077: %v", err)
	}
	actual, err := installer.ClientTreeDigest(destination)
	if err != nil || actual != expected {
		t.Fatalf("mode-sensitive destination differs: %s %v", actual, err)
	}
	for path, want := range map[string]os.FileMode{destination: 0700, filepath.Join(destination, "scripts"): 0700, filepath.Join(destination, "skill.md"): 0644, filepath.Join(destination, "scripts", "maestro-mcp"): 0755} {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != want {
			t.Fatalf("permission mismatch for %s: %v %v; want %04o", path, info, err, want)
		}
	}
	after, err := installer.ClientTreeDigest(source)
	if err != nil || after != expected {
		t.Fatal("source changed during copy", err)
	}
	parent, err := os.Stat(root)
	if err != nil || parent.Mode().Perm()&0077 != 0 {
		t.Fatal("private candidate parent changed", err)
	}
	t.Run("existing-destination-preserved", func(t *testing.T) {
		occupied := filepath.Join(root, "occupied")
		sentinel := []byte("existing candidate is not replaceable")
		if err := os.WriteFile(occupied, sentinel, 0600); err != nil {
			t.Fatal(err)
		}
		if err := copyQualifiedClientTree(source, occupied, expected); err == nil {
			t.Fatal("existing destination overwritten")
		}
		got, err := os.ReadFile(occupied)
		if err != nil || string(got) != string(sentinel) {
			t.Fatal("occupied destination changed", err)
		}
	})
	t.Run("changed-source-refused-before-copy", func(t *testing.T) {
		rejected := filepath.Join(root, "must-not-exist")
		if err := os.WriteFile(asset, []byte("changed after qualification"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := copyQualifiedClientTree(source, rejected, expected); err == nil || !strings.Contains(err.Error(), "qualified native source changed") {
			t.Fatalf("changed qualified source admitted: %v", err)
		}
		if _, err := os.Lstat(rejected); !os.IsNotExist(err) {
			t.Fatal("changed source created destination")
		}
	})
}
