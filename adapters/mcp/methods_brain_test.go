package product_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMethodAuthoredPageIntegration(t *testing.T) {
	for _, stdio := range []bool{false, true} {
		name := "sdk"
		if stdio {
			name = "compiled_stdio"
		}
		t.Run(name, func(t *testing.T) {
			f := newOwnerFixture(t, stdio)
			cs := f.connect(t, cadenceApprove)
			workspace := contentCall(t, cs, "maestro_context", map[string]any{}, false)["workspace_id"].(string)
			status := brainCall(t, cs, "status", nil, false)
			if status["workspace_id"] != workspace || status["mutation_available"] != true {
				t.Fatal("composed schema lost admitted authored-page capability", status)
			}
			const uri = "maestro://methods/maestro-operator/resources/authored-brain.md"
			canonical, err := os.ReadFile("../../bundles/base/skills/maestro-operator/resources/authored-brain.md")
			if err != nil {
				t.Fatal(err)
			}
			var recipe strings.Builder
			for offset := 0; ; {
				var page struct {
					Text string `json:"text"`
					Next int    `json:"next_offset"`
					More bool   `json:"has_more"`
				}
				methodQueryDecode(t, cs, "maestro_query", map[string]any{"operation": "methods.read", "resource_uri": uri, "offset": offset, "max_bytes": 4096}, &page)
				recipe.WriteString(page.Text)
				if !page.More {
					break
				}
				if page.Next <= offset {
					t.Fatal("authored method resource did not advance")
				}
				offset = page.Next
			}
			if recipe.String() != string(canonical) {
				t.Fatal("query method reader lost the canonical authored recipe")
			}
			var plan struct {
				State      string `json:"execution_state"`
				Operations []struct {
					Name string `json:"name"`
				} `json:"operations"`
			}
			methodQueryDecode(t, cs, "maestro_query", map[string]any{"operation": "methods.plan", "method_id": "maestro-operator"}, &plan)
			found := false
			for _, operation := range plan.Operations {
				found = found || operation.Name == "brain.create"
			}
			if !found || plan.State != "not_executed" {
				t.Fatal("operator plan omits its authored-page route or claims execution", plan)
			}
			out, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "maestro_query", Arguments: map[string]any{"operation": "methods.read", "resource_uri": uri, "page": "projects/ungranted.md"}})
			if err == nil && (out == nil || !out.IsError) {
				t.Fatal("composed variants accepted an unrelated filesystem argument")
			}
			brainCall(t, cs, "prepare", nil, false)
			const page = "projects/composed-method.md"
			body := authoredBrief(workspace, page, "Composed method proof")
			written := brainCall(t, cs, "create", map[string]any{"page": page, "text": body}, false)
			if err = cs.Close(); err != nil {
				t.Fatal(err)
			}
			cs = f.connect(t, nil)
			read := brainCall(t, cs, "read", map[string]any{"page": page, "purpose": "Continue the page discovered through the method query"}, false)
			if written["state"] != "written" || read["text"] != body || read["revision"] != written["revision"] || read["workspace_id"] != workspace {
				t.Fatal("method-guided reviewed authoring did not survive restart", written, read)
			}
		})
	}
}
