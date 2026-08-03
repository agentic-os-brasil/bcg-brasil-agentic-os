package sessionctx

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/atlas"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/execution"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ownerctx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/profile"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
)

func TestBuildReturnsBoundedPointersAndOmitsSensitiveOwnerFacets(t *testing.T) {
	packet := Build(Sources{
		Profile: profile.State{Profile: "standard", Source: "configured"},
		Workspace: workspace.Inspection{
			State: "ready", WorkspacePath: "/work/case-a", WorkspaceID: "workspace-a", BrainReadable: true,
		},
		Owner: ownerctx.Status{Initialized: true, Facets: map[string]ownerctx.Facet{
			"voice":                 {Pointer: ownerctx.Pointer{Path: "owner/self/voice.md", Available: true, State: "available"}, Readers: []string{"session", "walter"}},
			"psychological-profile": {Pointer: ownerctx.Pointer{Path: "owner/self/psychological-profile.md", Available: true, State: "available"}, Readers: []string{"walter"}, Sensitivity: "sensitive"},
		}, OperatingState: ownerctx.Pointer{Path: "owner/operating/work-state.md", Available: true, State: "available"}},
		Atlas:     atlas.Status{Managed: atlas.Pointer{State: "unavailable"}, Owner: atlas.Pointer{Path: "/local/atlas/owner", Available: true, State: "available"}, Workspace: atlas.Pointer{Path: "/work/case-a/brain", Available: true, State: "available"}},
		Execution: execution.ActivePointer{Path: execution.ActivePointerPath, Available: true, State: execution.ActivePointerAvailable},
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
	if _, ok := packet.Owner.Facets["psychological-profile"]; ok {
		t.Fatalf("sensitive Walter-only facet leaked into packet: %#v", packet.Owner.Facets)
	}
	if packet.Atlas.Owner.Path != "bcgos://atlas/owner" || packet.Atlas.Workspace.Path != "bcgos://atlas/workspace" {
		t.Fatalf("atlas references must be portable: %#v", packet.Atlas)
	}
	if packet.Skills.CatalogPointer != "bundles/base/skills/catalog.json" || packet.Agents.CatalogPointer != "bundles/base/agents/catalog.json" || packet.Agents.Hub != "maestro" || packet.Agents.DefinitionsState != "available" || packet.Agents.RuntimeState != "unavailable" || packet.Memory.State != "unavailable" {
		t.Fatalf("bounded sources = %#v", packet)
	}
	if packet.Execution.Active.Path != execution.ActivePointerPath || !packet.Execution.Active.Available || packet.Execution.Active.State != execution.ActivePointerAvailable {
		t.Fatalf("execution pointer = %#v", packet.Execution)
	}
	encoded, err := json.Marshal(packet)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "/local/") || strings.Contains(string(encoded), "/work/") {
		t.Fatalf("packet leaked an absolute local path: %s", encoded)
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
			"psychological-profile": {
				Pointer:     ownerctx.Pointer{Path: "owner/self/psychological-profile.md", Available: true, State: "available"},
				Readers:     []string{"session", "walter"},
				Sensitivity: "sensitive",
			},
		}},
		Atlas: atlas.Status{Managed: atlas.Pointer{State: "unavailable"}},
	})
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
			State: "review_required", ReviewDigest: reviewDigest,
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
