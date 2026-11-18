package maintenance

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ownerctx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

func (p *productExecution) validateWeeklyCommitted() error {
	plan := p.weeklyPlan
	if plan == nil || plan.mode != WeeklyWorking || plan.committed == nil || plan.committed.Execution == nil {
		return ErrQualificationRequired
	}
	working := plan.committed
	e := working.Execution
	if err := e.Validate(); err != nil {
		return err
	}
	if e.OwnerScope != p.service.workspaceID || e.RegistrationSHA256 != plan.registration.Digest() || e.QualificationSHA256 != plan.qualification || e.ActivationSHA256 != plan.activation || e.ImplementationSHA256 != p.service.build {
		return ErrProductConflict
	}
	command := Command{JobID: p.jobID, Trigger: TriggerWeekly, ScheduledFor: working.Source.Options.ScheduledFor}
	expectedOccurrence := productDigest([]any{p.service.workspaceID, p.jobID, working.Source.Options.ScheduledFor})
	if working.Receipt.OccurrenceID != expectedOccurrence || e.OccurrenceSHA256 != command.OccurrenceDigest() || e.RequestedAt.Before(working.Source.Options.ScheduledFor) {
		return ErrProductConflict
	}
	ctx := p.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	return ownerctx.WithCurrentWeeklySelfWorking(ctx, filepath.Join(p.service.home, "brain"), *working, p.service.now().UTC(), func() error { return nil })
}
func weeklyReceiptFromExecution(jobID string, e *ownerctx.WeeklySelfExecution) Receipt {
	return Receipt{SchemaVersion: CommandSchemaVersion, AttemptID: e.AttemptID, OccurrenceDigest: e.OccurrenceSHA256, CommandID: e.CommandID, JobID: jobID, WorkspaceID: e.OwnerScope, Trigger: TriggerWeekly, State: ReceiptSucceeded, RecordedAt: e.RequestedAt, Deadline: e.Deadline, WeeklyMode: WeeklyWorking, ReasonCode: ReasonCompleted}
}
func (s *Service) reconcileWeeklyCommitted(ctx context.Context, p *productExecution, occ scheduler.Occurrence, now time.Time) (Receipt, error) {
	receipt := weeklyReceiptFromExecution(p.jobID, p.weeklyPlan.committed.Execution)
	if err := receipt.Validate(); err != nil {
		return Receipt{}, err
	}
	if !p.weeklyPlan.committed.Source.Options.ScheduledFor.Equal(occ.ScheduledFor) {
		return Receipt{}, ErrProductConflict
	}
	prior, err := s.receipts.Receipts(s.workspaceID, p.jobID)
	if err != nil {
		return Receipt{}, err
	}
	for _, r := range prior {
		if r.OccurrenceDigest != receipt.OccurrenceDigest {
			continue
		}
		if r.State == ReceiptSucceeded && r != receipt || r.CommandID == receipt.CommandID && r.AttemptID == receipt.AttemptID && r != receipt {
			return Receipt{}, ErrProductConflict
		}
	}
	command := Command{JobID: p.jobID, Trigger: TriggerWeekly, ScheduledFor: occ.ScheduledFor}
	worker := s.worker(p)
	lease, err := s.scheduler.TryAcquireLease(s.workspaceID, p.jobID, command.OccurrenceKey(), fmt.Sprintf("product-reconcile-%d", os.Getpid()), now, worker.Deadline+leaseQuarantineGrace)
	if err != nil {
		return Receipt{}, err
	}
	if err = worker.armLease(lease); err != nil {
		return Receipt{}, errors.Join(err, worker.releaseLease(lease))
	}
	// The original effect and original receipt identity are retained. This only
	// completes missing terminal metadata under the current lease/control fence.
	err = worker.publishOccurrence(ctx, s.workspaceID, occ, receipt.RecordedAt, lease, receipt)
	releaseErr := worker.releaseLease(lease)
	if err != nil || releaseErr != nil {
		return rejectedTerminalReceipt(receipt), errors.Join(err, releaseErr)
	}
	return receipt, nil
}

// Called only inside the existing current lease and publication fences.
func (s *Service) weeklySchedulerAlreadyClosed(occ scheduler.Occurrence, receipt Receipt) (bool, error) {
	prior, err := s.scheduler.Receipts(s.workspaceID)
	if err != nil {
		return false, err
	}
	for _, r := range prior {
		if r.JobID == receipt.JobID && r.ScheduledFor.Equal(occ.ScheduledFor) {
			if r.State == scheduler.Succeeded {
				if !r.AttemptedAt.Equal(receipt.RecordedAt) {
					return false, ErrProductConflict
				}
				return true, nil
			}
		}
	}
	return false, nil
}

func (s *Service) weeklyTerminalComplete(receipt Receipt, scheduled time.Time) (bool, error) {
	path := filepath.Join(s.receipts.Root, "workspaces", s.workspaceID, "receipts", receipt.JobID, receipt.CommandID+"--"+receipt.AttemptID+".json")
	body, err := scheduler.ReadPrivateStateFile(path, maximumReceiptBytes)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var actual Receipt
	if decodeProduct(body, &actual) != nil || actual != receipt {
		return false, ErrProductConflict
	}
	name := receipt.RecordedAt.UTC().Format("20060102T150405.000000000Z") + "-" + scheduled.UTC().Format("20060102T150405.000000000Z") + ".json"
	body, err = scheduler.ReadPrivateStateFile(filepath.Join(s.scheduler.Root, "workspaces", s.workspaceID, "receipts", receipt.JobID, name), 64<<10)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var terminal scheduler.Receipt
	if decodeProduct(body, &terminal) != nil || terminal.WorkspaceID != s.workspaceID || terminal.JobID != receipt.JobID || terminal.State != scheduler.Succeeded || !terminal.ScheduledFor.Equal(scheduled) || !terminal.AttemptedAt.Equal(receipt.RecordedAt) {
		return false, ErrProductConflict
	}
	return true, nil
}
