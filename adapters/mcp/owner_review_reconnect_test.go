package product

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/processidentity"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Exercise the existing error envelopes directly. No server, session, model,
// installer, private profile or protected operation participates in this test.
func TestOwnerReviewReconnectErrorMappings(t *testing.T) {
	const action = "Reconnect only the Maestro connection in the client that made this request, then retry."
	for _, test := range []struct {
		name      string
		err       error
		reconnect bool
		codes     [3]string
	}{
		{"typed", errOwnerReviewRecordChanged, true, [3]string{"restart_required", "restart_required", "restart_required"}},
		{"wrapped", fmt.Errorf("PRIVATE_RECONNECT_MARKER: %w", errOwnerReviewRecordChanged), true, [3]string{"restart_required", "restart_required", "restart_required"}},
		{"same_text", errors.New("owner_review_carrier_changed"), false, [3]string{"execution_rejected", "wiki_unavailable", "maintenance_rejected"}},
		{"wrapped_same_text", fmt.Errorf("PRIVATE_RECONNECT_MARKER: %w", errors.New("owner_review_carrier_changed")), false, [3]string{"execution_rejected", "wiki_unavailable", "maintenance_rejected"}},
		{"unavailable", errors.New("owner_review_carrier_unavailable"), false, [3]string{"execution_rejected", "wiki_unavailable", "maintenance_rejected"}},
		{"canceled", context.Canceled, false, [3]string{"execution_rejected", "wiki_unavailable", "canceled"}},
		{"process_changed", processidentity.ErrChanged, false, [3]string{"restart_required", "restart_required", "restart_required"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			for index, mapper := range []struct {
				name    string
				project func(any, error) *mcp.CallToolResult
			}{{"product", result}, {"wiki", wikiResult}, {"maintenance", maintenanceResult}} {
				t.Run(mapper.name, func(t *testing.T) {
					out := mapper.project(nil, test.err)
					if !out.IsError || len(out.Content) != 1 {
						t.Fatal("refusal lost its closed error envelope")
					}
					text, ok := out.Content[0].(*mcp.TextContent)
					if !ok {
						t.Fatal("refusal lost its text metadata")
					}
					var fields map[string]json.RawMessage
					if json.Unmarshal([]byte(text.Text), &fields) != nil || string(fields["code"]) != fmt.Sprintf("%q", test.codes[index]) {
						t.Fatalf("wrong closed category: %s", text.Text)
					}
					if index != 0 && string(fields["retryable"]) != "false" {
						t.Fatal("reconnection changed retry policy")
					}
					if test.reconnect {
						if string(fields["next_action"]) != fmt.Sprintf("%q", action) {
							t.Fatal("typed stale record lost normal reconnection guidance")
						}
					} else if fields["next_action"] != nil {
						t.Fatal("unrelated refusal acquired reconnect guidance")
					}
					want := 1
					if index != 0 {
						want++
					}
					if test.reconnect {
						want++
					}
					if len(fields) != want || strings.Contains(text.Text, "PRIVATE_RECONNECT_MARKER") || strings.Contains(text.Text, "owner_review_carrier_changed") {
						t.Fatal("unexpected metadata or private cause escaped")
					}
				})
			}
			if maintenanceErrorCode(test.err) != test.codes[2] || MaintenanceCommandFailure(test.err, "admission").Diagnostic.Code != test.codes[2] {
				t.Fatal("maintenance command diagnostic lost the same typed category")
			}
		})
	}
}
