// Package codexadapter exposes the Codex lifecycle surfaces verified by the
// current runtime contract. Native observation remains separate from local
// configuration and contract tests.
package codexadapter

import (
	"fmt"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/lifecycle"
)

// Surfaces is the installed Codex topology for the current supported runtime.
// Codex exposes all five canonical command-hook events; none is qualified by
// this topology report alone.
func Surfaces() []lifecycle.Surface {
	return []lifecycle.Surface{
		{
			SemanticEvent: lifecycle.SessionStart, NativeBinding: "SessionStart",
			Implementation: "configured", EvidenceClass: lifecycle.EvidenceContractTested,
			NativeObservation: "not_observed", CapabilityState: "unavailable",
			Blocker: "native Codex SessionStart observation is pending",
		},
		configured(lifecycle.ContextInject, "UserPromptSubmit"),
		configured(lifecycle.PreActionGuard, "PreToolUse"),
		configured(lifecycle.PostActionObserve, "PostToolUse"),
		configured(lifecycle.StopFinalize, "Stop"),
	}
}

func RequireSurface(event string) error {
	for _, surface := range Surfaces() {
		if surface.SemanticEvent != event {
			continue
		}
		return nil
	}
	return fmt.Errorf("unsupported lifecycle event %q", event)
}

func configured(event, binding string) lifecycle.Surface {
	return lifecycle.Surface{
		SemanticEvent: event, NativeBinding: binding, Implementation: "configured",
		EvidenceClass: lifecycle.EvidenceContractTested, NativeObservation: "not_observed",
		CapabilityState: "unavailable", Blocker: "native Codex observation is pending",
	}
}
