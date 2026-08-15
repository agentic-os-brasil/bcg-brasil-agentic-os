package sessionctx

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/atlas"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/continuoususe"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/execution"
	basememory "github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/memory"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ownerctx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/priorwork"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/profile"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
)

func TestBuildReturnsBoundedPointersAndOmitsUnapprovedSensitiveOwnerFacets(t *testing.T) {
	continuous, err := continuoususe.Build(continuoususe.Source{WorkspaceState: "ready", CalibrationState: "complete", CalibrationTrack: "quick", OpenTasksState: "empty", OpenWorkState: "available", WorkState: "paused", CheckpointState: "available", MemoryState: "available"})
	if err != nil {
		t.Fatal(err)
	}
	packet := Build(Sources{
		Profile: profile.State{Profile: "standard", Source: "configured"},
		Workspace: workspace.Inspection{
			State: "ready", WorkspacePath: "/work/case-a", WorkspaceID: "workspace-a", BrainReadable: true,
		},
		Owner: ownerctx.Status{Initialized: true, Facets: map[string]ownerctx.Facet{
			"owner-identity":        {Pointer: ownerctx.Pointer{Path: "owner/self/owner-identity.md", Available: true, State: "available"}, Readers: []string{"session", "yoda"}, Sensitivity: "identity"},
			"personal-context":      {Pointer: ownerctx.Pointer{Path: "owner/self/personal-context.md", Available: true, State: "available"}, Readers: []string{"session", "yoda"}, Sensitivity: "sensitive"},
			"voice":                 {Pointer: ownerctx.Pointer{Path: "owner/self/voice.md", Available: true, State: "available"}, Readers: []string{"session", "yoda"}},
			"motivations":           {Pointer: ownerctx.Pointer{Path: "owner/self/motivations.md", Available: true, State: "available"}, Readers: []string{"session", "yoda"}},
			"quality-bar":           {Pointer: ownerctx.Pointer{Path: "owner/self/quality-bar.md", Available: true, State: "available"}, Readers: []string{"session", "yoda"}},
			"psychological-profile": {Pointer: ownerctx.Pointer{Path: "owner/self/psychological-profile.md", Available: true, State: "available"}, Readers: []string{"yoda"}, Sensitivity: "sensitive"},
		}, OperatingState: ownerctx.Pointer{Path: "owner/operating/work-state.md", Available: true, State: "available"}},
		Atlas:         atlas.Status{Managed: atlas.Pointer{State: "unavailable"}, Owner: atlas.Pointer{Path: "/local/atlas/owner", Available: true, State: "available"}, Workspace: atlas.Pointer{Path: "/work/case-a/brain", Available: true, State: "available"}},
		Execution:     execution.ActivePointer{Path: execution.ActivePointerPath, Available: true, State: execution.ActivePointerAvailable},
		Memory:        MemorySource{State: "available", Bundle: basememory.ContextBundle{Sections: []basememory.ContextSection{{Layer: "L1", Content: "selected methods: bcg-case-kickoff", DrillDown: "versions/tx/l1.json", Truncated: false}}}},
		ContinuousUse: continuous,
	})
	if err := packet.Validate(); err != nil {
		t.Fatal(err)
	}
	if packet.State != "ready" || packet.InteractionProfile.ID != "standard" || packet.Workspace.ID != "workspace-a" {
		t.Fatalf("packet = %#v", packet)
	}
	if _, ok := packet.Owner.Facets["voice"]; !ok {
		t.Fatalf("session-readable facet missing: %#v", packet.Owner.Facets)
	}
	if _, ok := packet.Owner.Facets["owner-identity"]; !ok {
		t.Fatalf("owner identity pointer missing: %#v", packet.Owner.Facets)
	}
	if _, ok := packet.Owner.Facets["personal-context"]; !ok {
		t.Fatalf("authorized personal context pointer missing: %#v", packet.Owner.Facets)
	}
	if _, ok := packet.Owner.Facets["motivations"]; !ok {
		t.Fatalf("motivations facet missing: %#v", packet.Owner.Facets)
	}
	if _, ok := packet.Owner.Facets["quality-bar"]; !ok {
		t.Fatalf("quality bar facet missing: %#v", packet.Owner.Facets)
	}
	if _, ok := packet.Owner.Facets["psychological-profile"]; ok {
		t.Fatalf("sensitive Yoda-only facet leaked into packet: %#v", packet.Owner.Facets)
	}
	if packet.Atlas.Owner.Path != "bcgos://atlas/owner" || packet.Atlas.Workspace.Path != "bcgos://atlas/workspace" {
		t.Fatalf("atlas references must be portable: %#v", packet.Atlas)
	}
	if packet.Skills.CatalogPointer != "bundles/base/skills/catalog.json" || packet.Agents.CatalogPointer != "bundles/base/agents/catalog.json" || packet.Agents.Hub != "maestro" || packet.Agents.DefinitionsState != "available" || packet.Agents.RuntimeState != "unavailable" || packet.Memory.State != "available" || len(packet.Memory.Layers) != 1 || packet.Memory.Layers[0].Pointer != "bcgos://memory/L1" {
		t.Fatalf("bounded sources = %#v", packet)
	}
	if packet.Execution.Active.Path != execution.ActivePointerPath || !packet.Execution.Active.Available || packet.Execution.Active.State != execution.ActivePointerAvailable {
		t.Fatalf("execution pointer = %#v", packet.Execution)
	}
	if packet.ContinuousUse.OpenWork.Pointer != execution.ActivePointerPath || packet.ContinuousUse.OpenWork.CheckpointState != "available" || packet.ContinuousUse.NextActions[0].ID != continuoususe.ActionResumeActiveWork {
		t.Fatalf("continuous-use projection = %#v", packet.ContinuousUse)
	}
	encoded, err := json.Marshal(packet)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "/local/") || strings.Contains(string(encoded), "/work/") || strings.Contains(string(encoded), "selected methods") || strings.Contains(string(encoded), "versions/tx") || strings.Contains(string(encoded), "item_id") || strings.Contains(string(encoded), "attempt_id") {
		t.Fatalf("packet leaked an absolute local path: %s", encoded)
	}
	var roundTrip Packet
	if err := json.Unmarshal(encoded, &roundTrip); err != nil {
		t.Fatal(err)
	}
	if err := roundTrip.Validate(); err != nil {
		t.Fatalf("pointer-only packet did not survive JSON round trip: %v", err)
	}
}

func TestBuildReportsEmptyAndUnavailableMemoryWithoutRawFallback(t *testing.T) {
	base := Sources{
		Profile:   profile.State{Profile: "standard", Source: "configured"},
		Workspace: workspace.Inspection{State: "ready", WorkspaceID: "workspace-a"},
	}
	empty := Build(func() Sources { value := base; value.Memory = MemorySource{State: "empty"}; return value }())
	if err := empty.Validate(); err != nil || empty.Memory.State != "empty" || len(empty.Memory.Sections) != 0 {
		t.Fatalf("empty memory packet = %#v, err=%v", empty.Memory, err)
	}
	unavailable := Build(base)
	if err := unavailable.Validate(); err != nil || unavailable.Memory.State != "unavailable" || len(unavailable.Memory.Sections) != 0 {
		t.Fatalf("unavailable memory packet = %#v, err=%v", unavailable.Memory, err)
	}
}

func TestBuildAcceptsOperationalBetaAgentRuntimeWithoutPromotingNativeEvidence(t *testing.T) {
	packet := Build(Sources{
		Profile:           profile.State{Profile: "standard", Source: "configured"},
		Workspace:         workspace.Inspection{State: "ready", WorkspaceID: "workspace-a"},
		AgentRuntimeState: "operational_beta",
	})
	if err := packet.Validate(); err != nil {
		t.Fatal(err)
	}
	if packet.Agents.RuntimeState != "operational_beta" || !strings.Contains(packet.Agents.Message, "native qualification") {
		t.Fatalf("agents=%#v", packet.Agents)
	}
}

func TestPacketValidateRejectsTamperedContinuousUse(t *testing.T) {
	packet := Build(Sources{
		Profile:   profile.State{Profile: "standard", Source: "configured"},
		Workspace: workspace.Inspection{State: "ready", WorkspaceID: "workspace-a"},
		Memory:    MemorySource{State: "empty"},
	})
	if err := packet.Validate(); err != nil {
		t.Fatalf("valid packet setup: %v", err)
	}
	packet.ContinuousUse.Runtimes = []continuoususe.RuntimeEvidence{{
		Runtime: "codex",
		CapabilityEvidence: continuoususe.CapabilityEvidence{
			State: continuoususe.EvidenceNativeQualified, NativeQualified: true,
		},
	}}
	if err := packet.Validate(); err == nil {
		t.Fatal("packet accepted tampered continuous-use evidence")
	}
}

func TestBuildFailsClosedForAmbiguousExecutionWithoutLeakingIdentity(t *testing.T) {
	packet := Build(Sources{
		Profile:   profile.State{Profile: "standard", Source: "configured"},
		Workspace: workspace.Inspection{State: "ready", WorkspaceID: "workspace-a"},
		Owner:     ownerctx.Status{Initialized: true},
		Atlas: atlas.Status{
			Managed:   atlas.Pointer{State: "unavailable"},
			Owner:     atlas.Pointer{Available: true, State: "available"},
			Workspace: atlas.Pointer{Available: true, State: "available"},
		},
		Execution: execution.ActivePointer{State: execution.ActivePointerAmbiguous},
	})
	if err := packet.Validate(); err != nil {
		t.Fatal(err)
	}
	if packet.State != "partial" || packet.Execution.Active.Available || packet.Execution.Active.Path != "" || packet.Execution.Active.State != execution.ActivePointerAmbiguous {
		t.Fatalf("ambiguous execution packet = %#v", packet)
	}
	body, err := json.Marshal(packet)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "item-") || strings.Contains(string(body), "attempt-") {
		t.Fatalf("ambiguous packet leaked execution identity: %s", body)
	}
	packet.State = "ready"
	packet.Omissions = nil
	if err := packet.Validate(); err == nil {
		t.Fatal("ambiguous execution validated without a partial omission")
	}
}

func TestBuildFailsClosedForSensitiveSessionReadableFacet(t *testing.T) {
	packet := Build(Sources{
		Profile:   profile.State{Profile: "standard", Source: "configured"},
		Workspace: workspace.Inspection{State: "ready", WorkspaceID: "workspace-a"},
		Owner: ownerctx.Status{Initialized: true, Facets: map[string]ownerctx.Facet{
			"personal-context": {
				Pointer:     ownerctx.Pointer{Path: "owner/self/personal-context.md", Available: true, State: "available"},
				Readers:     []string{"session", "yoda"},
				Sensitivity: "sensitive",
			},
			"psychological-profile": {
				Pointer:     ownerctx.Pointer{Path: "owner/self/psychological-profile.md", Available: true, State: "available"},
				Readers:     []string{"session", "yoda"},
				Sensitivity: "sensitive",
			},
		}},
		Atlas: atlas.Status{Managed: atlas.Pointer{State: "unavailable"}},
	})
	if _, ok := packet.Owner.Facets["personal-context"]; !ok {
		t.Fatalf("authorized sensitive context should be pointer-only in packet: %#v", packet.Owner.Facets)
	}
	if _, ok := packet.Owner.Facets["psychological-profile"]; ok {
		t.Fatalf("sensitive session-readable facet leaked into packet: %#v", packet.Owner.Facets)
	}
}

func TestBuildAllowsOnlyKnownSessionSafeFacets(t *testing.T) {
	packet := Build(Sources{
		Profile:   profile.State{Profile: "standard", Source: "configured"},
		Workspace: workspace.Inspection{State: "ready", WorkspaceID: "workspace-a"},
		Owner: ownerctx.Status{Initialized: true, Facets: map[string]ownerctx.Facet{
			"unreviewed-future-facet": {
				Pointer:     ownerctx.Pointer{Path: "owner/self/unreviewed-future-facet.md", Available: true, State: "available"},
				Readers:     []string{"session"},
				Sensitivity: "professional",
			},
		}},
		Atlas: atlas.Status{Managed: atlas.Pointer{State: "unavailable"}},
	})
	if len(packet.Owner.Facets) != 0 {
		t.Fatalf("unreviewed facet entered the packet: %#v", packet.Owner.Facets)
	}
}

func TestBuildReportsMissingSourcesWithoutReadingBodies(t *testing.T) {
	packet := Build(Sources{
		Profile:   profile.State{Profile: "standard", Source: "default"},
		Workspace: workspace.Inspection{State: "uninitialized", WorkspacePath: "/work/new"},
		Owner:     ownerctx.Status{Tasks: ownerctx.Pointer{State: "unavailable"}},
		Atlas:     atlas.Status{Managed: atlas.Pointer{State: "unavailable"}},
	})
	if err := packet.Validate(); err != nil {
		t.Fatal(err)
	}
	if packet.State != "partial" || len(packet.Omissions) == 0 {
		t.Fatalf("packet = %#v", packet)
	}
}

func TestReviewRequiredOnboardingCarriesOnlyItsBoundedDigest(t *testing.T) {
	reviewDigest := strings.Repeat("b", 64)
	packet := Build(Sources{
		Profile:   profile.State{Profile: "standard", Source: "configured"},
		Workspace: workspace.Inspection{State: "ready", WorkspaceID: "workspace-a"},
		Owner: ownerctx.Status{Initialized: true, Onboarding: ownerctx.OnboardingStatus{
			State: "review_required", Track: ownerctx.OnboardingTrackComplete, ReviewDigest: reviewDigest,
		}},
		Atlas: atlas.Status{Managed: atlas.Pointer{State: "unavailable"}},
	})
	if err := packet.Validate(); err != nil {
		t.Fatal(err)
	}
	if packet.Owner.Onboarding.ReviewDigest != reviewDigest || packet.Owner.Onboarding.NextQuestion != "" {
		t.Fatalf("review projection = %#v", packet.Owner.Onboarding)
	}
	packet.Owner.Onboarding.ReviewDigest = ""
	if err := packet.Validate(); err == nil {
		t.Fatal("review-required onboarding validated without a digest")
	}
}

func TestValidateRejectsMalformedSelfExpansionMetadata(t *testing.T) {
	packet := Build(Sources{Profile: profile.State{Profile: "standard", Source: "default"}, Workspace: workspace.Inspection{State: "missing"}, Owner: ownerctx.Status{Initialized: true, Onboarding: ownerctx.OnboardingStatus{State: "complete", Track: "complete"}}, Memory: MemorySource{State: "empty"}})
	packet.Owner.Expansion = SelfExpansion{State: "action_required", Total: 6, Current: 1, Unknown: 1, Stale: 1, NextFacet: "psychological-profile"}
	if err := packet.Validate(); err == nil {
		t.Fatal("malformed SELF expansion counts and unsafe next facet were accepted")
	}
}

func TestSelectedSkillPointersUseClosedReasonsAndCannotEscapeTheRuntimeProjection(t *testing.T) {
	base := Build(Sources{
		Profile:   profile.State{Profile: "standard", Source: "configured"},
		Workspace: workspace.Inspection{State: "ready", WorkspaceID: "workspace-a"},
		Owner: ownerctx.Status{Initialized: true, Onboarding: ownerctx.OnboardingStatus{
			State: "complete", Track: ownerctx.OnboardingTrackQuick,
		}},
		Atlas: atlas.Status{Managed: atlas.Pointer{State: "unavailable"}},
	})
	valid := SkillSelection{ID: "ingest-content", Reason: "explicit_skill_reference", Pointer: ".claude/skills/ingest-content/SKILL.md"}
	operator := SkillSelection{ID: "execution-continuity", Reason: "deterministic_operational_method", Pointer: ".claude/skills/execution-continuity/SKILL.md"}
	onboarding := SkillSelection{ID: "maestro-onboarding", Reason: "deterministic_onboarding_state", Pointer: ".claude/skills/maestro-onboarding/SKILL.md"}
	base.Skills.Selected = []SkillSelection{operator, onboarding, valid}
	if err := base.Validate(); err != nil {
		t.Fatalf("three bounded selected skill pointers were rejected: %v", err)
	}

	invalid := []SkillSelection{
		{ID: "ingest-content", Reason: "caller_claimed_safe", Pointer: valid.Pointer},
		{ID: "ingest-content", Reason: valid.Reason, Pointer: "/tmp/skills/ingest-content/SKILL.md"},
		{ID: "ingest-content", Reason: valid.Reason, Pointer: ".claude/skills/find-prior-work/SKILL.md"},
		{ID: "../ingest-content", Reason: valid.Reason, Pointer: valid.Pointer},
	}
	for _, selection := range invalid {
		packet := base
		packet.Skills.Selected = []SkillSelection{selection}
		if err := packet.Validate(); err == nil {
			t.Fatalf("unsafe selected skill pointer validated: %#v", selection)
		}
	}
}

func TestSessionPacketCarriesOnlyBoundedSharePointSourceGuidance(t *testing.T) {
	packet := Build(Sources{
		Profile:   profile.State{Profile: "standard", Source: "configured"},
		Workspace: workspace.Inspection{State: "ready", WorkspaceID: strings.Repeat("a", 32)},
		Owner: ownerctx.Status{Initialized: true, Onboarding: ownerctx.OnboardingStatus{
			State: "complete", Track: ownerctx.OnboardingTrackQuick,
		}},
		SharePointSource: priorwork.SourceSelectionStatus{
			SchemaVersion: 1, State: priorwork.SourceSelected, FolderCount: 2,
			Pointer:         "source-selections/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/versions/source-deadbeef.json",
			SourceAuthority: "sharepoint", LocalProjection: "metadata_and_source_pointers_only",
			AuthorizationState: "pending_signed_enrollment", CollectionRuntime: "claude",
			CollectionState: "unavailable", CodexCollectionState: "unavailable/corporate_policy",
		},
	})
	if err := packet.Validate(); err != nil {
		t.Fatal(err)
	}
	if packet.SharePointSource.State != priorwork.SourceSelected || packet.SharePointSource.FolderCount != 2 || packet.SharePointSource.CollectionRuntime != "claude" || packet.SharePointSource.CodexCollectionState != "unavailable/corporate_policy" {
		t.Fatalf("source guidance = %#v", packet.SharePointSource)
	}
	body, err := json.Marshal(packet)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "sharepoint.com") || strings.Contains(string(body), "Authorized-Folder") {
		t.Fatalf("session packet exposed SharePoint source details: %s", body)
	}

	packet.SharePointSource.CollectionRuntime = "codex"
	if err := packet.Validate(); err == nil {
		t.Fatal("session packet accepted Codex as the SharePoint collection runtime")
	}
}
