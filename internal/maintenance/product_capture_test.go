package maintenance

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/execution"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/lifecycle"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/memory"
)

func emitProductSignal(t *testing.T, s *Service, id string) {
	t.Helper()
	a := memory.CaptureAttestor{Root: s.engine.Root}
	if err := s.engine.PrepareOwnedCaptureAuthority(s.workspaceID); err != nil {
		t.Fatal(err)
	}
	if err := a.Prepare(s.workspaceID); err != nil {
		t.Fatal(err)
	}
	r, err := lifecycle.RecordConfined(filepath.Join(s.home, "state"), s.workspaceID, lifecycle.Receipt{SchemaVersion: 1, Runtime: "claude", Event: lifecycle.StopFinalize, State: "observed", Provenance: lifecycle.AdapterCommand, OccurredAt: s.now(), IdempotencyKey: id})
	if err != nil {
		t.Fatal(err)
	}
	signal, err := lifecycle.SignalFromReceipt(s.workspaceID, r)
	if err != nil {
		t.Fatal(err)
	}
	signal, err = a.SealSignal(signal)
	if err != nil {
		t.Fatal(err)
	}
	if err = lifecycle.RecordSignalConfined(filepath.Join(s.home, "state"), signal); err != nil {
		t.Fatal(err)
	}
}

func TestProductCaptureJobFromRealSources(t *testing.T) {
	s := productFixture(t)
	if err := s.engine.PrepareOwnedCaptureAuthority(s.workspaceID); err != nil {
		t.Fatal(err)
	}
	a := memory.CaptureAttestor{Root: s.engine.Root}
	if err := a.Prepare(s.workspaceID); err != nil {
		t.Fatal(err)
	}
	r, err := lifecycle.RecordConfined(filepath.Join(s.home, "state"), s.workspaceID, lifecycle.Receipt{SchemaVersion: 1, Runtime: "claude", Event: lifecycle.StopFinalize, State: "observed", Provenance: lifecycle.AdapterCommand, OccurredAt: s.now(), IdempotencyKey: strings.Repeat("a", 32)})
	if err != nil {
		t.Fatal(err)
	}
	signal, err := lifecycle.SignalFromReceipt(s.workspaceID, r)
	if err != nil {
		t.Fatal(err)
	}
	signal, err = a.SealSignal(signal)
	if err != nil {
		t.Fatal(err)
	}
	if err = lifecycle.RecordSignalConfined(filepath.Join(s.home, "state"), signal); err != nil {
		t.Fatal(err)
	}
	store, err := execution.OpenConfinedStore(filepath.Join(s.home, "state"))
	if err != nil {
		t.Fatal(err)
	}
	store.Now = s.now
	item, err := store.Create(execution.CreateInput{WorkspaceID: s.workspaceID, Objective: "PRIVATE_OBJECTIVE_NEVER_CAPTURE", InitialNextStep: "PRIVATE_NEXT_STEP", Criteria: []execution.Criterion{{ID: "artifact_1", Type: execution.CriterionArtifactSnapshot, TargetRef: "bcgos://workspace/result.md"}}, AllowedRefs: []string{"bcgos://workspace/result.md"}})
	if err != nil {
		t.Fatal(err)
	}
	item, err = store.Start(s.workspaceID, item.State.ItemID, item.State.StateRevision)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(s.options.WorkspacePath, "result.md"), []byte("PRIVATE_DOCUMENT_BODY"), 0600); err != nil {
		t.Fatal(err)
	}
	evidence, err := store.CollectEvidence(s.workspaceID, item.State.ItemID, execution.EvidenceInput{WorkspaceRoot: s.options.WorkspacePath, ExpectedRevision: item.State.StateRevision, AttemptID: item.Attempt.AttemptID, CriterionID: "artifact_1"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.Complete(s.workspaceID, item.State.ItemID, execution.CompletionInput{WorkspaceRoot: s.options.WorkspacePath, ExpectedRevision: evidence.Item.State.StateRevision, AttemptID: item.Attempt.AttemptID})
	if err != nil {
		t.Fatal(err)
	}
	productApply(t, s, "qualify", "memory-l1-capture", "")
	page, err := s.engine.ListCaptures(s.workspaceID, 0, 10)
	if err != nil || page.Total != 2 {
		t.Fatalf("captures %#v %v", page, err)
	}
	classes := map[string]bool{}
	for _, c := range page.Captures {
		record, err := s.engine.ReadCapture(s.workspaceID, c.ID)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(record.Text, "PRIVATE_") {
			t.Fatal("private source body entered capture")
		}
		classes[strings.Split(record.Text, ":")[0]] = true
	}
	if !classes[lifecycle.ObservedOnly] || !classes[lifecycle.CommittedEffect] {
		t.Fatalf("untruthful classes %#v", classes)
	}
	productApply(t, s, "qualify", MemoryLightDreamJobID, "")
	bundle, err := s.engine.AssembleContext(s.workspaceID)
	if err != nil || len(bundle.Sections) == 0 {
		t.Fatalf("context %#v %v", bundle, err)
	}
}

func TestProductCapturePauseRevokeAndReplay(t *testing.T) {
	s := productFixture(t)
	now := s.now()
	s.now = func() time.Time { return now }
	s.engine.Now = s.now
	emitProductSignal(t, s, strings.Repeat("a", 32))
	productApply(t, s, "qualify", MemoryCaptureJobID, "")
	productApply(t, s, "enroll", MemoryCaptureJobID, "UTC")
	productApply(t, s, "pause", MemoryCaptureJobID, "")
	now = now.Add(25 * time.Hour)
	emitProductSignal(t, s, strings.Repeat("b", 32))
	if _, err := s.RunDue(context.Background()); err != nil {
		t.Fatal(err)
	}
	if page, err := s.engine.ListCaptures(s.workspaceID, 0, 10); err != nil || page.Total != 1 {
		t.Fatalf("paused capture %#v %v", page, err)
	}
	productApply(t, s, "resume", MemoryCaptureJobID, "")
	if _, err := s.RunDue(context.Background()); err != nil {
		t.Fatal(err)
	}
	page, err := s.engine.ListCaptures(s.workspaceID, 0, 10)
	if err != nil || page.Total != 2 {
		t.Fatalf("resumed capture %#v %v", page, err)
	}
	if _, err = s.engine.DeleteCapture(s.workspaceID, page.Captures[0].ID, 1); err != nil {
		t.Fatal(err)
	}
	now = now.Add(25 * time.Hour)
	before, err := s.engine.VisibilityRevision(s.workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.RunDue(context.Background()); err != nil {
		t.Fatal(err)
	}
	if after, err := s.engine.VisibilityRevision(s.workspaceID); err != nil || after != before {
		t.Fatal("replay revived deleted capture or advanced visibility")
	}
	productApply(t, s, "revoke", MemoryCaptureJobID, "")
	now = now.Add(25 * time.Hour)
	emitProductSignal(t, s, strings.Repeat("c", 32))
	if _, err = s.RunDue(context.Background()); err != nil {
		t.Fatal(err)
	}
	if page, err := s.engine.ListCaptures(s.workspaceID, 0, 10); err != nil || page.Total != 1 {
		t.Fatalf("revoked capture %#v %v", page, err)
	}
}

func TestProductCaptureRejectsChangedAndForgedSources(t *testing.T) {
	for _, field := range []string{"attestation", "version", "workspace_id"} {
		t.Run(field, func(t *testing.T) {
			s := productFixture(t)
			emitProductSignal(t, s, strings.Repeat("a", 32))
			dir := filepath.Join(s.home, "state", "runtime", "capture-signals", s.workspaceID)
			entries, err := os.ReadDir(dir)
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range entries {
				if filepath.Ext(entry.Name()) != ".json" {
					continue
				}
				path := filepath.Join(dir, entry.Name())
				body, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				var value map[string]any
				if err = json.Unmarshal(body, &value); err != nil {
					t.Fatal(err)
				}
				value[field] = "forged"
				body, _ = json.Marshal(value)
				if err = os.WriteFile(path, body, 0600); err != nil {
					t.Fatal(err)
				}
			}
			before := productSnapshot(t, s.home)
			if _, err = s.Review(Operation{Kind: "qualify", JobID: MemoryCaptureJobID}); err == nil {
				t.Fatal("forged source admitted")
			}
			if after := productSnapshot(t, s.home); !reflect.DeepEqual(before, after) {
				t.Fatal("forged source changed state")
			}
		})
	}
	s := productFixture(t)
	emitProductSignal(t, s, strings.Repeat("a", 32))
	review, err := s.Review(Operation{Kind: "qualify", JobID: MemoryCaptureJobID})
	if err != nil {
		t.Fatal(err)
	}
	emitProductSignal(t, s, strings.Repeat("b", 32))
	before := productSnapshot(t, s.home)
	if _, err = s.Apply(context.Background(), review, approveProduct); err == nil {
		t.Fatal("stale source review applied")
	}
	if after := productSnapshot(t, s.home); !reflect.DeepEqual(before, after) {
		t.Fatal("stale source review changed state")
	}
}

// A capture can commit immediately before UTC midnight while its worker loses
// the acknowledgement. A later run must use its real ingestion day, however
// long the client was closed, rather than recapture it or silently omit it.
func TestProductCaptureCommittedBeforeMidnightReachesL1AfterLongDowntime(t *testing.T) {
	s := productFixture(t)
	now := time.Date(2026, 7, 20, 23, 59, 58, 0, time.UTC)
	s.now = func() time.Time { return now }
	s.engine.Now = s.now
	emitProductSignal(t, s, strings.Repeat("a", 32))
	productApply(t, s, "qualify", MemoryCaptureJobID, "")
	productApply(t, s, "qualify", MemoryLightDreamJobID, "")
	productApply(t, s, "enroll", MemoryLightDreamJobID, "UTC")
	now = now.Add(time.Second)
	emitProductSignal(t, s, strings.Repeat("b", 32))
	s.engine.FaultPoint = func(point string) error {
		if point == "signal_published" {
			return errors.New("synthetic_committed_capture_interruption")
		}
		return nil
	}
	c, err := s.loadControl(MemoryCaptureJobID)
	if err != nil {
		t.Fatal(err)
	}
	r, err := s.Review(Operation{Kind: "qualify", JobID: MemoryCaptureJobID, ExpectedRevision: c.Revision})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Apply(context.Background(), r, approveProduct); err == nil {
		t.Fatal("expected interruption after canonical capture publication")
	}
	s.engine.FaultPoint = nil
	page, err := s.engine.ListCaptures(s.workspaceID, 0, 10)
	if err != nil || page.Total != 2 {
		t.Fatalf("committed captures %#v %v", page, err)
	}
	now = now.Add(8 * 24 * time.Hour)
	emitProductSignal(t, s, strings.Repeat("c", 32))
	productApply(t, s, "qualify", MemoryCaptureJobID, "")
	visibility, err := s.engine.VisibilityRevision(s.workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	// Both the historical day and today are pending. Qualification must pin
	// the historical input selected before publication, never the next day.
	status := productApply(t, s, "qualify", MemoryLightDreamJobID, "")
	if status.Qualification == nil {
		t.Fatal("historical period qualification missing")
	}
	body, err := os.ReadFile(filepath.Join(s.engine.Root, strings.TrimPrefix(status.Qualification.Artifact, "memory/")))
	var qualified memory.Artifact
	if err != nil || json.Unmarshal(body, &qualified) != nil || qualified.Period != "2026-07-20" {
		t.Fatal("qualification pointed at another period")
	}
	artifact, _, err := s.engine.ActiveArtifact(s.workspaceID, "L1/2026-07-20")
	if err != nil || strings.Count(artifact.Content, lifecycle.ObservedOnly) != 2 {
		t.Fatalf("historical input stranded %#v %v", artifact, err)
	}
	for _, signal := range mustProductCaptureSources(t, s) {
		if replay, err := s.engine.CaptureSignal(signal); err != nil || replay.Added {
			t.Fatalf("recovery duplicated %#v %v", replay, err)
		}
	}
	if after, err := s.engine.VisibilityRevision(s.workspaceID); err != nil || after != visibility {
		t.Fatal("replay advanced visibility")
	}
	currentKey := "L1/" + now.UTC().Format("2006-01-02")
	if _, _, err := s.engine.ActiveArtifact(s.workspaceID, currentKey); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("more than one pending day was published")
	}
	now = now.Add(4 * time.Hour)
	if _, err = s.RunDue(context.Background()); err != nil {
		t.Fatal(err)
	}
	if current, _, err := s.engine.ActiveArtifact(s.workspaceID, currentKey); err != nil || strings.Count(current.Content, lifecycle.ObservedOnly) != 1 {
		t.Fatalf("next pending day %#v %v", current, err)
	}
}

func mustProductCaptureSources(t *testing.T, s *Service) []lifecycle.Signal {
	t.Helper()
	sources, err := s.captureSources()
	if err != nil {
		t.Fatal(err)
	}
	return sources
}

func TestProductCaptureDeletionRequiresFreshLightQualification(t *testing.T) {
	s := productFixture(t)
	now := s.now()
	s.now = func() time.Time { return now }
	s.engine.Now = s.now
	emitProductSignal(t, s, strings.Repeat("a", 32))
	productApply(t, s, "qualify", MemoryCaptureJobID, "")
	q := productApply(t, s, "qualify", MemoryLightDreamJobID, "").Qualification
	if q == nil || q.DailyInput == nil {
		t.Fatal("selected daily input witness missing")
	}
	productApply(t, s, "enroll", MemoryLightDreamJobID, "UTC")
	for _, field := range []string{"period", "fingerprint", "date_shape", "hash_shape", "id"} {
		t.Run(field, func(t *testing.T) {
			bad := *q
			selected := *q.DailyInput
			bad.DailyInput = &selected
			switch field {
			case "period":
				selected.Period = "2025-01-01"
			case "fingerprint":
				selected.Fingerprint = strings.Repeat("0", 64)
			case "date_shape":
				selected.Period = "2026-02-30"
			case "hash_shape":
				selected.Fingerprint = strings.Repeat("z", 64)
			}
			bad.ID = ""
			bad.ID = productDigest(bad)
			if field == "id" {
				bad.ID = strings.Repeat("0", 64)
			}
			if err := s.verifyQualification(&bad); err == nil {
				t.Fatal("tampered daily qualification accepted")
			}
		})
	}
	page, err := s.engine.ListCaptures(s.workspaceID, 0, 10)
	if err != nil || page.Total != 1 {
		t.Fatalf("captures %#v %v", page, err)
	}
	if _, err = s.engine.DeleteCapture(s.workspaceID, page.Captures[0].ID, 1); err != nil {
		t.Fatal(err)
	}
	if err = s.verifyQualification(q); !errors.Is(err, ErrQualificationRequired) {
		t.Fatalf("purged evidence remained qualified: %v", err)
	}
	if _, err = s.RunDue(context.Background()); !errors.Is(err, ErrQualificationRequired) {
		t.Fatalf("purged qualification ran: %v", err)
	}
	before, err := s.engine.VisibilityRevision(s.workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	for _, signal := range mustProductCaptureSources(t, s) {
		if replay, err := s.engine.CaptureSignal(signal); err != nil || !replay.Suppressed || replay.Added {
			t.Fatalf("deleted-source replay %#v %v", replay, err)
		}
	}
	if after, err := s.engine.VisibilityRevision(s.workspaceID); err != nil || after != before {
		t.Fatal("historical qualification advanced deleted visibility")
	}
	if bundle, err := s.engine.AssembleContext(s.workspaceID); err != nil || len(bundle.Sections) != 0 {
		t.Fatalf("deleted context resurrected %#v %v", bundle, err)
	}
	// New eligible input alone cannot restore a qualification whose actual
	// artifact was purged. A fresh reviewed owner approval is required.
	now = now.Add(time.Second)
	emitProductSignal(t, s, strings.Repeat("b", 32))
	productApply(t, s, "qualify", MemoryCaptureJobID, "")
	if _, err = s.RunDue(context.Background()); !errors.Is(err, ErrQualificationRequired) {
		t.Fatalf("new input silently requalified L1: %v", err)
	}
	c, err := s.loadControl(MemoryLightDreamJobID)
	if err != nil {
		t.Fatal(err)
	}
	review, err := s.Review(Operation{Kind: "qualify", JobID: MemoryLightDreamJobID, ExpectedRevision: c.Revision})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := productSnapshot(t, s.home)
	if _, err = s.Apply(context.Background(), review, func(context.Context, ProductReview) error { return errors.New("declined") }); err == nil {
		t.Fatal("declined requalification accepted")
	}
	if !reflect.DeepEqual(snapshot, productSnapshot(t, s.home)) {
		t.Fatal("declined requalification changed state")
	}
	status, err := s.Apply(context.Background(), review, approveProduct)
	if err != nil || status.Qualification == nil || status.Qualification.ID == q.ID {
		t.Fatalf("fresh qualification %#v %v", status, err)
	}
	bundle, err := s.engine.AssembleContext(s.workspaceID)
	body, _ := json.Marshal(bundle)
	if err != nil || !strings.Contains(string(body), strings.Repeat("b", 32)) || strings.Contains(string(body), strings.Repeat("a", 32)) {
		t.Fatalf("fresh context retained denied input %s %v", body, err)
	}
}
