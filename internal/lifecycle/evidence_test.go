package lifecycle

import (
	"strings"
	"testing"
)

func TestEvaluateNativeQualificationRequiresNativeObservation(t *testing.T) {
	for _, evidenceClass := range []string{EvidenceConfigured, EvidenceContractTested, EvidenceAdapterObserved} {
		result, err := EvaluateNativeQualification(QualificationInput{
			Runtime: "claude", Event: SessionStart, RuntimeVersion: "2.1.177",
			EvidenceClass: evidenceClass, NativeSurface: true, NativeObservation: false,
		})
		if err != nil || result.State != "unavailable" || result.EvidenceClass != evidenceClass || result.Blocker == "" {
			t.Fatalf("evidence class %q result = %#v, err = %v", evidenceClass, result, err)
		}
	}
}

func TestEvaluateNativeQualificationBlocksUnsupportedClaudeVersion(t *testing.T) {
	result, err := EvaluateNativeQualification(QualificationInput{
		Runtime: "claude", Event: PreActionGuard, RuntimeVersion: "2.1.119",
		EvidenceClass: EvidenceNativeQualified, NativeSurface: true, NativeObservation: true,
	})
	if err != nil || result.State != "blocked" || !strings.Contains(result.Blocker, ClaudeMinimumVersion) {
		t.Fatalf("result = %#v, err = %v", result, err)
	}
}

func TestEvaluateNativeQualificationBlocksUnsupportedCodexVersion(t *testing.T) {
	result, err := EvaluateNativeQualification(QualificationInput{
		Runtime: "codex", Event: PreActionGuard, RuntimeVersion: "0.144.0",
		EvidenceClass: EvidenceNativeQualified, NativeSurface: true, NativeObservation: true,
	})
	if err != nil || result.State != "blocked" || !strings.Contains(result.Blocker, CodexMinimumVersion) {
		t.Fatalf("result = %#v, err = %v", result, err)
	}
}

func TestEvaluateNativeQualificationQualifiesOnlyWithCompleteEvidence(t *testing.T) {
	result, err := EvaluateNativeQualification(QualificationInput{
		Runtime: "claude", Event: StopFinalize, RuntimeVersion: ClaudeMinimumVersion,
		EvidenceClass: EvidenceNativeQualified, NativeSurface: true, NativeObservation: true,
	})
	if err != nil || result.State != "qualified" || result.EvidenceClass != EvidenceNativeQualified || result.Blocker != "" {
		t.Fatalf("result = %#v, err = %v", result, err)
	}
}

func TestEvaluateNativeQualificationRejectsUnknownEvidenceAndEvents(t *testing.T) {
	if _, err := EvaluateNativeQualification(QualificationInput{
		Runtime: "claude", Event: "unknown", RuntimeVersion: ClaudeMinimumVersion,
		EvidenceClass: EvidenceNativeQualified, NativeSurface: true, NativeObservation: true,
	}); err == nil {
		t.Fatal("unknown lifecycle event was accepted")
	}
	if _, err := EvaluateNativeQualification(QualificationInput{
		Runtime: "claude", Event: SessionStart, RuntimeVersion: ClaudeMinimumVersion,
		EvidenceClass: "native-runtime", NativeSurface: true, NativeObservation: true,
	}); err == nil {
		t.Fatal("unknown evidence class was accepted")
	}
}
