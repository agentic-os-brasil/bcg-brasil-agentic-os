package markitdown

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ingest"
)

func TestMarkItDownAdapterWritesBoundedMetadataSafeArtifact(t *testing.T) {
	t.Setenv("GO_WANT_HELPER_PROCESS", "1")
	root := t.TempDir()
	sourcePath := filepath.Join(root, "brief.docx")
	if err := os.WriteFile(sourcePath, []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	artifactRoot := filepath.Join(root, "data")

	result, err := (Adapter{
		Command:      []string{os.Args[0], "-test.run=TestMarkItDownHelperProcess"},
		ArtifactRoot: artifactRoot,
		WorkspaceID:  "workspace-a",
		Route:        "markitdown_fallback",
		Policy:       ingest.DefaultPolicy(),
		Timeout:      5 * time.Second,
	}).Convert(context.Background(), ingest.Request{SourcePath: sourcePath, WorkspacePath: root, Policy: ingest.DefaultPolicy()})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != ingest.StatusUsable || result.Fidelity != ingest.FidelityTextual {
		t.Fatalf("result = %+v", result)
	}
	if result.Route != "markitdown_fallback" {
		t.Fatalf("route = %q", result.Route)
	}
	if strings.Contains(result.ArtifactRef, string(filepath.Separator)+"Users"+string(filepath.Separator)) {
		t.Fatalf("artifact ref leaked absolute path: %q", result.ArtifactRef)
	}
	artifactPath := filepath.Join(artifactRoot, filepath.FromSlash(result.ArtifactRef))
	body, err := os.ReadFile(artifactPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "# Converted\n\nLocal fixture\n" {
		t.Fatalf("artifact = %q", body)
	}
}

func TestMarkItDownAdapterReportsUnavailableWithoutCommand(t *testing.T) {
	root := t.TempDir()
	sourcePath := filepath.Join(root, "brief.docx")
	if err := os.WriteFile(sourcePath, []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := (Adapter{ArtifactRoot: filepath.Join(root, "data"), WorkspaceID: "workspace-a", Policy: ingest.DefaultPolicy()}).Convert(context.Background(), ingest.Request{SourcePath: sourcePath, WorkspacePath: root, Policy: ingest.DefaultPolicy()})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != ingest.StatusUnavailable || len(result.Warnings) == 0 {
		t.Fatalf("result = %+v", result)
	}
}

func TestMarkItDownAdapterBlocksInvalidWorkspaceID(t *testing.T) {
	root := t.TempDir()
	sourcePath := filepath.Join(root, "brief.docx")
	if err := os.WriteFile(sourcePath, []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := (Adapter{Command: []string{os.Args[0]}, ArtifactRoot: filepath.Join(root, "data"), WorkspaceID: "../escape", Policy: ingest.DefaultPolicy()}).Convert(context.Background(), ingest.Request{SourcePath: sourcePath, WorkspacePath: root, Policy: ingest.DefaultPolicy()})
	if err == nil || !strings.Contains(err.Error(), "workspace identity") {
		t.Fatalf("error = %v", err)
	}
}

func TestMarkItDownHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	var request struct {
		OutputPath string `json:"output_path"`
	}
	if err := json.NewDecoder(os.Stdin).Decode(&request); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(request.OutputPath, []byte("# Converted\n\nLocal fixture\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := json.NewEncoder(os.Stdout).Encode(map[string]any{
		"status":   "usable",
		"fidelity": "textual",
		"format":   "docx",
		"warnings": []string{},
	}); err != nil {
		t.Fatal(err)
	}
}
