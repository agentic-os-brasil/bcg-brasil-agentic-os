package product_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/maintenance"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// This is foreground engineering evidence. The ordinary shipping binary uses
// its system clock, no native/admission override and no real host registration.
func TestProductDarwinSDKAndShippingStdioLifecycle(t *testing.T) {
	for _, stdio := range []bool{false, true} {
		t.Run(fmt.Sprint(stdio), func(t *testing.T) {
			f := newOwnerFixture(t, stdio)
			var offset time.Duration
			var forms []map[string]any
			connect := func(approve bool) *mcp.ClientSession {
				handler := func(ctx context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
					var form map[string]any
					i := strings.Index(req.Params.Message, "{")
					if i < 0 || json.Unmarshal([]byte(req.Params.Message[i:]), &form) != nil {
						t.Error("actual form lacks JSON record")
						return &mcp.ElicitResult{Action: "decline"}, nil
					}
					if form["operation"] == "maintenance.qualify" && form["job_id"] == maintenance.DarwinDailyJobID {
						projection, ok := form["darwin_review"].(map[string]any)
						if !ok {
							t.Error("shipping/SDK form lacks Darwin transition")
							return &mcp.ElicitResult{Action: "decline"}, nil
						}
						expected := map[string]string{"absent": "initialize", "stale": "refresh", "current": "assess"}
						state, _ := projection["cache_state"].(string)
						external, _ := projection["external_sha256"].(string)
						parent, ok := projection["parent"].(map[string]any)
						if expected[state] == "" || projection["effect"] != expected[state] || len(external) != 64 || !ok || parent["present"] != (state != "absent") {
							t.Error("wrong exact Darwin form transition", projection)
						}
						if state != "absent" {
							sha, _ := parent["sha256"].(string)
							if len(sha) != 64 {
								t.Error("form lacks exact parent SHA")
							}
						}
						forms = append(forms, projection)
					}
					if !approve {
						return &mcp.ElicitResult{Action: "decline"}, nil
					}
					return cadenceApprove(ctx, req)
				}
				if stdio {
					return f.connect(t, handler)
				}
				bound, e := product.Open(f.options)
				if e != nil {
					t.Fatal(e)
				}
				product.SetMaintenanceClockForTest(bound, func() time.Time { return time.Now().UTC().Add(offset) })
				a, z := mcp.NewInMemoryTransports()
				ss, e := bound.Server().Connect(context.Background(), a, nil)
				if e != nil {
					t.Fatal(e)
				}
				t.Cleanup(func() { ss.Close() })
				cs, e := mcp.NewClient(&mcp.Implementation{Name: "darwin-engineering-sdk", Version: "1"}, &mcp.ClientOptions{ElicitationHandler: handler}).Connect(context.Background(), z, nil)
				if e != nil {
					t.Fatal(e)
				}
				t.Cleanup(func() { cs.Close() })
				return cs
			}
			cs := connect(true)
			before := cadenceTree(t, f)
			status := cadenceCall(t, cs, "status", nil, false)
			if cadenceTree(t, f) != before {
				t.Fatal("fresh status wrote state")
			}
			daily, weekly, monthly := maintenance.DarwinDailyJobID, maintenance.DarwinWeeklyJobID, maintenance.DarwinMonthlyJobID
			operatorReview := func(revision uint64) map[string]any {
				request := map[string]any{"operation": "maintenance.qualify", "job_id": daily, "expected_revision": revision}
				if stdio {
					return memoryCLI(t, f, "maintenance-review", request, nil, false)
				}
				b, e := product.Open(f.options)
				if e != nil {
					t.Fatal(e)
				}
				local, e := b.MaintenanceReview(product.MaintenanceMutation{Operation: "maintenance.qualify", JobID: daily, ExpectedRevision: revision})
				if e != nil {
					t.Fatal(e)
				}
				body, e := json.Marshal(local)
				if e != nil {
					t.Fatal(e)
				}
				var out map[string]any
				if e = json.Unmarshal(body, &out); e != nil {
					t.Fatal(e)
				}
				return out
			}
			operatorInitial := operatorReview(0)
			initialProjection := operatorInitial["review"].(map[string]any)["darwin_review"].(map[string]any)
			if initialProjection["cache_state"] != "absent" || initialProjection["effect"] != "initialize" || initialProjection["parent"].(map[string]any)["present"] != false {
				t.Fatal("local operator did not show absent initialization", operatorInitial)
			}
			declined := connect(false)
			cadenceCall(t, declined, "qualify", map[string]any{"job_id": daily, "expected_revision": 0}, true)
			if cadenceTree(t, f) != before {
				t.Fatal("declined Darwin initialization wrote state")
			}
			q := cadenceCall(t, cs, "qualify", map[string]any{"job_id": daily, "expected_revision": 0}, false)
			if q["qualification_state"] != "qualified" || q["state"] != "inactive" {
				t.Fatal(q)
			}
			initial := cadenceCall(t, cs, "report", map[string]any{"job_id": daily}, false)
			if initial["finding"] != "current" || initial["native_observation"] != "not_observed" {
				t.Fatal(initial)
			}
			cadenceCall(t, cs, "qualify", map[string]any{"job_id": "runtime-health-check", "expected_revision": 0}, false)
			stale := cadenceCall(t, cs, "report", map[string]any{"job_id": daily}, false)
			if stale["finding"] != "stale" {
				t.Fatal("real upstream execution did not stale cache", stale)
			}
			operatorRefresh := operatorReview(uint64(q["revision"].(float64)))
			refreshProjection := operatorRefresh["review"].(map[string]any)["darwin_review"].(map[string]any)
			oldParent := initial["darwin"].(map[string]any)["evidence"].(map[string]any)["after"].(map[string]any)["sha256"]
			if refreshProjection["cache_state"] != "stale" || refreshProjection["effect"] != "refresh" || refreshProjection["parent"].(map[string]any)["sha256"] != oldParent {
				t.Fatal("local operator did not show exact stale parent", operatorRefresh)
			}
			repaired := cadenceCall(t, cs, "qualify", map[string]any{"job_id": daily, "expected_revision": q["revision"]}, false)
			if repaired["qualification"].(map[string]any)["id"] == q["qualification"].(map[string]any)["id"] {
				t.Fatal("real refresh lacks new qualified evidence")
			}
			current := cadenceCall(t, cs, "report", map[string]any{"job_id": daily}, false)
			if current["finding"] != "current" {
				t.Fatal(current)
			}
			// Close and restart the actual transport/process before unchanged replay.
			if e := cs.Close(); e != nil {
				t.Fatal(e)
			}
			cs = connect(true)
			replay := cadenceCall(t, cs, "qualify", map[string]any{"job_id": daily, "expected_revision": repaired["revision"]}, false)
			if replay["qualification"].(map[string]any)["id"] != repaired["qualification"].(map[string]any)["id"] {
				t.Fatal("restart replay minted clock qualification")
			}
			for _, id := range []string{weekly, monthly} {
				p := cadenceCall(t, cs, "qualify", map[string]any{"job_id": id, "expected_revision": 0}, false)
				if p["qualification_state"] != "qualified" {
					t.Fatal(p)
				}
				r := cadenceCall(t, cs, "report", map[string]any{"job_id": id}, false)
				evidence := r["darwin"].(map[string]any)["evidence"].(map[string]any)
				if evidence["outcome"] != "proposal_emitted" || evidence["proposal_count"].(float64) < 1 {
					t.Fatal(r)
				}
				again := cadenceCall(t, cs, "qualify", map[string]any{"job_id": id, "expected_revision": p["revision"]}, false)
				if again["qualification"].(map[string]any)["id"] != p["qualification"].(map[string]any)["id"] {
					t.Fatal("proposal replay minted qualification")
				}
				if id == monthly {
					cadenceCall(t, cs, "enroll", map[string]any{"job_id": id, "expected_revision": again["revision"], "timezone": "UTC"}, true)
				}
			}
			enrolled := cadenceCall(t, cs, "enroll", map[string]any{"job_id": daily, "expected_revision": replay["revision"], "timezone": "UTC"}, false)
			status = cadenceCall(t, cs, "status", nil, false)
			active := 0
			for _, raw := range status["jobs"].([]any) {
				if raw.(map[string]any)["state"] == "active" {
					active++
				}
			}
			if active != 1 {
				t.Fatal("separate enrollment activated another job")
			}
			if !stdio {
				offset = 72 * time.Hour
			}
			run := cadenceCall(t, cs, "run", nil, false)
			if !stdio {
				if len(run["due"].([]any)) != 1 {
					t.Fatal("foreground catch-up not one", run)
				}
			} else if run["state"] != "no_due_work" {
				t.Fatal("stdio invented work before boundary", run)
			}
			paused := cadenceCall(t, cs, "pause", map[string]any{"job_id": daily, "expected_revision": enrolled["revision"]}, false)
			resumed := cadenceCall(t, cs, "resume", map[string]any{"job_id": daily, "expected_revision": paused["revision"]}, false)
			cadenceCall(t, cs, "revoke", map[string]any{"job_id": daily, "expected_revision": resumed["revision"]}, false)
			if root := os.Getenv("MAESTRO_DARWIN_TEST_ROOT"); root != "" {
				if !filepath.IsAbs(root) {
					t.Fatal("retained root must be absolute")
				}
				if e := os.MkdirAll(root, 0700); e != nil {
					t.Fatal(e)
				}
				proof := map[string]any{"mode": map[bool]string{true: "shipping_stdio", false: "sdk"}[stdio], "initial": initial, "stale": stale, "current": current, "qualification_preserved": true, "run": run, "native": "not_observed", "real_host_registration": false}
				proof["darwin_review_forms"] = forms
				proof["operator_initial_review"] = operatorInitial
				proof["operator_refresh_review"] = operatorRefresh
				if stdio {
					body, e := os.ReadFile(f.binary)
					if e != nil {
						t.Fatal(e)
					}
					sum := sha256.Sum256(body)
					proof["binary_sha256"] = hex.EncodeToString(sum[:])
				}
				body, e := json.MarshalIndent(proof, "", "  ")
				if e != nil {
					t.Fatal(e)
				}
				if e = os.WriteFile(filepath.Join(root, fmt.Sprintf("darwin-%t-proof.json", stdio)), body, 0600); e != nil {
					t.Fatal(e)
				}
			}
		})
	}
}
