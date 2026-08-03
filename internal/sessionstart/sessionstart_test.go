package sessionstart

import (
	"reflect"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/profile"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/sessionctx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
)

func TestBuildCreatesEquivalentBoundedEnvelopeForClaudeAndCodex(t *testing.T) {
	packet := sessionctx.Build(sessionctx.Sources{
		Profile:   profile.State{Profile: "standard", Source: "configured"},
		Workspace: workspace.Inspection{State: "ready", WorkspaceID: "workspace-a"},
	})
	claude, err := Build("claude", packet)
	if err != nil {
		t.Fatal(err)
	}
	codex, err := Build("codex", packet)
	if err != nil {
		t.Fatal(err)
	}
	if claude.SchemaVersion != 1 || claude.Event != "session_start" || claude.Runtime != "claude" || claude.State != packet.State {
		t.Fatalf("claude envelope = %#v", claude)
	}
	if codex.Runtime != "codex" || !reflect.DeepEqual(codex.Packet, claude.Packet) || codex.State != claude.State {
		t.Fatalf("cross-runtime envelope differs: claude=%#v codex=%#v", claude, codex)
	}
	if claude.AdapterDeliveryState != "contract_only" || claude.InjectionState != "unavailable" || claude.Message == "" {
		t.Fatalf("envelope must not claim native injection: %#v", claude)
	}
}

func TestBuildRejectsUnknownRuntime(t *testing.T) {
	packet := sessionctx.Build(sessionctx.Sources{Profile: profile.State{Profile: "standard", Source: "configured"}, Workspace: workspace.Inspection{State: "ready"}})
	if _, err := Build("other", packet); err == nil {
		t.Fatal("Build accepted an unknown runtime")
	}
}
