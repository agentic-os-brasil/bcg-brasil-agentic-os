package spike

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Server exposes the real ledger through the official MCP SDK over stdio.
func (b *Bound) Server() *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "maestro-architecture-spike", Version: "0.0.1"}, nil)
	empty := map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false}
	mcp.AddTool(s, &mcp.Tool{Name: "maestro_context", Description: "Read the bounded durable next action and checkpoint for this server's synthetic case. Read this before work or mutation; never infer context from another case.", InputSchema: empty}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, ContextOutput, error) {
		out, err := b.Context(ctx)
		return nil, out, err
	})
	checkpointSchema := map[string]any{"type": "object", "additionalProperties": false, "required": []string{"expected_revision", "attempt_id", "summary", "next_step", "pause"}, "properties": map[string]any{
		"expected_revision": map[string]any{"type": "integer", "minimum": 1},
		"attempt_id":        map[string]any{"type": "string", "maxLength": 96},
		"summary":           map[string]any{"type": "string", "maxLength": 768},
		"next_step":         map[string]any{"type": "string", "maxLength": 768},
		"pause":             map[string]any{"type": "boolean"},
	}}
	mcp.AddTool(s, &mcp.Tool{Name: "maestro_checkpoint", Description: "Save a bounded synthetic handoff to Maestro's real filesystem ledger. Use the current revision and attempt from maestro_context. Set pause=true to stop the attempt for a later genuine resume.", InputSchema: checkpointSchema}, func(ctx context.Context, _ *mcp.CallToolRequest, input CheckpointInput) (*mcp.CallToolResult, CheckpointOutput, error) {
		out, err := b.Checkpoint(ctx, input)
		return nil, out, err
	})
	resumeSchema := map[string]any{"type": "object", "additionalProperties": false, "required": []string{"expected_revision"}, "properties": map[string]any{"expected_revision": map[string]any{"type": "integer", "minimum": 1}}}
	mcp.AddTool(s, &mcp.Tool{Name: "maestro_resume", Description: "Resume this bound case's paused execution at its current revision. Creates a new attempt; old attempts cannot save checkpoints.", InputSchema: resumeSchema}, func(ctx context.Context, _ *mcp.CallToolRequest, input ResumeInput) (*mcp.CallToolResult, ContextOutput, error) {
		out, err := b.Resume(ctx, input)
		return nil, out, err
	})
	return s
}
