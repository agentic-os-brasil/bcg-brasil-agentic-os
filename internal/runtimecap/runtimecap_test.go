package runtimecap_test

import (
	"strings"
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
	for _, capability := range claude.Capabilities {
		if capability.ID == "agent_orchestration" && (!capability.Configured || capability.AdapterObserved || capability.NativeQualified) {
			t.Fatalf("agent orchestration evidence levels drifted: %#v", capability)
		}
	}
}

func TestClaudeLifecycleIsOperationalBetaWhileQualificationRemainsTelemetry(t *testing.T) {
	manifest, err := baseruntime.Manifest()
	if err != nil {
		t.Fatal(err)
	}
	report, err := manifest.Report("claude", true)
	if err != nil {
		t.Fatal(err)
	}
	events := map[string]bool{
		"session_start": true, "pre_action_guard": true, "post_action_observe": true,
		"stop_finalize": true, "context_inject": true,
	}
	for _, capability := range report.Capabilities {
		if !events[capability.SemanticEvent] || capability.ID == "agent_orchestration" {
			continue
		}
		if capability.State != "operational_beta" ||
			!strings.Contains(capability.Reason, "native qualification remains telemetry") ||
			capability.NativeQualified {
			t.Fatalf("Claude lifecycle capability reports stale state: %#v", capability)
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
	foundAgentOrchestration := false
	for _, capability := range report.Capabilities {
		if capability.SemanticEvent == "context_inject" && capability.State != "unavailable" {
			t.Fatalf("context injection state = %s", capability.State)
		}
		if capability.ID == "agent_orchestration" {
			foundAgentOrchestration = true
			if capability.State != "unavailable" {
				t.Fatalf("agent orchestration state = %s", capability.State)
			}
		}
	}
	if !foundAgentOrchestration {
		t.Fatal("agent orchestration capability missing")
	}
}

func TestReportDoesNotCallDetectedRuntimeReadyWhenRequiredCapabilityIsUnavailable(t *testing.T) {
	manifest, err := baseruntime.Manifest()
	if err != nil {
		t.Fatal(err)
	}
	for _, runtime := range []string{"claude", "codex"} {
		report, err := manifest.Report(runtime, true)
		if err != nil {
			t.Fatal(err)
		}
		want := "capabilities_unavailable"
		if runtime == "claude" {
			want = "operational_beta"
		}
		if report.State != want {
			t.Fatalf("%s aggregate state = %q", runtime, report.State)
		}
	}
}

func TestSharePointCollectionBoundaryIsRuntimeHonest(t *testing.T) {
	manifest, err := baseruntime.Manifest()
	if err != nil {
		t.Fatal(err)
	}
	for _, runtime := range []string{"claude", "codex"} {
		report, err := manifest.Report(runtime, true)
		if err != nil {
			t.Fatal(err)
		}
		foundCollection := false
		foundQuery := false
		for _, capability := range report.Capabilities {
			switch capability.ID {
			case "sharepoint_work_collection":
				foundCollection = true
				if capability.State != "unavailable" {
					t.Fatalf("%s collection state=%s", runtime, capability.State)
				}
				if runtime == "codex" && capability.Reason != "corporate_policy" {
					t.Fatalf("Codex collection reason=%q", capability.Reason)
				}
			case "sharepoint_work_local_query":
				foundQuery = true
				if capability.State != "native" {
					t.Fatalf("%s local query state=%s", runtime, capability.State)
				}
			}
		}
		if !foundCollection || !foundQuery {
			t.Fatalf("%s missing prior-work capabilities", runtime)
		}
	}
}
