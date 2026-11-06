package product_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

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
	out, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: tool, Arguments: args})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Content) != 1 {
		t.Fatalf("%s: unexpected content count %d", operation, len(out.Content))
	}
	text, ok := out.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("%s: expected text result", operation)
	}
	if out.IsError != fail {
		bounded := text.Text
		if len(bounded) > 1024 {
			bounded = bounded[:1024]
		}
		t.Fatalf("brain.%s: is_error=%v result=%q", operation, out.IsError, bounded)
	}
	var value map[string]any
	if err = json.Unmarshal([]byte(text.Text), &value); err != nil {
		t.Fatal(err)
	}
	return value
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

func TestAuthoredBrainExactAppendRetryConverges(t *testing.T) {
	for _, stdio := range []bool{false, true} {
		t.Run(fmt.Sprint(stdio), func(t *testing.T) {
			f := newOwnerFixture(t, stdio)
			cs := f.connect(t, func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
				return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approved": true}}, nil
			})
			workspace := contentCall(t, cs, "maestro_context", map[string]any{}, false)["workspace_id"].(string)
			const page = "projects/retry.md"
			created := brainCall(t, cs, "create", map[string]any{"page": page, "text": authoredBrief(workspace, page, "Retry brief")}, false)
			args := map[string]any{"page": page, "section": "## Decisions", "text": "- One exact reviewed effect.", "expected_page_revision": created["revision"]}
			first := brainCall(t, cs, "append", args, false)
			if err := cs.Close(); err != nil {
				t.Fatal(err)
			}
			cs = f.connect(t, func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
				return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approved": true}}, nil
			})
			retried := brainCall(t, cs, "append", args, false)
			if retried["state"] != "unchanged" || retried["revision"] != first["revision"] {
				t.Fatal("exact retry did not converge to canonical receipt", retried)
			}
			read := brainCall(t, cs, "read", map[string]any{"page": page, "purpose": "Verify exact retry has one effect"}, false)
			if strings.Count(read["text"].(string), "One exact reviewed effect.") != 1 {
				t.Fatal("retry duplicated authored entry", read)
			}
		})
	}
}

func TestAuthoredBrainReadOnlyRefusalsAndPreservedLegacy(t *testing.T) {
	f := newOwnerFixture(t, false)
	cs := f.connect(t, func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return &mcp.ElicitResult{Action: "decline"}, nil
	})
	beforeHome, beforeWorkspace := contentTree(t, f.options.UserHome, false), contentTree(t, f.options.WorkspacePath, false)
	brainCall(t, cs, "status", nil, false)
	brainCall(t, cs, "templates", nil, false)
	brainCall(t, cs, "read", map[string]any{"page": "absent.md", "purpose": "Check a named absent page"}, true)
	brainCall(t, cs, "prepare", nil, true)
	if !reflect.DeepEqual(beforeHome, contentTree(t, f.options.UserHome, false)) || !reflect.DeepEqual(beforeWorkspace, contentTree(t, f.options.WorkspacePath, false)) {
		t.Fatal("read-only or declined preparation changed state")
	}
	workspace := contentCall(t, cs, "maestro_context", map[string]any{}, false)["workspace_id"].(string)
	body := authoredBrief(workspace, "projects/invalid.md", "Invalid")
	for _, page := range []string{"../outside.md", "/absolute.md", "owner/self/record.md", "memory/private.md", ".atlasops/receipt.md", "projects\\alias.md"} {
		brainCall(t, cs, "create", map[string]any{"page": page, "text": body}, true)
	}
	brainCall(t, cs, "create", map[string]any{"page": "projects/invalid.md", "text": body, "approved": true}, true)
	brainCall(t, cs, "create", map[string]any{"page": "projects/invalid.md", "text": strings.Replace(body, "workspace/"+workspace, "workspace/foreign", 1)}, true)
	if !reflect.DeepEqual(beforeWorkspace, contentTree(t, f.options.WorkspacePath, false)) {
		t.Fatal("invalid authored inputs changed workspace")
	}
	legacy := filepath.Join(f.options.WorkspacePath, "brain", "projects", "template-project.md")
	putContentFile(t, legacy, []byte("# Hand-edited legacy template\n\n## Decisions\n"))
	cs = f.connect(t, func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approved": true}}, nil
	})
	first := brainCall(t, cs, "read", map[string]any{"page": "projects/template-project.md", "purpose": "Inspect a hand-edited legacy template"}, false)
	if len(first["diagnostics"].([]any)) == 0 {
		t.Fatal("legacy metadata was silently claimed valid", first)
	}
	brainCall(t, cs, "prepare", nil, false)
	brainCall(t, cs, "prepare", nil, false)
	if string(readContentFile(t, legacy)) != "# Hand-edited legacy template\n\n## Decisions\n" {
		t.Fatal("preparation overwrote a legacy template")
	}
	brainCall(t, cs, "append", map[string]any{"page": "projects/template-project.md", "section": "## Decisions", "text": "- Preserve original prose.", "expected_page_revision": first["revision"]}, false)
	after := brainCall(t, cs, "read", map[string]any{"page": "projects/template-project.md", "purpose": "Use preserved legacy prose"}, false)
	if !strings.HasPrefix(after["text"].(string), "# Hand-edited legacy template\n") || len(after["diagnostics"].([]any)) == 0 {
		t.Fatal("append silently migrated a legacy page", after)
	}
}

func TestAuthoredBrainChangedReviewAndUTF8ReadBounds(t *testing.T) {
	f := newOwnerFixture(t, false)
	var intervene func() error
	cs := f.connect(t, func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		if intervene != nil {
			if err := intervene(); err != nil {
				return nil, err
			}
			intervene = nil
		}
		return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approved": true}}, nil
	})
	workspace := contentCall(t, cs, "maestro_context", map[string]any{}, false)["workspace_id"].(string)
	const page = "projects/utf8.md"
	body := authoredBrief(workspace, page, "Unicode brief") + strings.Repeat("colaboração 漢字 🎷\n", 900)
	created := brainCall(t, cs, "create", map[string]any{"page": page, "text": body}, false)
	first := brainCall(t, cs, "read", map[string]any{"page": page, "purpose": "Read a bounded UTF-8 source page", "max_bytes": 4096}, false)
	text := first["text"].(string)
	if len(text) > 4096 || !utf8.ValidString(text) || !strings.HasPrefix(body, text) {
		t.Fatal("read exceeded UTF-8 page bound", len(text))
	}
	continued := brainCall(t, cs, "read", map[string]any{"page": page, "purpose": "Continue the named UTF-8 page", "offset": len(text), "max_bytes": 4096}, false)
	if !utf8.ValidString(continued["text"].(string)) || !strings.HasPrefix(body[len(text):], continued["text"].(string)) {
		t.Fatal("page continuation split or lost UTF-8")
	}
	later := strings.Replace(body, "## Decisions", "## Decisions\n- Preserve newer reviewed work.", 1)
	intervene = func() error {
		return os.WriteFile(filepath.Join(f.options.WorkspacePath, "brain", filepath.FromSlash(page)), []byte(later), 0600)
	}
	brainCall(t, cs, "update", map[string]any{"page": page, "text": authoredBrief(workspace, page, "Unicode brief"), "expected_page_revision": created["revision"]}, true)
	if string(readContentFile(t, filepath.Join(f.options.WorkspacePath, "brain", filepath.FromSlash(page)))) != later {
		t.Fatal("changed review overwrote newer bytes")
	}
	brainCall(t, cs, "create", map[string]any{"page": "projects/oversize.md", "text": authoredBrief(workspace, "projects/oversize.md", "Oversize") + strings.Repeat("x", 256<<10)}, true)
}
