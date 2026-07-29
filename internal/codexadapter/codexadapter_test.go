package codexadapter

import (
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/lifecycle"
)

func TestSurfacesExposeAllCodexLifecycleEventsWithoutNativeQualification(t *testing.T) {
	surfaces := Surfaces()
	if len(surfaces) != 5 {
		t.Fatalf("surfaces = %#v", surfaces)
	}
	for _, surface := range surfaces {
		switch surface.SemanticEvent {
		case lifecycle.SessionStart:
			if surface.NativeBinding != "SessionStart" || surface.Implementation != "configured" || surface.NativeObservation != "not_observed" || surface.CapabilityState != "unavailable" {
				t.Fatalf("SessionStart surface = %#v", surface)
			}
		case lifecycle.ContextInject, lifecycle.PreActionGuard, lifecycle.PostActionObserve, lifecycle.StopFinalize:
			if surface.NativeBinding == "none" || surface.Implementation != "configured" || surface.EvidenceClass != lifecycle.EvidenceContractTested || surface.NativeObservation != "not_observed" || surface.CapabilityState != "unavailable" || surface.Blocker == "" {
				t.Fatalf("unqualified Codex surface = %#v", surface)
			}
		default:
			t.Fatalf("unexpected Codex event = %#v", surface)
		}
	}
}

func TestRequireSurfaceAcceptsOnlyCanonicalCodexEvents(t *testing.T) {
	for _, event := range []string{lifecycle.SessionStart, lifecycle.ContextInject, lifecycle.PreActionGuard, lifecycle.PostActionObserve, lifecycle.StopFinalize} {
		if err := RequireSurface(event); err != nil {
			t.Fatalf("Codex event %q was not recognized: %v", event, err)
		}
	}
	if err := RequireSurface("unknown"); err == nil {
		t.Fatal("unknown Codex event was accepted")
	}
}
