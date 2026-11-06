package product_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func brainCall(t *testing.T, cs *mcp.ClientSession, operation string, fields map[string]any, fail bool) map[string]any {
	t.Helper()
	tool := "maestro_query"
	if operation == "prepare" || operation == "create" || operation == "append" || operation == "update" || operation == "set_field" || operation == "link" {
		tool = "maestro_apply"
	}
	args := map[string]any{"operation": "brain." + operation}
	for key, value := range fields {
		args[key] = value
	}
	return contentCall(t, cs, tool, args, fail)
}

func authoredBrief(workspace, page, title string) string {
	return fmt.Sprintf("---\nid: %s\ntitle: %s\nsummary: A reviewed synthetic kickoff brief.\ntype: project-brief\nscope: workspace/%s\nstatus: draft\nsensitivity: internal\nupdated: 2026-11-06\n---\n# %s\n\n- **Status:** draft\n\n## Decisions\n\n## Related\n", strings.TrimSuffix(page, ".md"), title, workspace, title)
}

func TestAuthoredBrainCreateAndRestartedRead(t *testing.T) {
	for _, stdio := range []bool{false, true} {
		t.Run(fmt.Sprint(stdio), func(t *testing.T) {
			f := newOwnerFixture(t, stdio)
			approve := func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
				return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approved": true}}, nil
			}
			first := f.connect(t, approve)
			ctx := contentCall(t, first, "maestro_context", map[string]any{}, false)
			workspace := ctx["workspace_id"].(string)
			status := brainCall(t, first, "status", nil, false)
			if status["workspace_id"] != workspace {
				t.Fatal("authored brain lost the admitted workspace", status)
			}
			templates := brainCall(t, first, "templates", nil, false)
			if len(templates["templates"].([]any)) != 4 {
				t.Fatal("fixed client/project/person/daily templates unavailable", templates)
			}
			brainCall(t, first, "prepare", nil, false)
			const page = "projects/kickoff.md"
			body := authoredBrief(workspace, page, "Kickoff brief")
			created := brainCall(t, first, "create", map[string]any{"page": page, "text": body}, false)
			if created["state"] != "written" || created["revision"] == "" {
				t.Fatal("attended ordinary text was not written", created)
			}
			if err := first.Close(); err != nil {
				t.Fatal(err)
			}
			second := f.connect(t, nil)
			read := brainCall(t, second, "read", map[string]any{"page": page, "purpose": "Continue the kickoff brief in another client"}, false)
			if read["text"] != body || read["revision"] != created["revision"] || read["workspace_id"] != workspace {
				t.Fatal("restarted client did not read the actual reviewed page", read)
			}
		})
	}
}

func TestAuthoredBrainReviewedAppendUsesCurrentRevision(t *testing.T) {
	for _, stdio := range []bool{false, true} {
		t.Run(fmt.Sprint(stdio), func(t *testing.T) {
			f := newOwnerFixture(t, stdio)
			cs := f.connect(t, func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
				return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approved": true}}, nil
			})
			workspace := contentCall(t, cs, "maestro_context", map[string]any{}, false)["workspace_id"].(string)
			const page = "projects/decisions.md"
			body := authoredBrief(workspace, page, "Decision record")
			created := brainCall(t, cs, "create", map[string]any{"page": page, "text": body}, false)
			args := map[string]any{"page": page, "section": "## Decisions", "text": "- Adopt the reviewed synthetic plan.", "expected_page_revision": created["revision"]}
			appended := brainCall(t, cs, "append", args, false)
			if appended["state"] != "written" || appended["revision"] == created["revision"] {
				t.Fatal("append did not advance the page revision", appended)
			}
			read := brainCall(t, cs, "read", map[string]any{"page": page, "purpose": "Verify the reviewed decision"}, false)
			if strings.Count(read["text"].(string), "Adopt the reviewed synthetic plan.") != 1 {
				t.Fatal("decision was not persisted exactly once", read)
			}
			args["text"] = "- Stale replacement must not land."
			brainCall(t, cs, "append", args, true)
			after := brainCall(t, cs, "read", map[string]any{"page": page, "purpose": "Verify stale edit preserved the current page"}, false)
			if after["text"] != read["text"] || after["revision"] != appended["revision"] {
				t.Fatal("stale append changed the page", after)
			}
		})
	}
}

func TestAuthoredBrainEditsAndLinksOneReviewedBrief(t *testing.T) {
	for _, stdio := range []bool{false, true} {
		t.Run(fmt.Sprint(stdio), func(t *testing.T) {
			f := newOwnerFixture(t, stdio)
			cs := f.connect(t, func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
				return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approved": true}}, nil
			})
			workspace := contentCall(t, cs, "maestro_context", map[string]any{}, false)["workspace_id"].(string)
			const page, target = "projects/brief.md", "people/sponsor.md"
			body := authoredBrief(workspace, page, "Reviewed brief")
			created := brainCall(t, cs, "create", map[string]any{"page": page, "text": body}, false)
			brainCall(t, cs, "create", map[string]any{"page": target, "text": authoredBrief(workspace, target, "Sponsor")}, false)
			updatedBody := strings.Replace(body, "A reviewed synthetic kickoff brief.", "The scope and decision have been reviewed.", 1)
			updated := brainCall(t, cs, "update", map[string]any{"page": page, "text": updatedBody, "expected_page_revision": created["revision"]}, false)
			field := brainCall(t, cs, "set_field", map[string]any{"page": page, "field": "Status", "value": "ready for discussion", "expected_page_revision": updated["revision"]}, false)
			linked := brainCall(t, cs, "link", map[string]any{"page": page, "section": "## Related", "target": target, "label": "Sponsor", "expected_page_revision": field["revision"]}, false)
			read := brainCall(t, cs, "read", map[string]any{"page": page, "purpose": "Use the complete reviewed brief from another conversation"}, false)
			expected := strings.Replace(updatedBody, "- **Status:** draft", "- **Status:** ready for discussion", 1)
			expected = strings.Replace(expected, "## Related\n", "## Related\n- [Sponsor](../people/sponsor.md)\n", 1)
			if linked["state"] != "written" || read["text"] != expected || read["revision"] != linked["revision"] {
				t.Fatal("reviewed brief lost prose, field or relative reference", read)
			}
			for _, operation := range []string{"update", "set_field", "link"} {
				args := map[string]any{"page": page, "expected_page_revision": created["revision"]}
				switch operation {
				case "update":
					args["text"] = body
				case "set_field":
					args["field"], args["value"] = "Status", "stale"
				case "link":
					args["section"], args["target"], args["label"] = "## Related", target, "Stale"
				}
				brainCall(t, cs, operation, args, true)
			}
			after := brainCall(t, cs, "read", map[string]any{"page": page, "purpose": "Verify stale edits did not overwrite the reviewed brief"}, false)
			if after["text"] != expected || after["revision"] != linked["revision"] {
				t.Fatal("stale edit changed the brief", after)
			}
		})
	}
}
