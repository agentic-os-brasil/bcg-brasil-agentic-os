package sessionhook

import (
	"strings"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/profile"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/sessionctx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
)

func TestBuildUsesNativeSessionStartContextWithoutSourceBodies(t *testing.T) {
	packet := sessionctx.Build(sessionctx.Sources{
		Profile:   profile.State{Profile: "standard", Source: "configured"},
		Workspace: workspace.Inspection{State: "ready", WorkspaceID: "workspace-a"},
	})
	output, err := BuildCodex(packet)
	if err != nil {
		t.Fatal(err)
	}
	if output.HookSpecificOutput.HookEventName != "SessionStart" || !strings.Contains(output.HookSpecificOutput.AdditionalContext, `"runtime":"codex"`) {
		t.Fatalf("output = %#v", output)
	}
	if strings.Contains(output.HookSpecificOutput.AdditionalContext, "professional self body") {
		t.Fatalf("output exposed a source body: %s", output.HookSpecificOutput.AdditionalContext)
	}
}

func TestClaudeAndCodexSerializationAreSeparateAdapterCalls(t *testing.T) {
	packet := sessionctx.Build(sessionctx.Sources{
		Profile:   profile.State{Profile: "standard", Source: "configured"},
		Workspace: workspace.Inspection{State: "ready", WorkspaceID: "workspace-a"},
	})
	claude, err := BuildClaude(packet)
	if err != nil {
		t.Fatal(err)
	}
	codex, err := BuildCodex(packet)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(claude.HookSpecificOutput.AdditionalContext, `"runtime":"claude"`) || !strings.Contains(codex.HookSpecificOutput.AdditionalContext, `"runtime":"codex"`) {
		t.Fatal("adapter output did not preserve runtime identity")
	}
}

func TestBuildOmitsOversizedPacketInsteadOfExpandingHookOutput(t *testing.T) {
	packet := sessionctx.Build(sessionctx.Sources{
		Profile: profile.State{
			Profile: "standard",
			Source:  "fallback",
			Warning: strings.Repeat("warning ", MaximumAdditionalContextBytes),
		},
		Workspace: workspace.Inspection{State: "ready", WorkspaceID: "workspace-a"},
	})
	output, err := BuildCodex(packet)
	if err != nil {
		t.Fatal(err)
	}
	if len(output.HookSpecificOutput.AdditionalContext) > MaximumAdditionalContextBytes {
		t.Fatalf("context was %d bytes", len(output.HookSpecificOutput.AdditionalContext))
	}
	if !strings.Contains(output.HookSpecificOutput.AdditionalContext, "omitted") {
		t.Fatalf("context = %q", output.HookSpecificOutput.AdditionalContext)
	}
}
