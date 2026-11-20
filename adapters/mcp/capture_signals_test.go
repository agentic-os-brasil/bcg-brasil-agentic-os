package product_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/maintenance"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestCaptureSignalsActualCodePacketAndMCP(t *testing.T) {
	f := newOwnerFixture(t, true)
	packet := filepath.Join(filepath.Dir(f.options.ProductRoot), "code-packet")
	f.cli(t, "plugin", []string{"--server", f.binary, "--destination", packet, "--version", "0.0.0", "--lifecycle-context"}, false)
	hookBody, err := os.ReadFile(filepath.Join(packet, "hooks", "hooks.json"))
	if err != nil {
		t.Fatal(err)
	}
	var hooks struct {
		Hooks map[string][]struct {
			Hooks []struct {
				Command string `json:"command"`
				Shell   string `json:"shell"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	if err = json.Unmarshal(hookBody, &hooks); err != nil {
		t.Fatal(err)
	}
	stopHook := hooks.Hooks["Stop"][0].Hooks[0]
	var stop *exec.Cmd
	if runtime.GOOS == "windows" {
		if stopHook.Shell != "powershell" {
			t.Fatal("emitted Windows Stop did not select its native PowerShell shell")
		}
		shell := filepath.Join(os.Getenv("SystemRoot"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
		stop = exec.Command(shell, "-NoLogo", "-NoProfile", "-NonInteractive", "-Command", stopHook.Command)
	} else {
		stop = exec.Command("/bin/bash", "-c", stopHook.Command)
	}
	stop.Dir = f.options.WorkspacePath
	for _, value := range os.Environ() {
		if !strings.HasPrefix(value, "CLAUDE_PLUGIN_ROOT=") {
			stop.Env = append(stop.Env, value)
		}
	}
	stop.Env = append(stop.Env, "CLAUDE_PLUGIN_ROOT="+packet)
	input, _ := json.Marshal(map[string]any{"hook_event_name": "Stop", "session_id": "signal-session", "prompt_id": "signal-turn", "cwd": f.options.WorkspacePath, "stop_hook_active": false, "last_assistant_message": "PRIVATE_ASSISTANT_BODY"})
	stop.Stdin = strings.NewReader(string(input))
	if out, err := stop.CombinedOutput(); err != nil || strings.TrimSpace(string(out)) != "{}" {
		t.Fatalf("actual emitted Stop: %q %v", out, err)
	}
	configBody, err := os.ReadFile(filepath.Join(packet, ".mcp.json"))
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		Servers map[string]struct {
			Command string   `json:"command"`
			Args    []string `json:"args"`
		} `json:"mcpServers"`
	}
	if err = json.Unmarshal(configBody, &config); err != nil {
		t.Fatal(err)
	}
	connect := func(approve bool) *mcp.ClientSession {
		t.Helper()
		server := config.Servers["maestro"]
		for i := range server.Args {
			server.Args[i] = strings.ReplaceAll(server.Args[i], "${CLAUDE_PLUGIN_ROOT}", packet)
		}
		cmd := exec.Command(strings.ReplaceAll(server.Command, "${CLAUDE_PLUGIN_ROOT}", packet), server.Args...)
		cmd.Dir = f.options.WorkspacePath
		client := mcp.NewClient(&mcp.Implementation{Name: "capture-signal-test", Version: "1"}, &mcp.ClientOptions{ElicitationHandler: func(_ context.Context, r *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
			if !strings.Contains(r.Params.Message, "body-free") && strings.Contains(r.Params.Message, maintenance.MemoryCaptureJobID) {
				t.Error("capture review omitted source/evidence boundary")
			}
			if !approve {
				return &mcp.ElicitResult{Action: "decline"}, nil
			}
			return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approved": true}}, nil
		}})
		cs, err := client.Connect(context.Background(), &mcp.CommandTransport{Command: cmd, TerminateDuration: time.Second}, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { cs.Close() })
		return cs
	}
	cs := connect(true)
	call := func(tool string, args map[string]any, fail bool) map[string]any {
		t.Helper()
		result, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: tool, Arguments: args})
		if err != nil || result.IsError != fail {
			t.Fatalf("%s: %+v %v", tool, result, err)
		}
		var out map[string]any
		if err = json.Unmarshal([]byte(result.Content[0].(*mcp.TextContent).Text), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	if page := memoryCall(t, cs, "memory.list", nil, false); page["total"] != float64(0) {
		t.Fatal("Stop captured memory before qualification")
	}
	item := call("maestro_apply", map[string]any{"operation": "execution.create", "objective": "PRIVATE_OBJECTIVE_BODY", "next_step": "PRIVATE_NEXT_BODY", "artifacts": []string{"result.md"}}, false)
	id := item["item_id"]
	item = call("maestro_apply", map[string]any{"operation": "execution.start", "item_id": id, "expected_revision": item["state_revision"]}, false)
	// A read, plan and failed completion are not committed-result sources.
	call("maestro_query", map[string]any{"operation": "execution.list"}, false)
	call("maestro_apply", map[string]any{"operation": "execution.create", "objective": "PRIVATE_PLAN_ONLY", "next_step": "PRIVATE_PLAN_STEP", "artifacts": []string{"never-produced.md"}}, false)
	failed, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "maestro_apply", Arguments: map[string]any{"operation": "execution.complete", "item_id": id, "expected_revision": item["state_revision"], "attempt_id": item["attempt_id"]}})
	if err != nil || !failed.IsError {
		t.Fatalf("unwitnessed completion was accepted: %+v %v", failed, err)
	}
	if err = os.WriteFile(filepath.Join(f.options.WorkspacePath, "result.md"), []byte("PRIVATE_DOCUMENT_BODY"), 0600); err != nil {
		t.Fatal(err)
	}
	item = call("maestro_apply", map[string]any{"operation": "execution.evidence", "item_id": id, "expected_revision": item["state_revision"], "attempt_id": item["attempt_id"], "criterion_id": "artifact_1"}, false)
	call("maestro_apply", map[string]any{"operation": "execution.complete", "item_id": id, "expected_revision": item["state_revision"], "attempt_id": item["attempt_id"]}, false)
	declined := connect(false)
	cadenceCall(t, declined, "qualify", map[string]any{"job_id": maintenance.MemoryCaptureJobID, "expected_revision": 0}, true)
	if page := memoryCall(t, cs, "memory.list", nil, false); page["total"] != float64(0) {
		t.Fatal("declined capture qualification wrote memory")
	}
	cadenceCall(t, cs, "qualify", map[string]any{"job_id": maintenance.MemoryCaptureJobID, "expected_revision": 0}, false)
	report := cadenceCall(t, cs, "report", map[string]any{"job_id": maintenance.MemoryCaptureJobID}, false)
	counts := report["counts"].(map[string]any)
	if counts["observed_only"] != float64(1) || counts["committed_results"] != float64(1) {
		t.Fatalf("truthful actual source classes: %v", report)
	}
	page := memoryCall(t, cs, "memory.list", nil, false)
	if page["total"] != float64(2) {
		t.Fatal(page)
	}
	for _, raw := range page["captures"].([]any) {
		row := raw.(map[string]any)
		read := memoryCall(t, cs, "memory.read", map[string]any{"capture_id": row["id"]}, false)
		if strings.Contains(read["text"].(string), "PRIVATE_") {
			t.Fatal("private body entered canonical capture")
		}
	}
	cadenceCall(t, cs, "qualify", map[string]any{"job_id": maintenance.MemoryLightDreamJobID, "expected_revision": 0}, false)
	view := memoryCall(t, cs, "memory.context", nil, false)
	body, _ := json.Marshal(view)
	if !strings.Contains(string(body), "observed_only") || !strings.Contains(string(body), "committed_effect") || strings.Contains(string(body), "PRIVATE_") {
		t.Fatalf("public dream context: %s", body)
	}
	if err = cs.Close(); err != nil {
		t.Fatal(err)
	}
	restarted := connect(true)
	after := memoryCall(t, restarted, "memory.context", nil, false)
	again, _ := json.Marshal(after)
	if string(again) != string(body) {
		t.Fatal("restarted packet changed canonical context")
	}
	t.Log("actual compiled packet Stop -> observed_only; artifact-backed MCP completion -> committed_effect; declined capture -> no canonical capture; qualified capture -> L1 -> exact restarted public readback; plan/read/failed completion excluded")
}
