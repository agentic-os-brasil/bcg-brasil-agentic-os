//go:build windows

package product_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Keep the historical native assertion: registration does not qualify the
// separate private-wiki filesystem backend or let a read initialize its state.
func TestMethodPlansResolveMaintenanceKeepWikiUnavailable(t *testing.T) {
	f := newMethodQueryFixture(t)
	client := f.connect(t, "")
	before := methodTreeSnapshot(t, f.root)
	var plan struct {
		State        string `json:"state"`
		Execution    string `json:"execution_state"`
		Dependencies []struct {
			ID          string   `json:"id"`
			State       string   `json:"state"`
			Fulfillment []string `json:"fulfillment_operations"`
			Missing     []string `json:"missing_operations"`
		} `json:"dependencies"`
	}
	methodQueryDecode(t, client, "maestro_query", map[string]any{"operation": "methods.plan", "method_id": "dream-memory"}, &plan)
	if plan.State != "requirements_unavailable" || plan.Execution != "not_executed" {
		t.Fatalf("method selection granted unattested native authority: %+v", plan)
	}
	want := map[string][]string{
		"domain.maintenance_lifecycle": {"maintenance.enroll", "maintenance.pause", "maintenance.qualify", "maintenance.report", "maintenance.resume", "maintenance.revoke", "maintenance.run", "maintenance.status"},
		"domain.workspace_navigation":  {"wiki.read", "wiki.source", "wiki.status"},
	}
	for _, dependency := range plan.Dependencies {
		if operations, ok := want[dependency.ID]; ok {
			if dependency.State != "available" || !reflect.DeepEqual(dependency.Fulfillment, operations) || len(dependency.Missing) != 0 {
				t.Fatalf("owning operation registration differs: %+v", dependency)
			}
			delete(want, dependency.ID)
		}
	}
	if len(want) != 0 {
		t.Fatalf("registered domains are absent: %v", want)
	}
	generation := strings.Repeat("a", 64)
	for _, request := range []map[string]any{
		{"operation": "wiki.status"},
		{"operation": "wiki.read", "generation": generation, "reference": "index.md", "offset": 0, "max_bytes": 64},
		{"operation": "wiki.source", "generation": generation, "reference": "maestro://memory/" + generation, "offset": 0, "max_bytes": 64},
	} {
		out := methodQueryCall(t, client, "maestro_query", request)
		if !out.IsError || len(out.Content) != 1 {
			t.Fatalf("unqualified native operation %v was admitted: %+v", request, out)
		}
		text, ok := out.Content[0].(*mcp.TextContent)
		if !ok {
			t.Fatal("native refusal is not bounded JSON text")
		}
		var refusal map[string]any
		if err := json.Unmarshal([]byte(text.Text), &refusal); err != nil || len(refusal) != 2 || refusal["code"] != "wiki_unavailable" || refusal["retryable"] != false {
			t.Fatalf("native operation %v lost its typed refusal: %s %v", request, text.Text, err)
		}
		if !reflect.DeepEqual(before, methodTreeSnapshot(t, f.root)) {
			t.Fatalf("unqualified native operation %v changed the fixture", request)
		}
	}
}
