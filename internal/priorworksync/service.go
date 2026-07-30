// Package priorworksync plans and records metadata-safe presence recovery for
// the SharePoint prior-work collector.
package priorworksync

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/priorwork"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

const (
	JobID       = "sharepoint-work-sync"
	WorkspaceID = "organization-sharepoint-work"
)

var (
	ErrCorporatePolicy       = fmt.Errorf("%w: corporate_policy", scheduler.ErrCapabilityUnavailable)
	ErrClaudeQualification   = fmt.Errorf("%w: qualifying native Claude SharePoint trial is pending", scheduler.ErrCapabilityUnavailable)
	ErrCollectorFailed       = errors.New("collector_failed")
	ErrPublicationUnverified = errors.New("publication_unverified")
	ErrPresenceClaimed       = errors.New("sharepoint-work-sync occurrence is already claimed")
)

type Collector interface {
	Collect(context.Context, scheduler.Occurrence) (priorwork.ApplyReport, error)
}

type CollectorFunc func(context.Context, scheduler.Occurrence) (priorwork.ApplyReport, error)

func (function CollectorFunc) Collect(ctx context.Context, occurrence scheduler.Occurrence) (priorwork.ApplyReport, error) {
	return function(ctx, occurrence)
}

type PublicationVerifier interface {
	Verify(priorwork.ApplyReport) error
}

type VerifierFunc func(priorwork.ApplyReport) error

func (function VerifierFunc) Verify(report priorwork.ApplyReport) error {
	return function(report)
}

type StoreVerifier struct {
	Store  priorwork.Store
	Access priorwork.AccessContext
}

func (verifier StoreVerifier) Verify(report priorwork.ApplyReport) error {
	return verifier.Store.VerifyPublication(report, verifier.Access)
}

type Service struct {
	Store     scheduler.Store
	Collector Collector
	Verifier  PublicationVerifier
	Clock     func() time.Time
}

type Report struct {
	SchemaVersion int                 `json:"schema_version"`
	JobID         string              `json:"job_id"`
	Runtime       string              `json:"runtime"`
	Due           bool                `json:"due"`
	Attempted     bool                `json:"attempted"`
	Receipts      []scheduler.Receipt `json:"receipts"`
}

func (service Service) RunPresence(
	ctx context.Context,
	runtime string,
	policy priorwork.SchedulePolicy,
) (Report, error) {
	if runtime != "claude" && runtime != "codex" {
		return Report{}, fmt.Errorf("unsupported prior-work runtime %q", runtime)
	}
	location, err := time.LoadLocation(policy.Timezone)
	if err != nil {
		return Report{}, errors.New("invalid prior-work scheduler timezone")
	}
	if policy.RefreshHours <= 0 || policy.RefreshHours > 8760 || policy.EnrolledAt.IsZero() {
		return Report{}, errors.New("invalid prior-work scheduler policy")
	}
	now := time.Now().In(location)
	if service.Clock != nil {
		now = service.Clock().In(location)
	}
	enrollment, err := service.Store.EnsureEnrollment(WorkspaceID, policy.EnrolledAt.In(location))
	if err != nil {
		return Report{}, err
	}
	claim, err := acquirePresenceClaim(service.Store.Root)
	if err != nil {
		return Report{}, err
	}
	defer claim.Release()
	if err := claim.Valid(); err != nil {
		return Report{}, err
	}
	receipts, err := service.Store.Receipts(WorkspaceID)
	if err != nil {
		return Report{}, err
	}
	occurrences, err := scheduler.PlanDue([]scheduler.Job{Job(policy)}, enrollment.EnrolledAt, receipts, now)
	if err != nil {
		return Report{}, err
	}
	report := Report{
		SchemaVersion: 1, JobID: JobID, Runtime: runtime,
		Due: len(occurrences) > 0, Attempted: len(occurrences) > 0,
		Receipts: []scheduler.Receipt{},
	}
	if len(occurrences) == 0 {
		return report, nil
	}
	executor := scheduler.ExecutorFunc(func(ctx context.Context, occurrence scheduler.Occurrence) error {
		if err := claim.Valid(); err != nil {
			return ErrPresenceClaimed
		}
		if runtime == "codex" {
			return ErrCorporatePolicy
		}
		if service.Collector == nil {
			return ErrClaudeQualification
		}
		publication, err := service.Collector.Collect(ctx, occurrence)
		if err != nil {
			return ErrCollectorFailed
		}
		if err := claim.Valid(); err != nil {
			return ErrPresenceClaimed
		}
		if publication.TriggerRef != OccurrenceRef(occurrence) {
			return ErrPublicationUnverified
		}
		if service.Verifier == nil || service.Verifier.Verify(publication) != nil {
			return ErrPublicationUnverified
		}
		if err := claim.Valid(); err != nil {
			return ErrPresenceClaimed
		}
		return nil
	})
	report.Receipts = scheduler.RunDue(ctx, executor, occurrences, now)
	for _, receipt := range report.Receipts {
		if err := claim.Valid(); err != nil {
			return Report{}, err
		}
		if err := service.Store.AppendReceipt(WorkspaceID, receipt); err != nil {
			return Report{}, err
		}
	}
	return report, nil
}

func Job(policy priorwork.SchedulePolicy) scheduler.Job {
	return scheduler.Job{
		ID: JobID, Cadence: scheduler.Interval, IntervalHours: policy.RefreshHours,
		MaxCatchUp: 1,
	}
}

func OccurrenceRef(occurrence scheduler.Occurrence) string {
	sum := sha256.Sum256([]byte(
		occurrence.JobID + "\x00" + occurrence.ScheduledFor.UTC().Format(time.RFC3339Nano),
	))
	return "occurrence-" + hex.EncodeToString(sum[:16])
}
