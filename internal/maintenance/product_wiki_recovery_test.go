//go:build !windows

package maintenance

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func productWikiGraphRoot(s *Service) string {
	return filepath.Join(s.engine.Root, "workspaces", s.workspaceID, ".private-wiki")
}

func assertProductWikiEventClosedOnce(t *testing.T, s *Service, event string, qualification *ProductQualification) Receipt {
	t.Helper()
	receipts, err := s.receipts.Receipts(s.workspaceID, WikiIncrementalJobID)
	if err != nil {
		t.Fatal(err)
	}
	var closed []Receipt
	for _, receipt := range receipts {
		if receipt.EventID == event && (receipt.State == ReceiptSucceeded || receipt.State == ReceiptReviewedNoChange) {
			if err := s.receipts.VerifyEventReceipt(receipt); err != nil {
				t.Fatal(err)
			}
			closed = append(closed, receipt)
		}
	}
	if len(closed) != 1 || closed[0].State != ReceiptReviewedNoChange {
		t.Fatalf("exact event has no unique durable no-change closure: %+v", closed)
	}
	before := productSnapshot(t, s.home)
	if replay, err := s.RunDue(context.Background()); err != nil || replay.State != "no_due_work" || len(replay.Due) != 0 || len(replay.Receipts) != 0 {
		t.Fatalf("closed exact event replay: %+v %v", replay, err)
	}
	if !reflect.DeepEqual(before, productSnapshot(t, s.home)) {
		t.Fatal("closed occurrence replay changed durable state")
	}
	status, err := s.Status()
	if err != nil {
		t.Fatal(err)
	}
	for _, job := range status.Jobs {
		if job.JobID == WikiIncrementalJobID && (job.Qualification == nil || job.Qualification.ID != qualification.ID || job.QualificationState != "qualified") {
			t.Fatal("no-change event closure replaced historical qualification")
		}
	}
	return closed[0]
}

func TestProductWikiEventRecoveryAfterPartialEffect(t *testing.T) {
	s, active := activeProductWiki(t)
	pending, err := s.engine.InspectPrivateWikiMaintenance(s.workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	oldGeneration := pending.Generation
	now := s.now()
	fenced := false
	var fenceErr error
	s.now = func() time.Time {
		if !fenced {
			body, err := os.ReadFile(filepath.Join(productWikiGraphRoot(s), "current.json"))
			var pointer struct {
				Generation string `json:"generation"`
			}
			if err == nil && json.Unmarshal(body, &pointer) == nil && pointer.Generation != oldGeneration {
				report, _, err := (DiagnosticStore{Root: s.receipts.Root}).Load(s.workspaceID, WikiIncrementalJobID, "")
				if err == nil && report.Wiki != nil && report.Wiki.Generation == pointer.Generation {
					fenced = true
					fenceErr = releaseActualProductLease(s)
				}
			}
		}
		return now
	}
	failed, err := s.RunDue(context.Background())
	if err == nil || !fenced || fenceErr != nil || failed.State != "completed_with_failures" || len(failed.Receipts) != 1 || failed.Receipts[0].State != ReceiptFailed {
		t.Fatalf("public Service did not preserve effect-before-terminal failure: %+v %v fenced=%v fence=%v", failed, err, fenced, fenceErr)
	}
	s.now = func() time.Time { return now.Add(time.Second) }
	current, err := s.engine.InspectPrivateWikiMaintenance(s.workspaceID)
	if err != nil || current.State != "current" || current.EventID != pending.EventID {
		t.Fatalf("actual graph effect missing: %+v %v", current, err)
	}
	graph := productSnapshot(t, productWikiGraphRoot(s))
	retry, err := s.RunDue(context.Background())
	if err != nil || retry.State != "processed" || len(retry.Receipts) != 1 || retry.Receipts[0].EventID != pending.EventID || retry.Receipts[0].State != ReceiptReviewedNoChange {
		t.Fatalf("current graph concealed unclosed failed event: %+v %v", retry, err)
	}
	if !reflect.DeepEqual(graph, productSnapshot(t, productWikiGraphRoot(s))) {
		t.Fatal("recovery duplicated or rewrote the graph")
	}
	assertProductWikiEventClosedOnce(t, s, pending.EventID, active.Qualification)
}

func TestProductWikiEventClosureAfterReconcile(t *testing.T) {
	s, active := activeProductWiki(t)
	pending, err := s.engine.InspectPrivateWikiMaintenance(s.workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	productApply(t, s, "qualify", WikiReconcileJobID, "")
	graph := productSnapshot(t, productWikiGraphRoot(s))
	run, err := s.RunDue(context.Background())
	if err != nil || run.State != "processed" || len(run.Receipts) != 1 || run.Receipts[0].State != ReceiptReviewedNoChange || run.Receipts[0].EventID != pending.EventID {
		t.Fatalf("reconciliation silently consumed incremental occurrence: %+v %v", run, err)
	}
	if !reflect.DeepEqual(graph, productSnapshot(t, productWikiGraphRoot(s))) {
		t.Fatal("ordinary no-change event duplicated graph publication")
	}
	closed := assertProductWikiEventClosedOnce(t, s, pending.EventID, active.Qualification)
	path := filepath.Join(s.receipts.Root, "workspaces", s.workspaceID, "receipts", WikiIncrementalJobID, closed.CommandID+"--"+closed.AttemptID+".json")
	alias := filepath.Join(t.TempDir(), "event-receipt-alias.json")
	if err := os.Link(path, alias); err != nil {
		t.Fatal(err)
	}
	before := productSnapshot(t, s.home)
	if out, err := s.RunDue(context.Background()); err == nil || out.State == "no_due_work" {
		t.Fatalf("hardlinked terminal evidence silently closed event: %+v %v", out, err)
	}
	if !reflect.DeepEqual(before, productSnapshot(t, s.home)) {
		t.Fatal("unsafe terminal evidence was repaired or bypassed")
	}
	if err := os.Remove(alias); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	now := s.now().Add(time.Second)
	s.now = func() time.Time { return now }
	retry, err := s.RunDue(context.Background())
	if err != nil || retry.State != "processed" || len(retry.Receipts) != 1 || retry.Receipts[0].State != ReceiptReviewedNoChange {
		t.Fatalf("missing terminal evidence silently closed event: %+v %v", retry, err)
	}
	closed = assertProductWikiEventClosedOnce(t, s, pending.EventID, active.Qualification)
	path = filepath.Join(filepath.Dir(path), closed.CommandID+"--"+closed.AttemptID+".json")
	if err := os.WriteFile(path, []byte("corrupt terminal evidence"), 0600); err != nil {
		t.Fatal(err)
	}
	before = productSnapshot(t, s.home)
	if out, err := s.RunDue(context.Background()); err == nil || out.State == "no_due_work" {
		t.Fatalf("corrupt terminal evidence silently closed event: %+v %v", out, err)
	}
	if !reflect.DeepEqual(before, productSnapshot(t, s.home)) {
		t.Fatal("corrupt closure evidence was repaired or bypassed")
	}
}

func TestProductWikiEventClosureRetainsRecoveryEvidence(t *testing.T) {
	s, active := activeProductWiki(t)
	pending, err := s.engine.InspectPrivateWikiMaintenance(s.workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	productApply(t, s, "qualify", WikiReconcileJobID, "")
	if run, err := s.RunDue(context.Background()); err != nil || run.State != "processed" {
		t.Fatalf("event did not close before recovery evidence: %+v %v", run, err)
	}
	closed := assertProductWikiEventClosedOnce(t, s, pending.EventID, active.Qualification)
	// Existing Worker recovery metadata can coexist with a success after lease
	// cleanup fails. This case tests its consumer; actual cleanup failures are
	// exercised by the retained generic Worker recovery regressions.
	recovery := (Worker{}).recoveryReceipt(closed, s.now())
	if err := s.receipts.AppendReceipt(recovery); err != nil {
		t.Fatal(err)
	}
	before := productSnapshot(t, s.home)
	if run, err := s.RunDue(context.Background()); err == nil || run.State == "no_due_work" {
		t.Fatalf("success concealed recovery-required evidence: %+v %v", run, err)
	}
	if !reflect.DeepEqual(before, productSnapshot(t, s.home)) {
		t.Fatal("existing recovery evidence was erased or bypassed")
	}
}
