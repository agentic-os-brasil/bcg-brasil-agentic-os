package docling_test

import (
	"context"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ingest"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ingest/docling"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
	"os"
	"path/filepath"
	"testing"
)

func TestUnseededRuntimeUnavailableWithoutCreatingState(t *testing.T) {
	root := t.TempDir()
	r := docling.New(docling.Options{DataRoot: root, CLIVersion: "0.1.0"})
	status := r.Status(context.Background())
	if status.State != "unavailable" || status.Reason != "authority_unavailable" {
		t.Fatalf("unseeded runtime = %+v", status)
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 0 {
		t.Fatalf("availability wrote state: %v %v", entries, err)
	}
}

func TestPartialPolicyCannotSilentlyWiden(t *testing.T) {
	root := t.TempDir()
	space := filepath.Join(root, "workspace")
	if err := os.Mkdir(space, 0700); err != nil {
		t.Fatal(err)
	}
	data := filepath.Join(root, "state")
	w, err := workspace.Initialize(workspace.Options{WorkspacePath: space, DataRoot: data})
	if err != nil {
		t.Fatal(err)
	}
	put(t, filepath.Join(space, "sample.docx"), read(t, "testdata/sample.docx"))
	r := docling.New(docling.Options{DataRoot: data, CLIVersion: "0.1.0"})
	result, err := r.Convert(context.Background(), ingest.Request{SourcePath: "sample.docx", WorkspacePath: space, Policy: ingest.Policy{MaxOutputBytes: 1, AllowedExts: map[string]bool{".docx": true}}}, w.WorkspaceID)
	if err != nil || result.Result.Status != ingest.StatusBlocked {
		t.Fatalf("partial policy widened: %+v %v", result.Result, err)
	}
}
