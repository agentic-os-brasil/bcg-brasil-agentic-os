package product_test

import (
	"context"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// The fixed managed inventory is independent of owner and workspace files.
func TestCanonicalAgentResourceDiscovery(t *testing.T) {
	fixture := newMethodQueryFixture(t)
	cs := fixture.connect(t, "")
	ctx := context.Background()
	var got []string
	methodResources := 0
	params := &mcp.ListResourcesParams{}
	seenCursors := map[string]bool{}
	for {
		resources, err := cs.ListResources(ctx, params)
		if err != nil {
			t.Fatal(err)
		}
		for _, resource := range resources.Resources {
			if strings.HasPrefix(resource.URI, "maestro://agents/") {
				got = append(got, strings.TrimPrefix(resource.URI, "maestro://agents/"))
			}
			if strings.HasPrefix(resource.URI, "maestro://methods/") {
				methodResources++
			}
		}
		if resources.NextCursor == "" {
			break
		}
		if seenCursors[resources.NextCursor] {
			t.Fatal("resource pagination repeated a cursor")
		}
		seenCursors[resources.NextCursor] = true
		params.Cursor = resources.NextCursor
	}
	sort.Strings(got)
	want := []string{
		"activation-policy.json", "case-agent/AGENT.md", "catalog.json", "client-account-agent/AGENT.md", "darwin/AGENT.md", "gamma-guardian/AGENT.md", "maestro/AGENT.md", "pa-expert-registry.json", "pa-expert/AGENT.md", "templates/case_agent/AGENT.md", "templates/client_account_agent/AGENT.md", "templates/pa_expert/AGENT.md", "templates/quality_guardian/AGENT.md", "yoda/AGENT.md",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("canonical agent resources: got %d %v; want 14 %v", len(got), got, want)
	}
	if methodResources != 248 {
		t.Fatalf("method assets lost: %d resources", methodResources)
	}
}
