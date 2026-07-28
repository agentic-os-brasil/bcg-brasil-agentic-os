package codexadapter

import (
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/lifecycle"
)

func TestSurfacesExposeOnlyCodexSessionStart(t *testing.T) {
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
			if surface.NativeBinding != "none" || surface.Implementation != "blocked" || surface.NativeObservation != "blocked" || surface.CapabilityState != "unavailable" || surface.Blocker == "" {
				t.Fatalf("blocked Codex surface = %#v", surface)
			}
		default:
			t.Fatalf("unexpected Codex event = %#v", surface)
		}
	}
}

func TestRequireSurfaceFailsClosedForUnsupportedCodexEvents(t *testing.T) {
	if err := RequireSurface(lifecycle.SessionStart); err != nil {
		t.Fatal(err)
	}
	for _, event := range []string{lifecycle.ContextInject, lifecycle.PreActionGuard, lifecycle.PostActionObserve, lifecycle.StopFinalize} {
		if err := RequireSurface(event); err == nil {
			t.Fatalf("Codex event %q was treated as natively available", event)
		}
	}
}
