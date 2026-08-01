package walterselfreview

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

func (adapter *blockingAdapter) ID() string { return "blocking-walter" }
func (adapter *blockingAdapter) Review(ctx context.Context, _ ModelInput) (SelfRefinementProposal, error) {
	close(adapter.started)
	select {
	case <-adapter.release:
		return adapter.proposal, nil
	case <-ctx.Done():
		return SelfRefinementProposal{}, ctx.Err()
	}
}

func (adapter *fakeAdapter) ID() string { return "test-walter" }
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
	hypothesis := &maestro.IntentHypothesis{ExpressedObjective: "answer the request", LatentIntentHypothesis: "preserve the user's decision purpose", EvidenceRefs: []string{"prompt-1"}, Confidence: .8, Alternatives: []string{"literal-only"}, Materiality: "low", DisconfirmationCondition: "current instruction contradicts it", WorkingPrompt: request.CurrentNormalized}
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
		t.Fatal("Walter implicitly projected all self facets")
	}
	base.ReviewFacets = []string{"psychological-profile"}
	if _, err := base.BuildRequest(command, now); err == nil {
		t.Fatal("unauthorized sensitive facet was projected")
	}
	base.SensitivePurpose = "authorized Walter self-review"
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
	proposal.Readers = []string{"walter"}
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
	ownerReceipt, err := ownerctx.SubmitRefinement(request.OwnerContextRoot, ownerctx.RefinementInput{Facet: proposal.Facet, Evidence: "walter-weekly:" + DigestJSON(proposal.EvidenceObservationIDs), ProposedBody: proposal.ProposedRefinement, OccurrenceID: request.OccurrenceID, WalterReviewRequestSHA256: RequestDigest(request), WalterReviewProposalID: proposal.ProposalID, WalterReviewProposalSHA256: DigestJSON(proposal), WalterReviewSensitivity: proposal.Sensitivity, WalterReviewReaders: proposal.Readers, WalterReviewRefinement: proposal.Refinement, WalterReviewConfirmation: string(proposal.ConfirmationRequirement), WalterReviewAdapterID: "test-walter", WalterReviewAuthorityID: "test-authority", WalterReviewFencingToken: reservation.Receipt.FencingToken})
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
	store := ReceiptStore{Root: t.TempDir(), LeaseDuration: 60 * time.Millisecond, LockWait: 40 * time.Millisecond}
	adapter := &blockingAdapter{started: make(chan struct{}), release: make(chan struct{}), proposal: testProposal(request)}
	resultCh := make(chan error, 1)
	go func() {
		_, _, err := Review(context.Background(), request, adapter, fakeAuthority(true), store, time.Now().UTC())
		resultCh <- err
	}()
	<-adapter.started
	time.Sleep(180 * time.Millisecond)
	if _, err := store.Reserve(request, time.Now().UTC()); !errors.Is(err, ErrOccurrenceBusy) {
		t.Fatalf("slow model lease was not renewed; reserve err=%v", err)
	}
	close(adapter.release)
	if err := <-resultCh; err != nil {
		t.Fatal(err)
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
	handler := Handler{Root: root, OwnerID: "owner", ScopeKind: ownerctx.PromptScopeGlobal, ScopeID: "owner", CurrentPrompt: "Current request", CurrentLanguage: "en-US", WorkingLanguage: "en-US", TranslatorID: "translator", TranslatorVersion: "v1", Translator: func(original, _, _ string) (string, error) { return original, nil }, ReviewFacets: []string{"voice"}, Store: ReceiptStore{Root: t.TempDir()}, MaintenanceStore: maintenance.Store{Root: t.TempDir()}, Now: func() time.Time { return now }}
	receipt, err := handler.Handle(context.Background(), command)
	if !errors.Is(err, ErrUnavailable) || receipt.State != maintenance.ReceiptUnavailable {
		t.Fatalf("unqualified handler did not fail closed: %+v %v", receipt, err)
	}
}
