package memory

import (
	"errors"
	"os"
	"testing"
	"time"
)

func TestLegacyCaptureRejectsOwnedIdentityBeforeEffects(t *testing.T) {
	if err := ownedIOAvailable(); err != nil {
		t.Skip(err)
	}
	for _, field := range []string{"id", "revision"} {
		t.Run(field, func(t *testing.T) {
			e := testEngine(t)
			at := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
			survivor := Capture{WorkspaceID: "case-a", RecordedAt: at, Kind: "legacy", Text: "existing readable survivor", Sanitized: true}
			path, err := e.Capture(survivor)
			if err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			visibility, err := e.VisibilityRevision("case-a")
			if err != nil {
				t.Fatal(err)
			}
			candidate := survivor
			candidate.Text = "forbidden owned identity"
			if field == "id" {
				candidate.ID = "0123456789abcdef0123456789abcdef"
			} else {
				candidate.Revision = 1
			}
			if _, err := e.Capture(candidate); err == nil {
				t.Fatal("journal writer accepted owned-only metadata")
			}
			after, err := os.ReadFile(path)
			if err != nil || string(after) != string(before) {
				t.Fatalf("rejected capture changed existing journal: %v", err)
			}
			if revision, err := e.VisibilityRevision("case-a"); err != nil || revision != visibility {
				t.Fatalf("rejected capture changed visibility: %d %v", revision, err)
			}
			if page, err := e.ListCaptures("case-a", 0, 10); err != nil || page.Total != 1 {
				t.Fatalf("survivor discovery poisoned: %#v %v", page, err)
			}
			candidate.WorkspaceID = "new-workspace"
			if _, err := e.Capture(candidate); err == nil {
				t.Fatal("fresh workspace accepted owned-only metadata")
			}
			if _, err := os.Stat(e.workspaceRoot(candidate.WorkspaceID)); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("rejected capture created filesystem state: %v", err)
			}
		})
	}
}

func TestReviewedOwnedCaptureVisibilityFenceAndSourceCompatibility(t *testing.T) {
	if err := ownedIOAvailable(); err != nil {
		t.Skip(err)
	}
	e := testEngine(t)
	if err := e.PrepareOwnedCaptureAuthority("case-a"); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	source, err := e.SealOwnedCapture("case-a", "", 0, "existing trusted source caller", at)
	if err != nil {
		t.Fatal(err)
	}
	first, err := e.SealReviewedOwnedCapture("case-a", 0, "one reviewed occurrence", at)
	if err != nil {
		t.Fatal(err)
	}
	second, err := e.SealReviewedOwnedCapture("case-a", 0, "one reviewed occurrence", at)
	if err != nil {
		t.Fatal(err)
	}
	winner, err := e.CaptureOwned("case-a", first)
	if err != nil || winner.VisibilityRevision != 1 {
		t.Fatalf("reviewed create: %#v %v", winner, err)
	}
	if _, err := e.CaptureOwned("case-a", second); !errors.Is(err, ErrCaptureConflict) {
		t.Fatalf("stale reviewed visibility admitted: %v", err)
	}
	if _, err := e.CaptureOwned("case-a", first); !errors.Is(err, ErrCaptureConflict) {
		t.Fatalf("reviewed replay did not require a fresh base: %v", err)
	}
	current, err := e.SealReviewedOwnedCapture("case-a", 1, "new reviewed occurrence", at)
	if err != nil {
		t.Fatal(err)
	}
	if r, err := e.CaptureOwned("case-a", current); err != nil || r.VisibilityRevision != 2 {
		t.Fatalf("current reviewed base refused: %#v %v", r, err)
	}
	legacy, err := e.CaptureOwned("case-a", source)
	if err != nil || legacy.VisibilityRevision != 3 {
		t.Fatalf("existing source seal gained an implicit fence: %#v %v", legacy, err)
	}
	replayed, err := e.CaptureOwned("case-a", source)
	if err != nil || replayed.ID != legacy.ID || replayed.Revision != legacy.Revision || replayed.VisibilityRevision != legacy.VisibilityRevision {
		t.Fatalf("existing source replay behavior changed: %#v %v", replayed, err)
	}
	if page, err := e.ListCaptures("case-a", 0, 10); err != nil || page.Total != 3 || page.VisibilityRevision != 3 {
		t.Fatalf("rejected reviewed inputs had effects: %#v %v", page, err)
	}
}

func TestReviewedOwnedCaptureFenceCannotBeRemovedOrRetargeted(t *testing.T) {
	if err := ownedIOAvailable(); err != nil {
		t.Skip(err)
	}
	e := testEngine(t)
	if err := e.PrepareOwnedCaptureAuthority("case-a"); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	reviewed, err := e.SealReviewedOwnedCapture("case-a", 0, "exact reviewed input", at)
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"remove-fence", "change-visibility", "change-text", "change-workspace"} {
		t.Run(kind, func(t *testing.T) {
			changed := reviewed
			switch kind {
			case "remove-fence":
				changed.hasReviewedVisibility = false
			case "change-visibility":
				changed.reviewedVisibilityRevision = 1
			case "change-text":
				changed.Text = "different input"
			case "change-workspace":
				changed.workspaceID = "case-b"
			}
			if _, err := e.CaptureOwned("case-a", changed); err == nil {
				t.Fatal("modified opaque input admitted")
			}
		})
	}
	if revision, err := e.VisibilityRevision("case-a"); err != nil || revision != 0 {
		t.Fatalf("modified fence caused effects: %d %v", revision, err)
	}
}
