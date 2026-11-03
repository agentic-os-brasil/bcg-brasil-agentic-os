package darwin_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/darwin"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/maintenance"
)

func TestProductDarwinDelayedWeeklyRecoveryR2(t *testing.T) {
	for _, mode := range []string{"recorded_schedule", "legacy_original_artifact", "legacy_missing_artifact"} {
		t.Run(mode, func(t *testing.T) { delayedWeeklyRecoveryR2(t, mode) })
	}
}
func delayedWeeklyRecoveryR2(t *testing.T, mode string) {
	var root string
	armed := false
	existingCount := 0
	s, _, now := productServiceWith(t, func(o *maintenance.ServiceOptions) {
		o.DarwinFactory = func(scope maintenance.DarwinScope) (maintenance.DarwinBinding, error) {
			root = scope.Root
			return darwin.NewProductBinding(scope)
		}
		o.InstalledGuard = func(publish func() error) error {
			if armed {
				entries, _ := os.ReadDir(filepath.Join(root, "proposals"))
				if len(entries) > existingCount {
					return errors.New("interrupted after real delayed weekly proposal")
				}
			}
			return publish()
		}
	})
	id := maintenance.DarwinWeeklyJobID
	qualify(t, s, id)
	mutate(t, s, "enroll", id)
	originalEntries, e := os.ReadDir(filepath.Join(root, "proposals"))
	if e != nil {
		t.Fatal(e)
	}
	existingCount = len(originalEntries)
	known := map[string]bool{}
	for _, entry := range originalEntries {
		known[entry.Name()] = true
	}
	*now = now.Add(time.Second)
	qualify(t, s, "runtime-health-check") // genuine external owning observation/receipt
	*now = now.Add(8*24*time.Hour + 13*time.Hour)
	requested := *now
	armed = true
	due, e := s.RunDue(context.Background())
	if e == nil || len(due.Due) != 1 {
		t.Fatalf("actual delayed due interruption missing: %+v %v", due, e)
	}
	scheduled := due.Due[0].ScheduledFor
	if !scheduled.Before(requested) {
		t.Fatalf("fixture did not exercise unequal schedule/request: %s %s", scheduled, requested)
	}
	entries, e := os.ReadDir(filepath.Join(root, "proposals"))
	if e != nil || len(entries) != existingCount+1 {
		t.Fatalf("new delayed immutable proposal absent: %v %v", entries, e)
	}
	var path string
	for _, entry := range entries {
		if !known[entry.Name()] {
			path = filepath.Join(root, "proposals", entry.Name())
		}
	}
	original, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	var artifact darwin.AssessmentProposalArtifact
	if e = json.Unmarshal(original, &artifact); e != nil || artifact.Validate() != nil {
		t.Fatal("actual original proposal invalid", e)
	}
	if !artifact.ScheduledFor.Equal(scheduled) || !artifact.RecordedAt.Equal(scheduled) {
		t.Fatal("original recurring proposal lost actual scheduled provenance")
	}
	journalPath := filepath.Join(root, "managed-state", "recovery.json")
	journalBefore, e := os.ReadFile(journalPath)
	if e != nil {
		t.Fatal(e)
	}
	var old map[string]any
	if e = json.Unmarshal(journalBefore, &old); e != nil {
		t.Fatal(e)
	}
	if old["requested_at"] != requested.UTC().Format(time.RFC3339Nano) || old["occurrence_digest"] != artifact.OccurrenceDigest {
		t.Fatal("actual delayed journal/request/occurrence mismatch")
	}
	// The compatibility cases retain the actual interrupted occurrence and
	// proposal, removing only the field absent from the rejected R1 wire.
	if mode != "recorded_schedule" {
		delete(old, "scheduled_for")
		journalBefore, e = json.Marshal(old)
		if e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(journalPath, journalBefore, 0600); e != nil {
			t.Fatal(e)
		}
		if mode == "legacy_missing_artifact" {
			if e = os.Remove(path); e != nil {
				t.Fatal(e)
			}
		}
	}
	cachePath := filepath.Join(root, "managed-state", "current.json")
	cacheBefore, e := os.ReadFile(cachePath)
	if e != nil {
		t.Fatal(e)
	}
	armed = false
	*now = now.Add(2 * time.Second)
	if mode == "legacy_missing_artifact" {
		r, e := s.Review(maintenance.Operation{Kind: "qualify", JobID: id, ExpectedRevision: revision(t, s, id)})
		if e != nil {
			t.Fatal(e)
		}
		if _, e = s.Apply(context.Background(), r, func(context.Context, maintenance.ProductReview) error { return nil }); !errors.Is(e, darwin.ErrProductRecovery) {
			t.Fatalf("missing legacy schedule authority did not refuse: %v", e)
		}
		unchanged, e := os.ReadFile(journalPath)
		if e != nil || !bytes.Equal(unchanged, journalBefore) {
			t.Fatal("refusal dropped/rewrote unresolved original journal")
		}
		return
	}
	recovered := qualify(t, s, id)
	if recovered.Qualification == nil {
		t.Fatal("later reviewed delayed recovery lacks qualification")
	}
	report, e := s.Report(id)
	if e != nil || report.Finding != "current" || report.Darwin.RecoveryState != "" {
		t.Fatalf("delayed recovery not normally current: %+v %v", report, e)
	}
	after, e := os.ReadFile(path)
	if e != nil || !bytes.Equal(after, original) {
		t.Fatal("delayed original proposal rewritten")
	}
	entries, e = os.ReadDir(filepath.Join(root, "proposals"))
	if e != nil || len(entries) != existingCount+1 {
		t.Fatal("delayed recovery duplicated proposal")
	}
	cacheAfter, e := os.ReadFile(cachePath)
	if e != nil || !bytes.Equal(cacheBefore, cacheAfter) {
		t.Fatal("recovery changed actual applied cache")
	}
	journalAfter, e := os.ReadFile(journalPath)
	if e != nil {
		t.Fatal(e)
	}
	var closed map[string]any
	if e = json.Unmarshal(journalAfter, &closed); e != nil {
		t.Fatal(e)
	}
	for _, key := range []string{"job_id", "occurrence_digest", "requested_at", "before", "after", "external_sha256"} {
		if old[key] != closed[key] {
			t.Fatalf("recovery lost original %s", key)
		}
	}
	if mode == "recorded_schedule" && closed["scheduled_for"] != scheduled.UTC().Format(time.RFC3339Nano) {
		t.Fatal("recovery journal did not retain original scheduled time separately")
	}
	if mode == "legacy_original_artifact" {
		if _, invented := closed["scheduled_for"]; invented {
			t.Fatal("legacy recovery invented missing journal schedule instead of verifying original artifact")
		}
	}
	if closed["completed_occurrence_digest"] != report.Darwin.Evidence.OccurrenceDigest || closed["completed_artifact_sha256"] != report.Darwin.Evidence.ArtifactSHA256 {
		t.Fatal("successor closure not bound to actual readback")
	}
	lineage := report.Darwin.Evidence.Recovery
	if lineage == nil || lineage.OriginalOccurrenceDigest != artifact.OccurrenceDigest {
		t.Fatal("original recurring occurrence lineage lost")
	}
	*now = now.Add(3 * time.Second)
	replay := qualify(t, s, id)
	if replay.Qualification == nil || replay.Qualification.ID != recovered.Qualification.ID || replay.Qualification.Artifact != recovered.Qualification.Artifact {
		t.Fatal("later unchanged replay minted qualification/artifact")
	}
	entries, e = os.ReadDir(filepath.Join(root, "proposals"))
	if e != nil || len(entries) != existingCount+1 {
		t.Fatal("later unchanged replay duplicated original proposal")
	}
}
