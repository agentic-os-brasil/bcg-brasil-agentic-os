package product_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestAuthoredBrainSDKCreatesPageWithoutConversionArtifact(t *testing.T) {
	f := newOwnerFixture(t, false)
	const page = "projects/sdk-baseline.md"
	var workspaceID, body string
	reviewed := 0
	client := f.connect(t, func(_ context.Context, request *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		var shown struct {
			Operation   string `json:"operation"`
			WorkspaceID string `json:"workspace_id"`
			Pages       []struct {
				Page string
				Text string
			} `json:"pages"`
		}
		start := strings.Index(request.Params.Message, "{")
		if start < 0 || json.Unmarshal([]byte(request.Params.Message[start:]), &shown) != nil || shown.Operation != "brain.create" || shown.WorkspaceID != workspaceID || len(shown.Pages) != 1 || shown.Pages[0].Page != page || shown.Pages[0].Text != body {
			t.Fatal("trusted review omitted or changed the exact authored page")
		}
		reviewed++
		return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approved": true}}, nil
	})
	workspaceID = contentCall(t, client, "maestro_context", map[string]any{}, false)["workspace_id"].(string)
	body = fmt.Sprintf("---\nid: projects/sdk-baseline\ntitle: Reviewed authored baseline\nsummary: A synthetic page selected in an attended conversation.\ntype: project-brief\nscope: workspace/%s\nstatus: draft\nsensitivity: internal\nupdated: 2026-11-09\n---\n# Reviewed authored baseline\n\n## Decisions\n\n- Record the exact reviewed text without converting a document.\n", workspaceID)
	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{Name: "maestro_apply", Arguments: map[string]any{"operation": "brain.create", "page": page, "text": body}})
	if err != nil {
		t.Fatal("public SDK call failed before an authored-page result", err)
	}
	if result.IsError {
		t.Fatalf("attended brain.create without a conversion artifact was rejected: %+v", result.Content)
	}
	if reviewed != 1 {
		t.Fatalf("authored page requires one exact trusted review, got %d", reviewed)
	}
	var written struct {
		State    string `json:"state"`
		Revision string `json:"revision"`
	}
	if len(result.Content) != 1 || json.Unmarshal([]byte(result.Content[0].(*mcp.TextContent).Text), &written) != nil || written.State != "written" || written.Revision == "" {
		t.Fatalf("authored creation did not return a real written revision: %+v", result.Content)
	}
	if actual, err := os.ReadFile(filepath.Join(f.options.WorkspacePath, "brain", filepath.FromSlash(page))); err != nil || string(actual) != body {
		t.Fatal("public authored creation did not persist the exact reviewed page", err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(f.root), "state", "content")); !os.IsNotExist(err) {
		t.Fatalf("attended page creation fabricated a conversion artifact store: %v", err)
	}
}
