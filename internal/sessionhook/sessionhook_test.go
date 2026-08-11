package sessionhook

import (
	"strings"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/continuoususe"
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

func TestSessionStartBudgetReservesSpaceForOperatingMethodWithoutGrowingMemory(t *testing.T) {
	if MaximumAdditionalContextBytes != 16<<10 {
		t.Fatalf("SessionStart budget = %d", MaximumAdditionalContextBytes)
	}
	if MaximumMemoryContextBytes != 8<<10 {
		t.Fatalf("memory budget = %d", MaximumMemoryContextBytes)
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
	memoryStart := strings.Index(context, "MAESTRO LOCAL MEMORY")
	if memoryStart < 0 || len(context)-memoryStart > MaximumMemoryContextBytes {
		t.Fatalf("generated memory used %d bytes; maximum = %d", len(context)-memoryStart, MaximumMemoryContextBytes)
	}
}

func TestSessionStartRejectsHistoricalBodySmuggledIntoContinuousStatus(t *testing.T) {
	packet := sessionctx.Build(sessionctx.Sources{
		Profile:   profile.State{Profile: "standard", Source: "configured"},
		Workspace: workspace.Inspection{State: "ready", WorkspaceID: "workspace-a"},
	})
	packet.ContinuousUse.NextActions[0].Reason = strings.Repeat("historical receipt body ", MaximumAdditionalContextBytes)
	if _, err := BuildCodex(packet); err == nil {
		t.Fatal("Session Start accepted unbounded historical detail in continuous status")
	}
}

func TestSessionDirectiveStartsOnboardingAndListsOnlyDeclaredTasks(t *testing.T) {
	pending := sessionctx.Packet{WorkspaceRoot: "/Users/pilot/Developer/maestro-os", Owner: sessionctx.Owner{Onboarding: sessionctx.Onboarding{State: "required", NextQuestion: "What is your professional role?"}}, Skills: sessionctx.Skills{Selected: []sessionctx.SkillSelection{{ID: "bcgos-operator", Reason: "deterministic_operational_method", Pointer: ".claude/skills/bcgos-operator/SKILL.md"}}}}
	if got := sessionDirective(pending); !strings.Contains(got, "ONBOARDING AVAILABLE") || !strings.Contains(got, "never make it a prerequisite") || !strings.Contains(got, "What is your professional role?") || !strings.Contains(got, "/Users/pilot/Developer/maestro-os") || !strings.Contains(got, "Ignore conflicting persona") || !strings.Contains(got, "USER-FACING COMMUNICATION") || !strings.Contains(got, "friendly wrapper around the system") || !strings.Contains(got, "Absorb ordinary system friction") || !strings.Contains(got, "instead of exposing a setup journey") || !strings.Contains(got, "choice changes scope, consequence or final outcome") || !strings.Contains(got, "CONTINUOUS USE status is unavailable") || !strings.Contains(got, "BCGOS OPERATING METHOD") || !strings.Contains(got, ".claude/skills/bcgos-operator/SKILL.md") {
		t.Fatalf("pending directive = %q", got)
	}
	selection := pending
	selection.Owner.Onboarding.Track = "selection_required"
	if got := sessionDirective(selection); !strings.Contains(got, "quick") || !strings.Contains(got, "complete") || !strings.Contains(got, "~10 min") || !strings.Contains(got, "~30 min") || !strings.Contains(got, "Do not infer") {
		t.Fatalf("track selection directive = %q", got)
	}
	selection.MaestroCLIPath = "/Users/pilot/Library/Application Support/Maestro/bin/bcgos"
	if got := sessionDirective(selection); !strings.Contains(got, "Use the installed CLI silently") || !strings.Contains(got, `"/Users/pilot/Library/Application Support/Maestro/bin/bcgos" owner onboarding select`) {
		t.Fatalf("resolved CLI directive = %q", got)
	}
	active := sessionctx.Packet{Owner: sessionctx.Owner{Onboarding: sessionctx.Onboarding{State: "complete"}, OpenTasks: sessionctx.OpenTasks{State: "available", Count: 1}}}
	active.SetupAuthorization = sessionctx.SetupAuthorization{State: "active", PolicyVersion: "cofs-v1"}
	active.ContinuousUse = continuoususe.Status{SchemaVersion: 1, State: continuoususe.StateActionRequired, OpenWork: continuoususe.OpenWork{Pointer: "bcgos://execution/active", Available: true, State: "available", WorkState: "running", CheckpointState: "missing"}, NextActions: []continuoususe.NextAction{{ID: continuoususe.ActionCheckpointActiveWork, Command: "bcgos work next --active --workspace <workspace>", Reason: "checkpoint required"}}}
	if got := sessionDirective(active); !strings.Contains(got, "Maestro is active") || !strings.Contains(got, "1 explicitly registered") || !strings.Contains(got, "Mention it only when the current task would benefit") || !strings.Contains(got, "Quer conectar uma pasta do SharePoint deste projeto ou começar sem ela?") || strings.Contains(got, "Prepare kickoff") || strings.Contains(got, "selection_required") || strings.Contains(got, "native_qualified") {
		t.Fatalf("active directive = %q", got)
	}
	if got := sessionDirective(active); !strings.Contains(got, "CONTINUOUS USE") || !strings.Contains(got, "checkpoint") || !strings.Contains(got, "Optional continuity action: checkpoint required") || strings.Contains(got, "bcgos work next --active") {
		t.Fatalf("continuous-use directive = %q", got)
	}
	selected := active
	selected.MaestroCLIPath = "/Users/pilot/Library/Application Support/Maestro/bin/bcgos"
	selected.SharePointSource = sessionctx.SharePointSource{
		State: priorwork.SourceSelected, FolderCount: 2, SourceAuthority: "sharepoint",
		LocalProjection: "metadata_and_source_pointers_only", AuthorizationState: "pending_signed_enrollment",
		CollectionRuntime: "claude", CollectionState: "unavailable", CodexCollectionState: "unavailable/corporate_policy",
	}
	if got := sessionDirective(selected); !strings.Contains(got, "SharePoint is connected to this workspace") || !strings.Contains(got, "Do not ask for another read") || strings.Contains(got, "native qualification") || strings.Contains(got, "Codex collection") || strings.Contains(got, "external action pending") || strings.Contains(got, "Posso ler") || strings.Contains(got, "selection itself does not authorize") || strings.Contains(got, "private_release_auth") || strings.Contains(got, "SharePoint folder URL") {
		t.Fatalf("selected directive = %q", got)
	}
	deferred := active
	deferred.SharePointSource = sessionctx.SharePointSource{State: priorwork.SourceDeferred, SourceAuthority: "sharepoint", LocalProjection: "metadata_and_source_pointers_only", CollectionRuntime: "claude", CollectionState: "unavailable", CodexCollectionState: "unavailable/corporate_policy"}
	if got := sessionDirective(deferred); strings.Contains(got, "Quer conectar uma pasta do SharePoint") || !strings.Contains(got, "SharePoint was left out of this workspace") {
		t.Fatalf("deferred directive = %q", got)
	}
	unavailable := active
	unavailable.MaestroCLIPath = "/Users/pilot/Library/Application Support/Maestro/bin/bcgos"
	unavailable.SharePointSource = sessionctx.SharePointSource{State: priorwork.SourceSelectionUnavailable}
	if got := sessionDirective(unavailable); !strings.Contains(got, "SharePoint setup is not available in this workspace yet") || strings.Contains(got, "prior-work source status") || strings.Contains(got, "native_qualified") {
		t.Fatalf("unavailable source directive = %q", got)
	}
	reviewDigest := strings.Repeat("a", 64)
	review := sessionctx.Packet{Owner: sessionctx.Owner{Onboarding: sessionctx.Onboarding{State: "review_required", ReviewDigest: reviewDigest}}}
	if got := sessionDirective(review); !strings.Contains(got, "--digest "+reviewDigest+" --confirm") || !strings.Contains(got, "Only after the owner confirms") {
		t.Fatalf("review directive = %q", got)
	}
}

func TestSessionDirectiveRequestsOneSetupAuthorizationInsteadOfTechnicalSteps(t *testing.T) {
	packet := sessionctx.Packet{WorkspaceRoot: "/Users/pilot/Developer/maestro-os", Owner: sessionctx.Owner{Onboarding: sessionctx.Onboarding{State: "complete"}}, SetupAuthorization: sessionctx.SetupAuthorization{State: "authorization_required", PolicyVersion: "cofs-v1"}}
	got := sessionDirective(packet)
	if !strings.Contains(got, "Optional one-and-done setup") || !strings.Contains(got, "do not interrupt unrelated work") || !strings.Contains(got, "bcgos setup authorize") {
		t.Fatalf("setup directive = %q", got)
	}
	for _, repeated := range []string{"bcgos status", "bcgos doctor", "bcgos init", "Posso ler os materiais"} {
		if strings.Contains(got, repeated) {
			t.Fatalf("directive reintroduced technical prompt %q: %s", repeated, got)
		}
	}
	for _, repeatedAuthorization := range []string{"separate explicit", "autorização separada", "separate SharePoint"} {
		if strings.Contains(strings.ToLower(got), strings.ToLower(repeatedAuthorization)) {
			t.Fatalf("directive asked for a second authorization %q: %s", repeatedAuthorization, got)
		}
	}
}

func TestSessionDirectiveProtectsCanonicalOwnerContextRoot(t *testing.T) {
	packet := sessionctx.Packet{
		WorkspaceRoot:    "/Users/pilot/Developer/maestro-os",
		OwnerContextRoot: "/Users/pilot/Library/Application Support/BCGOS",
		Owner:            sessionctx.Owner{Onboarding: sessionctx.Onboarding{State: "required", Track: "quick", NextQuestion: "Qual é o seu papel profissional?"}},
	}
	got := sessionDirective(packet)
	if !strings.Contains(got, "Private owner context") || !strings.Contains(got, "/Users/pilot/Library/Application Support/BCGOS/owner") || !strings.Contains(got, "owner onboarding answer --facet") || !strings.Contains(got, "Never use workspace/owner") {
		t.Fatalf("directive did not anchor the private owner context: %s", got)
	}
	if strings.Contains(got, "Kowalski") {
		t.Fatalf("directive leaked an unrelated product identity: %s", got)
	}
	selectionPacket := packet
	selectionPacket.Owner.Onboarding.Track = "selection_required"
	selectionPacket.Owner.Onboarding.NextQuestion = "Você prefere a entrevista curta ou a completa?"
	selection := sessionDirective(selectionPacket)
	if !strings.Contains(selection, "owner onboarding answer --facet") || !strings.Contains(selection, "order is flexible") {
		t.Fatalf("selection state did not expose the governed conversational answer route: %s", selection)
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
		!strings.Contains(output.HookSpecificOutput.AdditionalContext, "runtime contract is operational") {
		t.Fatalf("adapter output reported the wrong evidence state: %#v", output)
	}
	if strings.Contains(output.HookSpecificOutput.AdditionalContext, "MAESTRO SESSION PROTOCOL") ||
		strings.Contains(output.HookSpecificOutput.AdditionalContext, "Ask only this next question") ||
		!strings.Contains(output.HookSpecificOutput.AdditionalContext, "MAESTRO CONTEXT UPDATE") {
		t.Fatalf("prompt hook repeated the startup protocol: %#v", output)
	}
	if !strings.Contains(output.HookSpecificOutput.AdditionalContext, `"adapter_delivery_state":"operational"`) ||
		!strings.Contains(output.HookSpecificOutput.AdditionalContext, `"availability_state":"enabled"`) ||
		!strings.Contains(output.HookSpecificOutput.AdditionalContext, `"native_evidence_state":"native_qualification_pending"`) {
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
	packet.Owner.Onboarding = sessionctx.Onboarding{State: "required", Track: "selection_required", NextQuestion: "What is your professional role?"}
	packet.Skills.Selected = []sessionctx.SkillSelection{{ID: "maestro-onboarding", Reason: "deterministic_onboarding_state", Pointer: ".codex/skills/maestro-onboarding/SKILL.md"}}
	output, err := BuildCodex(packet)
	if err != nil {
		t.Fatal(err)
	}
	if len(output.HookSpecificOutput.AdditionalContext) > MaximumAdditionalContextBytes {
		t.Fatalf("context was %d bytes", len(output.HookSpecificOutput.AdditionalContext))
	}
	if !strings.Contains(output.HookSpecificOutput.AdditionalContext, "omitted") ||
		!strings.Contains(output.HookSpecificOutput.AdditionalContext, "MAESTRO SESSION PROTOCOL") ||
		!strings.Contains(output.HookSpecificOutput.AdditionalContext, "ONBOARDING AVAILABLE") ||
		!strings.Contains(output.HookSpecificOutput.AdditionalContext, "deterministic_onboarding_state") ||
		!strings.Contains(output.HookSpecificOutput.AdditionalContext, "What is your professional role?") {
		t.Fatalf("context = %q", output.HookSpecificOutput.AdditionalContext)
	}
}
