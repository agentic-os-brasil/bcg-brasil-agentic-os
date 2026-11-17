package memory

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/lifecycle"
)

func TestObservedCaptureExactlyOnceAndDeletion(t *testing.T) {
	e := testEngine(t)
	at := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	attestor := CaptureAttestor{Root: e.Root}
	signal := lifecycle.Signal{SchemaVersion: 1, WorkspaceID: strings.Repeat("c", 32), Producer: lifecycle.LifecycleProducer, Version: lifecycle.SignalVersion, Runtime: "claude", Event: lifecycle.StopFinalize, EvidenceClass: lifecycle.ObservedOnly, SourceID: strings.Repeat("a", 32), SourceRevision: 1, SourceDigest: strings.Repeat("b", 64), OccurredAt: at}
	if _, err := attestor.SealSignal(signal); err == nil {
		t.Fatal("runtime producer created authority")
	}
	if err := e.PrepareOwnedCaptureAuthority(strings.Repeat("c", 32)); err != nil {
		t.Fatal(err)
	}
	if err := attestor.Prepare(strings.Repeat("c", 32)); err != nil {
		t.Fatal(err)
	}
	signal, err := attestor.SealSignal(signal)
	if err != nil {
		t.Fatal(err)
	}
	first, err := e.CaptureSignal(signal)
	if err != nil || !first.Added {
		t.Fatalf("first admission: %#v %v", first, err)
	}
	replay, err := e.CaptureSignal(signal)
	if err != nil || replay.Added || replay.Capture.ID != first.Capture.ID || replay.Capture.VisibilityRevision != first.Capture.VisibilityRevision {
		t.Fatalf("replay: %#v %v", replay, err)
	}
	e.Synthesizer = DeterministicL1Synthesizer{MaxRunes: 4096, MaxEntries: 64, MaxInputBytes: 1 << 20, MaxInputEntries: 256, Attestor: attestor}
	if _, err := e.DreamDailyAttested(context.Background(), strings.Repeat("c", 32), at); err != nil {
		t.Fatal(err)
	}
	if _, err := e.DeleteCapture(strings.Repeat("c", 32), first.Capture.ID, 1); err != nil {
		t.Fatal(err)
	}
	visibility, err := e.VisibilityRevision(strings.Repeat("c", 32))
	if err != nil {
		t.Fatal(err)
	}
	replay, err = e.CaptureSignal(signal)
	if err != nil || replay.Added || !replay.Suppressed {
		t.Fatalf("deleted replay: %#v %v", replay, err)
	}
	if after, err := e.VisibilityRevision(strings.Repeat("c", 32)); err != nil || after != visibility {
		t.Fatalf("replay changed visibility: %d %v", after, err)
	}
	if page, err := e.ListCaptures(strings.Repeat("c", 32), 0, 10); err != nil || page.Total != 0 {
		t.Fatalf("resurrected: %#v %v", page, err)
	}
}

func signalFixture(t *testing.T) (*Engine, CaptureAttestor, lifecycle.Signal) {
	t.Helper()
	e := testEngine(t)
	wp := strings.Repeat("c", 32)
	if err := e.PrepareOwnedCaptureAuthority(wp); err != nil {
		t.Fatal(err)
	}
	a := CaptureAttestor{Root: e.Root}
	if err := a.Prepare(wp); err != nil {
		t.Fatal(err)
	}
	s, err := a.SealSignal(lifecycle.Signal{SchemaVersion: 1, WorkspaceID: wp, Producer: lifecycle.LifecycleProducer, Version: lifecycle.SignalVersion, Runtime: "claude", Event: lifecycle.StopFinalize, EvidenceClass: lifecycle.ObservedOnly, SourceID: strings.Repeat("a", 32), SourceRevision: 1, SourceDigest: strings.Repeat("b", 64), OccurredAt: e.Now()})
	if err != nil {
		t.Fatal(err)
	}
	return e, a, s
}

func TestObservedCaptureDenialsAndConcurrentReplay(t *testing.T) {
	e, a, signal := signalFixture(t)
	for _, mutate := range []func(*lifecycle.Signal){
		func(s *lifecycle.Signal) { s.WorkspaceID = strings.Repeat("d", 32) },
		func(s *lifecycle.Signal) { s.Version = "untrusted-version" },
		func(s *lifecycle.Signal) { s.EvidenceClass = "native_observed" },
		func(s *lifecycle.Signal) { s.SourceDigest = strings.Repeat("e", 64) },
	} {
		bad := signal
		mutate(&bad)
		if _, err := e.CaptureSignal(bad); err == nil {
			t.Fatal("forged signal admitted")
		}
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := e.CaptureSignal(signal); err != nil && !errors.Is(err, ErrDreamInProgress) {
				t.Errorf("concurrent admission: %v", err)
			}
		}()
	}
	wg.Wait()
	first, err := e.CaptureSignal(signal)
	if err != nil {
		t.Fatal(err)
	}
	if page, err := e.ListCaptures(signal.WorkspaceID, 0, 10); err != nil || page.Total != 1 || page.VisibilityRevision != 1 {
		t.Fatalf("concurrent duplicate %#v %v", page, err)
	}
	changed := signal
	changed.SourceDigest = strings.Repeat("f", 64)
	changed, err = a.SealSignal(changed)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.CaptureSignal(changed); err == nil {
		t.Fatal("changed occurrence replaced original")
	}
	input, err := e.SealOwnedCapture(signal.WorkspaceID, first.Capture.ID, 1, "owner corrected metadata", e.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.CorrectCapture(signal.WorkspaceID, first.Capture.ID, 1, input); err != nil {
		t.Fatal(err)
	}
	before, err := e.VisibilityRevision(signal.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if replay, err := e.CaptureSignal(signal); err != nil || !replay.Suppressed || replay.Added {
		t.Fatalf("corrected replay %#v %v", replay, err)
	}
	if after, err := e.VisibilityRevision(signal.WorkspaceID); err != nil || after != before {
		t.Fatal("replay advanced corrected visibility")
	}
}

func TestObservedCaptureMidnightAdmissionRecovery(t *testing.T) {
	e, a, s := signalFixture(t)
	beforeMidnight := time.Date(2026, 7, 20, 23, 59, 59, 0, time.UTC)
	now := beforeMidnight
	e.Now = func() time.Time { return now }
	s.OccurredAt = beforeMidnight
	s, err := a.SealSignal(s)
	if err != nil {
		t.Fatal(err)
	}
	e.FaultPoint = func(point string) error {
		if point == "signal_admitted" {
			return errors.New("synthetic_interruption")
		}
		return nil
	}
	if _, err = e.CaptureSignal(s); err == nil {
		t.Fatal("interruption did not occur")
	}
	now = beforeMidnight.Add(2 * time.Second)
	e.FaultPoint = nil
	result, err := e.CaptureSignal(s)
	if err != nil || !result.Added {
		t.Fatalf("recovery %#v %v", result, err)
	}
	record, err := e.ReadCapture(s.WorkspaceID, result.Capture.ID)
	if err != nil || !record.RecordedAt.Equal(now) || !strings.Contains(record.Text, "2026-07-20T23:59:59Z") {
		t.Fatalf("actual ingestion/source times %#v %v", record, err)
	}
	e.Synthesizer = DeterministicL1Synthesizer{MaxRunes: 4096, MaxEntries: 64, MaxInputBytes: 1 << 20, MaxInputEntries: 256, Attestor: a}
	if _, err = e.DreamDailyAttested(context.Background(), s.WorkspaceID, now); err != nil {
		t.Fatal(err)
	}
	if bundle, err := e.AssembleContext(s.WorkspaceID); err != nil || len(bundle.Sections) != 1 || !strings.Contains(bundle.Sections[0].Content, "observed_only") {
		t.Fatalf("recovered capture stranded %#v %v", bundle, err)
	}
	now = now.Add(24 * time.Hour)
	replay, err := e.CaptureSignal(s)
	if err != nil || replay.Added || replay.Capture.ID != result.Capture.ID {
		t.Fatalf("later replay %#v %v", replay, err)
	}
}
