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

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func dailySDK(t *testing.T, c *mcp.ClientSession, operation, scope string, fields map[string]any, fail bool) map[string]any {
	t.Helper()
	args := map[string]any{"operation": "daily." + operation, "daily_scope": scope}
	for k, v := range fields {
		args[k] = v
	}
	tool := "maestro_apply"
	if operation == "status" {
		tool = "maestro_query"
	}
	return contentCall(t, c, tool, args, fail)
}
func dailyFields(id string) map[string]any {
	return map[string]any{"checkpoint_id": id, "session_id": "session-a", "captured_at": "2026-11-15T23:59:00-03:00", "local_date": "2026-11-15", "summary": "Synthetic selected checkpoint", "decisions": []string{"Keep the exact scope"}, "next_actions": []string{"Continue the selected task"}}
}
func dailyApprove(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
	return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approved": true}}, nil
}
func TestDailyQueuePublicJourney(t *testing.T) {
	for _, stdio := range []bool{false, true} {
		t.Run(fmt.Sprint(stdio), func(t *testing.T) {
			f := newOwnerFixture(t, stdio)
			reviewCount := 0
			handler := func(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
				reviewCount++
				if !strings.Contains(req.Params.Message, `"scope": "owner"`) {
					t.Fatal("review omitted exact scope")
				}
				return dailyApprove(context.Background(), req)
			}
			client := f.connect(t, handler)
			tools, e := client.ListTools(context.Background(), nil)
			if e != nil || len(tools.Tools) != 3 {
				t.Fatal(tools, e)
			}
			plan := contentCall(t, client, "maestro_query", map[string]any{"operation": "methods.plan", "method_id": "maestro-operator"}, false)
			queueOwner := false
			for _, raw := range plan["dependencies"].([]any) {
				dep := raw.(map[string]any)
				if dep["id"] == "domain.daily_checkpoint_queue" {
					queueOwner = dep["state"] == "available" && len(dep["fulfillment_operations"].([]any)) == 4
				}
			}
			if !queueOwner || plan["execution_state"] != "not_executed" {
				t.Fatal("queue declaration does not reflect actual registered operations", plan)
			}
			before := methodTreeSnapshot(t, filepath.Dir(f.root))
			status := dailySDK(t, client, "status", "owner", nil, false)
			if status["pending"] != float64(0) || !reflect.DeepEqual(before, methodTreeSnapshot(t, filepath.Dir(f.root))) {
				t.Fatal("query created state", status)
			}
			fields := dailyFields("queue-a")
			queued := dailySDK(t, client, "enqueue", "owner", fields, false)
			if queued["state"] != "queued" {
				t.Fatal(queued)
			}
			if _, e = os.Stat(filepath.Join(f.root, "daily/2026-11-15.md")); !os.IsNotExist(e) {
				t.Fatal("enqueue was confused with publication", e)
			}
			queuedBytes, e := os.ReadFile(filepath.Join(f.root, ".maestro/daily-pending/queue-a.json"))
			if e != nil {
				t.Fatal(e)
			}
			var bound map[string]any
			if json.Unmarshal(queuedBytes, &bound) != nil || bound["workspace"] != f.options.WorkspacePath || bound["scope"] != "owner" || bound["provenance"] != "agent-authored" {
				t.Fatal("server binding missing", bound)
			}
			dailySDK(t, client, "enqueue", "owner", fields, false)
			changed := dailyFields("queue-a")
			changed["summary"] = "Conflicting payload"
			dailySDK(t, client, "enqueue", "owner", changed, true)
			dailySDK(t, client, "drain", "owner", map[string]any{"checkpoint_ids": []string{"queue-a"}}, false)
			page, e := os.ReadFile(filepath.Join(f.root, "daily/2026-11-15.md"))
			if e != nil || !strings.Contains(string(page), fields["summary"].(string)) || strings.Count(string(page), "<!-- maestro:daily-id queue-a ") != 1 {
				t.Fatal("exact daily body not published", e)
			}
			if _, e = os.Stat(filepath.Join(f.root, "daily/2026-11-16.md")); !os.IsNotExist(e) {
				t.Fatal("capture calendar changed after midnight", e)
			}
			status = dailySDK(t, client, "status", "owner", nil, false)
			generation := status["generation"].(string)
			if len(generation) != 64 || status["dream"] != generation || status["pending"] != float64(0) {
				t.Fatal(status)
			}
			if e = client.Close(); e != nil {
				t.Fatal(e)
			}
			client = f.connect(t, handler)
			replay := dailySDK(t, client, "drain", "owner", map[string]any{"checkpoint_ids": []string{"queue-a"}}, false)
			if replay["generation"] != generation {
				t.Fatal("restart replay advanced generation", replay)
			}
			ack := dailySDK(t, client, "ack", "owner", map[string]any{"generation": generation}, false)
			if ack["state"] != "acknowledged" || ack["evidence"] != "agent_asserted_success" {
				t.Fatal(ack)
			}
			dailySDK(t, client, "ack", "owner", map[string]any{"generation": generation}, false)
			dailySDK(t, client, "enqueue", "owner", dailyFields("queue-b"), false)
			dailySDK(t, client, "drain", "owner", map[string]any{"checkpoint_ids": []string{"queue-b"}}, false)
			dailySDK(t, client, "ack", "owner", map[string]any{"generation": generation}, true)
			current := dailySDK(t, client, "status", "owner", nil, false)
			if current["dream"] == "" || current["dream"] == generation {
				t.Fatal("stale ack consumed new work", current)
			}
			if reviewCount != 8 {
				t.Fatal("mutations bypassed native review", reviewCount)
			}
		})
	}
}
func TestDailyQueueConfirmationPreservesState(t *testing.T) {
	for _, kind := range []string{"decline", "unavailable", "pending"} {
		t.Run(kind, func(t *testing.T) {
			f := newOwnerFixture(t, false)
			var handler func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error)
			if kind == "decline" {
				handler = func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
					return &mcp.ElicitResult{Action: "decline"}, nil
				}
			}
			if kind == "pending" {
				f.manualInputResponses = true

			}
			client := f.connect(t, handler)
			before := methodTreeSnapshot(t, filepath.Dir(f.root))
			if kind == "pending" {
				args := dailyFields("pending")
				args["operation"] = "daily.enqueue"
				args["daily_scope"] = "owner"
				out, e := client.CallTool(context.Background(), &mcp.CallToolParams{Name: "maestro_apply", Arguments: args, Meta: mcp.Meta{mcp.MetaKeyClientCapabilities: map[string]any{"elicitation": map[string]any{"form": map[string]any{}}}}})
				if e != nil || out == nil || len(out.InputRequests) != 1 {
					t.Fatal("pending roundtrip not preserved", out, e)
				}
			} else {
				out := dailySDK(t, client, "enqueue", "owner", dailyFields("pending"), true)
				want := "confirmation_declined"
				if kind == "unavailable" {
					want = "confirmation_unavailable"
				}
				if out["code"] != want {
					t.Fatal(out)
				}
			}
			if !reflect.DeepEqual(before, methodTreeSnapshot(t, filepath.Dir(f.root))) {
				t.Fatal("unapproved call changed queue")
			}
		})
	}
}
func dailyActivateCase(t *testing.T, f ownerFixture, client *mcp.ClientSession, mode string) (string, string) {
	t.Helper()
	in := map[string]any{"operation": "case.setup.prepare", "mode": mode, "project_name": "Synthetic daily scope", "expected_setup_revision": "absent"}
	if mode == "client" {
		in["account_id"] = "synthetic"
		in["account_owner"] = "owner"
		in["account_mandate"] = "Keep the synthetic daily case local"
	}
	status := contentCall(t, client, "maestro_apply", in, false)
	id := status["workspace_id"].(string)
	page := status["setup"].(map[string]any)["brief_page"].(string)
	body := authoredBrief(id, page, "Synthetic daily scope")
	if mode == "client" {
		body = strings.Replace(body, "scope: workspace/"+id, "scope: account/synthetic/case/"+id, 1)
		body = strings.Replace(body, "sensitivity: internal", "sensitivity: client-confidential", 1)
	}
	created := brainCall(t, client, "create", map[string]any{"page": page, "text": body}, false)
	status = contentCall(t, client, "maestro_apply", map[string]any{"operation": "case.setup.finish", "expected_setup_revision": status["setup_revision"], "expected_page_revision": created["revision"]}, false)
	if status["state"] != "active" {
		t.Fatal(status)
	}
	return id, page
}
func TestDailyQueueAdmittedScopesAndLateSelectionChange(t *testing.T) {
	for _, mode := range []string{"client", "internal"} {
		t.Run(mode, func(t *testing.T) {
			f := newOwnerFixture(t, false)
			change := false
			client := f.connect(t, func(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
				if change && strings.Contains(req.Params.Message, `"operation": "daily.drain"`) {
					p := filepath.Join(f.options.WorkspacePath, "brain/accounts/.active")
					if e := os.WriteFile(p, []byte("synthetic/foreign\n"), 0600); e != nil {
						t.Fatal(e)
					}
				}
				return dailyApprove(context.Background(), req)
			})
			scope := "case"
			if mode == "internal" {
				scope = "workspace"
			}
			dailySDK(t, client, "status", scope, nil, true)
			id, _ := dailyActivateCase(t, f, client, mode)
			dailySDK(t, client, "enqueue", scope, dailyFields("scoped"), false)
			root := filepath.Join(f.options.WorkspacePath, "brain")
			if mode == "client" {
				root = filepath.Join(root, "accounts/synthetic/cases", id)
			}
			if _, e := os.Stat(filepath.Join(f.root, ".maestro/daily-pending/scoped.json")); !os.IsNotExist(e) {
				t.Fatal("case fell back to owner", e)
			}
			if mode == "client" {
				change = true
				dailySDK(t, client, "drain", scope, map[string]any{"checkpoint_ids": []string{"scoped"}}, true)
				change = false
				if _, e := os.Stat(filepath.Join(root, ".maestro/daily-pending/scoped.json")); e != nil {
					t.Fatal("selection change lost input", e)
				}
				if e := os.WriteFile(filepath.Join(f.options.WorkspacePath, "brain/accounts/.active"), []byte("synthetic/"+id+"\n"), 0600); e != nil {
					t.Fatal(e)
				}
			}
			dailySDK(t, client, "drain", scope, map[string]any{"checkpoint_ids": []string{"scoped"}}, false)
			body, e := os.ReadFile(filepath.Join(root, "daily/2026-11-15.md"))
			if e != nil {
				t.Fatal(e)
			}
			expected := "scope: account/synthetic/case/" + id
			sensitivity := "client-confidential"
			if mode == "internal" {
				expected = "scope: workspace/" + id
				sensitivity = "internal"
			}
			if !strings.Contains(string(body), expected) || !strings.Contains(string(body), "sensitivity: "+sensitivity) {
				t.Fatal("daily scope metadata incorrect", string(body))
			}
		})
	}
}

func TestDailyQueueEscapedReviewBudgetPreservesQueue(t *testing.T) {
	f := newOwnerFixture(t, false)
	calls := 0
	client := f.connect(t, func(ctx context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		calls++
		return dailyApprove(ctx, req)
	})
	if e := os.MkdirAll(filepath.Join(f.root, "daily"), 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(f.root, "daily/2026-11-15.md"), []byte(strings.Repeat(`"`, 600000)), 0600); e != nil {
		t.Fatal(e)
	}
	dailySDK(t, client, "enqueue", "owner", dailyFields("budget"), false)
	before := methodTreeSnapshot(t, filepath.Dir(f.root))
	out := dailySDK(t, client, "drain", "owner", map[string]any{"checkpoint_ids": []string{"budget"}}, true)
	if out["code"] != "daily_review_too_large" || calls != 1 || !reflect.DeepEqual(before, methodTreeSnapshot(t, filepath.Dir(f.root))) {
		t.Fatal("escaped oversized review reached effects", out, calls)
	}
}
