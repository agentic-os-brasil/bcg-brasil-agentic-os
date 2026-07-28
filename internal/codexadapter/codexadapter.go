// Package codexadapter exposes only the Codex lifecycle surfaces verified by
// the current runtime contract. Unsupported events are represented as blocked
// rather than being emulated with local commands or unit-test receipts.
package codexadapter

import (
	"fmt"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/lifecycle"
)

// Surfaces is the installed Codex topology for the current supported runtime.
// SessionStart has a bounded command seam; the remaining canonical events do
// not have a native product surface and must stay blocked.
func Surfaces() []lifecycle.Surface {
	return []lifecycle.Surface{
		{
			SemanticEvent: lifecycle.SessionStart, NativeBinding: "SessionStart",
			Implementation: "configured", EvidenceClass: lifecycle.EvidenceContractTested,
			NativeObservation: "not_observed", CapabilityState: "unavailable",
			Blocker: "native Codex SessionStart observation is pending",
		},
		blocked(lifecycle.ContextInject),
		blocked(lifecycle.PreActionGuard),
		blocked(lifecycle.PostActionObserve),
		blocked(lifecycle.StopFinalize),
	}
}

func RequireSurface(event string) error {
	for _, surface := range Surfaces() {
		if surface.SemanticEvent != event {
			continue
		}
		if surface.Implementation == "blocked" {
			return fmt.Errorf("Codex lifecycle event %q is blocked: %s", event, surface.Blocker)
		}
		return nil
	}
	return fmt.Errorf("unsupported lifecycle event %q", event)
}

func blocked(event string) lifecycle.Surface {
	return lifecycle.Surface{
		SemanticEvent: event, NativeBinding: "none", Implementation: "blocked",
		NativeObservation: "blocked", CapabilityState: "unavailable",
		Blocker: "the current Codex runtime exposes only a SessionStart command seam; no native product surface is available for this event",
	}
}
