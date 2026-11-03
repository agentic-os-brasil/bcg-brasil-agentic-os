package product_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/maintenance"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestProductDarwinReviewProjectionR1(t *testing.T) {
	for _, stale := range []bool{false, true} {
		t.Run(map[bool]string{false: "initialization", true: "refresh"}[stale], func(t *testing.T) {
			f := newOwnerFixture(t, false)
			approved := f.connect(t, cadenceApprove)
			var revision uint64
			if stale {
				q := cadenceCall(t, approved, "qualify", map[string]any{"job_id": maintenance.DarwinDailyJobID, "expected_revision": 0}, false)
				revision = uint64(q["revision"].(float64))
				cadenceCall(t, approved, "qualify", map[string]any{"job_id": "runtime-health-check", "expected_revision": 0}, false)
			}
			var message string
			cs := f.connect(t, func(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
				message = req.Params.Message
				return &mcp.ElicitResult{Action: "decline"}, nil
			})
			before := cadenceTree(t, f)
			cadenceCall(t, cs, "qualify", map[string]any{"job_id": maintenance.DarwinDailyJobID, "expected_revision": revision}, true)
			if cadenceTree(t, f) != before {
				t.Fatal("declined exact review changed bytes/modes/inventory")
			}
			form := map[string]any{}
			if i := strings.Index(message, "{"); i < 0 || json.Unmarshal([]byte(message[i:]), &form) != nil {
				t.Fatal("actual trusted form was not JSON")
			}
			b, e := product.Open(f.options)
			if e != nil {
				t.Fatal(e)
			}
			local, e := b.MaintenanceReview(product.MaintenanceMutation{Operation: "maintenance.qualify", JobID: maintenance.DarwinDailyJobID, ExpectedRevision: revision})
			if e != nil {
				t.Fatal(e)
			}
			body, e := json.Marshal(local.Review)
			if e != nil {
				t.Fatal(e)
			}
			var operator map[string]any
			if e = json.Unmarshal(body, &operator); e != nil {
				t.Fatal(e)
			}
			for name, projection := range map[string]map[string]any{"trusted_form": form, "local_operator": operator} {
				raw, ok := projection["darwin_review"].(map[string]any)
				if !ok {
					t.Errorf("%s lacks canonical Darwin cache/source/effect projection: %v", name, projection)
					continue
				}
				expectedState, expectedEffect := "absent", "initialize"
				if stale {
					expectedState = "stale"
					expectedEffect = "refresh"
				}
				if raw["cache_state"] != expectedState || raw["effect"] != expectedEffect {
					t.Errorf("%s wrong transition: %v", name, raw)
				}
				parent, ok := raw["parent"].(map[string]any)
				if !ok || parent["present"] != stale || len(raw["external_sha256"].(string)) != 64 {
					t.Errorf("%s missing exact source/parent: %v", name, raw)
				}
				if stale && len(parent["sha256"].(string)) != 64 {
					t.Errorf("%s missing old parent SHA: %v", name, raw)
				}
			}
		})
	}
}
