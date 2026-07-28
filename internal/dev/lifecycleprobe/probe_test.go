package lifecycleprobe

import (
	"errors"
	"testing"
)

func TestProbeKeepsOldClaudeVersionBlocked(t *testing.T) {
	result, err := Probe("claude", func(string) (string, error) { return "/usr/local/bin/claude", nil }, func(string) (string, error) { return "2.1.119 (Claude Code)", nil })
	if err != nil || result.State != "blocked" || result.NativeObservation != "not_observed" || result.CapabilityState != "unavailable" || result.Blocker == "" {
		t.Fatalf("result = %#v, %v", result, err)
	}
	if len(result.Surfaces) != 5 || result.Surfaces[0].CapabilityState != "unavailable" {
		t.Fatalf("Claude lifecycle surfaces = %#v", result.Surfaces)
	}
}

func TestProbeDoesNotPromoteNewEnoughClaudeWithoutNativeObservation(t *testing.T) {
	result, err := Probe("claude", func(string) (string, error) { return "/usr/local/bin/claude", nil }, func(string) (string, error) { return "2.1.177", nil })
	if err != nil || result.State != "not_observed" || result.CapabilityState != "unavailable" {
		t.Fatalf("result = %#v, %v", result, err)
	}
}

func TestProbeReportsCodexBindingGapWithoutClaimingNativeEvidence(t *testing.T) {
	result, err := Probe("codex", func(string) (string, error) { return "/usr/local/bin/codex", nil }, func(string) (string, error) { return "codex-cli 0.144.1", nil })
	if err != nil || result.State != "blocked" || result.CapabilityState != "unavailable" || result.Blocker == "" {
		t.Fatalf("result = %#v, %v", result, err)
	}
	if len(result.Surfaces) != 5 {
		t.Fatalf("Codex lifecycle surfaces = %#v", result.Surfaces)
	}
	for _, surface := range result.Surfaces[1:] {
		if surface.Implementation != "blocked" || surface.NativeObservation != "blocked" || surface.CapabilityState != "unavailable" {
			t.Fatalf("Codex unsupported surface = %#v", surface)
		}
	}
}

func TestProbeReportsMissingExecutableWithoutFailingTheAudit(t *testing.T) {
	result, err := Probe("claude", func(string) (string, error) { return "", errors.New("not found") }, nil)
	if err != nil || result.State != "blocked" || result.ExecutableDetected {
		t.Fatalf("result = %#v, %v", result, err)
	}
}
