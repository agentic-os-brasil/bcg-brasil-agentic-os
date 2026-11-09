//go:build darwin || linux

package docling_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ingest"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
)

func TestRetainedXLSXNativeLimits(t *testing.T) {
	r, root := retainedRuntime(t)
	space := filepath.Join(root, "workspace")
	w, err := workspace.Inspect(space, filepath.Join(root, "state"))
	if err != nil {
		t.Fatal(err)
	}
	proof := os.Getenv("MAESTRO_XLSX_LIMITS_EVIDENCE")
	if proof != "" {
		if err := os.Mkdir(proof, 0700); err != nil {
			t.Fatal(err)
		}
	}
	save := func(t *testing.T, name string, metadata any, document, markdown []byte) {
		t.Helper()
		if proof == "" {
			return
		}
		raw, err := json.Marshal(metadata)
		if err != nil {
			t.Fatal(err)
		}
		put(t, filepath.Join(proof, name+".result.json"), raw)
		if len(document) > 0 {
			put(t, filepath.Join(proof, name+".document.json"), document)
			put(t, filepath.Join(proof, name+".md"), markdown)
		}
	}
	t.Run("safe clipping refusal", func(t *testing.T) {
		// The pinned clipper shrinks this single column to its only value at
		// A1 before openpyxl allocates merged cells. No large geometry is run.
		body := workbookFixture(t, worksheetFixture("A1", `<row r="1"><c r="A1" t="inlineStr"><is><t>CLIP_ANCHOR</t></is></c></row>`, []string{"A1:A100002"}, nil))
		put(t, filepath.Join(space, "safe-clip.xlsx"), body)
		got, err := r.Convert(context.Background(), ingest.Request{SourcePath: "safe-clip.xlsx", WorkspacePath: space, Policy: ingest.DefaultPolicy()}, w.WorkspaceID)
		save(t, "safe-clip", got, got.Document, got.Markdown)
		if got.Result.Status == ingest.StatusUsable {
			var d struct {
				Tables []struct {
					Data struct {
						Rows  int `json:"num_rows"`
						Cells []struct {
							Text string `json:"text"`
							Span int    `json:"row_span"`
						} `json:"table_cells"`
					} `json:"data"`
				} `json:"tables"`
			}
			if json.Unmarshal(got.Document, &d) != nil || len(d.Tables) != 1 || d.Tables[0].Data.Rows != 1 || len(d.Tables[0].Data.Cells) != 1 || d.Tables[0].Data.Cells[0].Text != "CLIP_ANCHOR" || d.Tables[0].Data.Cells[0].Span != 1 {
				t.Fatalf("safe clipping oracle unexpected: %+v", d)
			}
			t.Log("Observed pinned native loss: declared row span=100002, actual span=1, usable/structured, no clipping warning in result")
		}
		if err != nil || got.Result.Status != ingest.StatusBlocked || len(got.Result.Warnings) != 1 || got.Result.Warnings[0] != "source_format_invalid" || len(got.Document) > 0 || len(got.Markdown) > 0 {
			t.Fatalf("over-budget input was not refused: %+v %v", got.Result, err)
		}
		if !bytes.Equal(body, read(t, filepath.Join(space, "safe-clip.xlsx"))) {
			t.Fatal("original changed")
		}
	})
	t.Run("ordinary populated", func(t *testing.T) {
		body := read(t, "testdata/policy/sheets.xlsx")
		put(t, filepath.Join(space, "ordinary.xlsx"), body)
		got, err := r.Convert(context.Background(), ingest.Request{SourcePath: "ordinary.xlsx", WorkspacePath: space, Policy: ingest.DefaultPolicy()}, w.WorkspaceID)
		save(t, "ordinary", got, got.Document, got.Markdown)
		if err != nil || got.Result.Status != ingest.StatusUsable || !bytes.Contains(got.Markdown, []byte("Metrics")) || !bytes.Contains(got.Markdown, []byte("29")) {
			t.Fatalf("populated XLSX refused: %+v %v", got.Result, err)
		}
		if !bytes.Equal(got.Document, read(t, filepath.Join(root, "outputs/sheets.xlsx.document.json"))) || !bytes.Equal(got.Markdown, read(t, filepath.Join(root, "outputs/sheets.xlsx.md"))) {
			t.Fatal("ordinary output differs from the independently oracled matrix")
		}
		if !bytes.Equal(body, read(t, filepath.Join(space, "ordinary.xlsx"))) {
			t.Fatal("original changed")
		}
	})
	for _, test := range []struct {
		name, rows string
		links      []string
		values     []string
	}{
		{"inferred coordinates", `<row><c><v>11</v></c><c><v>12</v></c></row><row r="3"><c r="D3"><v>31</v></c><c><v>32</v></c></row>`, nil, []string{"11", "12", "31", "32"}},
		{"bounded hyperlink", `<row r="1"><c r="A1" t="inlineStr"><is><t>LINK_ANCHOR</t></is></c></row>`, []string{"A1:A4"}, []string{"LINK_ANCHOR", "INTERNAL_TARGET"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			body := workbookFixture(t, worksheetFixture("", test.rows, nil, test.links))
			put(t, filepath.Join(space, "bounded-native.xlsx"), body)
			got, err := r.Convert(context.Background(), ingest.Request{SourcePath: "bounded-native.xlsx", WorkspacePath: space, Policy: ingest.DefaultPolicy()}, w.WorkspaceID)
			save(t, test.name, got, got.Document, got.Markdown)
			if err != nil || got.Result.Status != ingest.StatusUsable {
				t.Fatalf("bounded parser case refused: %+v %v", got.Result, err)
			}
			var d policyDocument
			if json.Unmarshal(got.Document, &d) != nil {
				t.Fatal("invalid document")
			}
			values := map[string]bool{}
			for _, table := range d.Tables {
				for _, cell := range table.Data.Cells {
					values[cell.Text] = true
				}
			}
			for _, value := range test.values {
				if !values[value] || !bytes.Contains(got.Markdown, []byte(value)) {
					t.Fatalf("independent bounded content oracle missing %q", value)
				}
			}
			if test.name == "inferred coordinates" {
				if len(d.Tables) != 2 || d.Tables[0].Data.Rows != 1 || d.Tables[0].Data.Cols != 2 || d.Tables[1].Data.Rows != 1 || d.Tables[1].Data.Cols != 2 {
					t.Fatalf("inferred table shape: %+v", d.Tables)
				}
				for table, want := range [][]string{{"11", "12"}, {"31", "32"}} {
					for column, value := range want {
						found := false
						for _, cell := range d.Tables[table].Data.Cells {
							if cell.Row == 0 && cell.Col == column && cell.Text == value {
								found = true
							}
						}
						if !found {
							t.Fatalf("inferred table %d column %d lost %q", table, column, value)
						}
					}
				}
			}
			if test.name == "bounded hyperlink" {
				if len(d.Tables) != 1 || d.Tables[0].Data.Rows != 4 || d.Tables[0].Data.Cols != 1 {
					t.Fatalf("bounded hyperlink span lost: %+v", d.Tables)
				}
				for row := 1; row <= 3; row++ {
					found := false
					for _, cell := range d.Tables[0].Data.Cells {
						if cell.Row == row && cell.Col == 0 && cell.Text == "INTERNAL_TARGET" {
							found = true
						}
					}
					if !found {
						t.Fatalf("hyperlink-expanded row %d lost", row)
					}
				}
			}
			if !bytes.Equal(body, read(t, filepath.Join(space, "bounded-native.xlsx"))) {
				t.Fatal("original changed")
			}
		})
	}
	entries, err := os.ReadDir(filepath.Join(root, "state/ingestion/docling/executions"))
	if err != nil || len(entries) != 0 {
		t.Fatalf("execution snapshot retained: %v %v", entries, err)
	}
}
