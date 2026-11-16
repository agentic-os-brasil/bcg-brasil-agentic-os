package product_test

import (
	"context"
	"encoding/json"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/zipruntime"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"strings"
	"testing"
)

func TestDailyQueueRawEscapedArguments(t *testing.T) {
	f := newOwnerFixture(t, false)
	q, e := zipruntime.OpenDailyQueue(f.root, f.options.WorkspacePath, "owner")
	if e != nil {
		t.Fatal(e)
	}
	defer q.Close()
	summary := strings.Repeat("<", 3000)
	_, e = q.ReviewEnqueue(zipruntime.DailyCheckpoint{SchemaVersion: 1, ID: "escaped-public", SessionID: "session-a", Workspace: f.options.WorkspacePath, Scope: "owner", CapturedAt: "2026-11-15T23:59:00-03:00", LocalDate: "2026-11-15", Summary: summary, Decisions: []string{}, NextActions: []string{}, Provenance: "agent-authored"})
	if e != nil {
		t.Fatal("synthetic checkpoint not admitted by canonical core:", e)
	}
	fields := dailyFields("escaped-public")
	fields["summary"] = summary
	fields["decisions"] = []string{}
	fields["next_actions"] = []string{}
	fields["operation"] = "daily.enqueue"
	fields["daily_scope"] = "owner"
	encoded, _ := json.Marshal(fields)
	t.Logf("valid checkpoint summary=%dB request=%dB exceeds16KiB=%v", len(summary), len(encoded), len(encoded) > 16<<10)
	count := 0
	c := f.connect(t, func(ctx context.Context, r *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		count++
		return dailyApprove(ctx, r)
	})
	out, e := c.CallTool(context.Background(), &mcp.CallToolParams{Name: "maestro_apply", Arguments: json.RawMessage(encoded)})
	if e != nil {
		t.Fatal(e)
	}
	if out.IsError {
		t.Fatalf("core-admitted explicitly escaped JSON checkpoint rejected before review (reviews=%d): %+v", count, out.Content)
	}
	if count != 1 {
		t.Fatal("did not reach exact trusted review", count)
	}
}
