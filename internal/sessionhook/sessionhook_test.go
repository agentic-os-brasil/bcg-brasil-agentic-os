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

func TestSessionDirectiveStartsOnboardingAndListsOnlyDeclaredTasks(t *testing.T) {
	pending := sessionctx.Packet{Owner: sessionctx.Owner{Onboarding: sessionctx.Onboarding{State: "required", NextQuestion: "What is your professional role?"}}}
	if got := sessionDirective(pending); !strings.Contains(got, "ONBOARDING IS NOT COMPLETE") || !strings.Contains(got, "What is your professional role?") || !strings.Contains(got, "Kowalski") {
		t.Fatalf("pending directive = %q", got)
	}
	active := sessionctx.Packet{Owner: sessionctx.Owner{Onboarding: sessionctx.Onboarding{State: "complete"}, OpenTasks: sessionctx.OpenTasks{State: "available", Count: 1}}}
	if got := sessionDirective(active); !strings.Contains(got, "Maestro is active") || !strings.Contains(got, "1 explicitly registered") || strings.Contains(got, "Prepare kickoff") {
		t.Fatalf("active directive = %q", got)
	}
	reviewDigest := strings.Repeat("a", 64)
	review := sessionctx.Packet{Owner: sessionctx.Owner{Onboarding: sessionctx.Onboarding{State: "review_required", ReviewDigest: reviewDigest}}}
	if got := sessionDirective(review); !strings.Contains(got, "--digest "+reviewDigest+" --confirm") || !strings.Contains(got, "Only after the owner confirms") {
		t.Fatalf("review directive = %q", got)
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

func TestClaudeContextInjectionUsesTheSameBoundedPacketWithNativeEventName(t *testing.T) {
	packet := sessionctx.Build(sessionctx.Sources{
		Profile:   profile.State{Profile: "standard", Source: "configured"},
		Workspace: workspace.Inspection{State: "ready", WorkspaceID: "workspace-a"},
	})
	output, err := BuildClaudeEvent(packet, "UserPromptSubmit")
	if err != nil || output.HookSpecificOutput.HookEventName != "UserPromptSubmit" ||
		!strings.Contains(output.HookSpecificOutput.AdditionalContext, `"runtime":"claude"`) ||
		!strings.Contains(output.HookSpecificOutput.AdditionalContext, `"event":"context_inject"`) {
		t.Fatalf("output = %#v, %v", output, err)
	}
	if strings.Contains(output.HookSpecificOutput.AdditionalContext, "wiring is not installed") ||
		!strings.Contains(output.HookSpecificOutput.AdditionalContext, "capability remains unavailable") {
		t.Fatalf("adapter output reported the wrong evidence state: %#v", output)
	}
	if strings.Contains(output.HookSpecificOutput.AdditionalContext, "MAESTRO SESSION PROTOCOL") ||
		strings.Contains(output.HookSpecificOutput.AdditionalContext, "Ask only this next question") ||
		!strings.Contains(output.HookSpecificOutput.AdditionalContext, "MAESTRO CONTEXT UPDATE") {
		t.Fatalf("prompt hook repeated the startup protocol: %#v", output)
	}
	if !strings.Contains(output.HookSpecificOutput.AdditionalContext, `"adapter_delivery_state":"adapter_payload_emitted"`) ||
		!strings.Contains(output.HookSpecificOutput.AdditionalContext, `"injection_state":"unavailable"`) {
		t.Fatalf("adapter output did not separate delivery from qualification: %#v", output)
	}
}

func TestCodexContextInjectionUsesTheSameBoundedPacketWithNativeEventName(t *testing.T) {
	packet := sessionctx.Build(sessionctx.Sources{
		Profile:   profile.State{Profile: "standard", Source: "configured"},
		Workspace: workspace.Inspection{State: "ready", WorkspaceID: "workspace-a"},
	})
	output, err := BuildCodexEvent(packet, "UserPromptSubmit")
	if err != nil || output.HookSpecificOutput.HookEventName != "UserPromptSubmit" ||
		!strings.Contains(output.HookSpecificOutput.AdditionalContext, `"runtime":"codex"`) ||
		!strings.Contains(output.HookSpecificOutput.AdditionalContext, `"event":"context_inject"`) {
		t.Fatalf("output = %#v, %v", output, err)
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
