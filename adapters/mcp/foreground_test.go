package product

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installer"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestForegroundAgentStatusIsDiscoverableWithoutExecution(t *testing.T) {
	b := localReviewFixture(t, "")
	client := localReviewSession(t, b, nil)
	out, err := client.CallTool(context.Background(), &mcp.CallToolParams{Name: "maestro_query", Arguments: map[string]any{"operation": "agents.status"}})
	if err != nil || out == nil || out.IsError {
		t.Fatalf("foreground status unavailable: %+v %v", out, err)
	}
	body, _ := json.Marshal(out)
	if !strings.Contains(string(body), "not_started") || !strings.Contains(string(body), "unavailable") {
		t.Fatal("status invented configured execution", string(body))
	}
}
func foregroundControlledFixture(t *testing.T) (*Bound, *bool, *int) {
	t.Helper()
	b := localReviewFixture(t, "local-browser")
	current, calls := true, 0
	r := ForegroundRuntime{RuntimeVersion: "controlled-fixture", ClientRecordSHA256: strings.Repeat("a", 64), Check: func(context.Context) error {
		if !current {
			return errors.New("controlled revocation")
		}
		return nil
	}}
	b.options.ForegroundInspect = func(context.Context, installer.ClientOptions) (ForegroundRuntime, error) { return r, nil }
	b.options.ForegroundExecute = func(ctx context.Context, got ForegroundRuntime, p ForegroundPacket, check func(context.Context) error) (ForegroundExecution, error) {
		calls++
		if err := check(ctx); err != nil {
			return ForegroundExecution{}, err
		}
		return ForegroundExecution{Result: ForegroundResult{1, ForegroundDigest(p), p.Role, "Controlled independent draft fixture; not native evidence"}, SessionID: "controlled-session", RuntimeVersion: r.RuntimeVersion, Model: p.Model}, nil
	}
	return b, &current, &calls
}
func foregroundCall(t *testing.T, b *Bound, client *mcp.ClientSession, fields map[string]any) *mcp.CallToolResult {
	t.Helper()
	scope, err := b.historyBinding("")
	if err != nil {
		t.Fatal(err)
	}
	args := map[string]any{"operation": "agents.run", "agent_role": "yoda", "agent_task": "Review the synthetic product decision", "agent_context": "Quoted synthetic draft", "agent_workspace_id": b.workspaceID, "agent_scope_id": scope.ScopeID, "agent_provider": "controlled", "agent_model": "controlled-model"}
	for k, v := range fields {
		args[k] = v
	}
	out, err := client.CallTool(context.Background(), &mcp.CallToolParams{Name: "maestro_apply", Arguments: args})
	if err != nil {
		t.Fatal(err)
	}
	return out
}
func TestForegroundExactReviewAndDerivedResult(t *testing.T) {
	for _, role := range []string{"yoda", "darwin", "gamma-guardian"} {
		t.Run(role, func(t *testing.T) {
			b, _, calls := foregroundControlledFixture(t)
			before := ownerState(t, b)
			reviews := 0
			b.localOwnerReview = func(_ context.Context, message string, _ any) (bool, error) {
				reviews++
				for _, part := range []string{"Quoted synthetic draft", "controlled-model", role, "no_tools_or_direct_filesystem_effects"} {
					if !strings.Contains(message, part) {
						t.Errorf("review omitted %q", part)
					}
				}
				return true, nil
			}
			out := foregroundCall(t, b, localReviewSession(t, b, nil), map[string]any{"agent_role": role})
			if out.IsError || *calls != 1 || reviews != 1 {
				t.Fatal("approved single call failed", out, *calls, reviews)
			}
			if ownerState(t, b) != before {
				t.Fatal("foreground draft changed owner truth")
			}
			body, _ := json.Marshal(out)
			if !strings.Contains(string(body), "derived_output_only") {
				t.Fatal("result lost effect classification")
			}
		})
	}
}
func TestForegroundRejectsInvalidRoleScopeAndAuthority(t *testing.T) {
	cases := []map[string]any{
		{"agent_role": "maestro"}, {"agent_role": "case-agent"}, {"agent_role": "client-account-agent"}, {"agent_role": "pa-expert"}, {"agent_role": "repository-spec-reviewer"}, {"agent_role": "../yoda"},
		{"agent_workspace_id": "foreign"}, {"agent_scope_id": "foreign"}, {"agent_model": "--settings=foreign"}, {"agent_context": strings.Repeat("a", 8193)},
		{"agent_role": "yoda", "executable": "/bin/sh"}, {"confirmed": true},
	}
	for _, fields := range cases {
		b, _, calls := foregroundControlledFixture(t)
		reviews := 0
		b.localOwnerReview = func(context.Context, string, any) (bool, error) { reviews++; return true, nil }
		out := foregroundCall(t, b, localReviewSession(t, b, nil), fields)
		if !out.IsError || *calls != 0 || reviews != 0 {
			t.Fatal("invalid foreground input reached runtime/review", fields, out, *calls, reviews)
		}
	}
}
func TestForegroundDeclineRevocationAndInvalidOutput(t *testing.T) {
	for _, mode := range []string{"decline", "revoked_during_review", "revoked_during_child", "wrong_digest", "oversize", "cancelled"} {
		t.Run(mode, func(t *testing.T) {
			b, current, calls := foregroundControlledFixture(t)
			b.localOwnerReview = func(context.Context, string, any) (bool, error) {
				if mode == "revoked_during_review" {
					*current = false
				}
				return mode != "decline", nil
			}
			original := b.options.ForegroundExecute
			b.options.ForegroundExecute = func(ctx context.Context, r ForegroundRuntime, p ForegroundPacket, check func(context.Context) error) (ForegroundExecution, error) {
				if mode == "cancelled" {
					return ForegroundExecution{}, context.Canceled
				}
				out, err := original(ctx, r, p, check)
				if mode == "revoked_during_child" {
					*current = false
				}
				if mode == "wrong_digest" {
					out.Result.PacketSHA256 = strings.Repeat("b", 64)
				}
				if mode == "oversize" {
					out.Result.Text = strings.Repeat("a", 8193)
				}
				return out, err
			}
			before := ownerState(t, b)
			out := foregroundCall(t, b, localReviewSession(t, b, nil), nil)
			if !out.IsError {
				t.Fatal("refused call returned publishable output", mode)
			}
			if ownerState(t, b) != before {
				t.Fatal("refused call changed last-good owner state")
			}
			if (mode == "decline" || mode == "revoked_during_review") && *calls != 0 {
				t.Fatal("refused review invoked child")
			}
		})
	}
}
