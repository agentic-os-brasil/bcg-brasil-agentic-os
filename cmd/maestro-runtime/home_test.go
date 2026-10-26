package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func homeTestDirectory(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func TestHomeInspectReportsUnconfirmedHomeWithoutCreatingIt(t *testing.T) {
	user := homeTestDirectory(t)
	product := homeTestDirectory(t)
	home := filepath.Join(user, "Maestro state")
	t.Setenv("HOME", user)
	t.Setenv("USERPROFILE", user)
	t.Setenv("MAESTRO_HOME", home)
	var out, diagnostic bytes.Buffer
	code := run([]string{"home", "inspect", "--product-root", product}, bytes.NewReader(nil), &out, &diagnostic)
	var result struct {
		Home       string `json:"home"`
		SelectedBy string `json:"selected_by"`
		Origin     string `json:"origin"`
		Writable   bool   `json:"writable"`
	}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("code=%d diagnostic=%s output=%s: %v", code, &diagnostic, &out, err)
	}
	if code != 0 || result.Home != home || result.SelectedBy != "env" || result.Origin != "fresh-unconfirmed" || result.Writable {
		t.Fatalf("code=%d result=%+v diagnostic=%s", code, result, &diagnostic)
	}
	if _, err := os.Stat(home); !os.IsNotExist(err) {
		t.Fatalf("inspection created HOME or hid an error: %v", err)
	}
	for _, root := range []string{user, product} {
		entries, err := os.ReadDir(root)
		if err != nil || len(entries) != 0 {
			t.Fatalf("inspection changed %s: %v %v", root, entries, err)
		}
	}
}

func TestHomeInspectRejectsInvalidSelectionAndArguments(t *testing.T) {
	t.Setenv("MAESTRO_HOME", "relative-state")
	product := homeTestDirectory(t)
	for _, args := range [][]string{
		{"home", "inspect", "--product-root", product},
		{"home", "inspect"},
		{"home", "activate", "--product-root", product},
		{"home", "inspect", "--product-root", product, "unexpected"},
	} {
		var out, diagnostic bytes.Buffer
		if code := run(args, bytes.NewReader(nil), &out, &diagnostic); code != 2 || diagnostic.Len() == 0 || out.Len() != 0 {
			t.Fatalf("args=%v code=%d output=%s diagnostic=%s", args, code, &out, &diagnostic)
		}
	}
}

func TestHomeInspectBoundsDiagnostics(t *testing.T) {
	user := homeTestDirectory(t)
	t.Setenv("HOME", user)
	t.Setenv("USERPROFILE", user)
	t.Setenv("MAESTRO_HOME", filepath.Join(user, ".maestro"))
	oversized := strings.Repeat("sensitive-path-", 4096)
	for _, args := range [][]string{
		{"home", "inspect", "--product-root", filepath.Join(user, oversized)},
		{"home", "inspect", "--" + oversized},
	} {
		var out, diagnostic bytes.Buffer
		code := run(args, bytes.NewReader(nil), &out, &diagnostic)
		if code != 2 || out.Len() != 0 || diagnostic.Len() == 0 || diagnostic.Len() > 1024 {
			t.Fatalf("unbounded error: code=%d stdout=%d stderr=%d", code, out.Len(), diagnostic.Len())
		}
	}
}
