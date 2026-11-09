//go:build !windows

package product

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/datahome"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ownerctx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestAuthoredBrainPartialPreparationReportsActualEffects(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	options := Options{ProductRoot: filepath.Join(base, "product"), UserHome: filepath.Join(base, "user"), WorkspacePath: filepath.Join(base, "workspace"), LookupEnv: func(string) (string, bool) { return "", false }}
	for _, path := range []string{options.ProductRoot, options.UserHome} {
		if err = os.Mkdir(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err = os.Chmod(options.ProductRoot, 0500); err != nil {
		t.Fatal(err)
	}
	initialized, err := datahome.InitializeFresh(datahome.Options{ProductRoot: options.ProductRoot, UserHome: options.UserHome, LookupEnv: options.LookupEnv})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = workspace.Initialize(workspace.Options{WorkspacePath: options.WorkspacePath, DataRoot: filepath.Join(initialized.Home, "state")}); err != nil {
		t.Fatal(err)
	}
	if _, err = ownerctx.Initialize(filepath.Join(initialized.Home, "brain")); err != nil {
		t.Fatal(err)
	}
	bound, err := Open(options)
	if err != nil {
		t.Fatal(err)
	}
	bound.afterBrainPrepareEffect = func() error { return errors.New("synthetic interruption after one actual prepared page effect") }
	a, z := mcp.NewInMemoryTransports()
	server, err := bound.Server().Connect(context.Background(), a, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "partial-brain-sdk-proof", Version: "1"}, &mcp.ClientOptions{ElicitationHandler: func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approved": true}}, nil
	}})
	session, err := client.Connect(context.Background(), z, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	call := func(tool, operation string) (map[string]any, bool) {
		out, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: tool, Arguments: map[string]any{"operation": operation}})
		if err != nil {
			t.Fatal(err)
		}
		var value map[string]any
		if len(out.Content) != 1 {
			t.Fatal("unexpected partial result", out)
		}
		if err = json.Unmarshal([]byte(out.Content[0].(*mcp.TextContent).Text), &value); err != nil {
			t.Fatal(err)
		}
		return value, out.IsError
	}
	status, failed := call("maestro_query", "brain.status")
	if failed {
		t.Fatal(status)
	}
	firstMissing := status["missing"].([]any)[0].(string)
	partial, failed := call("maestro_apply", "brain.prepare")
	firstPath := filepath.Join(options.WorkspacePath, "brain", filepath.FromSlash(firstMissing))
	actual, err := os.ReadFile(firstPath)
	if err != nil || len(actual) == 0 {
		t.Fatal("test interruption did not follow an actual prepared page publication", err)
	}
	if !failed || partial["code"] != "brain_prepare_incomplete" || partial["state"] != "incomplete" {
		t.Fatal("partial publication was reported as an opaque failure or success", partial)
	}
	completed, pending := partial["completed"].([]any), partial["pending"].([]any)
	if len(completed) == 0 || len(pending) == 0 {
		t.Fatal("partial preparation lost confirmed or unconfirmed page effects", partial)
	}
	confirmed := false
	for _, entry := range completed {
		result := entry.(map[string]any)
		if result["page"] == firstMissing && result["state"] == "written" {
			confirmed = true
		}
	}
	if !confirmed {
		t.Fatal("actual first prepared page missing from confirmed effects", partial)
	}
	bound.afterBrainPrepareEffect = nil
	retried, failed := call("maestro_apply", "brain.prepare")
	if failed || retried["state"] != "ready" {
		t.Fatal("explicit reviewed retry could not finish preparation", retried)
	}
	preserved, err := os.ReadFile(firstPath)
	if err != nil || string(preserved) != string(actual) {
		t.Fatal("explicit retry overwrote the prior completed page", err)
	}
	final, failed := call("maestro_query", "brain.status")
	if failed || final["prepared"] != true {
		t.Fatal("finished preparation remained incomplete", final)
	}
}
