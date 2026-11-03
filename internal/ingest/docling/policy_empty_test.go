//go:build darwin || linux

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
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
)

// Reuse only an explicitly seeded, canonically verified engineering fixture.
// This proof never runs the retained E4 runtime or an installed pathname directly.
func TestRetainedPolicyWorkbookContent(t *testing.T) {
	r, root := retainedRuntime(t)
	space := filepath.Join(root, "workspace")
	w, err := workspace.Inspect(space, filepath.Join(root, "state"))
	if err != nil {
		t.Fatal(err)
	}
	proof := os.Getenv("MAESTRO_POLICY_WORKBOOK_EVIDENCE")
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
		body, err := json.Marshal(metadata)
		if err != nil {
			t.Fatal(err)
		}
		put(t, filepath.Join(proof, name+".result.json"), body)
		if len(document) > 0 {
			put(t, filepath.Join(proof, name+".document.json"), document)
			put(t, filepath.Join(proof, name+".md"), markdown)
		}
	}
	for _, name := range []string{"empty.xls", "empty.xlsx", "uncached.xls"} {
		t.Run(name, func(t *testing.T) {
			body := read(t, filepath.Join("testdata/policy", name))
			put(t, filepath.Join(space, name), body)
			got, err := r.Convert(context.Background(), ingest.Request{SourcePath: name, WorkspacePath: space, Policy: ingest.DefaultPolicy()}, w.WorkspaceID)
			save(t, name, got, got.Document, got.Markdown)
			if err != nil || got.Result.Status != ingest.StatusPartial || len(got.Result.Warnings) != 1 || got.Result.Warnings[0] != "converter_content_empty" || len(got.Document) > 0 || len(got.Markdown) > 0 {
				t.Fatalf("sheet-name metadata became usable content: %+v %v", got.Result, err)
			}
			if !bytes.Equal(body, read(t, filepath.Join(space, name))) {
				t.Fatal("empty original changed")
			}
		})
	}
	for _, name := range []string{"legacy.xls", "sheets.xlsx"} {
		t.Run(name, func(t *testing.T) {
			body := read(t, filepath.Join("testdata/policy", name))
			put(t, filepath.Join(space, name), body)
			got, err := r.Convert(context.Background(), ingest.Request{SourcePath: name, WorkspacePath: space, Policy: ingest.DefaultPolicy()}, w.WorkspaceID)
			save(t, name, got, got.Document, got.Markdown)
			marker := "37"
			if strings.HasSuffix(name, ".xlsx") {
				marker = "29"
			}
			if err != nil || got.Result.Status != ingest.StatusUsable || !bytes.Contains(got.Markdown, []byte(marker)) {
				t.Fatalf("real cell content refused: %+v %v", got.Result, err)
			}
			if !bytes.Equal(body, read(t, filepath.Join(space, name))) {
				t.Fatal("populated original changed")
			}
		})
	}
	entries, err := os.ReadDir(filepath.Join(root, "state/ingestion/docling/executions"))
	if err != nil || len(entries) != 0 {
		t.Fatalf("execution snapshot retained: %v %v", entries, err)
	}
}
