package product_test

import (
	"context"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/atlas"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/atlasops"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestAuthoredBrainDuplicateMetadataRefusesCreateAndUpdateWithoutEffects(t *testing.T) {
	for _, operation := range []string{"create", "update"} {
		for _, tc := range []struct{ name, extra string }{
			{"required-list-first", "id: [ignored]\n"},
			{"required-object-first", "id: {ignored: value}\n"},
			{"required-scalar-first", "id: ignored\n"},
			{"optional-object-scalar", "context: {source: first}\ncontext: second\n"},
			{"optional-object-object", "context: {source: first}\ncontext: {source: second}\n"},
			{"optional-list-list", "tags: [first]\ntags: [second]\n"},
		} {
			t.Run(operation+"/"+tc.name, func(t *testing.T) {
				f := newOwnerFixture(t, false)
				reviews := 0
				client := f.connect(t, func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
					reviews++
					return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approved": true}}, nil
				})
				workspace := contentCall(t, client, "maestro_context", map[string]any{}, false)["workspace_id"].(string)
				const page = "projects/metadata.md"
				body := authoredBrief(workspace, page, "Reviewed metadata")
				// A legitimate single structured optional value stays byte-preserving.
				body = strings.Replace(body, "---\n", "---\ncontext: {source: reviewed}\ntags: [synthetic, reviewed]\n", 1)
				seedPage := page
				if operation == "create" {
					seedPage = "projects/seed.md"
				}
				seedBody := strings.ReplaceAll(body, "projects/metadata", strings.TrimSuffix(seedPage, ".md"))
				seeded := brainCall(t, client, "create", map[string]any{"page": seedPage, "text": seedBody}, false)
				if actual := string(readContentFile(t, filepath.Join(f.options.WorkspacePath, "brain", filepath.FromSlash(seedPage)))); actual != seedBody {
					t.Fatal("single structured optional values were rewritten")
				}
				engine, err := atlasops.OpenWorkspace(atlas.Options{DataRoot: filepath.Join(filepath.Dir(f.root), "state"), WorkspacePath: f.options.WorkspacePath, WorkspaceID: workspace})
				if err != nil {
					t.Fatal(err)
				}
				if _, err = engine.CreateGrant(atlasops.GrantRequest{Ritual: "metadata-refusal", RitualVersion: "1", Segment: "projects", Operations: []string{"append-entry"}, Cadence: "manual", Reader: atlasops.ReaderOwnerSession}); err != nil {
					t.Fatal(err)
				}
				beforeHome := methodTreeSnapshot(t, f.options.UserHome)
				beforeWorkspace := methodTreeSnapshot(t, f.options.WorkspacePath)
				priorReviews := reviews
				// Replace the positive optional fields so each negative has only its named duplicate.
				candidate := strings.Replace(authoredBrief(workspace, page, "Reviewed metadata"), "---\n", "---\n"+tc.extra, 1)
				args := map[string]any{"operation": "brain." + operation, "page": page, "text": candidate}
				if operation == "update" {
					args["expected_page_revision"] = seeded["revision"]
				}
				result, err := client.CallTool(t.Context(), &mcp.CallToolParams{Name: "maestro_apply", Arguments: args})
				if err != nil {
					t.Fatal(err)
				}
				if !result.IsError {
					t.Error("duplicate metadata was accepted by the real authored operation")
				}
				if reviews != priorReviews {
					t.Error("ambiguous metadata reached trusted review")
				}
				if !reflect.DeepEqual(beforeHome, methodTreeSnapshot(t, f.options.UserHome)) || !reflect.DeepEqual(beforeWorkspace, methodTreeSnapshot(t, f.options.WorkspacePath)) {
					t.Error("duplicate metadata changed page, journal, grant or owner bytes/modes")
				}
			})
		}
	}
}
