package runtimecap_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	baseruntime "github.com/agentic-os-brasil/bcg-brasil-agentic-os/bundles/base/runtime"
)

type lifecycleFixture struct {
	SchemaVersion     int                   `json:"schema_version"`
	ReceiptProvenance string                `json:"receipt_provenance"`
	Events            []lifecycleFixtureRow `json:"events"`
}

type lifecycleFixtureRow struct {
	SemanticEvent string                  `json:"semantic_event"`
	Claude        lifecycleFixtureRuntime `json:"claude"`
	Codex         lifecycleFixtureRuntime `json:"codex"`
}

type lifecycleFixtureRuntime struct {
	Binding        string `json:"binding"`
	Implementation string `json:"implementation"`
	EvidenceClass  string `json:"evidence_class"`
	NativeEvidence string `json:"native_evidence"`
	ManifestState  string `json:"manifest_state"`
	Blocker        string `json:"blocker"`
}

func TestLifecycleConformanceFixtureSeparatesClaudeBetaAvailabilityFromQualification(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "adapters", "conformance", "lifecycle.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture lifecycleFixture
	if err := json.Unmarshal(body, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.SchemaVersion != 1 || fixture.ReceiptProvenance != "adapter_command" || len(fixture.Events) != 7 {
		t.Fatalf("fixture = %#v", fixture)
	}
	capabilities, err := baseruntime.Manifest()
	if err != nil {
		t.Fatal(err)
	}
	type runtimeCapability struct {
		State     string
		Mechanism string
		Reason    string
	}
	byEvent := map[string]struct{ Claude, Codex runtimeCapability }{}
	for _, capability := range capabilities.Capabilities {
		claude := capability.Runtimes["claude"]
		codex := capability.Runtimes["codex"]
		byEvent[capability.SemanticEvent] = struct{ Claude, Codex runtimeCapability }{
			Claude: runtimeCapability{State: claude.State, Mechanism: claude.Mechanism, Reason: claude.Reason},
			Codex:  runtimeCapability{State: codex.State, Mechanism: codex.Mechanism, Reason: codex.Reason},
		}
	}
	seen := map[string]bool{}
	for _, row := range fixture.Events {
		states, ok := byEvent[row.SemanticEvent]
		if !ok || seen[row.SemanticEvent] || row.Claude.ManifestState != "operational_beta" || row.Codex.ManifestState != "unavailable" || states.Claude.State != row.Claude.ManifestState || states.Codex.State != row.Codex.ManifestState {
			t.Fatalf("fixture row is not fail-closed: %#v; capability=%#v", row, states)
		}
		if row.Claude.EvidenceClass != "contract-tested" || row.Claude.NativeEvidence != "beta_telemetry" || row.Claude.Blocker == "" || row.Codex.Blocker == "" || row.Codex.NativeEvidence == "" {
			t.Fatalf("fixture evidence state is incomplete: %#v", row)
		}
		seen[row.SemanticEvent] = true
	}
	if fixture.Events[0].Codex.Implementation != "configured" || fixture.Events[0].Codex.Binding != "SessionStart" {
		t.Fatalf("Codex SessionStart fixture = %#v", fixture.Events[0].Codex)
	}
	if fixture.Events[0].Codex.NativeEvidence != "not_observed" || fixture.Events[0].Codex.EvidenceClass != "contract-tested" {
		t.Fatalf("Codex SessionStart native evidence must stay unqualified: %#v", fixture.Events[0].Codex)
	}
	for _, row := range fixture.Events[:5] {
		if row.Codex.Implementation != "configured" || row.Codex.EvidenceClass != "contract-tested" || row.Codex.NativeEvidence != "not_observed" || row.Codex.Blocker == "" {
			t.Fatalf("Codex lifecycle surface must remain configured but unobserved: %#v", row.Codex)
		}
	}
	codexSessionStart := byEvent[fixture.Events[0].SemanticEvent].Codex
	if codexSessionStart.Mechanism != "workspace-local Codex SessionStart binding implemented" || codexSessionStart.Reason != "qualifying native conformance evidence is pending" {
		t.Fatalf("Codex SessionStart capability details drifted: %#v", codexSessionStart)
	}
}
