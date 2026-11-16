package docling_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ingest"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ingest/docling"
)

func TestAuthenticatedNativeDocuments(t *testing.T) {
	if os.Getenv("MAESTRO_POLICY_TEST_PROFILE") != docling.DocumentsProfile {
		t.Skip("explicit fresh native-document profile fixture required")
	}
	r, root, space, id := policyFixtureRuntime(t)
	descriptor, err := docling.DescribeProfile("darwin", "arm64", docling.DocumentsProfile)
	if err != nil {
		t.Fatal(err)
	}
	outputs := filepath.Join(root, "outputs")
	if err := os.Mkdir(outputs, 0700); err != nil {
		t.Fatal(err)
	}
	convert := func(t *testing.T, name string) docling.Conversion {
		t.Helper()
		body := read(t, "testdata/documents/"+name)
		put(t, filepath.Join(space, name), body)
		got, err := r.Convert(context.Background(), ingest.Request{SourcePath: name, WorkspacePath: space, Policy: ingest.DefaultPolicy()}, id)
		if err != nil {
			t.Fatal(err)
		}
		metadata, _ := json.MarshalIndent(got, "", "  ")
		put(t, filepath.Join(outputs, name+".result.json"), metadata)
		if got.Result.SourceSHA256 != sum(body) || !bytes.Equal(body, read(t, filepath.Join(space, name))) {
			t.Fatal("source identity changed")
		}
		if len(got.Document) > 0 {
			put(t, filepath.Join(outputs, name+".document.json"), got.Document)
			put(t, filepath.Join(outputs, name+".md"), got.Markdown)
		}
		return got
	}
	for _, name := range []string{"native.pdf", "native.pptx"} {
		t.Run(name, func(t *testing.T) {
			got := convert(t, name)
			if got.Result.Status != ingest.StatusUsable || !got.Provenance.NetworkDenied || got.Provenance.LockSHA256 != descriptor.LockSHA256 {
				t.Fatalf("actual conversion: %+v", got)
			}
			var document struct {
				Schema string                     `json:"schema_name"`
				Pages  map[string]json.RawMessage `json:"pages"`
				Texts  []struct {
					Text string `json:"text"`
					Prov []struct {
						Page int            `json:"page_no"`
						BBox map[string]any `json:"bbox"`
					} `json:"prov"`
				} `json:"texts"`
				Tables   []json.RawMessage `json:"tables"`
				Pictures []struct {
					Image json.RawMessage `json:"image"`
				} `json:"pictures"`
			}
			if json.Unmarshal(got.Document, &document) != nil || document.Schema != "DoclingDocument" || len(document.Pages) != 2 {
				t.Fatal("actual structured two-page document missing")
			}
			pages := map[int]bool{}
			for _, item := range document.Texts {
				if strings.TrimSpace(item.Text) != "" {
					for _, p := range item.Prov {
						if len(p.BBox) != 0 {
							pages[p.Page] = true
						}
					}
				}
			}
			if !pages[1] || !pages[2] {
				t.Fatal("page/bounding-box provenance missing", pages)
			}
			for _, picture := range document.Pictures {
				if len(picture.Image) > 0 && string(picture.Image) != "null" {
					t.Fatal("image rendering happened")
				}
			}
			if name == "native.pdf" {
				if got.Result.Parser != "docling.pdf_native_text" || got.Result.Fidelity != ingest.FidelityTextual || len(got.Result.Warnings) != 1 || got.Result.Warnings[0] != "pdf_native_text_only" || !bytes.Contains(got.Markdown, []byte("PAGE ONE 731")) || !bytes.Contains(got.Markdown, []byte("PAGE TWO 947")) || len(document.Tables) != 0 {
					t.Fatal("native PDF content/fidelity oracle", got.Result)
				}
			} else {
				if got.Result.Parser != "docling.pptx" || got.Result.Fidelity != ingest.FidelityStructured || !bytes.Contains(got.Markdown, []byte("FIRST 421")) || !bytes.Contains(got.Markdown, []byte("SECOND 683")) || len(document.Tables) != 1 || !bytes.Contains(document.Tables[0], []byte(`"text": "41"`)) {
					t.Fatal("native PPTX text/table oracle", got.Result)
				}
			}
			t.Logf("actual %s parser=%s fidelity=%s document=%d markdown=%d", name, got.Result.Parser, got.Result.Fidelity, len(got.Document), len(got.Markdown))
		})
	}
	for name, want := range map[string]string{"nontext.pdf": "pdf_native_text_unavailable", "scanned.pdf": "pdf_native_text_unavailable", "mixed-image.pdf": "pdf_native_text_unavailable", "mixed.pdf": "pdf_native_text_unavailable", "protected.pdf": "pdf_protected_unavailable"} {
		t.Run(name, func(t *testing.T) {
			got := convert(t, name)
			if got.Result.Status != ingest.StatusUnavailable || len(got.Result.Warnings) != 1 || got.Result.Warnings[0] != want || len(got.Document) != 0 || len(got.Markdown) != 0 {
				t.Fatalf("finite PDF refusal: %+v", got)
			}
		})
	}
	status := r.Status(context.Background())
	if status.State != "ready" || status.Profile != docling.DocumentsProfile {
		t.Fatal("actual verified profile status", status)
	}
}
