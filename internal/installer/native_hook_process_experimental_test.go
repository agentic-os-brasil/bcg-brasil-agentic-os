//go:build maestro_experimental_installed_seed

package installer

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/datahome"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
)

// Crosses the real producer -> compiled consumer parser -> actual hook handler.
// The test executable alone supplies an unbound, synthetic trusted constructor;
// this does not fabricate installed custody or native host qualification.
func TestStableNativeHookCompiledDelivery(t *testing.T) {
	base := t.TempDir()
	productRoot, user, work := filepath.Join(base, "product"), filepath.Join(base, "owner"), filepath.Join(base, "workspace")
	for _, path := range []string{productRoot, user} {
		if err := os.Mkdir(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("MAESTRO_HOME", filepath.Join(user, "Developer", "brain"))
	home, err := datahome.InitializeFresh(datahome.Options{ProductRoot: productRoot, UserHome: user})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = workspace.Initialize(workspace.Options{WorkspacePath: work, DataRoot: filepath.Join(home.Home, "state")}); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(base, "controlled-native-hook")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	module, err := filepath.Abs("../../adapters/mcp")
	if err != nil {
		t.Fatal(err)
	}
	build := exec.Command("go", "test", "-mod=readonly", "-tags=maestro_experimental_installed_seed", "-c", "-o", binary, "./cmd/maestro-mcp")
	build.Dir = module
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("compile actual consumer test process: %v %s", err, output)
	}
	tail := []string{"--product-root", productRoot, "--user-home", user, "--workspace", work, "--installed-manifest", strings.Repeat("a", 64), "--installed-client-inputs", strings.Repeat("b", 64), "--experimental-seed-sha256", strings.Repeat("c", 64), "--experimental-installed-root", productRoot}
	call := func(event string, fields map[string]any) string {
		t.Helper()
		launch, err := nativeHookLaunch(QualifiedLaunch{Executable: binary, Args: append([]string{"serve"}, tail...)}, event, strings.Repeat("d", 64), strings.Repeat("e", 32))
		if err != nil {
			t.Fatal(err)
		}
		input := map[string]any{"session_id": "synthetic-delivery", "prompt_id": "synthetic-turn", "cwd": work, "hook_event_name": event}
		for key, value := range fields {
			input[key] = value
		}
		body, err := json.Marshal(input)
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		child := exec.CommandContext(ctx, launch.Executable, launch.Args...)
		child.Stdin = bytes.NewReader(body)
		var output, diagnostic bytes.Buffer
		child.Stdout, child.Stderr = &output, &diagnostic
		if err := child.Run(); err != nil {
			t.Fatalf("ordinary %s hook producer cannot reach real handler: %v: %s", event, err, diagnostic.String())
		}
		if !json.Valid(output.Bytes()) {
			t.Fatalf("invalid %s hook output: %s", event, output.String())
		}
		return strings.TrimSpace(output.String())
	}
	for _, event := range []string{"SessionStart", "UserPromptSubmit"} {
		t.Run(event, func(t *testing.T) {
			output := call(event, nil)
			if !strings.Contains(output, `"additionalContext"`) || !strings.Contains(output, "maestro_context") {
				t.Fatalf("actual hook context not delivered: %s", output)
			}
		})
	}
	tool := map[string]any{"tool_name": "Agent", "tool_use_id": "synthetic-dispatch", "tool_input": map[string]any{"subagent_type": "maestro-product:case-agent"}}
	if output := call("PreToolUse", tool); output != "{}" {
		t.Fatalf("ordinary delegation refused: %s", output)
	}
	actor := map[string]any{"agent_id": "synthetic-agent", "agent_type": "maestro-product:case-agent", "stop_hook_active": false}
	if output := call("SubagentStart", actor); !strings.Contains(output, "additionalContext") {
		t.Fatalf("correlated specialist context not delivered: %s", output)
	}
	if output := call("PreToolUse", map[string]any{"tool_name": "Read", "tool_use_id": "synthetic-outside", "tool_input": map[string]any{"file_path": filepath.Join(base, "outside.md")}, "agent_id": actor["agent_id"], "agent_type": actor["agent_type"]}); !strings.Contains(output, `"permissionDecision":"deny"`) {
		t.Fatalf("specialist scope fence lost: %s", output)
	}
	if output := call("SubagentStop", actor); output != "{}" {
		t.Fatalf("correlated specialist stop failed: %s", output)
	}
	if output := call("PostToolUse", tool); output != "{}" {
		t.Fatalf("foreground return failed: %s", output)
	}
	if output := call("Stop", map[string]any{"stop_hook_active": false}); output != "{}" {
		t.Fatalf("observed route could not finalize: %s", output)
	}
}
