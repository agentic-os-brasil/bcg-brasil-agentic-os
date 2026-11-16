//go:build !windows

package product

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestDailyQueuePublicPartialReceipt(t *testing.T) {
	bound, _, session := scopeEffectFixture(t)
	fields := func(id string) map[string]any {
		return map[string]any{"daily_scope": "owner", "checkpoint_id": id, "session_id": "partial-session", "captured_at": "2026-11-15T23:59:00-03:00", "local_date": "2026-11-15", "summary": "Selected " + id, "decisions": []string{}, "next_actions": []string{}}
	}
	effectTool(t, session, "daily.enqueue", fields("first"))
	effectTool(t, session, "daily.enqueue", fields("second"))
	changed := false
	var replacement []byte
	bound.observeWorkspaceTransition = func(stage, _ string) {
		if stage == "daily_consumed" && !changed {
			changed = true
			path := filepath.Join(bound.home, "brain/.maestro/daily-pending/second.json")
			body, e := os.ReadFile(path)
			if e != nil {
				t.Fatal(e)
			}
			var cp map[string]any
			if json.Unmarshal(body, &cp) != nil {
				t.Fatal("invalid fixture")
			}
			cp["summary"] = "Newer unreviewed second checkpoint"
			replacement, _ = json.Marshal(cp)
			if e = os.WriteFile(path, replacement, 0600); e != nil {
				t.Fatal(e)
			}
		}
	}
	out, e := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "maestro_apply", Arguments: map[string]any{"operation": "daily.drain", "daily_scope": "owner", "checkpoint_ids": []string{"first", "second"}}})
	if e != nil || out == nil {
		t.Fatal(out, e)
	}
	var value map[string]any
	if json.Unmarshal([]byte(out.Content[0].(*mcp.TextContent).Text), &value) != nil {
		t.Fatal("invalid result")
	}
	if !changed || !out.IsError {
		t.Fatal("expected deterministic second-input refusal", changed, value)
	}
	committed, ok := value["committed"].([]any)
	if !ok || len(committed) != 1 || committed[0].(map[string]any)["id"] != "first" {
		t.Fatal("first actual committed effect was discarded", value)
	}
	pending, ok := value["unconfirmed"].([]any)
	if !ok || len(pending) != 1 || pending[0] != "second" || value["state"] != "partial" {
		t.Fatal("remaining outcome not explicit", value)
	}
	if value["generation"] != "" && value["generation"] != nil {
		t.Fatal("failed batch claimed unverified current generation", value)
	}
	actual, e := os.ReadFile(filepath.Join(bound.home, "brain/.maestro/daily-pending/second.json"))
	if e != nil || string(actual) != string(replacement) {
		t.Fatal("newer second input lost", e)
	}
}
