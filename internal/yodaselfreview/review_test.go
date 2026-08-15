package yodaselfreview

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/maestro"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/maintenance"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ownerctx"
)

type fakeAdapter struct {
	mu       sync.Mutex
	called   int
	proposal SelfRefinementProposal
}

type blockingAdapter struct {
	started  chan struct{}
	release  chan struct{}
	proposal SelfRefinementProposal
}

type deadlineAdapter struct {
	canceled chan struct{}
}

type hungAdapter struct {
	started chan struct{}
	release chan struct{}
}

func (adapter *deadlineAdapter) ID() string { return "deadline-yoda" }
func (adapter *deadlineAdapter) Review(ctx context.Context, _ ModelInput) (SelfRefinementProposal, error) {
	<-ctx.Done()
	close(adapter.canceled)
	return SelfRefinementProposal{}, ctx.Err()
}

func (adapter *hungAdapter) ID() string { return "hung-yoda" }
func (adapter *hungAdapter) Review(_ context.Context, _ ModelInput) (SelfRefinementProposal, error) {
	close(adapter.started)
	<-adapter.release
	return SelfRefinementProposal{}, errors.New("released after deadline")
}

func (adapter *blockingAdapter) ID() string { return "blocking-yoda" }
func (adapter *blockingAdapter) Review(ctx context.Context, _ ModelInput) (SelfRefinementProposal, error) {
	close(adapter.started)
	select {
	case <-adapter.release:
		return adapter.proposal, nil
	case <-ctx.Done():
		return SelfRefinementProposal{}, ctx.Err()
	}
}

func (adapter *fakeAdapter) ID() string { return "test-yoda" }
func (adapter *fakeAdapter) Review(_ context.Context, _ ModelInput) (SelfRefinementProposal, error) {
	adapter.mu.Lock()
	defer adapter.mu.Unlock()
	adapter.called++
	return adapter.proposal, nil
}

type fakeAuthority bool

func (fakeAuthority) ID() string             { return "test-authority" }
func (a fakeAuthority) Approved(string) bool { return bool(a) }

func testRequest(t *testing.T) Request {
	t.Helper()
	root := t.TempDir()
	if _, err := ownerctx.Initialize(root); err != nil {
		t.Fatal(err)
	}
	snapshot, err := ownerctx.ProjectSnapshot(root, []string{"voice"})
	if err != nil {
		t.Fatal(err)
	}
	history := []PromptEvidence{{ID: "prompt-1", OriginalText: "preserve the thesis", NormalizedText: "preserve the thesis", SourceLanguage: "en-US", WorkingLanguage: "en-US", OriginalSHA256: maestro.SHA256Hex("preserve the thesis"), NormalizedSHA256: maestro.SHA256Hex("preserve the thesis"), QuotedData: true, Instructional: false}}
	translation := TranslationReceipt{TranslatorID: "translator", TranslatorVersion: "v1", SourceLanguage: "en-US", WorkingLanguage: "en-US", OriginalSHA256: maestro.SHA256Hex("Current request"), WorkingSHA256: maestro.SHA256Hex("Current request"), HistorySHA256: Digest(maestro.SHA256Hex("preserve the thesis") + ":" + maestro.SHA256Hex("preserve the thesis"))}
	translation.ReceiptSHA256 = DigestJSON(translation)
	observation := ownerctx.ObservationReceipt{ID: "observation-1", State: ownerctx.ObservationCorroborated, Signal: ownerctx.SignalExplicitCorrection, Facet: "voice", Claim: "preserve-intent", EvidenceType: "owner_correction", SourceEvent: "interaction.completed", SourceDigest: Digest("source"), EpisodeID: "episode-1", ScopeKind: "global", ScopeID: "owner", Confidence: .9, Sensitivity: "professional", OwnerConfirmed: true, Persisted: true}
	return Request{SchemaVersion: SchemaVersion, WeekID: "2026-W31", OccurrenceID: "occurrence-1", OwnerID: "owner", ScopeKind: ownerctx.PromptScopeGlobal, ScopeID: "owner", CurrentOriginal: "Current request", CurrentNormalized: "Current request", PromptHistory: history, Translation: translation, Observations: []ownerctx.ObservationReceipt{observation}, CanonicalSnapshot: snapshot, ReviewFacets: []string{"voice"}, OwnerContextRoot: root}
}

func testProposal(request Request) SelfRefinementProposal {
	policy, _ := request.CanonicalSnapshot.Policy("voice")
	return SelfRefinementProposal{SchemaVersion: SchemaVersion, ProposalID: "proposal-1", State: "proposed", WeekID: request.WeekID, Facet: "voice", PriorClaim: "prior", ProposedRefinement: "Use a concise, decision-ready voice.", EvidenceObservationIDs: []string{"observation-1"}, Confidence: .9, Sensitivity: policy.Sensitivity, Readers: policy.Readers, Refinement: policy.Refinement, ConfirmationRequirement: ConfirmationRequirement(policy.ConfirmationRequirement), CanonicalSnapshotVersion: request.CanonicalSnapshot.Version, CanonicalSnapshotSHA256: request.CanonicalSnapshot.CanonicalSourceDigest, PromptHistorySHA256: PromptHistoryDigest(request.PromptHistory), TranslationReceiptSHA256: request.Translation.ReceiptSHA256}
}

func TestTranslationReceiptAndQuotedHistoryAreRequired(t *testing.T) {
	request := testRequest(t)
	if err := ValidateRequest(request); err != nil {
		t.Fatal(err)
	}
	request.PromptHistory[0].Instructional = true
	if err := ValidateRequest(request); err == nil {
		t.Fatal("history marked instructional was accepted")
	}
	request = testRequest(t)
	request.Translation.TranslatorVersion = "tampered"
	if err := ValidateRequest(request); err == nil {
		t.Fatal("stale translator receipt was accepted")
	}
}

func TestIntentHypothesisBindingIsIffAndBounded(t *testing.T) {
	request := testRequest(t)
	hypothesis := &maestro.IntentHypothesis{ExpressedObjective: "answer the request", LatentIntentHypothesis: "preserve the user's decision purpose", EvidenceRefs: []string{"current_prompt"}, Confidence: .8, Alternatives: []string{"literal-only"}, Materiality: "low", DisconfirmationCondition: "current instruction contradicts it", WorkingPrompt: request.CurrentNormalized}
	request.IntentHypothesis = hypothesis
	request.IntentHypothesisSHA256 = DigestJSON(hypothesis)
	if err := ValidateRequest(request); err != nil {
		t.Fatal(err)
	}
	proposal := testProposal(request)
	proposal.IntentHypothesisSHA256 = request.IntentHypothesisSHA256
	if err := ValidateProposal(request, proposal); err != nil {
		t.Fatal(err)
	}
	for _, refs := range [][]string{nil, {"prompt-1"}, {"observation-1"}} {
		request.IntentHypothesis.EvidenceRefs = refs
		request.IntentHypothesisSHA256 = DigestJSON(request.IntentHypothesis)
		if err := ValidateRequest(request); err == nil {
			t.Fatalf("hypothesis evidence refs without canonical current prompt were accepted: %#v", refs)
		}
	}
	request.IntentHypothesis.EvidenceRefs = []string{"current_prompt", "prompt-1"}
	request.IntentHypothesisSHA256 = DigestJSON(request.IntentHypothesis)
	if err := ValidateRequest(request); err != nil {
		t.Fatal("valid current/history hypothesis references were rejected:", err)
	}
	request.IntentHypothesis.EvidenceRefs = []string{"forged-ref"}
	request.IntentHypothesisSHA256 = DigestJSON(request.IntentHypothesis)
	if err := ValidateRequest(request); err == nil {
		t.Fatal("arbitrary hypothesis evidence reference was accepted")
	}
	request.IntentHypothesis.EvidenceRefs = []string{"prompt-1", "prompt-1"}
	request.IntentHypothesisSHA256 = DigestJSON(request.IntentHypothesis)
	if err := ValidateRequest(request); err == nil {
		t.Fatal("duplicate hypothesis evidence reference was accepted")
	}
	request.IntentHypothesis.EvidenceRefs = []string{"prompt-1"}
	request.IntentHypothesis.WorkingPrompt = "different working prompt"
	request.IntentHypothesisSHA256 = DigestJSON(request.IntentHypothesis)
	if err := ValidateRequest(request); err == nil {
		t.Fatal("hypothesis working prompt was not bound to current normalized prompt")
	}
	proposal.IntentHypothesisSHA256 = ""
	if err := ValidateProposal(request, proposal); err == nil {
		t.Fatal("proposal omitted required hypothesis binding")
	}
	request = testRequest(t)
	proposal = testProposal(request)
	proposal.IntentHypothesisSHA256 = Digest("unexpected")
	if err := ValidateProposal(request, proposal); err == nil {
		t.Fatal("proposal added an unexpected hypothesis binding")
	}
	request = testRequest(t)
	request.IntentHypothesis = &maestro.IntentHypothesis{ExpressedObjective: strings.Repeat("x", MaxContextBytes+1), LatentIntentHypothesis: "purpose", Confidence: .5, Materiality: "low", DisconfirmationCondition: "never", WorkingPrompt: "request"}
	request.IntentHypothesisSHA256 = DigestJSON(request.IntentHypothesis)
	if err := ValidateRequest(request); err == nil {
		t.Fatal("oversized intent hypothesis was accepted")
	}
}

func TestChangedIntentHypothesisCannotReplayOccurrence(t *testing.T) {
	request := testRequest(t)
	store := ReceiptStore{Root: t.TempDir()}
	if _, err := store.Reserve(request, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	request.IntentHypothesis = &maestro.IntentHypothesis{ExpressedObjective: "changed", LatentIntentHypothesis: "changed purpose", Confidence: .6, Materiality: "low", DisconfirmationCondition: "new evidence", WorkingPrompt: request.CurrentNormalized}
	request.IntentHypothesisSHA256 = DigestJSON(request.IntentHypothesis)
	if _, err := store.Reserve(request, time.Now().UTC()); err == nil {
		t.Fatal("changed intent hypothesis replayed the same occurrence")
	}
}

func TestCommandOccurrenceIsStableWhileHypothesisChanges(t *testing.T) {
	root := t.TempDir()
	if _, err := ownerctx.Initialize(root); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	command := maintenance.Command{SchemaVersion: maintenance.CommandSchemaVersion, CommandID: "hypothesis-command", JobID: WeeklyJobID, WorkspaceID: "workspace-1", Trigger: maintenance.TriggerWeekly, ScheduledFor: now, RequestedAt: now, Deadline: now.Add(time.Minute), ProposalOnly: true}
	makeHypothesis := func(value string) *maestro.IntentHypothesis {
		return &maestro.IntentHypothesis{ExpressedObjective: value, LatentIntentHypothesis: "serve the current purpose", EvidenceRefs: []string{"current_prompt"}, Confidence: .7, Materiality: "low", DisconfirmationCondition: "owner correction", WorkingPrompt: "Current request"}
	}
	handler := Handler{Root: root, OwnerID: "owner", CurrentPrompt: "Current request", CurrentLanguage: "en-US", WorkingLanguage: "en-US", TranslatorID: "translator", TranslatorVersion: "v1", ReviewFacets: []string{"voice"}, Translator: func(original, _, _ string) (string, error) { return original, nil }, IntentHypothesis: makeHypothesis("first")}
	first, err := handler.BuildRequest(command, now)
	if err != nil {
		t.Fatal(err)
	}
	handler.IntentHypothesis = makeHypothesis("second")
	second, err := handler.BuildRequest(command, now)
	if err != nil {
		t.Fatal(err)
	}
	if first.OccurrenceID != command.OccurrenceDigest() || second.OccurrenceID != command.OccurrenceDigest() || RequestDigest(first) == RequestDigest(second) {
		t.Fatalf("hypothesis changed occurrence identity or request digest: first=%q second=%q", first.OccurrenceID, second.OccurrenceID)
	}
	store := ReceiptStore{Root: t.TempDir()}
	if _, err := store.Reserve(first, now); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Reserve(second, now); err == nil {
		t.Fatal("changed hypothesis was accepted against an existing occurrence reservation")
	}
}

func TestWeeklyProposalRequiresCorroboratedObservationState(t *testing.T) {
	request := testRequest(t)
	proposal := testProposal(request)
	for _, state := range []ownerctx.ObservationState{ownerctx.ObservationCaptured, ownerctx.ObservationEligible, ownerctx.ObservationProposed} {
		request.Observations[0].State = state
		if err := ValidateProposal(request, proposal); err == nil {
			t.Fatalf("weekly proposal accepted observation state %q without corroboration", state)
		}
	}
}

func TestYodaWeeklyEligibilityRequiresExplicitOwnerSignals(t *testing.T) {
	base := testRequest(t)
	cases := []struct {
		name      string
		signal    ownerctx.SignalClass
		claim     string
		confirmed bool
		eligible  bool
	}{
		{name: "explicit instruction", signal: ownerctx.SignalExplicitInstruction, claim: "preserve_intent", confirmed: true, eligible: true},
		{name: "explicit correction", signal: ownerctx.SignalExplicitCorrection, claim: "preserve_intent", confirmed: true, eligible: true},
		{name: "specific endorsement", signal: ownerctx.SignalExplicitEndorsement, claim: "endorses_concise_style", confirmed: true, eligible: true},
		{name: "generic endorsement", signal: ownerctx.SignalExplicitEndorsement, claim: "ok", confirmed: true, eligible: false},
		{name: "observed pattern", signal: ownerctx.SignalObservedPattern, claim: "preserve_intent", confirmed: true, eligible: false},
		{name: "inferred hypothesis", signal: ownerctx.SignalInferredHypothesis, claim: "preserve_intent", confirmed: true, eligible: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			request := base
			observation := base.Observations[0]
			observation.Signal, observation.Claim, observation.OwnerConfirmed = tc.signal, tc.claim, tc.confirmed
			request.Observations = []ownerctx.ObservationReceipt{observation}
			if got := ownerctx.IsYodaWeeklyEligible(observation); got != tc.eligible {
				t.Fatalf("weekly eligibility = %v, want %v", got, tc.eligible)
			}
			proposal := testProposal(request)
			err := ValidateProposal(request, proposal)
			if tc.eligible && err != nil {
				t.Fatalf("eligible explicit evidence was rejected: %v", err)
			}
			if !tc.eligible && err == nil {
				t.Fatal("ineligible evidence was accepted by ValidateProposal")
			}
		})
	}
}

func TestBuildRequestExcludesCorroboratedObservedPattern(t *testing.T) {
	root := t.TempDir()
	if _, err := ownerctx.Initialize(root); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	input := ownerctx.ObservationInput{SchemaVersion: 1, Signal: ownerctx.SignalObservedPattern, Facet: "voice", Claim: "preserve_intent", EvidenceType: "observed_pattern", SourceEvent: "interaction.completed", SourceDigest: Digest("observed-a"), EpisodeID: "episode-observed-a", ScopeKind: "global", ScopeID: "owner", Confidence: .9, Sensitivity: "professional", ExpiresAt: now.Add(time.Hour), AuthenticatedOwner: true, Material: true}
	first, evaluation, err := ownerctx.AppendObservation(root, input)
	if err != nil || !evaluation.Persist {
		t.Fatalf("observed pattern was not persisted for the selection test: receipt=%+v evaluation=%+v err=%v", first, evaluation, err)
	}
	eligible, err := ownerctx.TransitionObservation(root, ownerctx.ObservationTransitionInput{ObservationID: first.ID, TransitionID: "observed-eligible", Next: ownerctx.ObservationEligible, ExpectedState: first.State, ExpectedRevision: first.Revision, OwnerAction: true})
	if err != nil {
		t.Fatal(err)
	}
	input.EpisodeID, input.SourceDigest = "episode-observed-b", Digest("observed-b")
	if _, _, err := ownerctx.AppendObservation(root, input); err != nil {
		t.Fatal(err)
	}
	corroborated, err := ownerctx.TransitionObservation(root, ownerctx.ObservationTransitionInput{ObservationID: first.ID, TransitionID: "observed-corroborated", Next: ownerctx.ObservationCorroborated, ExpectedState: eligible.State, ExpectedRevision: eligible.Revision, OwnerAction: true})
	if err != nil {
		t.Fatal(err)
	}
	if corroborated.State != ownerctx.ObservationCorroborated {
		t.Fatalf("observation did not reach corroborated state: %+v", corroborated)
	}
	command := maintenance.Command{SchemaVersion: maintenance.CommandSchemaVersion, CommandID: "observed-pattern-command", JobID: WeeklyJobID, WorkspaceID: "workspace-1", Trigger: maintenance.TriggerWeekly, ScheduledFor: now, RequestedAt: now, Deadline: now.Add(time.Minute), ProposalOnly: true}
	handler := Handler{Root: root, OwnerID: "owner", ScopeKind: ownerctx.PromptScopeGlobal, ScopeID: "owner", CurrentPrompt: "Current request", CurrentLanguage: "en-US", WorkingLanguage: "en-US", TranslatorID: "translator", TranslatorVersion: "v1", ReviewFacets: []string{"voice"}, Translator: func(original, _, _ string) (string, error) { return original, nil }}
	request, err := handler.BuildRequest(command, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(request.Observations) != 0 {
		t.Fatalf("observed-pattern evidence entered the weekly request: %+v", request.Observations)
	}
}

func TestRequestBoundsAndTranslationExpansionFailClosed(t *testing.T) {
	request := testRequest(t)
	request.CurrentOriginal = strings.Repeat("x", MaxContextBytes+1)
	if err := ValidateRequest(request); err == nil {
		t.Fatal("oversized current prompt was accepted")
	}
	request = testRequest(t)
	request.CurrentNormalized = strings.Repeat("x", len([]byte(request.CurrentOriginal))*MaxTranslationExpansion+MaxTranslationOverhead+1)
	request.Translation.WorkingSHA256 = maestro.SHA256Hex(request.CurrentNormalized)
	request.Translation.ReceiptSHA256 = DigestJSON(request.Translation)
	if err := ValidateRequest(request); err == nil {
		t.Fatal("unbounded translation expansion was accepted")
	}
	request = testRequest(t)
	request.CurrentOriginal = strings.Repeat("x", 9000)
	request.CurrentNormalized = request.CurrentOriginal
	request.PromptHistory[0].OriginalText = strings.Repeat("y", 6000)
	request.PromptHistory[0].NormalizedText = request.PromptHistory[0].OriginalText
	request.PromptHistory[0].OriginalSHA256 = maestro.SHA256Hex(request.PromptHistory[0].OriginalText)
	request.PromptHistory[0].NormalizedSHA256 = maestro.SHA256Hex(request.PromptHistory[0].NormalizedText)
	request.Translation.OriginalSHA256 = maestro.SHA256Hex(request.CurrentOriginal)
	request.Translation.WorkingSHA256 = maestro.SHA256Hex(request.CurrentNormalized)
	request.Translation.HistorySHA256 = Digest(request.PromptHistory[0].OriginalSHA256 + ":" + request.PromptHistory[0].NormalizedSHA256)
	request.Translation.ReceiptSHA256 = DigestJSON(request.Translation)
	if err := ValidateRequest(request); err == nil {
		t.Fatal("combined model context bound was not enforced")
	}
}

func TestSensitiveFacetRequiresExplicitAuthorizationAndIsNotImplicitlyProjected(t *testing.T) {
	root := t.TempDir()
	if _, err := ownerctx.Initialize(root); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.August, 1, 12, 0, 0, 0, time.UTC)
	command := maintenance.Command{SchemaVersion: maintenance.CommandSchemaVersion, CommandID: "command-sensitive", JobID: WeeklyJobID, WorkspaceID: "workspace-1", Trigger: maintenance.TriggerWeekly, ScheduledFor: now, RequestedAt: now, Deadline: now.Add(time.Minute), ProposalOnly: true}
	base := Handler{Root: root, OwnerID: "owner", CurrentPrompt: "Current request", CurrentLanguage: "en-US", WorkingLanguage: "en-US", TranslatorID: "translator", TranslatorVersion: "v1", Translator: func(original, _, _ string) (string, error) { return original, nil }}
	if _, err := base.BuildRequest(command, now); err == nil {
		t.Fatal("Yoda implicitly projected all self facets")
	}
	base.ReviewFacets = []string{"psychological-profile"}
	if _, err := base.BuildRequest(command, now); err == nil {
		t.Fatal("unauthorized sensitive facet was projected")
	}
	base.SensitivePurpose = "authorized Yoda self-review"
	base.OwnerAuthorized = true
	request, err := base.BuildRequest(command, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := request.CanonicalSnapshot.Facets["psychological-profile"]; !ok {
		t.Fatal("explicitly authorized sensitive facet was not projected")
	}
}

func TestCanonicalPolicyAndEvidenceFacetAreExact(t *testing.T) {
	request := testRequest(t)
	proposal := testProposal(request)
	proposal.Readers = []string{"yoda"}
	if err := ValidateProposal(request, proposal); err == nil {
		t.Fatal("adapter-asserted reader policy was accepted")
	}
	proposal = testProposal(request)
	request.Observations[0].Facet = "preferences"
	if err := ValidateProposal(request, proposal); err == nil {
		t.Fatal("cross-facet evidence was accepted")
	}
}

func TestWeeklyReviewUnavailableAfterReservationAndProposalOnly(t *testing.T) {
	request := testRequest(t)
	store := ReceiptStore{Root: t.TempDir()}
	proposal, receipt, err := Review(context.Background(), request, nil, nil, store, time.Now().UTC())
	if !errors.Is(err, ErrUnavailable) || receipt.State != ReceiptUnavailable || proposal.ProposalID != "" {
		t.Fatalf("unexpected unavailable result: %+v %+v %v", proposal, receipt, err)
	}
	body, _ := os.ReadFile(filepath.Join(store.Root, "weekly-receipts.jsonl"))
	if strings.Contains(string(body), "Current request") {
		t.Fatal("receipt leaked prompt text")
	}
	if err := ApplyCanonicalMutation(testProposal(request), request.CanonicalSnapshot); !errors.Is(err, ErrProposalOnly) {
		t.Fatal("proposal-only contract was bypassed")
	}
}

func TestWeeklyReviewReservesBeforeConcurrentModelCall(t *testing.T) {
	request := testRequest(t)
	request.OwnerContextRoot = t.TempDir()
	if _, err := ownerctx.Initialize(request.OwnerContextRoot); err != nil {
		t.Fatal(err)
	}
	request.CanonicalSnapshot, _ = ownerctx.ProjectSnapshot(request.OwnerContextRoot, []string{"voice"})
	proposal := testProposal(request)
	store := ReceiptStore{Root: t.TempDir()}
	adapter := &fakeAdapter{proposal: proposal}
	authority := fakeAuthority(true)
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := Review(context.Background(), request, adapter, authority, store, time.Now().UTC())
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	for err := range results {
		if err != nil && !errors.Is(err, ErrOccurrenceBusy) {
			t.Fatal(err)
		}
	}
	adapter.mu.Lock()
	called := adapter.called
	adapter.mu.Unlock()
	if called != 1 {
		t.Fatalf("model was called %d times for one occurrence", called)
	}
}

func TestWeeklyReviewResumesAfterProposalCommitCrash(t *testing.T) {
	request := testRequest(t)
	request.CanonicalSnapshot, _ = ownerctx.ProjectSnapshot(request.OwnerContextRoot, []string{"preferences", "voice"})
	request.ReviewFacets = []string{"preferences", "voice"}
	proposal := testProposal(request)
	now := time.Now().UTC().Truncate(time.Second)
	store := ReceiptStore{Root: t.TempDir(), LeaseDuration: time.Second}
	reservation, err := store.Reserve(request, now)
	if err != nil {
		t.Fatal(err)
	}
	ownerReceipt, err := ownerctx.SubmitRefinement(request.OwnerContextRoot, ownerctx.RefinementInput{Facet: proposal.Facet, Evidence: "yoda-weekly:" + DigestJSON(proposal.EvidenceObservationIDs), ProposedBody: proposal.ProposedRefinement, OccurrenceID: request.OccurrenceID, YodaReviewRequestSHA256: RequestDigest(request), YodaReviewProposalID: proposal.ProposalID, YodaReviewProposalSHA256: DigestJSON(proposal), YodaReviewSensitivity: proposal.Sensitivity, YodaReviewReaders: proposal.Readers, YodaReviewRefinement: proposal.Refinement, YodaReviewConfirmation: string(proposal.ConfirmationRequirement), YodaReviewAdapterID: "test-yoda", YodaReviewAuthorityID: "test-authority", YodaReviewFencingToken: reservation.Receipt.FencingToken})
	if err != nil {
		t.Fatal(err)
	}
	if ownerReceipt.ID == "" {
		t.Fatal("crash simulation did not commit ownerctx proposal")
	}
	changed := proposal
	changed.ProposalID = "different-proposal"
	changed.Facet = "preferences"
	changed.ProposedRefinement = "A different body returned after restart."
	adapter := &fakeAdapter{proposal: changed}
	result, receipt, err := Review(context.Background(), request, adapter, fakeAuthority(true), store, now.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if result.ProposalID != proposal.ProposalID || receipt.State != ReceiptProposal || receipt.OwnerctxProposalID != ownerReceipt.ID || receipt.OwnerctxProposalSHA256 != ownerReceipt.ProposalSHA256 || receipt.ProposalSHA256 != DigestJSON(proposal) {
		t.Fatalf("recovery did not bind actual ownerctx proposal: result=%+v receipt=%+v owner=%+v", result, receipt, ownerReceipt)
	}
	adapter.mu.Lock()
	called := adapter.called
	adapter.mu.Unlock()
	if called != 0 {
		t.Fatalf("resume invoked variable-output model %d times", called)
	}
	body, err := os.ReadFile(filepath.Join(store.Root, "weekly-receipts.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if lines := strings.Count(strings.TrimSpace(string(body)), "\n") + 1; lines != 3 {
		t.Fatalf("expected initial reservation, renewal and one terminal receipt; got %d lines", lines)
	}
	entries, err := os.ReadDir(filepath.Join(request.OwnerContextRoot, "owner", "refinement", "proposals"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("expected exactly one ownerctx proposal after recovery: entries=%d err=%v", len(entries), err)
	}
}

func TestSlowModelLeaseRenewsAndStaleWorkerCannotBeTakenOver(t *testing.T) {
	request := testRequest(t)
	// Keep the lease comfortably above filesystem scheduling jitter. The test
	// still spans multiple renewal intervals, but does not turn a loaded CI
	// worker into a false fencing failure.
	store := ReceiptStore{Root: t.TempDir(), LeaseDuration: 500 * time.Millisecond, LockWait: 100 * time.Millisecond}
	adapter := &blockingAdapter{started: make(chan struct{}), release: make(chan struct{}), proposal: testProposal(request)}
	resultCh := make(chan error, 1)
	go func() {
		_, _, err := Review(context.Background(), request, adapter, fakeAuthority(true), store, time.Now().UTC())
		resultCh <- err
	}()
	<-adapter.started
	deadline := time.Now().Add(3 * time.Second)
	for {
		receipts, err := store.read(filepath.Join(store.Root, "weekly-receipts.jsonl"))
		if err != nil {
			t.Fatal(err)
		}
		if len(receipts) >= 2 {
			latest, found := latestReceipt(receipts, request.OccurrenceID)
			if found && latest.LeaseUntil.After(time.Now().UTC()) {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("renewal loop did not produce a live renewed lease")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := store.Reserve(request, time.Now().UTC()); !errors.Is(err, ErrOccurrenceBusy) {
		t.Fatalf("slow model lease was not renewed; reserve err=%v", err)
	}
	close(adapter.release)
	if err := <-resultCh; err != nil {
		t.Fatal(err)
	}
}

func TestOldWorkerCannotCommitAfterFenceTakeover(t *testing.T) {
	request := testRequest(t)
	store := ReceiptStore{Root: t.TempDir(), LeaseDuration: 10 * time.Millisecond}
	now := time.Now().UTC()
	oldReservation, err := store.Reserve(request, now)
	if err != nil {
		t.Fatal(err)
	}
	newReservation, err := store.Reserve(request, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if oldReservation.Receipt.FencingToken == newReservation.Receipt.FencingToken {
		t.Fatal("lease takeover did not rotate fencing token")
	}
	proposal := testProposal(request)
	input := ownerctx.RefinementInput{Facet: proposal.Facet, Evidence: "yoda-weekly:" + DigestJSON(proposal.EvidenceObservationIDs), ProposedBody: proposal.ProposedRefinement, OccurrenceID: request.OccurrenceID, YodaReviewRequestSHA256: RequestDigest(request), YodaReviewProposalID: proposal.ProposalID, YodaReviewProposalSHA256: DigestJSON(proposal), YodaReviewSensitivity: proposal.Sensitivity, YodaReviewReaders: proposal.Readers, YodaReviewRefinement: proposal.Refinement, YodaReviewConfirmation: string(proposal.ConfirmationRequirement)}
	if _, err := store.CommitOwnerctxProposal(oldReservation, request.OwnerContextRoot, input, time.Now().UTC()); err == nil {
		t.Fatal("stale worker committed after fencing takeover")
	}
	entries, err := os.ReadDir(filepath.Join(request.OwnerContextRoot, "owner", "refinement", "proposals"))
	if (err != nil && !errors.Is(err, os.ErrNotExist)) || len(entries) != 0 {
		t.Fatalf("stale worker wrote an ownerctx proposal: entries=%d err=%v", len(entries), err)
	}
	if _, err := store.CommitOwnerctxProposal(newReservation, request.OwnerContextRoot, input, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
}

func TestHandlerDeadlineCancelsAdapterAndPreventsOwnerctxWrite(t *testing.T) {
	root := t.TempDir()
	if _, err := ownerctx.Initialize(root); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	command := maintenance.Command{SchemaVersion: maintenance.CommandSchemaVersion, CommandID: "deadline-command", JobID: WeeklyJobID, WorkspaceID: "workspace-1", Trigger: maintenance.TriggerWeekly, ScheduledFor: now, RequestedAt: now, Deadline: now.Add(40 * time.Millisecond), ProposalOnly: true}
	adapter := &deadlineAdapter{canceled: make(chan struct{})}
	handler := Handler{Root: root, OwnerID: "owner", CurrentPrompt: "Current request", CurrentLanguage: "en-US", WorkingLanguage: "en-US", TranslatorID: "translator", TranslatorVersion: "v1", ReviewFacets: []string{"voice"}, Translator: func(original, _, _ string) (string, error) { return original, nil }, Adapter: adapter, Authority: fakeAuthority(true), Store: ReceiptStore{Root: t.TempDir()}, MaintenanceStore: maintenance.Store{Root: t.TempDir()}, Now: func() time.Time { return now }}
	if _, err := handler.Handle(context.Background(), command); err == nil {
		t.Fatal("deadline cancellation was not surfaced")
	}
	select {
	case <-adapter.canceled:
	case <-time.After(time.Second):
		t.Fatal("adapter was not canceled by command deadline")
	}
	entries, err := os.ReadDir(filepath.Join(root, "owner", "refinement", "proposals"))
	if (err != nil && !errors.Is(err, os.ErrNotExist)) || len(entries) != 0 {
		t.Fatalf("deadline path wrote ownerctx proposal: entries=%d err=%v", len(entries), err)
	}
}

func TestHandlerDeadlineBoundsHungAdapterAndPreventsLateOwnerctxWrite(t *testing.T) {
	root := t.TempDir()
	if _, err := ownerctx.Initialize(root); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	command := maintenance.Command{SchemaVersion: maintenance.CommandSchemaVersion, CommandID: "hung-deadline-command", JobID: WeeklyJobID, WorkspaceID: "workspace-1", Trigger: maintenance.TriggerWeekly, ScheduledFor: now, RequestedAt: now, Deadline: now.Add(35 * time.Millisecond), ProposalOnly: true}
	adapter := &hungAdapter{started: make(chan struct{}), release: make(chan struct{})}
	handler := Handler{Root: root, OwnerID: "owner", CurrentPrompt: "Current request", CurrentLanguage: "en-US", WorkingLanguage: "en-US", TranslatorID: "translator", TranslatorVersion: "v1", ReviewFacets: []string{"voice"}, Translator: func(original, _, _ string) (string, error) { return original, nil }, Adapter: adapter, Authority: fakeAuthority(true), Store: ReceiptStore{Root: t.TempDir()}, MaintenanceStore: maintenance.Store{Root: t.TempDir()}, Now: func() time.Time { return now }}
	startedAt := time.Now()
	resultCh := make(chan error, 1)
	go func() {
		_, err := handler.Handle(context.Background(), command)
		resultCh <- err
	}()
	<-adapter.started
	select {
	case err := <-resultCh:
		if err == nil {
			t.Fatal("hung adapter unexpectedly succeeded")
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("handler waited past command deadline for hung adapter")
	}
	if elapsed := time.Since(startedAt); elapsed > 450*time.Millisecond {
		t.Fatalf("handler exceeded deadline bound: %s", elapsed)
	}
	close(adapter.release)
	entries, err := os.ReadDir(filepath.Join(root, "owner", "refinement", "proposals"))
	if (err != nil && !errors.Is(err, os.ErrNotExist)) || len(entries) != 0 {
		t.Fatalf("late hung adapter path wrote ownerctx proposal: entries=%d err=%v", len(entries), err)
	}
}

func TestAdvisoryLockDoesNotDeleteSuccessorLockOnTimeout(t *testing.T) {
	store := ReceiptStore{Root: t.TempDir(), LockWait: 40 * time.Millisecond}
	entered := make(chan struct{})
	release := make(chan struct{})
	firstDone := make(chan error, 1)
	go func() {
		firstDone <- store.withLock(func(string) error {
			close(entered)
			<-release
			return nil
		})
	}()
	<-entered
	if err := store.withLock(func(string) error { return nil }); !errors.Is(err, ErrOccurrenceBusy) {
		t.Fatalf("second worker unexpectedly acquired advisory lock: %v", err)
	}
	close(release)
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(store.Root, "weekly-receipts.lock"))
	if err != nil || !info.Mode().IsRegular() {
		t.Fatalf("advisory lock path was removed or replaced: info=%v err=%v", info, err)
	}
}

func TestOversizedCurrentPromptNeverCallsTranslator(t *testing.T) {
	root := t.TempDir()
	if _, err := ownerctx.Initialize(root); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	command := maintenance.Command{SchemaVersion: maintenance.CommandSchemaVersion, CommandID: "oversized-command", JobID: WeeklyJobID, WorkspaceID: "workspace-1", Trigger: maintenance.TriggerWeekly, ScheduledFor: now, RequestedAt: now, Deadline: now.Add(time.Minute), ProposalOnly: true}
	called := 0
	handler := Handler{Root: root, OwnerID: "owner", CurrentPrompt: strings.Repeat("x", MaxContextBytes+1), CurrentLanguage: "en-US", WorkingLanguage: "en-US", TranslatorID: "translator", TranslatorVersion: "v1", ReviewFacets: []string{"voice"}, Translator: func(string, string, string) (string, error) { called++; return "translated", nil }}
	if _, err := handler.BuildRequest(command, now); err == nil {
		t.Fatal("oversized prompt was accepted")
	}
	if called != 0 {
		t.Fatalf("translator was called %d times for oversized raw input", called)
	}
}

func TestOccurrenceDigestIsStableAcrossCommandRetries(t *testing.T) {
	root := t.TempDir()
	if _, err := ownerctx.Initialize(root); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.August, 1, 12, 0, 0, 0, time.UTC)
	command := maintenance.Command{SchemaVersion: maintenance.CommandSchemaVersion, CommandID: "command-one", JobID: WeeklyJobID, WorkspaceID: "workspace-1", Trigger: maintenance.TriggerWeekly, ScheduledFor: now, RequestedAt: now, Deadline: now.Add(time.Minute), ProposalOnly: true}
	retry := command
	retry.CommandID = "command-two"
	handler := Handler{Root: root, OwnerID: "owner", CurrentPrompt: "Current request", CurrentLanguage: "en-US", WorkingLanguage: "en-US", TranslatorID: "translator", TranslatorVersion: "v1", Translator: func(original, _, _ string) (string, error) { return original, nil }, ReviewFacets: []string{"voice"}}
	first, err := handler.BuildRequest(command, now)
	if err != nil {
		t.Fatal(err)
	}
	second, err := handler.BuildRequest(retry, now)
	if err != nil {
		t.Fatal(err)
	}
	if first.OccurrenceID != second.OccurrenceID || first.OccurrenceID == command.CommandID {
		t.Fatalf("command retry changed occurrence identity: first=%q second=%q", first.OccurrenceID, second.OccurrenceID)
	}
}

func TestReceiptStoreRejectsSymlinkAndPartialLine(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(root, "symlink-root")); err != nil {
		t.Fatal(err)
	}
	if _, err := (ReceiptStore{Root: filepath.Join(root, "symlink-root")}).Reserve(testRequest(t), time.Now().UTC()); err == nil {
		t.Fatal("symlinked receipt parent was followed")
	}
	store := ReceiptStore{Root: filepath.Join(root, "store")}
	request := testRequest(t)
	if _, err := store.Reserve(request, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(store.Root, "weekly-receipts.jsonl")
	f, err := os.OpenFile(file, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteString("{\"partial\"")
	_ = f.Close()
	if _, err := store.Reserve(request, time.Now().UTC()); err == nil {
		t.Fatal("partial receipt line was silently recovered")
	}
}

func TestFacetPolicyRejectsWrongPromotionMode(t *testing.T) {
	request := testRequest(t)
	proposal := testProposal(request)
	proposal.Facet = "decision-rules"
	proposal.ConfirmationRequirement = ConfirmationAutomaticWithAudit
	if err := ValidateProposal(request, proposal); err == nil {
		t.Fatal("proposal-only facet accepted automatic policy")
	}
	proposal = testProposal(request)
	proposal.Facet = "intrinsic-intent"
	proposal.ConfirmationRequirement = ConfirmationExplicit
	if err := ValidateProposal(request, proposal); err == nil {
		t.Fatal("intrinsic-purpose hypothesis should remain ephemeral")
	}
}

func TestMaintenanceHandlerReturnsUnavailableUntilQualified(t *testing.T) {
	root := t.TempDir()
	if _, err := ownerctx.Initialize(root); err != nil {
		t.Fatal(err)
	}
	if _, err := ownerctx.RecordUserPrompt(root, ownerctx.PromptHistoryInput{OwnerID: "owner", OccurrenceID: "prompt-occurrence", Prompt: "Current request", Language: "en-US", Source: "owner", SessionID: "session", ScopeKind: ownerctx.PromptScopeGlobal, ScopeID: "owner"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ownerctx.AppendObservation(root, ownerctx.ObservationInput{SchemaVersion: 1, Signal: ownerctx.SignalExplicitCorrection, Facet: "voice", Claim: "concise", EvidenceType: "owner_correction", SourceEvent: "interaction.completed", SourceDigest: Digest("source"), EpisodeID: "episode-1", ScopeKind: "global", ScopeID: "owner", Confidence: .9, Sensitivity: "professional", ExpiresAt: time.Now().UTC().Add(time.Hour), AuthenticatedOwner: true, Material: true, OwnerConfirmed: true}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	command := maintenance.Command{SchemaVersion: maintenance.CommandSchemaVersion, CommandID: "command-1", JobID: WeeklyJobID, WorkspaceID: "workspace-1", Trigger: maintenance.TriggerWeekly, ScheduledFor: now, RequestedAt: now, Deadline: now.Add(time.Minute), ProposalOnly: true}
	maintenanceRoot := t.TempDir()
	handler := Handler{Root: root, OwnerID: "owner", ScopeKind: ownerctx.PromptScopeGlobal, ScopeID: "owner", CurrentPrompt: "Current request", CurrentLanguage: "en-US", WorkingLanguage: "en-US", TranslatorID: "translator", TranslatorVersion: "v1", Translator: func(original, _, _ string) (string, error) { return original, nil }, ReviewFacets: []string{"voice"}, Store: ReceiptStore{Root: t.TempDir()}, MaintenanceStore: maintenance.Store{Root: maintenanceRoot}, Now: func() time.Time { return now }}
	executed, err := handler.Execute(context.Background(), command)
	if !errors.Is(err, ErrUnavailable) || executed.State != maintenance.ReceiptUnavailable {
		t.Fatalf("execution seam did not return typed unavailable result: %+v %v", executed, err)
	}
	if receipts, readErr := handler.MaintenanceStore.Receipts(command.WorkspaceID, command.JobID); readErr != nil || len(receipts) != 0 {
		t.Fatalf("execution seam published a terminal receipt: %#v %v", receipts, readErr)
	}
	receipt, err := handler.Handle(context.Background(), command)
	if err != nil || receipt.State != maintenance.ReceiptUnavailable {
		t.Fatalf("unqualified handler did not fail closed: %+v %v", receipt, err)
	}
	if receipts, readErr := handler.MaintenanceStore.Receipts(command.WorkspaceID, command.JobID); readErr != nil || len(receipts) != 1 {
		t.Fatalf("direct handler did not publish exactly one terminal receipt: %#v %v", receipts, readErr)
	}
}
