package sessionctx

import (
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/atlas"
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
		Atlas: atlas.Status{Managed: atlas.Pointer{State: "unavailable"}, Owner: atlas.Pointer{Path: "/local/atlas/owner", Available: true, State: "available"}, Workspace: atlas.Pointer{Path: "/work/case-a/brain", Available: true, State: "available"}},
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
	if packet.Skills.CatalogPointer != "bundles/base/skills/catalog.json" || packet.Memory.State != "unavailable" {
		t.Fatalf("bounded sources = %#v", packet)
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
