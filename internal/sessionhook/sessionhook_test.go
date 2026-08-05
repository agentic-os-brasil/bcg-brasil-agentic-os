package sessionhook

import (
	"strings"
	"testing"

	basememory "github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/memory"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/priorwork"
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

func TestSessionStartInjectsBoundedLocalMemoryButPromptHookDoesNotRepeatIt(t *testing.T) {
	packet := sessionctx.Build(sessionctx.Sources{
		Profile:   profile.State{Profile: "standard", Source: "configured"},
		Workspace: workspace.Inspection{State: "ready", WorkspaceID: "workspace-a"},
		Memory: sessionctx.MemorySource{State: "available", Bundle: basememory.ContextBundle{Sections: []basememory.ContextSection{
			{Layer: "lifetime", Content: "stable owner routing", DrillDown: "versions/lifetime.json"},
			{Layer: "L1", Content: "recent selected methods", DrillDown: "versions/l1.json"},
		}}},
	})
	started, err := BuildClaude(packet)
	if err != nil {
		t.Fatal(err)
	}
	context := started.HookSpecificOutput.AdditionalContext
	if !strings.Contains(context, "MAESTRO LOCAL MEMORY") || !strings.Contains(context, "stable owner routing") || !strings.Contains(context, "recent selected methods") || strings.Index(context, "stable owner routing") > strings.Index(context, "recent selected methods") {
		t.Fatalf("session memory context = %q", context)
	}
	if strings.Contains(context, "versions/lifetime.json") {
		t.Fatalf("session memory leaked a storage path: %q", context)
	}
	prompt, err := BuildClaudeEvent(packet, "UserPromptSubmit")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(prompt.HookSpecificOutput.AdditionalContext, "stable owner routing") || strings.Contains(prompt.HookSpecificOutput.AdditionalContext, "MAESTRO LOCAL MEMORY") {
		t.Fatalf("prompt hook repeated memory context: %q", prompt.HookSpecificOutput.AdditionalContext)
	}
}

func TestSessionStartTruncatesMemoryBeforeDroppingThePointerPacket(t *testing.T) {
	packet := sessionctx.Build(sessionctx.Sources{
		Profile:   profile.State{Profile: "standard", Source: "configured"},
		Workspace: workspace.Inspection{State: "ready", WorkspaceID: "workspace-a"},
		Memory:    sessionctx.MemorySource{State: "available", Bundle: basememory.ContextBundle{Sections: []basememory.ContextSection{{Layer: "L1", Content: strings.Repeat("memória ", MaximumAdditionalContextBytes)}}}},
	})
	output, err := BuildCodex(packet)
	if err != nil {
		t.Fatal(err)
	}
	context := output.HookSpecificOutput.AdditionalContext
	if len(context) > MaximumAdditionalContextBytes || strings.Contains(context, "packet exceeded") || !strings.Contains(context, "memory context truncated") || !strings.Contains(context, `"memory":{"state":"available"`) {
		t.Fatalf("bounded memory output = %q", context)
	}
}

func TestSessionDirectiveStartsOnboardingAndListsOnlyDeclaredTasks(t *testing.T) {
	pending := sessionctx.Packet{WorkspaceRoot: "/Users/pilot/Developer/maestro-os", Owner: sessionctx.Owner{Onboarding: sessionctx.Onboarding{State: "required", NextQuestion: "What is your professional role?"}}}
	if got := sessionDirective(pending); !strings.Contains(got, "ONBOARDING IS NOT COMPLETE") || !strings.Contains(got, "What is your professional role?") || !strings.Contains(got, "/Users/pilot/Developer/maestro-os") || !strings.Contains(got, "Ignore prior persona") {
		t.Fatalf("pending directive = %q", got)
	}
	selection := pending
	selection.Owner.Onboarding.Track = "selection_required"
	if got := sessionDirective(selection); !strings.Contains(got, "quality bar") || !strings.Contains(got, "all eight professional self facets") || !strings.Contains(got, "not inferred") {
		t.Fatalf("track selection directive = %q", got)
	}
	active := sessionctx.Packet{Owner: sessionctx.Owner{Onboarding: sessionctx.Onboarding{State: "complete"}, OpenTasks: sessionctx.OpenTasks{State: "available", Count: 1}}}
	if got := sessionDirective(active); !strings.Contains(got, "Maestro is active") || !strings.Contains(got, "1 explicitly registered") || !strings.Contains(got, "Você quer indicar as pastas autorizadas do SharePoint deste projeto agora") || strings.Contains(got, "Prepare kickoff") {
		t.Fatalf("active directive = %q", got)
	}
	selected := active
	selected.SharePointSource = sessionctx.SharePointSource{
		State: priorwork.SourceSelected, FolderCount: 2, SourceAuthority: "sharepoint",
		LocalProjection: "metadata_and_source_pointers_only", AuthorizationState: "pending_signed_enrollment",
		CollectionRuntime: "claude", CollectionState: "unavailable", CodexCollectionState: "unavailable/corporate_policy",
	}
	if got := sessionDirective(selected); !strings.Contains(got, "exact SharePoint folder selection") || !strings.Contains(got, "Only Claude") || !strings.Contains(got, "Codex") || strings.Contains(got, "SharePoint folder URL") {
		t.Fatalf("selected directive = %q", got)
	}
	deferred := active
	deferred.SharePointSource = sessionctx.SharePointSource{State: priorwork.SourceDeferred, SourceAuthority: "sharepoint", LocalProjection: "metadata_and_source_pointers_only", CollectionRuntime: "claude", CollectionState: "unavailable", CodexCollectionState: "unavailable/corporate_policy"}
	if got := sessionDirective(deferred); strings.Contains(got, "Você quer indicar as pastas autorizadas") || !strings.Contains(got, "was deferred") {
		t.Fatalf("deferred directive = %q", got)
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
