package memory

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"testing"
	"time"
)

func TestCaptureAttestorSealsAndVerifiesTrustedHookMetadata(t *testing.T) {
	root := filepath.Join(t.TempDir(), "memory")
	digest := sha256.Sum256([]byte("session-a\x00meeting-close"))
	capture := Capture{WorkspaceID: "case-a", RecordedAt: time.Now().UTC(), Kind: "skill_route", Text: "codex:meeting-close", Sanitized: true, ProducerID: "codex.context-injection", SanitizerID: SkillRouteSanitizerID, SourceDigest: hex.EncodeToString(digest[:])}
	sealed, err := (CaptureAttestor{Root: root}).Seal(capture)
	if err != nil {
		t.Fatal(err)
	}
	if sealed.SchemaVersion != AttestedCaptureSchemaVersion || len(sealed.Attestation) != 64 {
		t.Fatalf("sealed capture=%#v", sealed)
	}
	if err := (CaptureAttestor{Root: root}).Verify(sealed); err != nil {
		t.Fatal(err)
	}
	sealed.Text = "tampered"
	if err := (CaptureAttestor{Root: root}).Verify(sealed); err == nil {
		t.Fatal("tampered capture passed attestation")
	}
}

func TestCaptureAttestorRejectsSelfDeclaredOrUnknownProducer(t *testing.T) {
	digest := sha256.Sum256([]byte("source"))
	for _, producer := range []string{"", "manual.cli", "unknown.hook"} {
		capture := Capture{WorkspaceID: "case-a", RecordedAt: time.Now().UTC(), Kind: "skill_route", Text: "meeting-close", Sanitized: true, ProducerID: producer, SanitizerID: SkillRouteSanitizerID, SourceDigest: hex.EncodeToString(digest[:])}
		if _, err := (CaptureAttestor{Root: filepath.Join(t.TempDir(), "memory")}).Seal(capture); err == nil {
			t.Fatalf("untrusted producer %q was sealed", producer)
		}
	}
}
