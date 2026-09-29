package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMigrationCommandReturnsVisibleBlockedState(t *testing.T) {
	root := t.TempDir()
	for _, p := range []string{"data/owner", "brain/owner"} {
		if err := os.MkdirAll(filepath.Join(root, p), 0700); err != nil {
			t.Fatal(err)
		}
	}
	var out, diagnostic bytes.Buffer
	code := run([]string{"migration", "--project", root}, strings.NewReader(""), &out, &diagnostic)
	var result map[string]any
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if code != 2 || result["state"] != "blocked" {
		t.Fatalf("%d %s %s", code, &out, &diagnostic)
	}
}
func TestContradictoryReadOnlyAndRollbackNeverWrite(t *testing.T) {
	root := t.TempDir()
	var out, diagnostic bytes.Buffer
	if code := run([]string{"migration", "--project", root, "--status", "--rollback", "anything"}, strings.NewReader(""), &out, &diagnostic); code != 2 {
		t.Fatal(code)
	}
	entries, _ := os.ReadDir(root)
	if len(entries) != 0 {
		t.Fatal("invalid command wrote files")
	}
}
