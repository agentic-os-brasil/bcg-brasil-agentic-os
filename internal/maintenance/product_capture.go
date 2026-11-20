package maintenance

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/execution"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/lifecycle"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/memory"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

const MemoryCaptureJobID = "memory-l1-capture"

type captureReport struct {
	SchemaVersion    int                     `json:"schema_version"`
	WorkspaceID      string                  `json:"workspace_id"`
	Implementation   string                  `json:"implementation"`
	Configuration    string                  `json:"configuration"`
	Source           string                  `json:"source"`
	RecordedAt       time.Time               `json:"recorded_at"`
	Captures         []memory.CaptureReceipt `json:"captures"`
	ObservedOnly     int                     `json:"observed_only"`
	CommittedResults int                     `json:"committed_results"`
}

// captureSources reads only the admitted receipt outbox and canonical ledger.
// Completion is historical record evidence, never original host invocation.
func (s *Service) captureSources() ([]lifecycle.Signal, error) {
	root := filepath.Join(s.home, "state")
	signals, err := lifecycle.Signals(root, s.workspaceID)
	if err != nil {
		return nil, errors.New("capture_source_unavailable")
	}
	a := memory.CaptureAttestor{Root: s.engine.Root}
	for _, signal := range signals {
		if signal.Producer != lifecycle.LifecycleProducer || a.VerifySignal(signal) != nil {
			return nil, errors.New("capture_source_untrusted")
		}
		r, err := lifecycle.ReceiptSource(root, s.workspaceID, signal.Event, signal.SourceID)
		if err != nil {
			return nil, errors.New("capture_source_changed")
		}
		actual, err := lifecycle.SignalFromReceipt(s.workspaceID, r)
		if err != nil || actual.Digest() != signal.Digest() {
			return nil, errors.New("capture_source_changed")
		}
	}
	store, err := execution.OpenConfinedStore(root)
	if err != nil {
		return nil, errors.New("capture_ledger_unavailable")
	}
	items, err := store.List(s.workspaceID)
	if err != nil {
		return nil, errors.New("capture_ledger_unavailable")
	}
	for _, item := range items {
		if item.State != execution.StateCompleted {
			continue
		}
		record, err := store.ObserveCompletion(s.workspaceID, item.ItemID)
		if err != nil {
			return nil, errors.New("capture_completion_unavailable")
		}
		signal := lifecycle.Signal{SchemaVersion: 1, WorkspaceID: s.workspaceID, Producer: lifecycle.ExecutionProducer, Version: lifecycle.SignalVersion, Runtime: "canonical", Event: lifecycle.ExecutionComplete, EvidenceClass: lifecycle.CommittedEffect, SourceID: record.SourceID, SourceRevision: record.Revision, SourceDigest: lifecycle.SignalSourceDigest(record), OccurredAt: record.OccurredAt}
		if signal.Validate() != nil {
			return nil, errors.New("capture_completion_invalid")
		}
		signals = append(signals, signal)
		if len(signals) > lifecycle.MaximumSignals {
			return nil, errors.New("capture_source_capacity")
		}
	}
	sort.Slice(signals, func(i, j int) bool { return signals[i].OccurrenceID() < signals[j].OccurrenceID() })
	return signals, nil
}

func (s *Service) captureSourceIdentity() (string, error) {
	signals, err := s.captureSources()
	if err != nil {
		return "", err
	}
	return productDigest(signals), nil
}

func (s *Service) captureReportRoot() string {
	return filepath.Join(s.receipts.Root, "workspaces", s.workspaceID, "capture-results")
}

func (s *Service) readCaptureReport(generation string) (captureReport, string, error) {
	var r captureReport
	if generation == "" {
		body, err := scheduler.ReadPrivateStateFile(filepath.Join(s.captureReportRoot(), "current.json"), 1024)
		if err != nil {
			return r, "", err
		}
		var pointer struct {
			Generation string `json:"generation"`
		}
		if decodeProduct(body, &pointer) != nil {
			return r, "", errors.New("capture_report_invalid")
		}
		generation = pointer.Generation
	}
	if len(generation) != 64 || !digestPattern.MatchString(generation) {
		return r, "", errors.New("capture_report_invalid")
	}
	path := filepath.Join(s.captureReportRoot(), "versions", generation+".json")
	body, err := scheduler.ReadPrivateStateFile(path, 1<<20)
	if err != nil {
		return r, "", err
	}
	if decodeProduct(body, &r) != nil || productDigest(r) != generation || r.SchemaVersion != 1 || r.WorkspaceID != s.workspaceID || len(r.Captures) == 0 || len(r.Captures) > lifecycle.MaximumSignals || r.RecordedAt.IsZero() || r.ObservedOnly < 0 || r.CommittedResults < 0 || r.ObservedOnly+r.CommittedResults != len(r.Captures) || !digestPattern.MatchString(r.Source) || !digestPattern.MatchString(r.Implementation) || !digestPattern.MatchString(r.Configuration) {
		return r, "", errors.New("capture_report_invalid")
	}
	seen := map[string]bool{}
	for _, c := range r.Captures {
		if !c.Eligible || len(c.ID) != 32 || seen[c.ID] || c.Revision != 1 || !digestPattern.MatchString(c.SourceDigest) || c.VisibilityRevision == 0 {
			return r, "", errors.New("capture_report_invalid")
		}
		seen[c.ID] = true
	}
	return r, path, nil
}

type productCaptureHandler struct {
	service  *Service
	expected string
}

func (h productCaptureHandler) ExecuteAuthorized(ctx context.Context, command Command, grant ExecutionGrant) (HandlerResult, error) {
	if ValidateExecutionGrant(grant, command) != nil || command.JobID != MemoryCaptureJobID || command.WorkspaceID != h.service.workspaceID || command.Trigger != TriggerPresence {
		return HandlerResult{}, errors.New("capture_grant_invalid")
	}
	signals, err := h.service.captureSources()
	if err != nil {
		return HandlerResult{}, err
	}
	if len(signals) == 0 || productDigest(signals) != h.expected {
		return HandlerResult{}, errors.New("capture_source_changed")
	}
	guard := func(publish func() error) error {
		return publishWithGrant(ctx, grant, command, func() error {
			actual, err := h.service.captureSourceIdentity()
			if err != nil || actual != h.expected {
				return errors.New("capture_source_changed")
			}
			return publish()
		})
	}
	e := *h.service.engine
	e.PublicationGuard = guard
	a := memory.CaptureAttestor{Root: e.Root}
	report := captureReport{SchemaVersion: 1, WorkspaceID: command.WorkspaceID, Implementation: h.service.build, Configuration: h.service.configuration(MemoryCaptureJobID), Source: h.expected, RecordedAt: command.RequestedAt}
	for _, signal := range signals {
		if err := ctx.Err(); err != nil {
			return HandlerResult{}, err
		}
		if signal.Producer == lifecycle.ExecutionProducer {
			signal, err = a.SealSignal(signal)
			if err != nil {
				return HandlerResult{}, errors.New("capture_authority_unavailable")
			}
		}
		receipt, err := e.CaptureSignal(signal)
		if err != nil {
			return HandlerResult{}, err
		}
		if receipt.Added {
			report.Captures = append(report.Captures, receipt.Capture)
			if signal.EvidenceClass == lifecycle.ObservedOnly {
				report.ObservedOnly++
			} else {
				report.CommittedResults++
			}
		}
	}
	if len(report.Captures) == 0 {
		return HandlerResult{State: ReceiptReviewedNoChange, ReasonCode: ReasonReviewedNoChange}, nil
	}
	generation := productDigest(report)
	body, err := json.Marshal(report)
	if err != nil {
		return HandlerResult{}, err
	}
	path := filepath.Join(h.service.captureReportRoot(), "versions", generation+".json")
	if err = guard(func() error {
		if err := scheduler.EnsurePrivateDirectory(filepath.Dir(path)); err != nil {
			return err
		}
		if err := scheduler.PublishNewPrivateFile(path, body); err != nil {
			if !errors.Is(err, os.ErrExist) {
				return err
			}
			old, readErr := scheduler.ReadPrivateStateFile(path, 1<<20)
			if readErr != nil || string(old) != string(body) {
				return errors.New("capture_report_conflict")
			}
		}
		return nil
	}); err != nil {
		return HandlerResult{}, err
	}
	pointer, _ := json.Marshal(struct {
		Generation string `json:"generation"`
	}{generation})
	if err = scheduler.UpdatePrivateFileGuarded(filepath.Join(h.service.captureReportRoot(), "current.json"), 1024, guard, func([]byte) ([]byte, error) { return pointer, nil }); err != nil {
		return HandlerResult{}, err
	}
	return HandlerResult{State: ReceiptSucceeded, ReasonCode: ReasonCompleted}, nil
}
