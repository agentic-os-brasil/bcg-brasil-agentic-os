package docling_test

import (
	"archive/zip"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ingest"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ingest/docling"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
)

func TestMalformedPolicyFormatsRefuseBeforeRuntime(t *testing.T) {
	root := t.TempDir()
	space, data := filepath.Join(root, "workspace"), filepath.Join(root, "state")
	if err := os.Mkdir(space, 0700); err != nil {
		t.Fatal(err)
	}
	w, err := workspace.Initialize(workspace.Options{WorkspacePath: space, DataRoot: data})
	if err != nil {
		t.Fatal(err)
	}
	r := docling.New(docling.Options{DataRoot: data, CLIVersion: "0.1.0"})
	for name, body := range map[string][]byte{
		"bad.txt": {0xff, 0xfe}, "bad.csv": []byte("a,b\n\"unclosed"), "bad.json": []byte(`{"x":}`),
		"deep.json":     []byte(strings.Repeat("[", 129) + "0" + strings.Repeat("]", 129)),
		"trailing.json": []byte(`{} {}`), "external.xml": []byte(`<!DOCTYPE r SYSTEM "file:///etc/passwd"><r/>`),
		"entity.xml": []byte(`<!DOCTYPE r [<!ENTITY x SYSTEM "https://example.invalid/x">]><r>&x;</r>`),
		"mixed.xml":  []byte(`<r>text</wrong>`), "many.xml": []byte(`<a/><b/>`),
		"bad.xls": []byte("not BIFF"), "bad.xlsx": read(t, "testdata/sample.docx"), "bad.epub": read(t, "testdata/sample.docx"),
		"bad.html": []byte("<p>nul\x00</p>"),
	} {
		t.Run(name, func(t *testing.T) {
			put(t, filepath.Join(space, name), body)
			got, err := r.Convert(context.Background(), ingest.Request{SourcePath: name, WorkspacePath: space, Policy: ingest.DefaultPolicy()}, w.WorkspaceID)
			if err != nil || got.Result.Status != ingest.StatusBlocked || len(got.Result.Warnings) != 1 || got.Result.Warnings[0] != "source_format_invalid" || len(got.Document) > 0 || len(got.Markdown) > 0 {
				t.Fatalf("malformed source reached runtime: %+v %v", got.Result, err)
			}
		})
	}
}

func TestOversizedWorksheetRefusesBeforeRuntime(t *testing.T) {
	root := t.TempDir()
	space, data := filepath.Join(root, "workspace"), filepath.Join(root, "state")
	if err := os.Mkdir(space, 0700); err != nil {
		t.Fatal(err)
	}
	w, err := workspace.Initialize(workspace.Options{WorkspacePath: space, DataRoot: data})
	if err != nil {
		t.Fatal(err)
	}
	raw := read(t, "testdata/policy/sheets.xlsx")
	archive, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}
	var body bytes.Buffer
	writer := zip.NewWriter(&body)
	for _, file := range archive.File {
		reader, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		var content bytes.Buffer
		if _, err := content.ReadFrom(reader); err != nil {
			t.Fatal(err)
		}
		_ = reader.Close()
		data := content.Bytes()
		if file.Name == "xl/worksheets/sheet1.xml" {
			data = bytes.Replace(data, []byte(`A1:C3`), []byte(`A1:XFD1048576`), 1)
		}
		out, err := writer.Create(file.Name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := out.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	put(t, filepath.Join(space, "sparse.xlsx"), body.Bytes())
	r := docling.New(docling.Options{DataRoot: data, CLIVersion: "0.1.0"})
	got, err := r.Convert(context.Background(), ingest.Request{SourcePath: "sparse.xlsx", WorkspacePath: space, Policy: ingest.DefaultPolicy()}, w.WorkspaceID)
	if err != nil || got.Result.Status != ingest.StatusBlocked || len(got.Result.Warnings) != 1 || got.Result.Warnings[0] != "source_format_invalid" {
		t.Fatalf("unbounded worksheet reached runtime: %+v %v", got.Result, err)
	}
}

func TestLiteralTextRequiresAuthenticatedPack(t *testing.T) {
	root := t.TempDir()
	space, data := filepath.Join(root, "workspace"), filepath.Join(root, "state")
	if err := os.Mkdir(space, 0700); err != nil {
		t.Fatal(err)
	}
	w, err := workspace.Initialize(workspace.Options{WorkspacePath: space, DataRoot: data})
	if err != nil {
		t.Fatal(err)
	}
	put(t, filepath.Join(space, "literal.txt"), []byte("# literal title\n<tag>ação</tag>\n"))
	r := docling.New(docling.Options{DataRoot: data, CLIVersion: "0.1.0"})
	got, err := r.Convert(context.Background(), ingest.Request{SourcePath: "literal.txt", WorkspacePath: space, Policy: ingest.DefaultPolicy()}, w.WorkspaceID)
	if err != nil || got.Result.Status != ingest.StatusUnavailable || len(got.Result.Warnings) != 1 || got.Result.Warnings[0] != "authority_unavailable" {
		t.Fatalf("admitted literal text must reach pack authentication: %+v %v", got.Result, err)
	}
	if len(got.Document) != 0 || len(got.Markdown) != 0 {
		t.Fatal("unverified content returned")
	}
}

func unseededPolicyWorkspace(t *testing.T) (*docling.Runtime, string, string, string) {
	t.Helper()
	root := t.TempDir()
	space, data := filepath.Join(root, "workspace"), filepath.Join(root, "state")
	if err := os.Mkdir(space, 0700); err != nil {
		t.Fatal(err)
	}
	w, err := workspace.Initialize(workspace.Options{WorkspacePath: space, DataRoot: data})
	if err != nil {
		t.Fatal(err)
	}
	return docling.New(docling.Options{DataRoot: data, CLIVersion: "0.1.0"}), root, space, w.WorkspaceID
}

func TestPolicySourceConfinementBeforeRuntime(t *testing.T) {
	r, root, space, id := unseededPolicyWorkspace(t)
	original := []byte("literal scope proof\n")
	put(t, filepath.Join(space, "literal.txt"), original)
	refuse := func(t *testing.T, name, workspaceID, reason string) {
		t.Helper()
		got, err := r.Convert(context.Background(), ingest.Request{SourcePath: name, WorkspacePath: space, Policy: ingest.DefaultPolicy()}, workspaceID)
		if err != nil || got.Result.Status != ingest.StatusBlocked || len(got.Result.Warnings) != 1 || got.Result.Warnings[0] != reason || len(got.Document) > 0 || len(got.Markdown) > 0 {
			t.Fatalf("unsafe source reached runtime: %+v %v", got.Result, err)
		}
	}
	t.Run("foreign workspace", func(t *testing.T) { refuse(t, "literal.txt", "foreign-workspace", "workspace_unavailable") })
	t.Run("traversal", func(t *testing.T) { refuse(t, "../literal.txt", id, "source_scope_invalid") })
	t.Run("absolute", func(t *testing.T) { refuse(t, filepath.Join(space, "literal.txt"), id, "source_scope_invalid") })
	t.Run("symlink", func(t *testing.T) {
		name := filepath.Join(space, "alias.txt")
		if err := os.Symlink(filepath.Join(space, "literal.txt"), name); err != nil {
			t.Fatal(err)
		}
		defer os.Remove(name)
		refuse(t, "alias.txt", id, "source_not_admitted")
	})
	t.Run("hardlink", func(t *testing.T) {
		name := filepath.Join(root, "alias.txt")
		if err := os.Link(filepath.Join(space, "literal.txt"), name); err != nil {
			t.Fatal(err)
		}
		defer os.Remove(name)
		refuse(t, "literal.txt", id, "source_changed_or_unsafe")
	})
	t.Run("replaced workspace", func(t *testing.T) {
		saved := filepath.Join(root, "saved")
		if err := os.Rename(space, saved); err != nil {
			t.Fatal(err)
		}
		defer func() {
			_ = os.Remove(filepath.Join(space, "literal.txt"))
			_ = os.Remove(space)
			if err := os.Rename(saved, space); err != nil {
				t.Fatal(err)
			}
		}()
		if err := os.Mkdir(space, 0700); err != nil {
			t.Fatal(err)
		}
		put(t, filepath.Join(space, "literal.txt"), original)
		refuse(t, "literal.txt", id, "workspace_unavailable")
	})
	if !bytes.Equal(read(t, filepath.Join(space, "literal.txt")), original) {
		t.Fatal("original source changed")
	}
}

func TestPolicyContainerRefusalsBeforeRuntime(t *testing.T) {
	r, _, space, id := unseededPolicyWorkspace(t)
	for _, test := range []struct {
		name, fixture, member string
		payload               []byte
		maximum               int64
		corrupt               bool
	}{
		{"traversal.docx", "sample.docx", "../escape.bin", []byte("x"), 0, false},
		{"alias.xlsx", "policy/sheets.xlsx", "XL/WORKBOOK.XML", []byte("x"), 0, false},
		{"office.docx", "sample.docx", "word/media/unmanaged.wmf", []byte("x"), 0, false},
		{"large.epub", "policy/spine.epub", "large.bin", bytes.Repeat([]byte("x"), 2<<20), 1 << 20, false},
		{"crc.epub", "policy/spine.epub", "crc.bin", []byte("crc-content"), 0, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			raw := read(t, filepath.Join("testdata", test.fixture))
			source, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
			if err != nil {
				t.Fatal(err)
			}
			var body bytes.Buffer
			writer := zip.NewWriter(&body)
			for _, f := range source.File {
				reader, err := f.Open()
				if err != nil {
					t.Fatal(err)
				}
				var data bytes.Buffer
				if _, err := data.ReadFrom(reader); err != nil {
					t.Fatal(err)
				}
				_ = reader.Close()
				out, err := writer.Create(f.Name)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := out.Write(data.Bytes()); err != nil {
					t.Fatal(err)
				}
			}
			out, err := writer.CreateHeader(&zip.FileHeader{Name: test.member, Method: zip.Store})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := out.Write(test.payload); err != nil {
				t.Fatal(err)
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
			raw = body.Bytes()
			if test.corrupt {
				archive, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
				if err != nil {
					t.Fatal(err)
				}
				for _, f := range archive.File {
					if f.Name == test.member {
						offset, err := f.DataOffset()
						if err != nil {
							t.Fatal(err)
						}
						raw[offset] ^= 1
					}
				}
			}
			put(t, filepath.Join(space, test.name), raw)
			policy := ingest.DefaultPolicy()
			if test.maximum > 0 {
				policy.MaxOutputBytes = test.maximum
			}
			got, err := r.Convert(context.Background(), ingest.Request{SourcePath: test.name, WorkspacePath: space, Policy: policy}, id)
			reason := "source_format_invalid"
			if filepath.Ext(test.name) == ".docx" {
				reason = "docx_expansion_invalid"
			}
			if err != nil || got.Result.Status != ingest.StatusBlocked || len(got.Result.Warnings) != 1 || got.Result.Warnings[0] != reason || len(got.Document) > 0 {
				t.Fatalf("unsafe container reached runtime: %+v %v", got.Result, err)
			}
		})
	}
}
