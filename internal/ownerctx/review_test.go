package ownerctx_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ownerctx"
)

func ownerRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if _, err := ownerctx.Initialize(root); err != nil {
		t.Fatal(err)
	}
	return root
}

func proposeOwner(t *testing.T, root, facet, body string) ownerctx.RefinementReceipt {
	t.Helper()
	r, err := ownerctx.SubmitRefinement(root, ownerctx.RefinementInput{Facet: facet, Evidence: "explicit synthetic owner answer", ProposedBody: body, ProducerID: "product-owner"})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestReviewedRefinementBindsExactProposalAndBase(t *testing.T) {
	root := ownerRoot(t)
	r := proposeOwner(t, root, "personal-context", "## Current\nSynthetic private marker.\n")
	review, err := ownerctx.ReviewRefinement(root, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if review.ProposedBody != "## Current\nSynthetic private marker.\n" || review.Evidence == "" || review.CurrentBody == "" || review.Policy != "confirmation_required" || review.BaseSHA256 == "" || review.ProposalSHA256 != r.ProposalSHA256 || len(review.ReviewDigest) != 64 {
		t.Fatalf("incomplete review: %#v", review)
	}
	if _, err := ownerctx.ApplyReviewedRefinement(root, r.ID, review.ReviewDigest, false); !errors.Is(err, ownerctx.ErrConfirmationRequired) {
		t.Fatalf("missing confirmation: %v", err)
	}
	if _, err := ownerctx.ApplyReviewedRefinement(root, r.ID, strings.Repeat("0", 64), true); err == nil {
		t.Fatal("wrong review accepted")
	}
	other := proposeOwner(t, root, "personal-context", "## Current\nNewer synthetic answer.\n")
	otherReview, err := ownerctx.ReviewRefinement(root, other.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ownerctx.ApplyReviewedRefinement(root, other.ID, otherReview.ReviewDigest, true); err != nil {
		t.Fatal(err)
	}
	if _, err := ownerctx.ApplyReviewedRefinement(root, r.ID, review.ReviewDigest, true); !errors.Is(err, ownerctx.ErrRevisionConflict) {
		t.Fatalf("stale base accepted: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(root, "owner", "self", "personal-context.md"))
	if err != nil || strings.Contains(string(body), "private marker") {
		t.Fatalf("newer work lost: %q %v", body, err)
	}
}

func TestReviewedOnboardingTrackConfirmationAndRestart(t *testing.T) {
	root := ownerRoot(t)
	if _, err := ownerctx.SelectOnboardingTrack(root, "quick"); err != nil {
		t.Fatal(err)
	}
	for _, step := range ownerctx.QuickStartInterview().Steps {
		r := proposeOwner(t, root, step.Facet, "## Current\nSynthetic answer for "+step.Facet+".\n")
		review, err := ownerctx.ReviewRefinement(root, r.ID)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ownerctx.ApplyReviewedRefinement(root, r.ID, review.ReviewDigest, true); err != nil {
			t.Fatal(err)
		}
	}
	review, err := ownerctx.ReviewOnboarding(root)
	if err != nil {
		t.Fatal(err)
	}
	if review.Track != "quick" || len(review.Facets) != 6 || len(review.ReviewDigest) != 64 {
		t.Fatalf("incomplete onboarding review: %#v", review)
	}
	if _, err := ownerctx.ConfirmOnboarding(root, review.ReviewDigest); err != nil {
		t.Fatal(err)
	}
	status, err := ownerctx.Inspect(root)
	if err != nil || status.Onboarding.State != "complete" {
		t.Fatalf("restart: %#v %v", status.Onboarding, err)
	}
	if _, err := ownerctx.SelectOnboardingTrack(root, "complete"); err != nil {
		t.Fatal(err)
	}
	if _, err := ownerctx.ConfirmOnboarding(root, review.ReviewDigest); err == nil {
		t.Fatal("changed track accepted old confirmation")
	}
	status, err = ownerctx.Inspect(root)
	if err != nil || status.Onboarding.State == "complete" {
		t.Fatalf("track change did not invalidate review: %#v %v", status.Onboarding, err)
	}
}

func TestReviewedConcurrentRefinementsPreserveWinner(t *testing.T) {
	root := ownerRoot(t)
	first := proposeOwner(t, root, "owner-identity", "## Current\nFirst synthetic answer.\n")
	second := proposeOwner(t, root, "owner-identity", "## Current\nSecond synthetic answer.\n")
	a, err := ownerctx.ReviewRefinement(root, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	b, err := ownerctx.ReviewRefinement(root, second.ID)
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, review := range []ownerctx.RefinementReview{a, b} {
		wg.Add(1)
		go func(review ownerctx.RefinementReview) {
			defer wg.Done()
			<-start
			_, err := ownerctx.ApplyReviewedRefinement(root, review.ID, review.ReviewDigest, true)
			results <- err
		}(review)
	}
	close(start)
	wg.Wait()
	close(results)
	success, conflict := 0, 0
	for err := range results {
		if err == nil {
			success++
		} else if errors.Is(err, ownerctx.ErrRevisionConflict) {
			conflict++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("concurrent writes: successes=%d conflicts=%d", success, conflict)
	}
}

func TestReviewedOwnerRejectsAmbiguousNULTextBeforeEffects(t *testing.T) {
	root := ownerRoot(t)
	if _, err := ownerctx.SelectOnboardingTrack(root, "quick"); err != nil {
		t.Fatal(err)
	}
	for _, step := range ownerctx.QuickStartInterview().Steps {
		r := proposeOwner(t, root, step.Facet, "Synthetic answer for "+step.Facet)
		review, err := ownerctx.ReviewRefinement(root, r.ID)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ownerctx.ApplyReviewedRefinement(root, r.ID, review.ReviewDigest, true); err != nil {
			t.Fatal(err)
		}
	}
	review, err := ownerctx.ReviewOnboarding(root)
	if err != nil {
		t.Fatal(err)
	}
	proposalDir := filepath.Join(root, "owner", "refinement", "proposals")
	entries, err := os.ReadDir(proposalDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range []ownerctx.RefinementInput{
		{Facet: "owner-identity", Evidence: "explicit synthetic answer", ProposedBody: "Alpha\x00personal-context\x00Beta"},
		{Facet: "owner-identity", Evidence: "bad\x00evidence", ProposedBody: "Synthetic answer"},
	} {
		if _, err := ownerctx.SubmitRefinement(root, input); err == nil {
			t.Fatal("NUL-bearing proposal persisted")
		}
	}
	current, err := os.ReadDir(proposalDir)
	if err != nil || len(current) != len(entries) {
		t.Fatalf("rejected intake persisted a proposal: %v", err)
	}
	identity := filepath.Join(root, "owner", "self", "owner-identity.md")
	if err := os.WriteFile(identity, []byte("Alpha\x00personal-context\x00Beta"), 0600); err != nil {
		t.Fatal(err)
	}
	if value, err := ownerctx.ReviewOnboarding(root); err == nil || len(value.Facets) != 0 {
		t.Fatalf("persisted NUL-bearing facet reviewed: %#v %v", value, err)
	}
	if _, err := ownerctx.ConfirmOnboarding(root, review.ReviewDigest); err == nil {
		t.Fatal("persisted NUL-bearing facet confirmed")
	}
	status, err := ownerctx.Inspect(root)
	if err != nil {
		t.Fatal(err)
	}
	if status.Onboarding.State == "complete" || status.Onboarding.ReviewDigest != "" {
		t.Fatalf("malformed facet retained digest/confirmation: %#v", status.Onboarding)
	}
}

func TestReviewedOwnerRejectsPersistedNULProposalBeforeEffects(t *testing.T) {
	root := ownerRoot(t)
	r := proposeOwner(t, root, "owner-identity", "Synthetic answer")
	review, err := ownerctx.ReviewRefinement(root, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "owner", "refinement", "proposals", r.ID+".json")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err := json.Unmarshal(body, &value); err != nil {
		t.Fatal(err)
	}
	value["proposed_body"] = "Alpha\x00personal-context\x00Beta"
	body, err = json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0600); err != nil {
		t.Fatal(err)
	}
	if got, err := ownerctx.ReviewRefinement(root, r.ID); err == nil || got.ProposedBody != "" {
		t.Fatalf("persisted malformed proposal reviewed: %#v %v", got, err)
	}
	if _, err := ownerctx.ApplyReviewedRefinement(root, r.ID, review.ReviewDigest, true); err == nil {
		t.Fatal("persisted malformed proposal applied")
	}
	if _, err := ownerctx.ApplyRefinement(root, r.ID, true); err == nil {
		t.Fatal("legacy apply accepted malformed proposal")
	}
	if entries, _ := os.ReadDir(filepath.Join(root, "owner", "refinement", "audits")); len(entries) != 0 {
		t.Fatal("malformed proposal produced an audit")
	}
}
