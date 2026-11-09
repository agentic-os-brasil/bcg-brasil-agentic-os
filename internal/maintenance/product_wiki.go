package maintenance

import (
	"context"
	"errors"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/memory"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

const (
	WikiIncrementalJobID = "wiki-incremental-sync"
	WikiReconcileJobID   = "wiki-reconcile"
	WikiIntegrityJobID   = "wiki-integrity-check"
)

func wikiJob(id string) bool {
	return id == WikiIncrementalJobID || id == WikiReconcileJobID || id == WikiIntegrityJobID
}

// WikiDiagnostic is durable verification metadata, never an alternate graph.
// Its opaque identities come from memory's complete source/graph inspection.
type WikiDiagnostic struct {
	SourceIdentity         string `json:"source_identity"`
	SourceWatermark        string `json:"source_watermark"`
	Generation             string `json:"generation"`
	GraphEvidence          string `json:"graph_evidence"`
	ProducerImplementation string `json:"producer_implementation"`
	ProducerConfiguration  string `json:"producer_configuration"`
	DenialGeneration       uint64 `json:"denial_generation"`
	Concepts               int    `json:"concepts"`
	Files                  int    `json:"files"`
}

func (w WikiDiagnostic) validate(implementation string) error {
	for _, identity := range []string{w.SourceIdentity, w.SourceWatermark, w.Generation, w.GraphEvidence, w.ProducerImplementation, w.ProducerConfiguration} {
		if !digestPattern.MatchString(identity) {
			return errors.New("invalid wiki evidence identity")
		}
	}
	if w.ProducerImplementation != implementation || w.Concepts < 1 || w.Concepts > 128 || w.Files < w.Concepts+4 || w.Files > 1024 {
		return errors.New("invalid wiki evidence coverage")
	}
	return nil
}

func (s *Service) wikiSourceIdentity(job string) (string, error) {
	snapshot, err := s.engine.InspectPrivateWikiMaintenance(s.workspaceID)
	if err != nil || snapshot.SourceIdentity == "" {
		return "", scheduler.ErrCapabilityUnavailable
	}
	if job == WikiIntegrityJobID {
		if snapshot.State != "current" || !snapshot.ImplementationCurrent || !snapshot.ConfigurationCurrent || !snapshot.RetirementComplete {
			return "", ErrQualificationRequired
		}
		return productDigest([]string{snapshot.SourceIdentity, snapshot.GraphEvidence}), nil
	}
	return snapshot.SourceIdentity, nil
}

func (s *Service) wikiReport(job string, snapshot memory.PrivateWikiMaintenanceSnapshot, command Command) DiagnosticReport {
	w := &WikiDiagnostic{SourceIdentity: snapshot.SourceIdentity, SourceWatermark: snapshot.SourceWatermark, Generation: snapshot.Generation, GraphEvidence: snapshot.GraphEvidence, ProducerImplementation: snapshot.Implementation, ProducerConfiguration: snapshot.Configuration, DenialGeneration: snapshot.DenialGeneration, Concepts: snapshot.Concepts, Files: snapshot.Files}
	report := DiagnosticReport{SchemaVersion: 1, WorkspaceID: s.workspaceID, JobID: job, Implementation: s.build, Configuration: s.configuration(job), ObservedAt: command.RequestedAt, Finding: "private_graph_verified", Wiki: w}
	report.SourceSHA256 = report.sourceDigest()
	return report
}

// Publication is entered from outside memory only at report/terminal seams.
// Compilation already holds the activation lock before invoking its guard.
func (p *productExecution) publishTerminal(publish func() error) error {
	if !wikiJob(p.jobID) {
		return p.guard(publish)
	}
	return p.service.engine.WithRetentionSnapshot(p.service.workspaceID, func(memory.RetentionState) error {
		return p.guard(publish)
	})
}

type productWikiHandler struct {
	service  *Service
	expected string
	job      string
}

func (h productWikiHandler) ExecuteAuthorized(ctx context.Context, command Command, grant ExecutionGrant) (HandlerResult, error) {
	if err := ValidateExecutionGrant(grant, command); err != nil {
		return HandlerResult{}, err
	}
	if command.JobID != h.job || command.WorkspaceID != h.service.workspaceID || !wikiJob(h.job) || (h.job == WikiIncrementalJobID) != (command.Trigger == TriggerEvent) {
		return HandlerResult{}, ErrProductConflict
	}
	before, err := h.service.engine.InspectPrivateWikiMaintenance(command.WorkspaceID)
	if err != nil || h.expected == "" {
		return HandlerResult{}, scheduler.ErrCapabilityUnavailable
	}
	identity, err := h.service.wikiSourceIdentity(h.job)
	if err != nil || identity != h.expected {
		return HandlerResult{}, ErrProductConflict
	}
	if h.job == WikiIncrementalJobID && (command.EventID != before.EventID || !command.ScheduledFor.Equal(before.EventAt)) {
		return HandlerResult{}, ErrProductConflict
	}
	changedCompilation := true
	if h.job != WikiIntegrityJobID {
		engine := *h.service.engine
		engine.PublicationGuard = func(publish func() error) error { return publishWithGrant(ctx, grant, command, publish) }
		result, err := engine.CompilePrivateWiki(ctx, command.WorkspaceID, before.Parent)
		if err != nil {
			return HandlerResult{}, err
		}
		changedCompilation = result.Changed
	}
	after, err := h.service.engine.InspectPrivateWikiMaintenance(command.WorkspaceID)
	if err != nil || after.State != "current" || !after.RetirementComplete || !after.ImplementationCurrent || !after.ConfigurationCurrent {
		return HandlerResult{}, scheduler.ErrCapabilityUnavailable
	}
	if !changedCompilation {
		return HandlerResult{State: ReceiptReviewedNoChange, ReasonCode: ReasonReviewedNoChange}, nil
	}
	report := h.service.wikiReport(h.job, after, command)
	changed, err := (DiagnosticStore{Root: h.service.receipts.Root}).Publish(report, func(publish func() error) error {
		return h.service.engine.WithRetentionSnapshot(command.WorkspaceID, func(memory.RetentionState) error {
			return publishWithGrant(ctx, grant, command, func() error {
				current, err := h.service.engine.InspectPrivateWikiMaintenance(command.WorkspaceID)
				if err != nil || current.State != "current" || current.GraphEvidence != after.GraphEvidence || current.SourceIdentity != after.SourceIdentity {
					return ErrProductConflict
				}
				return publish()
			})
		})
	})
	if err != nil {
		return HandlerResult{}, err
	}
	if !changed {
		return HandlerResult{State: ReceiptReviewedNoChange, ReasonCode: ReasonReviewedNoChange}, nil
	}
	return HandlerResult{State: ReceiptSucceeded, ReasonCode: ReasonCompleted}, nil
}
