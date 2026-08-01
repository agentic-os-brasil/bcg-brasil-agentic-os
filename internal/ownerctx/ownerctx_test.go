package ownerctx

import (
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
	if _, err := transitionObservation(t, root, firstReceipt.ID, ObservationProposed, ""); err != nil {
		t.Fatal(err)
	}
	canonical, err := canonicalDigestForFacet(root, "voice")
	if err != nil {
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
