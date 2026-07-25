package federation

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestCompileWorkspaceIsNonInterferingForPrivateCanaries(t *testing.T) {
	base := WorkspaceObservation{
		InstallationID: "a1b2c3d4e5f60708",
		Period:         "2026-W30",
		ProductVersion: "v0.1.0",
		Runtime:        RuntimeClaude,
		WorkspaceID:    "client-alpha-secret",
		PrivateText:    "CANARY-ALPHA-DO-NOT-EXPORT",
		Signals: []Signal{{
			Kind: SignalFriction, Capability: CapabilityWorkspaceAgentSetup,
			Stage: StageFirstUse, Evidence: EvidenceTwoToThree,
			Confidence: ConfidenceHigh, Outcome: OutcomeBlocked,
		}},
		Candidates: []SkillCandidate{{
			Class: CandidateContextGuidance, Trigger: TriggerFrictionReduction,
			Dependencies: []CapabilityID{CapabilityWorkspaceAgentSetup},
			Evidence:     EvidenceTwoToThree, SafetyFlags: []SafetyFlag{SafetyRequiresReview},
			Fingerprint: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		}},
	}
	changed := base
	changed.WorkspaceID = "client-beta-secret"
	changed.PrivateText = "CANARY-BETA-DO-NOT-EXPORT"

	first, err := CompileWorkspace(base)
	if err != nil {
		t.Fatal(err)
	}
	second, err := CompileWorkspace(changed)
	if err != nil {
		t.Fatal(err)
	}
	encodedFirst, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	encodedSecond, err := json.Marshal(second)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(encodedFirst, encodedSecond) {
		t.Fatalf("private workspace content changed export:\nfirst=%s\nsecond=%s", encodedFirst, encodedSecond)
	}
	for _, canary := range []string{"client-alpha-secret", "client-beta-secret", "CANARY-ALPHA-DO-NOT-EXPORT", "CANARY-BETA-DO-NOT-EXPORT"} {
		if strings.Contains(string(encodedFirst), canary) {
			t.Fatalf("private canary leaked into batch: %s", encodedFirst)
		}
	}
}

func TestParseRejectsUnknownOrUnapprovedWireData(t *testing.T) {
	validPayload := `{
		"schema_version":1,
		"installation_id":"a1b2c3d4e5f60708",
		"period":"2026-W30",
		"product_version":"v0.1.0",
		"runtime":"claude",
		"signals":[{
			"kind":"friction",
			"capability":"workspace-agent-setup",
			"stage":"first_use",
			"evidence":"two_to_three",
			"confidence":"high",
			"outcome":"blocked"
		}],
		"candidates":[]
	}`
	payload := strings.Replace(validPayload, `"candidates":[]`, `"candidates":[],"workspace_path":"/client/private"`, 1)
	if _, err := Parse(strings.NewReader(payload)); err == nil {
		t.Fatal("unknown workspace path was accepted")
	}

	payload = strings.Replace(validPayload, `"kind":"friction"`, `"kind":"free_text"`, 1)
	if _, err := Parse(strings.NewReader(payload)); err == nil {
		t.Fatal("unapproved signal kind was accepted")
	}
}

func TestParseRejectsDuplicateKeysAndOversizedBatch(t *testing.T) {
	duplicate := `{"schema_version":1,"schema_version":1,"installation_id":"a1b2c3d4e5f60708","period":"2026-W30","product_version":"v0.1.0","runtime":"claude","signals":[],"candidates":[]}`
	if _, err := Parse(strings.NewReader(duplicate)); err == nil {
		t.Fatal("duplicate object key was accepted")
	}
	batch := Batch{SchemaVersion: 1, InstallationID: "a1b2c3d4e5f60708", Period: "2026-W30", ProductVersion: "v0.1.0", Runtime: RuntimeClaude}
	for i := 0; i < 9; i++ {
		batch.Signals = append(batch.Signals, Signal{Kind: SignalFriction, Capability: CapabilityWorkspaceAgentSetup, Stage: StageFirstUse, Evidence: EvidenceOnce, Confidence: ConfidenceLow, Outcome: OutcomeNeutral})
	}
	if err := batch.Validate(); err == nil {
		t.Fatal("oversized signal batch was accepted")
	}
}

func TestBatchRejectsPortableSkillContentUntilPortableCollectorExists(t *testing.T) {
	batch := Batch{
		SchemaVersion:  1,
		InstallationID: "a1b2c3d4e5f60708",
		Period:         "2026-W30",
		ProductVersion: "v0.1.0",
		Runtime:        RuntimeCodex,
		Signals: []Signal{{
			Kind: SignalAdoption, Capability: CapabilityInteractionProfile,
			Stage: StageExecution, Evidence: EvidenceOnce,
			Confidence: ConfidenceMedium, Outcome: OutcomeNeutral,
		}},
		PortableSkillContent: "Never export this before the dedicated collector exists.",
	}
	if err := batch.Validate(); err == nil {
		t.Fatal("portable skill content was accepted before its collector contract exists")
	}
}
