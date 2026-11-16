package product_test

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/priorwork"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"os"
	"path/filepath"
	"testing"
)

func TestSharePointEnrollmentPublicJourney(t *testing.T) {
	for _, stdio := range []bool{false, true} {
		name := "sdk"
		if stdio {
			name = "shipping-stdio"
		}
		t.Run(name, func(t *testing.T) { assertSharePointEnrollmentJourney(t, stdio) })
	}
}

func assertSharePointEnrollmentJourney(t *testing.T, stdio bool) {
	t.Helper()
	f := newOwnerFixture(t, stdio)
	cs := f.connect(t, nil)
	out, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "maestro_query", Arguments: map[string]any{"operation": "sharepoint.enrollment.status"}})
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err := json.Unmarshal([]byte(out.Content[0].(*mcp.TextContent).Text), &value); err != nil {
		t.Fatal(err)
	}
	if out.IsError || value["local_control"] != "selection_required" {
		t.Fatalf("missing public local enrollment control: %v", value)
	}
	root := filepath.Join(filepath.Dir(f.root), "atlases", "organization", "sharepoint-work")
	if _, err := os.Lstat(root); !os.IsNotExist(err) {
		t.Fatalf("read created state: %v", err)
	}
	for _, invalid := range []map[string]any{
		{"operation": "sharepoint.enrollment.select", "folder_urls": []string{"https://example.sharepoint.com/sites/project/Documents/Exact"}, "purpose": "collect"},
		{"operation": "sharepoint.enrollment.select", "folder_urls": []string{"https://example.sharepoint.com/sites/project/Documents/Exact"}, "purpose": "prior_work_retrieval", "workspace_id": strings.Repeat("a", 32)},
		{"operation": "sharepoint.enrollment.revoke", "folder_urls": []string{"https://example.sharepoint.com/sites/project/Documents/Exact"}},
	} {
		result, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "maestro_apply", Arguments: invalid})
		if err == nil && (result == nil || !result.IsError) {
			t.Fatal("invalid public scope accepted", invalid)
		}
	}
	input := map[string]any{"operation": "sharepoint.enrollment.select", "folder_urls": []string{"https://example.sharepoint.com/sites/project/Documents/Exact"}, "purpose": "prior_work_retrieval"}
	refused := contentCall(t, cs, "maestro_apply", input, true)
	if refused["code"] != "confirmation_unavailable" {
		t.Fatal(refused)
	}
	if _, err := os.Lstat(root); !os.IsNotExist(err) {
		t.Fatalf("unavailable review created state: %v", err)
	}
	allow := false
	var presentation string
	cs = f.connect(t, func(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		presentation = req.Params.Message
		if !allow {
			return &mcp.ElicitResult{Action: "decline"}, nil
		}
		return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approved": true}}, nil
	})
	refused = contentCall(t, cs, "maestro_apply", input, true)
	if refused["code"] != "confirmation_declined" {
		t.Fatal(refused)
	}
	if _, err := os.Lstat(root); !os.IsNotExist(err) {
		t.Fatalf("decline created state: %v", err)
	}
	allow = true
	selected := contentCall(t, cs, "maestro_apply", input, false)
	for _, needle := range []string{"https://example.sharepoint.com/sites/project/Documents/Exact", "prior_work_retrieval", "workspace_id", root, "current", "proposed"} {
		if !strings.Contains(presentation, needle) {
			t.Fatal("full review missing", needle)
		}
	}
	assertBoundedSharePointSelection(t, selected, "selected")
	first := selected["selection"].(map[string]any)["fingerprint"]
	repeated := contentCall(t, cs, "maestro_apply", input, false)
	if repeated["selection"].(map[string]any)["fingerprint"] != first {
		t.Fatal("unchanged selection did not converge")
	}
	revoked := contentCall(t, cs, "maestro_apply", map[string]any{"operation": "sharepoint.enrollment.revoke"}, false)
	assertBoundedSharePointSelection(t, revoked, "revoked")
	store := priorwork.SourceSelectionStore{Root: root, CustodyRoot: filepath.Dir(f.root)}
	id := revoked["selection"].(map[string]any)["workspace_id"].(string)
	if _, err := store.SelectedFolders(id); err == nil {
		t.Fatal("revoked local source remained available")
	}
	readback := contentCall(t, f.connect(t, nil), "maestro_query", map[string]any{"operation": "sharepoint.enrollment.status"}, false)
	assertBoundedSharePointSelection(t, readback, "revoked")
}

func assertBoundedSharePointSelection(t *testing.T, out map[string]any, state string) {
	t.Helper()
	if out["local_control"] != state || out["signed_enrollment"] != "not_attested" || out["external_access_verified"] != false || out["source_content_verified"] != false || out["ingestion_state"] != "not_executed" {
		t.Fatal("selection implied external authority", out)
	}
	provider := out["provider"].(map[string]any)
	if provider["configured"] != false || provider["state"] != "unavailable" {
		t.Fatal(provider)
	}
	body, _ := json.Marshal(out)
	if strings.Contains(string(body), "https://example.sharepoint.com") {
		t.Fatal("status disclosed folder URLs")
	}
}

func TestSharePointEnrollmentRejectsChangedReview(t *testing.T) {
	f := newOwnerFixture(t, false)
	root := filepath.Join(filepath.Dir(f.root), "atlases", "organization", "sharepoint-work")
	store := priorwork.SourceSelectionStore{Root: root, CustodyRoot: filepath.Dir(f.root)}
	plain := f.connect(t, nil)
	initial := contentCall(t, plain, "maestro_query", map[string]any{"operation": "sharepoint.enrollment.status"}, false)
	id := initial["selection"].(map[string]any)["workspace_id"].(string)
	cs := f.connect(t, func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		if _, err := store.Select(id, []string{"https://example.sharepoint.com/sites/project/Documents/Concurrent"}); err != nil {
			t.Fatal(err)
		}
		return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approved": true}}, nil
	})
	out := contentCall(t, cs, "maestro_apply", map[string]any{"operation": "sharepoint.enrollment.select", "folder_urls": []string{"https://example.sharepoint.com/sites/project/Documents/Stale"}, "purpose": "prior_work_retrieval"}, true)
	if out["code"] != "revision_conflict" {
		t.Fatal(out)
	}
	folders, err := store.SelectedFolders(id)
	if err != nil || len(folders) != 1 || !strings.HasSuffix(folders[0], "/Concurrent") {
		t.Fatal("stale review overwrote latest selection", folders, err)
	}
}
