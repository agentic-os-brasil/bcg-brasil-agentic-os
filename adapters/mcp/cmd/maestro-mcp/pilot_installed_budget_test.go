package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// This SDK transport fixture tests request policy only, not authenticated
// runtime readiness, installed authority or native-client qualification.
func installedPilotProtocolSession(t *testing.T, handle func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error)) *mcp.ClientSession {
	t.Helper()
	server := mcp.NewServer(&mcp.Implementation{Name: "installed-pilot-protocol", Version: "1"}, nil)
	server.AddTool(&mcp.Tool{Name: "maestro_query", InputSchema: map[string]any{"type": "object", "properties": map[string]any{"operation": map[string]any{"type": "string"}}, "required": []string{"operation"}, "additionalProperties": false}}, handle)
	ctx, cancel := context.WithCancel(t.Context())
	a, z := mcp.NewInMemoryTransports()
	done := make(chan error, 1)
	go func() { done <- server.Run(ctx, a) }()
	connect, stop := context.WithTimeout(ctx, 20*time.Second)
	session, err := mcp.NewClient(&mcp.Implementation{Name: "installed-policy-test", Version: "1"}, nil).Connect(connect, z, nil)
	stop()
	if err != nil {
		cancel()
		<-done
		t.Fatal(err)
	}
	t.Cleanup(func() { cancel(); session.Close(); <-done })
	return session
}

func TestInstalledPilotStatusHasItsOwnVerificationBudget(t *testing.T) {
	session := installedPilotProtocolSession(t, func(ctx context.Context, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		timer := time.NewTimer(21 * time.Second)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timer.C:
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: `{"sentinel":"complete"}`}}}, nil
	})
	var out struct {
		Sentinel string `json:"sentinel"`
	}
	err := installedPilotCall(t.Context(), session, "content_status", "maestro_query", map[string]any{"operation": "content.status"}, &out)
	if err != nil || out.Sentinel != "complete" {
		t.Fatalf("legitimate status read lost its independent budget: result=%+v err=%v", out, err)
	}
}

func TestInstalledPilotStatusKeepsEarlierCallerDeadline(t *testing.T) {
	session := installedPilotProtocolSession(t, func(ctx context.Context, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		timer := time.NewTimer(time.Second)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timer.C:
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: `{"sentinel":"late"}`}}}, nil
	})
	ctx, cancel := context.WithTimeout(t.Context(), 25*time.Millisecond)
	defer cancel()
	var out struct {
		Sentinel string `json:"sentinel"`
	}
	err := installedPilotCall(ctx, session, "content_status", "maestro_query", map[string]any{"operation": "content.status"}, &out)
	if !errors.Is(err, context.DeadlineExceeded) || err.Error() != "current_in_process_sdk_content_status_deadline_exceeded" || out.Sentinel != "" {
		t.Fatalf("status overrode caller deadline or accepted late data: out=%+v err=%v", out, err)
	}
}

func TestInstalledPilotStatusDoesNotTurnRefusalIntoReadiness(t *testing.T) {
	session := installedPilotProtocolSession(t, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "DO_NOT_EXPORT_PRIVATE_FIXTURE_CAUSE"}}}, nil
	})
	var out struct {
		Sentinel string `json:"sentinel"`
	}
	err := installedPilotCall(t.Context(), session, "content_status", "maestro_query", map[string]any{"operation": "content.status"}, &out)
	if err == nil || err.Error() != "current_in_process_sdk_content_status_failed" || out.Sentinel != "" {
		t.Fatalf("refused read was admitted or exposed its cause: out=%+v err=%v", out, err)
	}
}
