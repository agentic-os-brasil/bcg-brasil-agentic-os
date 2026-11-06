package product_test

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func scopedKnowledgeCollect(t *testing.T, session *mcp.ClientSession, page string, fail bool) map[string]any {
	t.Helper()
	out, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "maestro_query", Arguments: map[string]any{"operation": "knowledge.collect", "purpose": "Read one named admitted legacy knowledge page", "pages": []string{page}}})
	if err != nil {
		t.Fatal(err)
	}
	text := out.Content[0].(*mcp.TextContent).Text
	if out.IsError != fail {
		if len(text) > 1024 {
			text = text[:1024]
		}
		t.Fatalf("knowledge.collect %s is_error=%v result=%q", page, out.IsError, text)
	}
	var value map[string]any
	if err = json.Unmarshal([]byte(text), &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func TestKnowledgeCollectSharesWorkspaceCaseAndPrivateScope(t *testing.T) {
	for _, stdio := range []bool{false, true} {
		t.Run(fmt.Sprint(stdio), func(t *testing.T) {
			f := newOwnerFixture(t, stdio)
			cs := f.connect(t, nil)
			root := filepath.Join(f.options.WorkspacePath, "brain")
			putContentFile(t, filepath.Join(root, "accounts", ".active"), []byte("a/selected\n"))
			const selected = "accounts/a/cases/selected/legacy.md"
			literal := "# Valid legacy converted knowledge\n\n## Evidence\n\nPreserve legacy prose without authored frontmatter.\n"
			putContentFile(t, filepath.Join(root, filepath.FromSlash(selected)), []byte(literal))
			positive := scopedKnowledgeCollect(t, cs, selected, false)
			pages := positive["pages"].([]any)
			if len(pages) != 1 || pages[0].(map[string]any)["content"] != literal {
				t.Fatal("selected legacy knowledge was disabled or rewritten", positive)
			}
			for _, page := range []string{"accounts/a/cases/foreign/legacy.md", "owner/private.md", "memory/private.md", "atlas/owner/private.md"} {
				path := filepath.Join(root, filepath.FromSlash(page))
				putContentFile(t, path, []byte("# Outside selected/private scope\n"))
				before := string(readContentFile(t, path))
				scopedKnowledgeCollect(t, cs, page, true)
				// Source admission precedes even unavailable converter discovery.
				out, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "maestro_apply", Arguments: map[string]any{"operation": "content.convert", "source_path": "brain/" + page}})
				if err != nil || !out.IsError {
					t.Fatal("unadmitted content source reached converter route", page, err)
				}
				if string(readContentFile(t, path)) != before {
					t.Fatal("scope refusal modified outside page", page)
				}
			}
			scopedKnowledgeCollect(t, cs, "Accounts/a/cases/foreign/legacy.md", true)
			scopedKnowledgeCollect(t, cs, "Owner/private.md", true)
		})
	}
}
