package runtimecap_test

import (
	"testing"

	baseruntime "github.com/agentic-os-brasil/bcg-brasil-agentic-os/bundles/base/runtime"
)

func TestManifestHasEquivalentClaudeAndCodexCapabilities(t *testing.T) {
	manifest, err := baseruntime.Manifest()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	claude, err := manifest.Report("claude", true)
	if err != nil {
		t.Fatal(err)
	}
	codex, err := manifest.Report("codex", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(claude.Capabilities) == 0 || len(claude.Capabilities) != len(codex.Capabilities) {
		t.Fatalf("capability counts claude=%d codex=%d", len(claude.Capabilities), len(codex.Capabilities))
	}
	for index, capability := range claude.Capabilities {
		other := codex.Capabilities[index]
		if capability.ID != other.ID || capability.SemanticEvent != other.SemanticEvent || capability.Criticality != other.Criticality {
			t.Fatalf("capability[%d] claude=%#v codex=%#v", index, capability, other)
		}
	}
}

func TestReportFailsClosedWhenRuntimeIsMissing(t *testing.T) {
	manifest, err := baseruntime.Manifest()
	if err != nil {
		t.Fatal(err)
	}
	report, err := manifest.Report("claude", false)
	if err != nil {
		t.Fatal(err)
	}
	if report.Detected || report.State != "unavailable" {
		t.Fatalf("report = %#v", report)
	}
	for _, capability := range report.Capabilities {
		if capability.State != "unavailable" {
			t.Fatalf("missing runtime reported %s as %s", capability.ID, capability.State)
		}
	}
}

func TestReportKeepsUnwiredProductHooksExplicitlyUnavailable(t *testing.T) {
	manifest, err := baseruntime.Manifest()
	if err != nil {
		t.Fatal(err)
	}
	report, err := manifest.Report("codex", true)
	if err != nil {
		t.Fatal(err)
	}
	for _, capability := range report.Capabilities {
		if capability.SemanticEvent == "context_inject" && capability.State != "unavailable" {
			t.Fatalf("context injection state = %s", capability.State)
		}
	}
}
