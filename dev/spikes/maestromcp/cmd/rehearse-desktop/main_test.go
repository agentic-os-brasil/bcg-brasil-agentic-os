package main

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestDestinationRefusesExistingAndAliasedOutput(t *testing.T) {
	base := t.TempDir()
	kept := filepath.Join(base, "existing")
	if err := os.Mkdir(kept, 0700); err != nil {
		t.Fatal(err)
	}
	witness := filepath.Join(kept, "keep.txt")
	if err := os.WriteFile(witness, []byte("preserve existing output"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := createDestination(kept); err == nil {
		t.Fatal("existing directory accepted")
	}
	if got, err := os.ReadFile(witness); err != nil || string(got) != "preserve existing output" {
		t.Fatal("existing output was changed")
	}
	alias := filepath.Join(base, "alias")
	if err := os.Symlink(base, alias); err == nil {
		if err := createDestination(filepath.Join(alias, "new")); err == nil {
			t.Fatal("aliased parent accepted")
		}
	}
	if err := createDestination(filepath.Join(base, "new physical output")); err != nil {
		t.Fatal(err)
	}
}

func TestRaceRequiresExplicitStoreConflict(t *testing.T) {
	for _, code := range []string{"revision_conflict", "item_busy", "generic error", "revision_conflict /private/path", ""} {
		r := &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: code}}}
		want := ""
		if code == "revision_conflict" || code == "item_busy" {
			want = code
		}
		if got := conflictCode(r); got != want {
			t.Fatalf("classification=%q for %q, want %q", got, code, want)
		}
	}
	if conflictCode(nil) != "" || conflictCode(&mcp.CallToolResult{}) != "" {
		t.Fatal("non-error classified as a conflict")
	}
}

func archiveFixture(t *testing.T, entries map[string][]byte) string {
	t.Helper()
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	for name, data := range entries {
		h := &zip.FileHeader{Name: name, Method: zip.Deflate}
		h.SetMode(0700)
		f, err := w.CreateHeader(h)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = f.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "fixture.mcpb")
	if err := os.WriteFile(path, b.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestExtractOnlyGeneratedPathsPreservesExecutableAndRefusesOverwrite(t *testing.T) {
	archive := archiveFixture(t, map[string][]byte{"manifest.json": []byte("{}"), "server/maestro-mcp-spike": []byte("synthetic binary")})
	destination := filepath.Join(t.TempDir(), "expanded")
	if err := extractBundle(archive, destination); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(destination, "server", "maestro-mcp-spike")
	if got, err := os.ReadFile(binary); err != nil || string(got) != "synthetic binary" {
		t.Fatal("binary bytes changed")
	}
	if info, err := os.Stat(binary); err != nil || info.Mode().Perm()&0100 == 0 {
		t.Fatal("executable permission lost")
	}
	if err := extractBundle(archive, destination); err == nil {
		t.Fatal("existing extraction accepted")
	}
}

func TestExtractRejectsTraversalAndUnexpectedFileBeforeCreatingOutput(t *testing.T) {
	for _, name := range []string{"../escape", "/absolute", "server/../../escape", "server\\escape", "unexpected.txt"} {
		t.Run(name, func(t *testing.T) {
			archive := archiveFixture(t, map[string][]byte{"manifest.json": []byte("{}"), "server/maestro-mcp-spike": []byte("bin"), name: []byte("bad")})
			destination := filepath.Join(t.TempDir(), "expanded")
			if err := extractBundle(archive, destination); err == nil {
				t.Fatal("unsafe entry accepted")
			}
			if _, err := os.Stat(destination); !os.IsNotExist(err) {
				t.Fatal("unsafe archive created output")
			}
		})
	}
}

func TestExtractRejectsDuplicateAndSymlinkEntriesBeforeCreatingOutput(t *testing.T) {
	for _, kind := range []string{"duplicate", "symlink", "special"} {
		t.Run(kind, func(t *testing.T) {
			var b bytes.Buffer
			w := zip.NewWriter(&b)
			for i, name := range []string{"manifest.json", "server/maestro-mcp-spike"} {
				h := &zip.FileHeader{Name: name, Method: zip.Deflate}
				h.SetMode(0700)
				if i == 1 {
					switch kind {
					case "duplicate":
						h.Name = "manifest.json"
					case "symlink":
						h.SetMode(os.ModeSymlink | 0700)
					case "special":
						h.SetMode(os.ModeNamedPipe | 0700)
					}
				}
				f, err := w.CreateHeader(h)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := f.Write([]byte("fixture")); err != nil {
					t.Fatal(err)
				}
			}
			if err := w.Close(); err != nil {
				t.Fatal(err)
			}
			base := t.TempDir()
			archive := filepath.Join(base, "bad.mcpb")
			if err := os.WriteFile(archive, b.Bytes(), 0600); err != nil {
				t.Fatal(err)
			}
			destination := filepath.Join(base, "expanded")
			if err := extractBundle(archive, destination); err == nil {
				t.Fatal("unsafe archive accepted")
			}
			if _, err := os.Stat(destination); !os.IsNotExist(err) {
				t.Fatal("unsafe archive created output")
			}
		})
	}
}
