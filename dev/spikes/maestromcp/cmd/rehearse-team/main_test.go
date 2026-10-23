package main

import (
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// A generic failure must never qualify the experiment's stale-writer proof.
func TestRejectionRequiresExactToolConflict(t *testing.T) {
	for _, tc := range []struct {
		name   string
		result *mcp.CallToolResult
		want   string
	}{
		{"missing", nil, ""},
		{"successful-text", &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "revision_conflict"}}}, ""},
		{"generic-error", &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "server unavailable"}}}, ""},
		{"extended-error", &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "revision_conflict: extra"}}}, ""},
		{"bounded-conflict", &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "revision_conflict"}}}, "revision_conflict"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := rejectionCode(tc.result); got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}
