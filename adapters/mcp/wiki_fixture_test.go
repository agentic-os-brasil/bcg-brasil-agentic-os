package product_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func wikiCall(t *testing.T, cs *mcp.ClientSession, operation string, fields map[string]any, fail bool) map[string]any {
	t.Helper()
	args := map[string]any{"operation": "wiki." + operation}
	for k, v := range fields {
		args[k] = v
	}
	out, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "maestro_query", Arguments: args})
	if err != nil {
		t.Fatal(err)
	}
	if out.IsError != fail || len(out.Content) != 1 {
		t.Fatalf("wiki.%s: %+v", operation, out)
	}
	var value map[string]any
	if err := json.Unmarshal([]byte(out.Content[0].(*mcp.TextContent).Text), &value); err != nil {
		t.Fatal(err)
	}
	return value
}
