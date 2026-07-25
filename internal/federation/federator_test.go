package federation

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLocalFederatorPrivateCanariesDoNotAlterCompiledBatch(t *testing.T) {
	first := validLocalPacket()
	first.WorkspaceID = "client-alpha-CANARY"
	first.PrivateContext = "confidential client material CANARY-ONE"
	second := first
	second.WorkspaceID = "client-beta-CANARY"
	second.PrivateContext = "different private material CANARY-TWO"

	firstBatch, err := FederateLocal(first)
	if err != nil {
		t.Fatal(err)
	}
	secondBatch, err := FederateLocal(second)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(firstBatch, secondBatch) {
		t.Fatalf("private context changed federated batch:\nfirst=%#v\nsecond=%#v", firstBatch, secondBatch)
	}
}

func TestLocalFederatorMapsQualitativePerceptionToClosedSignal(t *testing.T) {
	batch, err := FederateLocal(validLocalPacket())
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Signals) != 1 || batch.Signals[0].Kind != SignalFriction || batch.Signals[0].Capability != CapabilityInteractionProfile {
		t.Fatalf("signals = %#v", batch.Signals)
	}
	if len(batch.Candidates) != 1 || batch.Candidates[0].Fingerprint == "" || batch.Candidates[0].Class != CandidateQualityGuard {
		t.Fatalf("candidates = %#v", batch.Candidates)
	}
}

func TestLocalFederatorRejectsUnknownQualitativePerception(t *testing.T) {
	packet := validLocalPacket()
	packet.Findings[0].Perception = QualitativePerception("user prose is not an allowed export")
	if _, err := FederateLocal(packet); err == nil {
		t.Fatal("unknown qualitative perception was accepted")
	}
}

func TestPortableSkillCollectorAcceptsOnlyBornPortablePackage(t *testing.T) {
	root := t.TempDir()
	writePortableSkill(t, root, "handoff-guard", "# Handoff guard\n\nUse the checklist before handoff.\n")
	collector := PortableSkillCollector{Root: root}
	packages, err := collector.Collect()
	if err != nil {
		t.Fatal(err)
	}
	if len(packages) != 1 || packages[0].Manifest.Origin != BornPortable || packages[0].Content == "" {
		t.Fatalf("packages = %#v", packages)
	}
	if _, err := CompileWorkspace(WorkspaceObservation{PortableSkillContent: packages[0].Content}); err == nil {
		t.Fatal("portable skill body was coerced into a typed workspace batch")
	}
}

func TestPortableSkillCollectorRejectsSymlinkedOrMislabeledContent(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, "handoff-guard")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "SKILL.md")
	if err := os.WriteFile(outside, []byte("client-specific-CANARY"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(directory, "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "manifest.json"), []byte(`{"schema_version":1,"skill_id":"handoff-guard","version":"1.0.0","origin":"born_portable","content_sha256":"`+hashContent("client-specific-CANARY")+`","generalizable":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := (PortableSkillCollector{Root: root}).Collect(); err == nil {
		t.Fatal("symlinked portable skill was accepted")
	}
}

func TestPublishedPortableSkillManifestSchemaIsRecognized(t *testing.T) {
	if err := ValidatePortableSkillManifestSchemaFile("../../schemas/portable-skill-manifest.schema.json"); err != nil {
		t.Fatal(err)
	}
}

func validLocalPacket() LocalDarwinPacket {
	return LocalDarwinPacket{
		InstallationID: "0123456789abcdef",
		Period:         "2026-W30",
		ProductVersion: "0.1.0",
		Runtime:        RuntimeCodex,
		WorkspaceID:    "local-only-workspace",
		Findings: []QualitativeFinding{{
			Perception: PerceptionNavigationFriction,
			Stage:      StageFirstUse,
			Evidence:   EvidenceTwoToThree,
			Confidence: ConfidenceHigh,
			Outcome:    OutcomeBlocked,
		}},
		Recipes: []RecipeFinding{{
			Recipe:   RecipeHandoffGuard,
			Evidence: EvidenceTwoToThree,
		}},
	}
}

func writePortableSkill(t *testing.T, root, id, content string) {
	t.Helper()
	directory := filepath.Join(root, id)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "SKILL.md"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	manifest := `{"schema_version":1,"skill_id":"` + id + `","version":"1.0.0","origin":"born_portable","content_sha256":"` + hashContent(content) + `","generalizable":true}`
	if err := os.WriteFile(filepath.Join(directory, "manifest.json"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
}

func hashContent(content string) string {
	digest := sha256.Sum256([]byte(content))
	return hex.EncodeToString(digest[:])
}
