package ownerctx

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitializeCreatesInspectablePointersWithoutOverwritingSelf(t *testing.T) {
	root := t.TempDir()
	if _, err := Initialize(root); err != nil {
		t.Fatal(err)
	}
	self := filepath.Join(root, "owner", "self", "voice.md")
	if err := os.WriteFile(self, []byte("# My SELF\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Initialize(root); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(self)
	if err != nil || string(body) != "# My SELF\n" {
		t.Fatalf("self = %q, err = %v", body, err)
	}
	status, err := Inspect(root)
	if err != nil {
		t.Fatal(err)
	}
	if !status.Initialized || !status.Facets["voice"].Available || status.Tasks.State != "unavailable" {
		t.Fatalf("status = %#v", status)
	}
	if profile := status.Facets["psychological-profile"]; profile.Sensitivity != "sensitive" || profile.Refinement != "confirmation_required" || profile.Readers[0] != "walter" {
		t.Fatalf("psychological profile = %#v", profile)
	}
}

func TestAutomaticRefinementAppliesVoiceWithAuditAndCanRevert(t *testing.T) {
	root := t.TempDir()
	if _, err := Initialize(root); err != nil {
		t.Fatal(err)
	}
	capability, err := AuthorizeProducer(root, "test-adapter")
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := SubmitRefinement(root, RefinementInput{Facet: "voice", Evidence: "three owner-approved client emails", ProposedBody: "# Voice\n\nDirect, concise and evidence-led.\n", ProducerID: "test-adapter", Capability: capability})
	if err != nil {
		t.Fatal(err)
	}
	if receipt.State != "applied" || receipt.AuditID == "" {
		t.Fatalf("receipt = %#v", receipt)
	}
	voice, err := os.ReadFile(filepath.Join(root, "owner", "self", "voice.md"))
	if err != nil || !strings.Contains(string(voice), "evidence-led") {
		t.Fatalf("voice = %q, err = %v", voice, err)
	}
	if _, err := RevertRefinement(root, receipt.AuditID, false); err == nil {
		t.Fatal("revert without confirmation succeeded")
	}
	if _, err := RevertRefinement(root, receipt.AuditID, true); err != nil {
		t.Fatal(err)
	}
	voice, err = os.ReadFile(filepath.Join(root, "owner", "self", "voice.md"))
	if err != nil || strings.Contains(string(voice), "evidence-led") {
		t.Fatalf("reverted voice = %q, err = %v", voice, err)
	}
}

func TestAutomaticFacetNeedsAnAuthorizedProducerOrOwnerConfirmation(t *testing.T) {
	root := t.TempDir()
	if _, err := Initialize(root); err != nil {
		t.Fatal(err)
	}
	receipt, err := SubmitRefinement(root, RefinementInput{Facet: "voice", Evidence: "untrusted process", ProposedBody: "# Changed\n", ProducerID: "unknown", Capability: "not-a-capability"})
	if err != nil {
		t.Fatal(err)
	}
	if receipt.State != "proposed" {
		t.Fatalf("untrusted automatic receipt = %#v", receipt)
	}
	if _, err := ApplyRefinement(root, receipt.ID, false); !errors.Is(err, ErrConfirmationRequired) {
		t.Fatalf("unconfirmed apply error = %v", err)
	}
	if _, err := ApplyRefinement(root, receipt.ID, true); err != nil {
		t.Fatal(err)
	}
}

func TestRevertRejectsAStaleAuditRatherThanErasingNewerChange(t *testing.T) {
	root := t.TempDir()
	if _, err := Initialize(root); err != nil {
		t.Fatal(err)
	}
	capability, err := AuthorizeProducer(root, "test-adapter")
	if err != nil {
		t.Fatal(err)
	}
	first, err := SubmitRefinement(root, RefinementInput{Facet: "voice", Evidence: "first", ProposedBody: "# First\n", ProducerID: "test-adapter", Capability: capability})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := SubmitRefinement(root, RefinementInput{Facet: "voice", Evidence: "second", ProposedBody: "# Second\n", ProducerID: "test-adapter", Capability: capability}); err != nil {
		t.Fatal(err)
	}
	if _, err := RevertRefinement(root, first.AuditID, true); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale revert error = %v", err)
	}
}

func TestSensitiveAndProposalOnlyRefinementsRequireConfirmation(t *testing.T) {
	root := t.TempDir()
	if _, err := Initialize(root); err != nil {
		t.Fatal(err)
	}
	for _, facet := range []string{"decision-rules", "psychological-profile"} {
		receipt, err := SubmitRefinement(root, RefinementInput{Facet: facet, Evidence: "owner-provided source", ProposedBody: "# Revised\n"})
		if err != nil {
			t.Fatalf("submit %s: %v", facet, err)
		}
		if receipt.State != "proposed" {
			t.Fatalf("receipt %s = %#v", facet, receipt)
		}
		if _, err := ApplyRefinement(root, receipt.ID, false); err == nil {
			t.Fatalf("apply %s without confirmation succeeded", facet)
		}
		if _, err := ApplyRefinement(root, receipt.ID, true); err != nil {
			t.Fatalf("apply %s with confirmation: %v", facet, err)
		}
	}
}

func TestInitializeCreatesProfessionalFacetsAndInterviewWithoutSensitiveDefaultRead(t *testing.T) {
	root := t.TempDir()
	if _, err := Initialize(root); err != nil {
		t.Fatal(err)
	}
	status, err := Inspect(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"professional-role", "communication-style", "voice", "preferences", "decision-rules", "working-boundaries", "psychological-profile"} {
		facet, ok := status.Facets[id]
		if !ok || !facet.Available {
			t.Fatalf("facet %q = %#v", id, facet)
		}
	}
	if status.Facets["voice"].Refinement != "automatic_with_audit" || status.Facets["decision-rules"].Refinement != "proposal_only" {
		t.Fatalf("unexpected refinement policy: %#v", status.Facets)
	}
	interview := ColdStartInterview()
	if len(interview.Steps) < 3 || interview.Steps[0].Facet != "professional-role" {
		t.Fatalf("interview = %#v", interview)
	}
	for _, step := range interview.Steps {
		if step.Facet == "psychological-profile" {
			t.Fatal("psychological profile must not be a default cold-start question")
		}
	}
}
