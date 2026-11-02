package memory

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func ownedInput(t *testing.T, e *Engine, id string, revision uint64, text string, at time.Time) OwnedCaptureInput {
	t.Helper()
	in, err := e.SealOwnedCapture("case-a", id, revision, text, at)
	if err != nil {
		t.Fatal(err)
	}
	return in
}

func TestOwnedMemoryLifecycle(t *testing.T) {
	if err := ownedIOAvailable(); err != nil {
		t.Skip(err)
	}
	e := testEngine(t)
	day := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	if _, err := e.SealOwnedCapture("case-a", "", 0, "unprepared", day); err == nil {
		t.Fatal("ordinary use prepared authority")
	}
	if err := e.PrepareOwnedCaptureAuthority("case-a"); err != nil {
		t.Fatal(err)
	}
	e.Synthesizer = DeterministicL1Synthesizer{MaxRunes: 4096, MaxEntries: 64, MaxInputBytes: 1 << 20, MaxInputEntries: 256, Attestor: CaptureAttestor{Root: e.Root}}
	r, err := e.CaptureOwned("case-a", ownedInput(t, e, "", 0, "unique-old-marker", day))
	if err != nil {
		t.Fatal(err)
	}
	restarted := *e
	got, err := restarted.ReadCapture("case-a", r.ID)
	if err != nil || got.Text != "unique-old-marker" || got.Revision != 1 || got.Kind != OwnedCaptureKind {
		t.Fatalf("readback %#v: %v", got, err)
	}
	if _, err := e.DreamDailyAttested(context.Background(), "case-a", day); err != nil {
		t.Fatal(err)
	}
	assertContextMarker(t, e, "unique-old-marker", true)
	oldManifest, _, err := e.latestManifest("case-a")
	if err != nil {
		t.Fatal(err)
	}
	r2, err := e.CorrectCapture("case-a", r.ID, r.Revision, ownedInput(t, e, r.ID, r.Revision, "unique-new-marker", day))
	if err != nil || r2.ID != r.ID || r2.Revision != 2 {
		t.Fatalf("correction %#v: %v", r2, err)
	}
	if _, _, err := e.readArtifactFromManifest("case-a", oldManifest, "L1/2026-07-20"); err == nil {
		t.Fatal("cached manifest bypassed barrier")
	}
	assertContextMarker(t, e, "unique-old-marker", false)
	if _, err := e.DeleteCapture("case-a", r.ID, 1); !errors.Is(err, ErrCaptureConflict) {
		t.Fatalf("stale delete: %v", err)
	}
	if _, err := e.DreamDailyAttested(context.Background(), "case-a", day); err != nil {
		t.Fatal(err)
	}
	assertContextMarker(t, e, "unique-new-marker", true)
	if _, err := e.DeleteCapture("case-a", r.ID, 2); err != nil {
		t.Fatal(err)
	}
	if _, err := restarted.ReadCapture("case-a", r.ID); !errors.Is(err, ErrCaptureUnavailable) {
		t.Fatalf("deleted capture readable: %v", err)
	}
	assertContextMarker(t, e, "unique-new-marker", false)
	if _, err := e.DreamDailyAttested(context.Background(), "case-a", day); err != nil {
		t.Fatalf("empty dream: %v", err)
	}
	if _, _, err := e.latestManifest("case-a"); err != nil {
		t.Fatalf("intentional empty view invalid: %v", err)
	}
	if err := filepath.WalkDir(e.workspaceRoot("case-a"), func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if strings.Contains(string(b), "unique-old-marker") || strings.Contains(string(b), "unique-new-marker") {
				t.Errorf("denied plaintext retained in %s", path)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func assertContextMarker(t *testing.T, e *Engine, marker string, want bool) {
	t.Helper()
	b, err := e.AssembleContext("case-a")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, s := range b.Sections {
		found = found || strings.Contains(s.Content, marker)
	}
	if found != want {
		t.Fatalf("marker %q visible=%v want=%v: %#v", marker, found, want, b)
	}
}

func TestOwnedMemoryRetentionAndInterruptedBarrier(t *testing.T) {
	if err := ownedIOAvailable(); err != nil {
		t.Skip(err)
	}
	e := testEngine(t)
	if err := e.PrepareOwnedCaptureAuthority("case-a"); err != nil {
		t.Fatal(err)
	}
	day := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	old, err := e.CaptureOwned("case-a", ownedInput(t, e, "", 0, "old-retention-marker", day))
	if err != nil {
		t.Fatal(err)
	}
	keep, err := e.CaptureOwned("case-a", ownedInput(t, e, "", 0, "recent-marker", day.Add(time.Hour)))
	if err != nil {
		t.Fatal(err)
	}
	v, err := e.VisibilityRevision("case-a")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.ApplyRetention("case-a", day.Add(time.Minute), v+1); !errors.Is(err, ErrCaptureConflict) {
		t.Fatalf("stale retention: %v", err)
	}
	e.FaultPoint = func(point string) error {
		if point == "after_visibility_barrier" {
			return errors.New("interrupted")
		}
		return nil
	}
	if _, err := e.ApplyRetention("case-a", day.Add(time.Minute), v); err == nil {
		t.Fatal("fault ignored")
	}
	e.FaultPoint = nil
	if _, err := e.ReadCapture("case-a", old.ID); !errors.Is(err, ErrCaptureUnavailable) {
		t.Fatalf("barrier bypass: %v", err)
	}
	if _, err := e.ReadCapture("case-a", keep.ID); err != nil {
		t.Fatalf("survivor lost: %v", err)
	}
	v, err = e.VisibilityRevision("case-a")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.ApplyRetention("case-a", day.Add(time.Minute), v); err != nil {
		t.Fatal(err)
	}
}
