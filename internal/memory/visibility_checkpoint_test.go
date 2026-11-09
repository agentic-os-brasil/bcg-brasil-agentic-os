//go:build !windows

package memory

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func installLegacyVisibility(t *testing.T, e *Engine, count int, nearBytes bool) (int, int) {
	t.Helper()
	directory := filepath.Join(e.workspaceRoot("case-a"), ".visibility")
	if err := os.RemoveAll(directory); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(directory, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(e.workspaceRoot("case-a"), ".visibility-checkpoint-required")); err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	denied := map[string]uint64{}
	total, last := 0, 0
	for rev := 1; rev <= count; rev++ {
		generation := 0
		if nearBytes && rev >= 101 {
			generation = 101
			if len(denied) == 0 {
				for n := 1; n <= 100; n++ {
					denied[fmt.Sprintf("%032x", n)] = 1
				}
			}
		}
		body, err := json.Marshal(map[string]any{"schema_version": 1, "workspace_id": "case-a", "revision": rev, "generation": generation, "denied": denied})
		if err != nil {
			t.Fatal(err)
		}
		if nearBytes && total+len(body) > 8<<20 {
			break
		}
		if err := os.WriteFile(filepath.Join(directory, fmt.Sprintf("%020d.json", rev)), body, 0600); err != nil {
			t.Fatal(err)
		}
		total += len(body)
		last = rev
	}
	return last, total
}

func checkpointEngine(t *testing.T) (*Engine, CaptureReceipt, time.Time) {
	t.Helper()
	e := testEngine(t)
	if err := e.PrepareOwnedCaptureAuthority("case-a"); err != nil {
		t.Fatal(err)
	}
	day := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	r, err := e.CaptureOwned("case-a", ownedInput(t, e, "", 0, "capacity-survivor", day))
	if err != nil {
		t.Fatal(err)
	}
	return e, r, day
}

func TestOwnedMemoryVisibilityCapacityRemainsReadable(t *testing.T) {
	t.Run("byte-budget", func(t *testing.T) {
		e, old, day := checkpointEngine(t)
		last, total := installLegacyVisibility(t, e, 4096, true)
		if _, err := e.ReadCapture("case-a", old.ID); err != nil {
			t.Fatal(err)
		}
		added, err := e.CaptureOwned("case-a", ownedInput(t, e, "", 0, "new-capacity-survivor", day))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := e.ReadCapture("case-a", added.ID); err != nil {
			t.Fatalf("successful capture unreadable after legacy revision=%d bytes=%d: %v", last, total, err)
		}
		if _, err := e.ListCaptures("case-a", 0, 10); err != nil {
			t.Fatal(err)
		}
		if _, err := e.ApplyRetention("case-a", day.Add(time.Hour), added.VisibilityRevision); err != nil {
			t.Fatalf("retention stranded: %v", err)
		}
	})
	for _, operation := range []string{"delete", "retention"} {
		t.Run("transition-budget-"+operation, func(t *testing.T) {
			e, r, day := checkpointEngine(t)
			installLegacyVisibility(t, e, 4096, false)
			if _, err := e.ReadCapture("case-a", r.ID); err != nil {
				t.Fatal(err)
			}
			var err error
			if operation == "delete" {
				_, err = e.DeleteCapture("case-a", r.ID, 1)
			} else {
				_, err = e.ApplyRetention("case-a", day.Add(time.Hour), 4096)
			}
			if err != nil {
				t.Fatalf("cleanup at former transition cap: %v", err)
			}
			if _, err := e.ReadCapture("case-a", r.ID); !errors.Is(err, ErrCaptureUnavailable) {
				t.Fatalf("cleanup readback: %v", err)
			}
			if _, err := e.ListCaptures("case-a", 0, 10); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestOwnedMemoryCheckpointCapacityReservesRemoval(t *testing.T) {
	e, r, day := checkpointEngine(t)
	b, err := e.readVisibility("case-a")
	if err != nil {
		t.Fatal(err)
	}
	b.Revision = 2
	b.Generation = 2
	for n := 1; n < maximumVisibilityIdentities; n++ {
		b.Denied[fmt.Sprintf("%032x", n)] = 1
	}
	body, _ := json.Marshal(b)
	if err := memoryPublish(filepath.Join(e.workspaceRoot("case-a"), ".visibility", "checkpoint.json"), body, false); err != nil {
		t.Fatal(err)
	}
	if _, err := e.CaptureOwned("case-a", ownedInput(t, e, "", 0, "over-budget", day)); !errors.Is(err, ErrCaptureCapacity) {
		t.Fatalf("new identity capacity outcome: %v", err)
	}
	if _, err := e.Capture(Capture{WorkspaceID: "case-a", RecordedAt: day, Text: "legacy-over-budget", Kind: "legacy", Sanitized: true}); !errors.Is(err, ErrCaptureCapacity) {
		t.Fatalf("legacy identity capacity outcome: %v", err)
	}
	if revision, err := e.VisibilityRevision("case-a"); err != nil || revision != 2 {
		t.Fatalf("rejected admission changed state: %d %v", revision, err)
	}
	if _, err := e.ReadCapture("case-a", r.ID); err != nil {
		t.Fatal(err)
	}
	page, err := e.ListCaptures("case-a", 0, 10)
	if err != nil || page.Total != 1 {
		t.Fatalf("capacity readback: %#v %v", page, err)
	}
	corrected, err := e.CorrectCapture("case-a", r.ID, 1, ownedInput(t, e, r.ID, 1, "corrected-at-capacity", day))
	if err != nil {
		t.Fatalf("reserved correction unavailable: %v", err)
	}
	if _, err := e.DeleteCapture("case-a", r.ID, corrected.Revision); err != nil {
		t.Fatalf("reserved deletion unavailable: %v", err)
	}
	revision, err := e.VisibilityRevision("case-a")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.ApplyRetention("case-a", day.Add(time.Hour), revision); err != nil {
		t.Fatalf("capacity cleanup retry: %v", err)
	}
	if page, err := e.ListCaptures("case-a", 0, 10); err != nil || page.Total != 0 {
		t.Fatalf("removed capacity view: %#v %v", page, err)
	}
}

func TestOwnedMemoryCheckpointPublicationInterruption(t *testing.T) {
	for _, point := range []string{"before_visibility_checkpoint_publish", "after_visibility_checkpoint_publish"} {
		t.Run(point, func(t *testing.T) {
			e, r, _ := checkpointEngine(t)
			e.FaultPoint = func(at string) error {
				if at == point {
					return errors.New("simulated interruption")
				}
				return nil
			}
			if _, err := e.DeleteCapture("case-a", r.ID, 1); err == nil {
				t.Fatal("fault not reached")
			}
			e.FaultPoint = nil
			_, err := e.ReadCapture("case-a", r.ID)
			if point == "before_visibility_checkpoint_publish" && err != nil {
				t.Fatalf("prior checkpoint lost: %v", err)
			}
			if point == "after_visibility_checkpoint_publish" && !errors.Is(err, ErrCaptureUnavailable) {
				t.Fatalf("published denial lost: %v", err)
			}
			if _, err := e.DeleteCapture("case-a", r.ID, 1); err != nil {
				t.Fatalf("interrupted cleanup not recoverable: %v", err)
			}
		})
	}
	t.Run("required-before-first-checkpoint", func(t *testing.T) {
		e, r, _ := checkpointEngine(t)
		installLegacyVisibility(t, e, 3, false)
		e.FaultPoint = func(point string) error {
			if point == "after_visibility_checkpoint_required" {
				return errors.New("migration interrupted")
			}
			return nil
		}
		if _, err := e.DeleteCapture("case-a", r.ID, 1); err == nil {
			t.Fatal("migration fault not reached")
		}
		e.FaultPoint = nil
		if _, err := e.ReadCapture("case-a", r.ID); err == nil {
			t.Fatal("missing required checkpoint reopened legacy history")
		}
		if _, err := e.AssembleContext("case-a"); err == nil {
			t.Fatal("missing required checkpoint became empty state")
		}
	})
	t.Run("interrupted-history-cleanup", func(t *testing.T) {
		e, r, day := checkpointEngine(t)
		installLegacyVisibility(t, e, 3, false)
		e.FaultPoint = func(point string) error {
			if point == "during_visibility_history_cleanup" {
				return errors.New("cleanup interrupted")
			}
			return nil
		}
		if _, err := e.DeleteCapture("case-a", r.ID, 1); err == nil {
			t.Fatal("cleanup fault not reached")
		}
		e.FaultPoint = nil
		if _, err := e.ReadCapture("case-a", r.ID); !errors.Is(err, ErrCaptureUnavailable) {
			t.Fatalf("partial compaction lost denial: %v", err)
		}
		newCapture, err := e.CaptureOwned("case-a", ownedInput(t, e, "", 0, "post-interruption-survivor", day))
		if err != nil {
			t.Fatal(err)
		}
		if newCapture.VisibilityRevision != 5 {
			t.Fatalf("compaction reset monotonic revision: %#v", newCapture)
		}
		if _, err := e.DeleteCapture("case-a", r.ID, 1); err != nil {
			t.Fatal(err)
		}
		if _, err := e.ReadCapture("case-a", newCapture.ID); err != nil {
			t.Fatal(err)
		}
	})
}

func TestOwnedMemoryEncodedReplacementPreflight(t *testing.T) {
	e, r, day := checkpointEngine(t)
	for _, input := range []struct {
		text string
		at   time.Time
	}{{strings.Repeat("\x01", MaxOwnedCaptureBytes), day}, {"invalid timestamp", time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)}} {
		if _, err := e.SealOwnedCapture("case-a", r.ID, r.Revision, input.text, input.at); err == nil {
			t.Fatal("unreadable replacement passed trusted sealing")
		}
		if current, err := e.ReadCapture("case-a", r.ID); err != nil || current.Revision != r.Revision {
			t.Fatalf("invalid replacement changed prior memory: %#v %v", current, err)
		}
	}
}

func TestOwnedMemoryJournalAppendAdmissionPreservesReadability(t *testing.T) {
	e, r, day := checkpointEngine(t)
	if _, err := e.Capture(Capture{WorkspaceID: "case-a", RecordedAt: day, Kind: "legacy", Text: strings.Repeat("x", 1<<20), Sanitized: true}); !errors.Is(err, ErrCaptureCapacity) {
		t.Fatalf("oversize append outcome: %v", err)
	}
	legacy := Capture{WorkspaceID: "case-a", RecordedAt: day, Kind: "legacy", Text: "duplicate-logical-capture", Sanitized: true}
	body, _ := json.Marshal(legacy)
	path := e.currentPath("case-a", "l1", filepath.Join("captures", "2026-07-20.jsonl"))
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, bytes.Repeat(append(body, '\n'), 4095), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Capture(legacy); !errors.Is(err, ErrCaptureCapacity) {
		t.Fatalf("raw-entry append outcome: %v", err)
	}
	if _, err := e.ReadCapture("case-a", r.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.ListCaptures("case-a", 0, 10); err != nil {
		t.Fatal(err)
	}
	v, err := e.VisibilityRevision("case-a")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.ApplyRetention("case-a", day.Add(time.Hour), v); err != nil {
		t.Fatalf("reserved raw-entry cleanup: %v", err)
	}
}
