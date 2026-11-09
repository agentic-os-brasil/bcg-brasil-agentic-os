package workspacepolicy_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspacepolicy"
)

func TestToolUsesNativeSearchPathsAndPreservesRawPathRefusals(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root, outside := filepath.Join(base, "workspace"), filepath.Join(base, "outside")
	for _, path := range []string{filepath.Join(root, "nested"), outside} {
		if err := os.MkdirAll(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "nested", "note.md"), []byte("Synthetic local search.\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, tool string
		input      map[string]string
		allowed    bool
	}{
		{"local-glob", "Glob", map[string]string{"pattern": "*.md"}, true},
		{"nested-glob", "Glob", map[string]string{"pattern": "nested/*.md"}, true},
		{"literal-glob", "Glob", map[string]string{"pattern": "nested/note.md"}, true},
		{"missing-glob", "Glob", map[string]string{"pattern": "missing/*.md"}, true},
		{"grep-regex", "Grep", map[string]string{"pattern": `../../[A-Z]+/.*`}, true},
		{"grep-backslash-regex", "Grep", map[string]string{"pattern": `word\b`}, true},
		{"grep-glob", "Grep", map[string]string{"pattern": "text", "glob": "nested/*.md"}, true},
		{"relative-read", "Read", map[string]string{"file_path": "nested/note.md"}, true},
		{"raw-read-backslash", "Read", map[string]string{"file_path": `nested\note.md`}, false},
		{"raw-search-backslash", "Grep", map[string]string{"pattern": "text", "path": `nested\child`}, false},
		{"raw-glob-backslash", "Glob", map[string]string{"pattern": `nested\*.md`}, false},
		{"raw-read-parent", "Read", map[string]string{"file_path": "../outside/note.md"}, false},
		{"raw-glob-parent", "Glob", map[string]string{"pattern": "../outside/*"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body, err := json.Marshal(tc.input)
			if err != nil {
				t.Fatal(err)
			}
			if err := workspacepolicy.Tool(root, root, tc.tool, body); (err == nil) != tc.allowed {
				t.Fatalf("tool=%s allowed=%t got=%v", tc.tool, tc.allowed, err)
			}
		})
	}
	alias := filepath.Join(root, "alias")
	if runtime.GOOS == "windows" {
		shell := filepath.Join(os.Getenv("SystemRoot"), "System32", "cmd.exe")
		if out, err := exec.Command(shell, "/c", "mklink", "/J", alias, outside).CombinedOutput(); err != nil {
			t.Fatalf("required ordinary-user junction unavailable: %s %v", out, err)
		}
	} else if err := os.Symlink(outside, alias); err != nil {
		t.Fatal(err)
	}
	for _, tool := range []string{"Glob", "Grep"} {
		body, err := json.Marshal(map[string]string{"pattern": "alias/*"})
		if tool == "Grep" {
			body, err = json.Marshal(map[string]string{"pattern": ".", "path": "alias"})
		}
		if err != nil {
			t.Fatal(err)
		}
		if err := workspacepolicy.Tool(root, root, tool, body); err == nil {
			t.Fatalf("%s admitted the real outside alias", tool)
		}
	}
	if entries, err := os.ReadDir(outside); err != nil || len(entries) != 0 {
		t.Fatal("policy inspection changed the outside alias target", err)
	}
}
