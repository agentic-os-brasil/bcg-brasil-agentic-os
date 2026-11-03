package memory

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestPrivateWikiMaintenanceInspectionPreservesAuthorityAndReads(t *testing.T) {
	e, _, day := privateWikiEngine(t)
	before := privateWikiTree(t, e.Root)
	initial, err := e.InspectPrivateWikiMaintenance("case-a")
	if err != nil || !initial.SourceReady || initial.State != "absent" || initial.EventID == "" || initial.EventAt.IsZero() || !initial.EventPending || !initial.RetirementComplete {
		t.Fatalf("real source/event not observable: %+v %v", initial, err)
	}
	if !reflect.DeepEqual(before, privateWikiTree(t, e.Root)) {
		t.Fatal("inspection initialized or repaired authority")
	}
	compiled, err := e.CompilePrivateWiki(context.Background(), "case-a", "")
	if err != nil {
		t.Fatal(err)
	}
	current, err := e.InspectPrivateWikiMaintenance("case-a")
	if err != nil || current.State != "current" || !current.ImplementationCurrent || !current.ConfigurationCurrent || current.EventPending || current.Generation != compiled.Generation || current.GraphEvidence == "" || current.Concepts < 1 {
		t.Fatalf("actual graph evidence absent: %+v %v", current, err)
	}
	if _, err := e.CaptureOwned("case-a", ownedInput(t, e, "", 0, "uncommitted maintenance capture", day.Add(time.Minute))); err != nil {
		t.Fatal(err)
	}
	afterCapture, err := e.InspectPrivateWikiMaintenance("case-a")
	if err != nil || afterCapture.SourceIdentity != current.SourceIdentity || afterCapture.GraphEvidence != current.GraphEvidence || afterCapture.EventPending {
		t.Fatal("raw capture revision became a source event")
	}
	if _, err := e.DreamDailyAttested(context.Background(), "case-a", day); err != nil {
		t.Fatal(err)
	}
	stale, err := e.InspectPrivateWikiMaintenance("case-a")
	if err != nil || stale.State != "stale" || stale.SourceIdentity == current.SourceIdentity || !stale.EventPending || stale.EventID == current.EventID || stale.Parent != current.Generation || stale.GraphEvidence != "" {
		t.Fatalf("changed committed source was not pinned: %+v %v", stale, err)
	}
}

func TestPrivateWikiMaintenanceMissingEventAndPartialMarkerAreReadOnly(t *testing.T) {
	e, _, _ := privateWikiEngine(t)
	snapshot, err := e.privateWikiSnapshot("case-a")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(e.privateWikiEventPath("case-a", snapshot.commitName)); err != nil {
		t.Fatal(err)
	}
	before := privateWikiTree(t, e.Root)
	missing, err := e.InspectPrivateWikiMaintenance("case-a")
	if err != nil || !missing.SourceReady || !missing.EventMissing || missing.EventID != "" || missing.EventPending {
		t.Fatalf("missing event was invented: %+v %v", missing, err)
	}
	if !reflect.DeepEqual(before, privateWikiTree(t, e.Root)) {
		t.Fatal("event inspection wrote recovery state")
	}
	if _, err := e.CompilePrivateWiki(context.Background(), "case-a", ""); err != nil {
		t.Fatal(err)
	}
	recovered, err := e.InspectPrivateWikiMaintenance("case-a")
	if err != nil || !recovered.EventRecovered || recovered.EventMissing {
		t.Fatalf("owning reconciliation recovery absent: %+v %v", recovered, err)
	}
	if err := os.Remove(filepath.Join(e.workspaceRoot("case-a"), ".visibility-checkpoint-required")); err != nil {
		t.Fatal(err)
	}
	before = privateWikiTree(t, e.Root)
	if _, err := e.InspectPrivateWikiMaintenance("case-a"); !errors.Is(err, ErrPrivateWikiCohort) {
		t.Fatalf("partial protected state admitted: %v", err)
	}
	if !reflect.DeepEqual(before, privateWikiTree(t, e.Root)) {
		t.Fatal("inspection repaired a protected marker")
	}
}

func TestPrivateWikiMaintenancePendingRetirementCannotGrantAView(t *testing.T) {
	e, capture, _ := privateWikiEngine(t)
	if _, err := e.CompilePrivateWiki(context.Background(), "case-a", ""); err != nil {
		t.Fatal(err)
	}
	e.FaultPoint = func(point string) error {
		if point == "wiki_before_retirement" {
			return errors.New("controlled retirement interruption")
		}
		return nil
	}
	_, _ = e.DeleteCapture("case-a", capture.ID, capture.Revision)
	before := privateWikiTree(t, e.Root)
	pending, err := e.InspectPrivateWikiMaintenance("case-a")
	if err != nil || pending.RetirementComplete || pending.State != "retirement_pending" || pending.Parent != "" || pending.GraphEvidence != "" || pending.EventID == "" {
		t.Fatalf("pending retirement granted graph evidence: %+v %v", pending, err)
	}
	if !reflect.DeepEqual(before, privateWikiTree(t, e.Root)) {
		t.Fatal("read-only retirement inspection performed cleanup")
	}
	e.FaultPoint = nil
	if _, err := e.CompilePrivateWiki(context.Background(), "case-a", pending.Parent); err == nil {
		t.Fatal("source-free retirement became a successful graph compilation")
	}
	retired, err := e.InspectPrivateWikiMaintenance("case-a")
	if err != nil || !retired.RetirementComplete || retired.State != "unavailable" || retired.GraphEvidence != "" {
		t.Fatalf("owning cleanup did not remain unavailable: %+v %v", retired, err)
	}
}
