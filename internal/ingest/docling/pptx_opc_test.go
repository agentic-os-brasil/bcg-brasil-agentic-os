package docling_test

import (
	"context"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ingest"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeDocumentPreflightBeforeAuthentication(t *testing.T) {
	r, _, space, id := unseededPolicyWorkspace(t)
	for _, name := range []string{"native.pdf", "native.pptx"} {
		put(t, filepath.Join(space, name), read(t, "testdata/documents/"+name))
		got, err := r.Convert(context.Background(), ingest.Request{SourcePath: name, WorkspacePath: space, Policy: ingest.DefaultPolicy()}, id)
		if err != nil || got.Result.Status != ingest.StatusUnavailable || len(got.Result.Warnings) != 1 || got.Result.Warnings[0] != "authority_unavailable" {
			t.Fatalf("well-formed source must require authentication: %+v %v", got.Result, err)
		}
	}
	base := read(t, "testdata/documents/native.pptx")
	for _, test := range []struct {
		name string
		edit func(map[string]string)
	}{
		{"alternate presentation", func(p map[string]string) {
			p["ppt/other.xml"] = p["ppt/presentation.xml"]
			p["[Content_Types].xml"] = strings.ReplaceAll(p["[Content_Types].xml"], "/ppt/presentation.xml", "/ppt/other.xml")
		}},
		{"duplicate slide selection", func(p map[string]string) {
			p["ppt/_rels/presentation.xml.rels"] = strings.ReplaceAll(p["ppt/_rels/presentation.xml.rels"], "slides/slide2.xml", "slides/slide1.xml")
		}},
		{"external relationship", func(p map[string]string) {
			p["ppt/_rels/presentation.xml.rels"] = strings.Replace(p["ppt/_rels/presentation.xml.rels"], `Target="slides/slide2.xml"`, `Target="https://127.0.0.1/slide.xml" TargetMode="External"`, 1)
		}},
		{"macro presentation", func(p map[string]string) {
			p["[Content_Types].xml"] = strings.ReplaceAll(p["[Content_Types].xml"], "application/vnd.openxmlformats-officedocument.presentationml.presentation.main+xml", "application/vnd.ms-powerpoint.presentation.macroEnabled.main+xml")
		}},
		{"metafile", func(p map[string]string) { p["ppt/media/image.emf"] = "metafile bait" }},
		{"macro payload", func(p map[string]string) { p["ppt/vbaProject.bin"] = "macro bait" }},
		{"DTD", func(p map[string]string) {
			p["ppt/slides/slide1.xml"] = `<!DOCTYPE s SYSTEM "file:///external">` + p["ppt/slides/slide1.xml"]
		}},
		{"slide type confusion", func(p map[string]string) {
			p["[Content_Types].xml"] = strings.ReplaceAll(p["[Content_Types].xml"], "presentationml.slide+xml", "presentationml.notesSlide+xml")
		}},
		{"root decoy", func(p map[string]string) {
			p["_rels/.rels"] = strings.ReplaceAll(p["_rels/.rels"], "ppt/presentation.xml", "ppt/slides/slide1.xml")
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			body := editXLSX(t, base, test.edit)
			name := "unsafe.pptx"
			put(t, filepath.Join(space, name), body)
			got, err := r.Convert(context.Background(), ingest.Request{SourcePath: name, WorkspacePath: space, Policy: ingest.DefaultPolicy()}, id)
			if err != nil || got.Result.Status != ingest.StatusBlocked || len(got.Result.Warnings) != 1 || got.Result.Warnings[0] != "source_format_invalid" || len(got.Document) != 0 || len(got.Markdown) != 0 {
				t.Fatalf("unsafe OPC reached authentication: %+v %v", got.Result, err)
			}
		})
	}
}
