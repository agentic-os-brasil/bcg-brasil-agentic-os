package ownerctx_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ownerctx"
)

func TestReviewedTrackSelection(t *testing.T) {
	root := ownerRoot(t)
	if _, err := ownerctx.SelectOnboardingTrack(root, "complete"); err != nil {
		t.Fatal(err)
	}
	review, err := ownerctx.ReviewOnboardingTrack(root, "quick")
	if err != nil {
		t.Fatal(err)
	}
	if review.Track != "quick" || review.CurrentOnboarding.Track != "complete" || len(review.ReviewDigest) != 64 || review.TrackRevision == 0 {
		t.Fatalf("invalid review: %+v", review)
	}
	if _, err = ownerctx.ApplyReviewedOnboardingTrack(root, "quick", review.ReviewDigest, false); !errors.Is(err, ownerctx.ErrConfirmationRequired) {
		t.Fatalf("missing approval: %v", err)
	}
	if _, err = ownerctx.ApplyReviewedOnboardingTrack(root, "complete", review.ReviewDigest, true); !errors.Is(err, ownerctx.ErrRevisionConflict) {
		t.Fatalf("retargeted review: %v", err)
	}
	status, err := ownerctx.ApplyReviewedOnboardingTrack(root, "quick", review.ReviewDigest, true)
	if err != nil || status.Onboarding.Track != "quick" {
		t.Fatalf("selection: %+v %v", status, err)
	}
	if _, err = ownerctx.ApplyReviewedOnboardingTrack(root, "quick", review.ReviewDigest, true); !errors.Is(err, ownerctx.ErrRevisionConflict) {
		t.Fatalf("replayed review: %v", err)
	}
}

func TestReviewedTrackSelectionRejectsSameValueCommit(t *testing.T) {
	root := ownerRoot(t)
	if _, err := ownerctx.SelectOnboardingTrack(root, "complete"); err != nil {
		t.Fatal(err)
	}
	review, err := ownerctx.ReviewOnboardingTrack(root, "quick")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = ownerctx.SelectOnboardingTrack(root, "complete"); err != nil {
		t.Fatal(err)
	}
	current, err := ownerctx.ReviewOnboardingTrack(root, "quick")
	if err != nil {
		t.Fatal(err)
	}
	if current.TrackRevision != review.TrackRevision+1 || current.ReviewDigest == review.ReviewDigest {
		t.Fatal("same-value selection did not advance reviewed state")
	}
	if _, err = ownerctx.ApplyReviewedOnboardingTrack(root, "quick", review.ReviewDigest, true); !errors.Is(err, ownerctx.ErrRevisionConflict) {
		t.Fatalf("stale selection: %v", err)
	}
	status, err := ownerctx.Inspect(root)
	if err != nil || status.Onboarding.Track != "complete" {
		t.Fatalf("newer selection lost: %+v %v", status, err)
	}
}

func TestReviewedTrackSelectionRevisionOverflow(t *testing.T) {
	root := ownerRoot(t)
	path := filepath.Join(root, "owner/registry.json")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]json.RawMessage
	if err = json.Unmarshal(body, &value); err != nil {
		t.Fatal(err)
	}
	value["onboarding_track_revision"] = json.RawMessage("18446744073709551615")
	body, err = json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, body, 0600); err != nil {
		t.Fatal(err)
	}
	review, err := ownerctx.ReviewOnboardingTrack(root, "quick")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = ownerctx.ApplyReviewedOnboardingTrack(root, "quick", review.ReviewDigest, true); err == nil {
		t.Fatal("reviewed revision overflow admitted")
	}
	if _, err = ownerctx.SelectOnboardingTrack(root, "quick"); err == nil {
		t.Fatal("legacy revision overflow admitted")
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != string(body) {
		t.Fatal("overflow mutated registry")
	}
}
