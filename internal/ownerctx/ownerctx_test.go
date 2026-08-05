package ownerctx

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
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
	if !status.Initialized || !status.Facets["voice"].Available || status.Tasks.State != "empty" {
		t.Fatalf("status = %#v", status)
	}
	if profile := status.Facets["psychological-profile"]; profile.Sensitivity != "sensitive" || profile.Refinement != "confirmation_required" || profile.Readers[0] != "walter" {
		t.Fatalf("psychological profile = %#v", profile)
	}
}

func TestInitializeAddsNewProfessionalFacetsWithoutOverwritingExistingRegistry(t *testing.T) {
	root := t.TempDir()
	if _, err := Initialize(root); err != nil {
		t.Fatal(err)
	}
	registryPath := filepath.Join(root, "owner", "registry.json")
	body, err := os.ReadFile(registryPath)
	if err != nil {
		t.Fatal(err)
	}
	var value registry
	if err := json.Unmarshal(body, &value); err != nil {
		t.Fatal(err)
	}
	delete(value.Facets, "motivations")
	delete(value.Facets, "quality-bar")
	value.OnboardingTrack = OnboardingTrackQuick
	value.OnboardingConfirmedSHA256 = "owner-reviewed-digest"
	body, err = json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(registryPath, append(body, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Initialize(root); err != nil {
		t.Fatal(err)
	}
	status, err := Inspect(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"motivations", "quality-bar"} {
		if facet, ok := status.Facets[id]; !ok || !facet.Available {
			t.Fatalf("migrated facet %q = %#v", id, facet)
		}
	}
	if status.Onboarding.Track != OnboardingTrackQuick || status.Onboarding.State != "required" {
		t.Fatalf("migration changed onboarding state unexpectedly: %#v", status.Onboarding)
	}
}

func TestOccurrenceBoundRefinementIsIdempotent(t *testing.T) {
	root := t.TempDir()
	if _, err := Initialize(root); err != nil {
		t.Fatal(err)
	}
	input := RefinementInput{Facet: "voice", Evidence: "weekly-occurrence-evidence", ProposedBody: "# Voice\n\nConcise.\n", OccurrenceID: "occurrence-digest-1"}
	first, err := SubmitRefinement(root, input)
	if err != nil {
		t.Fatal(err)
	}
	second, err := SubmitRefinement(root, input)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == "" || first.ID != second.ID || first.ProposalSHA256 == "" || first.ProposalSHA256 != second.ProposalSHA256 {
		t.Fatalf("idempotent receipts differ: first=%+v second=%+v", first, second)
	}
	entries, err := os.ReadDir(filepath.Join(root, "owner", "refinement", "proposals"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("proposal retry created %d proposal files", len(entries))
	}
}

func TestOccurrenceBoundRefinementRejectsDivergentWriterWithoutOverwrite(t *testing.T) {
	root := t.TempDir()
	if _, err := Initialize(root); err != nil {
		t.Fatal(err)
	}
	first, err := SubmitRefinement(root, RefinementInput{Facet: "voice", Evidence: "same-occurrence", ProposedBody: "# First\n", OccurrenceID: "occurrence-divergent"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := SubmitRefinement(root, RefinementInput{Facet: "preferences", Evidence: "same-occurrence", ProposedBody: "# Divergent\n", OccurrenceID: "occurrence-divergent"}); err == nil {
		t.Fatal("divergent occurrence writer replaced the existing proposal")
	}
	body, err := os.ReadFile(filepath.Join(root, "owner", "refinement", "proposals", first.ID+".json"))
	if err != nil || !strings.Contains(string(body), "# First") || strings.Contains(string(body), "# Divergent") {
		t.Fatalf("existing proposal was overwritten: body=%q err=%v", body, err)
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
	for _, id := range []string{"professional-role", "communication-style", "voice", "preferences", "motivations", "quality-bar", "decision-rules", "working-boundaries", "psychological-profile"} {
		facet, ok := status.Facets[id]
		if !ok || !facet.Available {
			t.Fatalf("facet %q = %#v", id, facet)
		}
	}
	if status.Facets["voice"].Refinement != "automatic_with_audit" || status.Facets["motivations"].Refinement != "proposal_only" || status.Facets["quality-bar"].Refinement != "proposal_only" || status.Facets["decision-rules"].Refinement != "proposal_only" {
		t.Fatalf("unexpected refinement policy: %#v", status.Facets)
	}
	interview := ColdStartInterview()
	if len(interview.Steps) != len(onboardingFacets) || interview.Steps[0].Facet != "professional-role" {
		t.Fatalf("interview = %#v", interview)
	}
	if interview.Steps[5].Facet != "quality-bar" || !strings.Contains(interview.Steps[5].Question, "QA") {
		t.Fatalf("quality bar question = %#v", interview.Steps[5])
	}
	for _, step := range interview.Steps {
		if step.Facet == "psychological-profile" {
			t.Fatal("psychological profile must not be a default cold-start question")
		}
	}
}

func TestOnboardingRequiresExplicitConfirmationAndCountsOnlyExplicitOpenTasks(t *testing.T) {
	root := t.TempDir()
	if _, err := Initialize(root); err != nil {
		t.Fatal(err)
	}
	status, err := Inspect(root)
	if err != nil || status.Onboarding.State != "required" || status.Onboarding.Track != "selection_required" || status.Onboarding.NextQuestion.Facet != "onboarding-track" || status.OpenTasks.State != "empty" {
		t.Fatalf("initial status = %#v err=%v", status, err)
	}
	if _, err := SelectOnboardingTrack(root, OnboardingTrackComplete); err != nil {
		t.Fatal(err)
	}
	for _, id := range onboardingFacets {
		template := facets[id]
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(template.Record.Path)), []byte(template.Body+"\nOwner-confirmed detail.\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(statePath)), []byte(stateTemplate+"- [ ] Prepare kickoff\n- [x] Already done\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	status, err = Inspect(root)
	if err != nil || status.Onboarding.State != "review_required" || status.OpenTasks.State != "available" || status.OpenTasks.Count != 1 {
		t.Fatalf("completed status = %#v err=%v", status, err)
	}
	reviewDigest := status.Onboarding.ReviewDigest
	if reviewDigest == "" {
		t.Fatal("review-required status omitted its digest")
	}
	voicePath := filepath.Join(root, filepath.FromSlash(facets["voice"].Record.Path))
	if err := os.WriteFile(voicePath, []byte(facets["voice"].Body+"\nChanged after review.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ConfirmOnboarding(root, reviewDigest); err == nil {
		t.Fatal("stale reviewed digest confirmed changed facets")
	}
	status, err = Inspect(root)
	if err != nil || status.Onboarding.ReviewDigest == "" || status.Onboarding.ReviewDigest == reviewDigest {
		t.Fatalf("refreshed review status = %#v err=%v", status, err)
	}
	status, err = ConfirmOnboarding(root, status.Onboarding.ReviewDigest)
	if err != nil || status.Onboarding.State != "complete" {
		t.Fatalf("confirmed status = %#v err=%v", status, err)
	}
}

func TestQuickOnboardingIsAnExplicitBoundedBaseline(t *testing.T) {
	root := t.TempDir()
	if _, err := Initialize(root); err != nil {
		t.Fatal(err)
	}
	status, err := SelectOnboardingTrack(root, OnboardingTrackQuick)
	if err != nil || status.Onboarding.Track != OnboardingTrackQuick || status.Onboarding.EstimatedMinutes != 7 || len(status.Onboarding.Remaining) != len(quickOnboardingFacets) {
		t.Fatalf("quick status = %#v err=%v", status, err)
	}
	for _, id := range quickOnboardingFacets {
		template := facets[id]
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(template.Record.Path)), []byte(template.Body+"\nOwner-confirmed detail.\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	status, err = Inspect(root)
	if err != nil || status.Onboarding.State != "review_required" || status.Onboarding.Track != OnboardingTrackQuick {
		t.Fatalf("quick review = %#v err=%v", status, err)
	}
	if _, err := ConfirmOnboarding(root, status.Onboarding.ReviewDigest); err != nil {
		t.Fatal(err)
	}
	status, err = SelectOnboardingTrack(root, OnboardingTrackComplete)
	if err != nil || status.Onboarding.State != "in_progress" || status.Onboarding.NextQuestion.Facet != "voice" {
		t.Fatalf("upgrade status = %#v err=%v", status, err)
	}
}

func TestSchemaV2ConfirmedCompleteOnboardingRemainsComplete(t *testing.T) {
	root := t.TempDir()
	if _, err := Initialize(root); err != nil {
		t.Fatal(err)
	}
	for _, id := range legacyOnboardingFacets {
		template := facets[id]
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(template.Record.Path)), []byte(template.Body+"\nLegacy owner detail.\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	legacyDigest := legacyOnboardingDigest(root)
	registryPath := filepath.Join(root, "owner", "registry.json")
	body, err := os.ReadFile(registryPath)
	if err != nil {
		t.Fatal(err)
	}
	var value registry
	if err := json.Unmarshal(body, &value); err != nil {
		t.Fatal(err)
	}
	delete(value.Facets, "motivations")
	delete(value.Facets, "quality-bar")
	value.SchemaVersion = 2
	value.OnboardingConfirmedAt = time.Now().UTC().Format(time.RFC3339Nano)
	value.OnboardingConfirmedSHA256 = legacyDigest
	value.OnboardingTrack = ""
	body, err = json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(registryPath, append(body, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	legacyVoice, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(facets["voice"].Record.Path)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Initialize(root); err != nil {
		t.Fatal(err)
	}
	status, err := Inspect(root)
	if err != nil || status.Onboarding.State != "complete" || status.Onboarding.Track != OnboardingTrackComplete {
		t.Fatalf("legacy status = %#v err=%v", status, err)
	}
	if facet, ok := status.Facets["motivations"]; !ok || !facet.Available {
		t.Fatalf("migrated motivations facet = %#v", facet)
	}
	if current, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(facets["voice"].Record.Path))); err != nil || string(current) != string(legacyVoice) {
		t.Fatalf("legacy voice was overwritten: current=%q before=%q err=%v", current, legacyVoice, err)
	}
}

func TestSnapshotIsAStaleCheckedProjectionOfCanonicalFacets(t *testing.T) {
	root := t.TempDir()
	if _, err := Initialize(root); err != nil {
		t.Fatal(err)
	}
	snapshot, err := ProjectSnapshot(root, []string{"communication-style", "decision-rules"})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.ProjectionOf != "ownerctx.facets.v2" || snapshot.Version == "" || len(snapshot.Facets) != 2 {
		t.Fatalf("snapshot = %#v", snapshot)
	}
	if err := PersistSnapshot(root, snapshot); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadSnapshot(root, snapshot.Version); err != nil {
		t.Fatal(err)
	}
	facetPath := filepath.Join(root, "owner", "self", "communication-style.md")
	if err := os.WriteFile(facetPath, []byte("# Updated\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadSnapshot(root, snapshot.Version); !errors.Is(err, ErrSnapshotStale) {
		t.Fatalf("stale snapshot error = %v", err)
	}
}

func TestSnapshotRejectsTamperedProjectedContentAndReaders(t *testing.T) {
	root := t.TempDir()
	if _, err := Initialize(root); err != nil {
		t.Fatal(err)
	}
	snapshot, err := ProjectSnapshot(root, []string{"voice"})
	if err != nil {
		t.Fatal(err)
	}
	tampered := snapshot
	facet := tampered.Facets["voice"]
	facet.Content = "forged content"
	tampered.Facets["voice"] = facet
	if err := tampered.Validate(); err == nil {
		t.Fatal("tampered projected content passed validation")
	}
	tampered = snapshot
	facet = tampered.Facets["voice"]
	facet.Readers = []string{"attacker"}
	tampered.Facets["voice"] = facet
	if err := tampered.Validate(); err == nil {
		t.Fatal("tampered projected readers passed validation")
	}
}

func TestSnapshotFailsClosedForFacetMutationBeyondProjectionBound(t *testing.T) {
	root := t.TempDir()
	if _, err := Initialize(root); err != nil {
		t.Fatal(err)
	}
	body := append([]byte(strings.Repeat("x", maximumOwnerProjectionBytes)), 'a')
	path := filepath.Join(root, "owner", "self", "voice.md")
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ProjectSnapshot(root, []string{"voice"}); err == nil {
		t.Fatal("oversized facet was silently projected")
	}
	body[len(body)-1] = 'b'
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ProjectSnapshot(root, []string{"voice"}); err == nil {
		t.Fatal("tampered oversized facet was accepted")
	}
}

func TestAtomicPrivateWriteRejectsSymlinkTarget(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink privilege is not available on all Windows runners")
	}
	root := t.TempDir()
	victim := filepath.Join(root, "victim")
	if err := os.WriteFile(victim, []byte("safe"), 0o600); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "target")
	if err := os.Symlink(victim, target); err != nil {
		t.Fatal(err)
	}
	if err := atomicPrivateWrite(target, []byte("forged")); err == nil {
		t.Fatal("atomic writer followed a symlink target")
	}
	body, err := os.ReadFile(victim)
	if err != nil || string(body) != "safe" {
		t.Fatalf("symlink victim changed: %q, err=%v", body, err)
	}
}

func TestAtomicPrivateWriteRejectsSymlinkAncestor(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink privilege is not available on all Windows runners")
	}
	root := t.TempDir()
	victim := filepath.Join(root, "victim")
	if err := os.Mkdir(victim, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(victim, link); err != nil {
		t.Fatal(err)
	}
	if err := atomicPrivateWrite(filepath.Join(link, "nested", "state.json"), []byte("forged")); err == nil {
		t.Fatal("atomic writer followed a symlink ancestor")
	}
}

func TestObservationEvaluatorDoesNotPersistEveryLoopOrInferSelfFacts(t *testing.T) {
	input := observationInput(SignalObservedPattern, "concise", "episode-a", false, true)
	receipt, evaluation, err := AppendObservation(t.TempDir(), input)
	if err != nil {
		t.Fatal(err)
	}
	if !evaluation.Evaluated || evaluation.Persist || receipt.Persisted {
		t.Fatalf("unauthenticated observation persisted: evaluation=%#v receipt=%#v", evaluation, receipt)
	}
	input = observationInput(SignalInferredHypothesis, "prefers_concise", "episode-a", true, true)
	_, evaluation, err = AppendObservation(t.TempDir(), input)
	if err != nil {
		t.Fatal(err)
	}
	if evaluation.Persist || evaluation.Reason != "hypothesis_is_task_local" {
		t.Fatalf("inferred hypothesis was persisted: %#v", evaluation)
	}
}

func TestObservationEvaluatorDoesNotTreatGenericAcknowledgementAsEndorsement(t *testing.T) {
	input := observationInput(SignalExplicitEndorsement, "ok", "episode-ack", true, true)
	if _, err := EvaluateInteraction(input); err == nil {
		t.Fatal("generic acknowledgement was accepted as an explicit endorsement")
	}
	input.Claim = "endorses_concise_style"
	evaluation, err := EvaluateInteraction(input)
	if err != nil || !evaluation.Persist {
		t.Fatalf("explicit endorsement was not retained: %#v, err=%v", evaluation, err)
	}
}

func TestObservationPromotionRequiresIndependentEpisodesAndCanonicalCAS(t *testing.T) {
	root := t.TempDir()
	if _, err := Initialize(root); err != nil {
		t.Fatal(err)
	}
	first := observationInput(SignalExplicitCorrection, "concise", "episode-a", true, true)
	first.DeclassifiedGlobal = true
	firstReceipt, _, err := AppendObservation(root, first)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := transitionObservation(t, root, firstReceipt.ID, ObservationEligible, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := transitionObservation(t, root, firstReceipt.ID, ObservationCorroborated, ""); err == nil {
		t.Fatal("single episode was accepted as corroboration")
	}
	sameEpisode := first
	sameEpisode.SourceEvent = "interaction.completed"
	sameEpisode.SourceDigest = digest("source-episode-a-second")
	if _, _, err := AppendObservation(root, sameEpisode); err != nil {
		t.Fatal(err)
	}
	if _, err := transitionObservation(t, root, firstReceipt.ID, ObservationCorroborated, ""); err == nil {
		t.Fatal("two digests from one episode were accepted as independent corroboration")
	}
	second := first
	second.EpisodeID, second.SourceEvent = "episode-b", "interaction.completed"
	second.SourceDigest = digest("source-b")
	second.DeclassifiedGlobal = true
	if _, _, err := AppendObservation(root, second); err != nil {
		t.Fatal(err)
	}
	if _, err := transitionObservation(t, root, firstReceipt.ID, ObservationCorroborated, ""); err != nil {
		t.Fatal(err)
	}
	canonical, err := canonicalDigestForFacet(root, "voice")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := transitionObservation(t, root, firstReceipt.ID, ObservationPromoted, canonical); err == nil {
		t.Fatal("corroborated observation bypassed the proposed state")
	}
	if _, err := transitionObservation(t, root, firstReceipt.ID, ObservationProposed, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := transitionObservation(t, root, firstReceipt.ID, ObservationPromoted, digest("stale")); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale promotion error = %v", err)
	}
	if _, err := transitionObservation(t, root, firstReceipt.ID, ObservationPromoted, canonical); err != nil {
		t.Fatal(err)
	}
}

func TestObservationSourceEventIsAllowlistedMetadataOnly(t *testing.T) {
	input := observationInput(SignalExplicitCorrection, "concise", "episode-source", true, true)
	input.SourceEvent = "raw user prompt or client body"
	if _, err := EvaluateInteraction(input); err == nil {
		t.Fatal("arbitrary source event text was accepted")
	}
}

func TestObservationAppendIsStableAndConcurrentIdempotent(t *testing.T) {
	root := t.TempDir()
	if _, err := Initialize(root); err != nil {
		t.Fatal(err)
	}
	input := observationInput(SignalExplicitCorrection, "concurrent", "episode-concurrent", true, true)
	const attempts = 12
	errorsCh := make(chan error, attempts)
	var wait sync.WaitGroup
	for i := 0; i < attempts; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, _, err := AppendObservation(root, input)
			errorsCh <- err
		}()
	}
	wait.Wait()
	close(errorsCh)
	for err := range errorsCh {
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := AppendObservation(root, input); err != nil {
		t.Fatal(err)
	}
	observations, err := ListObservations(root)
	if err != nil || len(observations) != 1 {
		t.Fatalf("concurrent retry was not idempotent: %#v, err=%v", observations, err)
	}
}

func TestGlobalObservationNeedsExplicitDeclassificationForPromotion(t *testing.T) {
	root := t.TempDir()
	if _, err := Initialize(root); err != nil {
		t.Fatal(err)
	}
	input := observationInput(SignalExplicitCorrection, "global_claim", "episode-global", true, true)
	receipt, _, err := AppendObservation(root, input)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := transitionObservation(t, root, receipt.ID, ObservationEligible, ""); err != nil {
		t.Fatal(err)
	}
	second := input
	second.EpisodeID = "episode-global-independent"
	second.SourceDigest = digest("global-source-independent")
	if _, _, err := AppendObservation(root, second); err != nil {
		t.Fatal(err)
	}
	if _, err := transitionObservation(t, root, receipt.ID, ObservationCorroborated, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := transitionObservation(t, root, receipt.ID, ObservationProposed, ""); err != nil {
		t.Fatal(err)
	}
	canonical, err := canonicalDigestForFacet(root, "voice")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := transitionObservation(t, root, receipt.ID, ObservationPromoted, canonical); err == nil {
		t.Fatal("global observation was promoted without explicit declassification")
	}
}

func TestExplicitSignalsCannotJumpToProposalBeforeCorroboration(t *testing.T) {
	for _, signal := range []SignalClass{SignalExplicitInstruction, SignalExplicitCorrection, SignalExplicitEndorsement} {
		t.Run(string(signal), func(t *testing.T) {
			root := t.TempDir()
			if _, err := Initialize(root); err != nil {
				t.Fatal(err)
			}
			input := observationInput(signal, "no_direct_jump", "episode-direct-jump", true, true)
			receipt, _, err := AppendObservation(root, input)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := transitionObservation(t, root, receipt.ID, ObservationProposed, ""); err == nil {
				t.Fatal("explicit signal jumped from captured to proposed")
			}
			if _, err := transitionObservation(t, root, receipt.ID, ObservationEligible, ""); err != nil {
				t.Fatal(err)
			}
			if _, err := transitionObservation(t, root, receipt.ID, ObservationProposed, ""); err == nil {
				t.Fatal("explicit signal jumped from eligible to proposed")
			}
		})
	}
}

func TestObservationTransitionsSerializeReadValidateCASAndAppend(t *testing.T) {
	root := t.TempDir()
	if _, err := Initialize(root); err != nil {
		t.Fatal(err)
	}
	input := observationInput(SignalExplicitCorrection, "transition_once", "episode-transition", true, true)
	receipt, _, err := AppendObservation(root, input)
	if err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 2)
	var wait sync.WaitGroup
	for i := 0; i < 2; i++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			current, currentErr := GetObservation(root, receipt.ID)
			if currentErr != nil {
				results <- currentErr
				return
			}
			_, err := TransitionObservation(root, ObservationTransitionInput{ObservationID: receipt.ID, TransitionID: "competing-" + string(rune('a'+index)), Next: ObservationEligible, ExpectedState: current.State, ExpectedRevision: current.Revision, OwnerAction: true})
			results <- err
		}(i)
	}
	wait.Wait()
	close(results)
	successes := 0
	failures := 0
	for err := range results {
		if err == nil {
			successes++
		} else {
			failures++
		}
	}
	if successes != 1 || failures != 1 {
		t.Fatalf("competing transitions were not serialized: successes=%d failures=%d", successes, failures)
	}
}

func TestObservationTransitionRetryIsIdempotentAndStaleCASFailsClosed(t *testing.T) {
	root := t.TempDir()
	if _, err := Initialize(root); err != nil {
		t.Fatal(err)
	}
	input := observationInput(SignalExplicitCorrection, "retryable", "episode-retry", true, true)
	receipt, _, err := AppendObservation(root, input)
	if err != nil {
		t.Fatal(err)
	}
	transition := ObservationTransitionInput{ObservationID: receipt.ID, TransitionID: "transition-retry", Next: ObservationEligible, ExpectedState: receipt.State, ExpectedRevision: receipt.Revision, OwnerAction: true}
	first, err := TransitionObservation(root, transition)
	if err != nil {
		t.Fatal(err)
	}
	retry, err := TransitionObservation(root, transition)
	if err != nil {
		t.Fatal(err)
	}
	if retry.ID != first.ID || retry.Revision != first.Revision || retry.TransitionID != first.TransitionID {
		t.Fatalf("retry did not return the original transition receipt: first=%#v retry=%#v", first, retry)
	}
	if !retry.OwnerAction {
		t.Fatalf("transition receipt lost explicit owner action: %#v", retry)
	}
	if _, err := TransitionObservation(root, ObservationTransitionInput{ObservationID: receipt.ID, TransitionID: transition.TransitionID, Next: transition.Next, ExpectedState: transition.ExpectedState, ExpectedRevision: transition.ExpectedRevision, OwnerAction: false}); err == nil {
		t.Fatal("transition replay changed owner authority and was accepted")
	}
	if _, err := TransitionObservation(root, ObservationTransitionInput{ObservationID: receipt.ID, TransitionID: "transition-stale", Next: ObservationProposed, ExpectedState: receipt.State, ExpectedRevision: receipt.Revision, OwnerAction: true}); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale competing transition error = %v", err)
	}
}

func TestObservationEvaluatorRequiresOwnerConfirmationForExplicitSelfSignals(t *testing.T) {
	input := observationInput(SignalExplicitCorrection, "unconfirmed", "episode-unconfirmed", true, true)
	input.OwnerConfirmed = false
	evaluation, err := EvaluateInteraction(input)
	if err != nil || evaluation.Persist || evaluation.Reason != "explicit_owner_confirmation_missing" {
		t.Fatalf("unconfirmed explicit signal evaluation = %#v err=%v", evaluation, err)
	}
}

func TestRefinementProposalIsInvalidatedByCanonicalCorrection(t *testing.T) {
	root := t.TempDir()
	if _, err := Initialize(root); err != nil {
		t.Fatal(err)
	}
	receipt, err := SubmitRefinement(root, RefinementInput{Facet: "voice", Evidence: "owner correction", ProposedBody: "# Proposed\n"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "owner", "self", "voice.md"), []byte("# New canonical\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyRefinement(root, receipt.ID, true); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("dependent proposal was not invalidated: %v", err)
	}
}

func TestDarwinMetadataReportContainsNoSemanticSelfContent(t *testing.T) {
	root := t.TempDir()
	if _, err := Initialize(root); err != nil {
		t.Fatal(err)
	}
	first := observationInput(SignalExplicitCorrection, "concise", "episode-a", true, true)
	first.DeclassifiedGlobal = true
	if _, _, err := AppendObservation(root, first); err != nil {
		t.Fatal(err)
	}
	second := first
	second.SourceEvent, second.EpisodeID = "interaction.completed", "episode-b"
	second.SourceDigest = first.SourceDigest
	if _, _, err := AppendObservation(root, second); err != nil {
		t.Fatal(err)
	}
	report, err := AnalyzeObservationMetadata(root, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if report.Total != 2 || report.DuplicateSourceDigests != 1 || report.ExpiringWithin != 2 || len(report.ReevaluateFacets) != 1 || report.ReevaluateFacets[0] != "voice" {
		t.Fatalf("metadata report = %#v", report)
	}
}

func TestResetDerivedSelfUsesTombstonesAndLeavesCanonicalFacetUntouched(t *testing.T) {
	root := t.TempDir()
	if _, err := Initialize(root); err != nil {
		t.Fatal(err)
	}
	snapshot, err := ProjectSnapshot(root, []string{"voice"})
	if err != nil {
		t.Fatal(err)
	}
	if err := PersistSnapshot(root, snapshot); err != nil {
		t.Fatal(err)
	}
	input := observationInput(SignalExplicitInstruction, "concise", "episode-reset", true, true)
	input.DeclassifiedGlobal = true
	if _, _, err := AppendObservation(root, input); err != nil {
		t.Fatal(err)
	}
	if err := ResetDerivedSelf(root, true); err != nil {
		t.Fatal(err)
	}
	observations, err := ListObservations(root)
	if err != nil || len(observations) != 1 || observations[0].State != ObservationRedacted {
		t.Fatalf("reset observations = %#v, err = %v", observations, err)
	}
	if _, err := os.Stat(filepath.Join(root, "owner", "self", "projections", snapshot.Version+".json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("derived snapshot was not deleted: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "owner", "self", "voice.md")); err != nil {
		t.Fatalf("canonical facet was removed: %v", err)
	}
}

func observationInput(signal SignalClass, claim, episode string, authenticated, material bool) ObservationInput {
	return ObservationInput{SchemaVersion: 1, Signal: signal, Facet: "voice", Claim: claim, EvidenceType: "owner_correction", SourceEvent: "interaction.completed", SourceDigest: digest("source-" + episode), EpisodeID: episode, ScopeKind: "global", ScopeID: "owner", Confidence: 0.9, Sensitivity: "professional", ExpiresAt: time.Now().UTC().Add(time.Hour), AuthenticatedOwner: authenticated, Material: material, OwnerConfirmed: authenticated}
}

func transitionObservation(t *testing.T, root, id string, next ObservationState, canonical string) (ObservationReceipt, error) {
	t.Helper()
	current, err := GetObservation(root, id)
	if err != nil {
		return ObservationReceipt{}, err
	}
	return TransitionObservation(root, ObservationTransitionInput{ObservationID: id, TransitionID: "transition-" + digest(id + "\x00" + string(next) + "\x00" + current.Revision)[:32], Next: next, ExpectedState: current.State, ExpectedRevision: current.Revision, ExpectedCanonicalDigest: canonical, OwnerAction: true})
}
