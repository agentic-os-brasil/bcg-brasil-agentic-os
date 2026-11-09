package darwin_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/darwin"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/maintenance"
)

func TestProductDarwinLaterClockRecoveryR1(t *testing.T) {
	for _, phase := range []string{"cache", "proposal", "monthly-proposal"} {
		t.Run(phase, func(t *testing.T) {
			var root string
			fail := true
			id := maintenance.DarwinDailyJobID
			if strings.Contains(phase, "proposal") {
				id = maintenance.DarwinWeeklyJobID
			}
			if phase == "monthly-proposal" {
				id = maintenance.DarwinMonthlyJobID
			}
			s, _, now := productServiceWith(t, func(o *maintenance.ServiceOptions) {
				o.DarwinFactory = func(scope maintenance.DarwinScope) (maintenance.DarwinBinding, error) {
					root = scope.Root
					return darwin.NewProductBinding(scope)
				}
				o.InstalledGuard = func(publish func() error) error {
					if fail {
						if phase == "cache" {
							if _, e := os.Stat(filepath.Join(root, "managed-state", "current.json")); e == nil {
								return errors.New("interrupted after applied cache")
							}
						}
						if strings.Contains(phase, "proposal") {
							entries, _ := os.ReadDir(filepath.Join(root, "proposals"))
							if len(entries) > 0 {
								return errors.New("interrupted after immutable proposal")
							}
						}
					}
					return publish()
				}
			})
			review, e := s.Review(maintenance.Operation{Kind: "qualify", JobID: id})
			if e != nil {
				t.Fatal(e)
			}
			if _, e = s.Apply(context.Background(), review, func(context.Context, maintenance.ProductReview) error { return nil }); e == nil {
				t.Fatal("interruption falsely succeeded")
			}
			cache := filepath.Join(root, "managed-state", "current.json")
			before, e := os.ReadFile(cache)
			if e != nil && !errors.Is(e, os.ErrNotExist) {
				t.Fatal(e)
			}
			originalJournal, e := os.ReadFile(filepath.Join(root, "managed-state", "recovery.json"))
			if e != nil {
				t.Fatal(e)
			}
			var original map[string]any
			if e = json.Unmarshal(originalJournal, &original); e != nil {
				t.Fatal(e)
			}
			var proposalBefore []byte
			var proposalName string
			if strings.Contains(phase, "proposal") {
				entries, e := os.ReadDir(filepath.Join(root, "proposals"))
				if e != nil || len(entries) != 1 {
					t.Fatal("original proposal missing")
				}
				proposalName = entries[0].Name()
				proposalBefore, e = os.ReadFile(filepath.Join(root, "proposals", proposalName))
				if e != nil {
					t.Fatal(e)
				}
			}
			fail = false
			*now = now.Add(2 * time.Second)
			recovered := qualify(t, s, id)
			if recovered.Qualification == nil {
				t.Fatal("later recovery missing real qualification")
			}
			report, e := s.Report(id)
			if e != nil {
				t.Fatal(e)
			}
			expected := "current"
			if phase == "monthly-proposal" {
				expected = "absent"
			}
			if report.Finding != expected || report.Darwin.RecoveryState != "" {
				t.Errorf("later-clock recovery left false pending state: %+v", report.Darwin)
			}
			after, e := os.ReadFile(cache)
			if e != nil && !errors.Is(e, os.ErrNotExist) || !bytes.Equal(before, after) {
				t.Fatal("recovery changed actual applied cache bytes")
			}
			journal, e := os.ReadFile(filepath.Join(root, "managed-state", "recovery.json"))
			if e != nil {
				t.Fatal(e)
			}
			var retained map[string]any
			if e = json.Unmarshal(journal, &retained); e != nil {
				t.Fatal(e)
			}
			for _, key := range []string{"job_id", "occurrence_digest", "requested_at", "before", "after", "external_sha256"} {
				if retained[key] != original[key] {
					t.Errorf("original transition lost %s", key)
				}
			}
			if retained["completed_occurrence_digest"] != report.Darwin.Evidence.OccurrenceDigest || retained["completed_artifact_sha256"] != report.Darwin.Evidence.ArtifactSHA256 {
				t.Fatal("journal did not bind exact finalized successor evidence")
			}
			lineage := report.Darwin.Evidence.Recovery
			if lineage == nil || lineage.JobID != id || lineage.OriginalOccurrenceDigest != original["occurrence_digest"] || len(lineage.TransitionSHA256) != 64 {
				t.Fatal("immutable successor lost exact original transition lineage")
			}
			*now = now.Add(3 * time.Second)
			replay := qualify(t, s, id)
			if replay.Qualification == nil || replay.Qualification.ID != recovered.Qualification.ID || replay.Qualification.Artifact != recovered.Qualification.Artifact {
				t.Errorf("later unchanged replay minted qualification/artifact")
			}
			if strings.Contains(phase, "proposal") {
				entries, e := os.ReadDir(filepath.Join(root, "proposals"))
				if e != nil || len(entries) != 1 {
					t.Errorf("later recovery/replay duplicated immutable proposal: %d", len(entries))
				}
				body, e := os.ReadFile(filepath.Join(root, "proposals", proposalName))
				if e != nil || !bytes.Equal(body, proposalBefore) {
					t.Fatal("original immutable proposal rewritten")
				}
			}
		})
	}
}

func TestProductDarwinEditedReviewProjectionR1(t *testing.T) {
	s, _, _ := productService(t)
	review, e := s.Review(maintenance.Operation{Kind: "qualify", JobID: maintenance.DarwinDailyJobID})
	if e != nil {
		t.Fatal(e)
	}
	if review.DarwinReview == nil || review.DarwinReview.Effect != "initialize" || review.DarwinReview.Parent.Present {
		t.Fatal("canonical absent projection missing")
	}
	review.DarwinReview.Effect = "assess"
	if _, e = s.Apply(context.Background(), review, func(context.Context, maintenance.ProductReview) error { return nil }); !errors.Is(e, maintenance.ErrProductConflict) {
		t.Fatalf("edited sealed review accepted: %v", e)
	}
	if revision(t, s, maintenance.DarwinDailyJobID) != 0 {
		t.Fatal("edited projection wrote control")
	}
}
