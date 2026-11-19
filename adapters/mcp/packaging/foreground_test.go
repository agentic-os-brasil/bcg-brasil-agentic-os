package packaging

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
)

func foregroundResponseFixture(t *testing.T, p product.ForegroundPacket, r product.ForegroundRuntime) []byte {
	t.Helper()
	candidate, _ := json.Marshal(product.ForegroundResult{SchemaVersion: 1, PacketSHA256: product.ForegroundDigest(p), Role: p.Role, Text: "Bounded synthetic review"})
	text, _ := json.Marshal(string(candidate))
	return []byte(`{"type":"system","subtype":"init","session_id":"synthetic-independent-session","model":"synthetic-model","claude_code_version":"synthetic-version","tools":[],"mcp_servers":[],"plugins":[],"skills":[],"agents":[]}` + "\n" +
		`{"type":"assistant","message":{"role":"assistant","model":"synthetic-model","content":[{"type":"text","text":` + string(text) + `}]}}` + "\n" +
		`{"type":"result","subtype":"success","is_error":false,"num_turns":1,"result":` + string(text) + `,"usage":{"input_tokens":12,"output_tokens":10},"total_cost_usd":0.01,"modelUsage":{"synthetic-model":{"provider":"firstParty"}}}`)
}
func TestForegroundStrictNativeResponseAndUsage(t *testing.T) {
	r := product.ForegroundRuntime{RuntimeVersion: "synthetic-version"}
	p := product.ForegroundPacket{SchemaVersion: 1, Role: "yoda", Provider: "declared-provider", Model: "synthetic-model"}
	body := foregroundResponseFixture(t, p, r)
	out, err := parseForegroundResponse(body, r, p)
	if err != nil || out.Result.Text != "Bounded synthetic review" || out.ObservedProvider != "firstParty" || out.ProviderEvidence != "native_result_model_usage" || out.CostUSD == nil || *out.CostUSD != 0.01 {
		t.Fatal("bounded native response not parsed", out, err)
	}
	for _, bad := range []string{
		strings.Replace(string(body), `"tools":[]`, `"tools":["Bash"]`, 1),
		strings.Replace(string(body), `"type":"assistant"`, `"type":"system","subtype":"hook_started"`, 1),
		strings.ReplaceAll(string(body), product.ForegroundDigest(p), strings.Repeat("b", 64)),
		strings.Replace(string(body), `"num_turns":1`, `"num_turns":2`, 1),
		strings.Replace(string(body), `"total_cost_usd":0.01`, `"total_cost_usd":-1`, 1),
		strings.Replace(string(body), `"total_cost_usd":0.01`, `"total_cost_usd":1e999`, 1),
		strings.ReplaceAll(string(body), "Bounded synthetic review", strings.Repeat("x", 8193)),
	} {
		if _, e := parseForegroundResponse([]byte(bad), r, p); e == nil {
			t.Fatal("invalid native response became publishable")
		}
	}
	absent := strings.Replace(string(body), `,"modelUsage":{"synthetic-model":{"provider":"firstParty"}}`, "", 1)
	out, err = parseForegroundResponse([]byte(absent), r, p)
	if err != nil || out.ObservedProvider != "" || out.ProviderEvidence != "unavailable" {
		t.Fatal("declared provider presented as observed", out, err)
	}
}
func TestForegroundMissingBindingNeverStartsAProcess(t *testing.T) {
	calls := 0
	r := product.ForegroundRuntime{Check: func(context.Context) error { calls++; return nil }}
	p := product.ForegroundPacket{SchemaVersion: 1, RuntimeSHA256: "foreign", Provider: "synthetic", Model: "synthetic"}
	if _, err := ExecuteForegroundNative(context.Background(), r, p, func(context.Context) error { calls++; return nil }); err == nil || calls != 0 {
		t.Fatal("invalid tuple reached runtime boundary", err, calls)
	}
}
