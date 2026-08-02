package darwin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/maintenance"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

// HousekeepingHandler adapts the existing command-gated Darwin implementation
// to the generic maintenance worker. The worker owns the occurrence lease;
// this handler owns only packet construction, scoped repair and Darwin health
// receipt publication. It is deliberately not a Walter/self implementation.
type HousekeepingHandler struct {
	Build        HealthPacketBuilder
	Guard        ToolGuard
	Invoker      ToolInvoker
	Store        Store
	CommandStore maintenance.Store
	Now          func() time.Time
}

// LocalProductHealthBuilder derives only bounded product-surface counts from
// scheduler receipts. It never reads prompts, workspace bodies or raw errors.
type LocalProductHealthBuilder struct {
	Scheduler          scheduler.Store
	Workspace          string
	Runtime            string
	ManagedStateRoot   string
	StateDocumentsRoot string
	Now                func() time.Time
}

func (builder LocalProductHealthBuilder) Build(ctx context.Context, occurrence scheduler.Occurrence) (HealthPacket, error) {
	if err := ctx.Err(); err != nil {
		return HealthPacket{}, err
	}
	workspace := builder.Workspace
	if workspace == "" {
		workspace = MaintenanceScope
	}
	receipts, err := builder.Scheduler.Receipts(workspace)
	if err != nil {
		return HealthPacket{}, err
	}
	missed := 0
	for _, receipt := range receipts {
		if receipt.State == scheduler.Failed || receipt.State == scheduler.Unavailable {
			missed++
		}
	}
	runtimeName := builder.Runtime
	if runtimeName == "" {
		runtimeName = "runtime-neutral"
	}
	windowDigest := sha256.Sum256([]byte(occurrence.JobID + "\x00" + occurrence.ScheduledFor.UTC().Format(time.RFC3339Nano)))
	stateDocuments := ProductSurface{State: "healthy"}
	if occurrence.JobID == "darwin-deep-weekly" {
		stateDocuments = reviewStateDocuments(builder.StateDocumentsRoot)
	}
	request := HealthRequest{SchemaVersion: SchemaVersion, WindowID: "wake-" + hex.EncodeToString(windowDigest[:])[:16], Runtime: runtimeName, Mode: HeadlessHousekeeping, Surfaces: HealthSurfaces{
		Doctor: ProductSurface{State: "healthy"}, Capability: ProductSurface{State: "healthy"}, Validation: ProductSurface{State: "healthy"},
		Scheduler: ProductSurface{State: map[bool]string{true: "warning", false: "healthy"}[missed > 0], Count: missed}, ManagedState: managedStateSurface(builder.ManagedStateRoot), StateDocuments: stateDocuments,
	}}
	return BuildHealthPacket(request)
}

type DeepReviewHandler struct {
	Build         HealthPacketBuilder
	Guard         ToolGuard
	Invoker       ToolInvoker
	Store         Store
	CommandStore  maintenance.Store
	ProposalStore ProposalStore
	Now           func() time.Time
}

func (handler DeepReviewHandler) ExecuteAuthorized(ctx context.Context, command maintenance.Command, grant maintenance.ExecutionGrant) (maintenance.HandlerResult, error) {
	if err := maintenance.ValidateExecutionGrant(grant, command); err != nil {
		return maintenance.HandlerResult{}, err
	}
	return handler.execute(ctx, command)
}

func (handler DeepReviewHandler) execute(ctx context.Context, command maintenance.Command) (maintenance.HandlerResult, error) {
	if command.JobID != "darwin-deep-weekly" || handler.Build == nil || handler.CommandStore.Root == "" {
		return maintenance.HandlerResult{}, errors.New("Darwin deep review handler is not configured")
	}
	proposalStore := handler.ProposalStore
	if proposalStore.Root == "" {
		proposalStore = ProposalStore{Root: handler.CommandStore.Root}
	}
	// The artifact is the durable owner boundary. Recover it before consulting
	// receipts or rebuilding health: a crash can occur after artifact publish
	// and before receipt append, and a changed health surface must not create a
	// second assessment for the same occurrence.
	if artifact, artifactErr := proposalStore.ReadOccurrence(command.JobID, command.OccurrenceDigest()); artifactErr == nil {
		attempt, err := newAttemptID()
		if err != nil {
			return maintenance.HandlerResult{}, err
		}
		now := time.Now().UTC()
		if handler.Now != nil {
			now = handler.Now().UTC()
		}
		recovered := maintenance.Receipt{SchemaVersion: maintenance.CommandSchemaVersion, AttemptID: attempt, OccurrenceDigest: command.OccurrenceDigest(), CommandID: command.CommandID, JobID: command.JobID, WorkspaceID: command.WorkspaceID, Trigger: command.Trigger, State: maintenance.ReceiptProposalEmitted, RecordedAt: now, Deadline: command.Deadline, ProposalCount: len(artifact.Assessment.Proposals), ProposalDigest: artifact.ProposalDigest, ProposalArtifactID: artifact.ArtifactID, ReasonCode: maintenance.ReasonProposalEmitted}
		if err := handler.CommandStore.AppendReceipt(recovered); err != nil {
			return maintenance.HandlerResult{}, err
		}
		return maintenance.HandlerResult{State: recovered.State, ProposalCount: recovered.ProposalCount, ProposalDigest: recovered.ProposalDigest, ProposalArtifactID: recovered.ProposalArtifactID, ReasonCode: recovered.ReasonCode}, nil
	} else if !errors.Is(artifactErr, os.ErrNotExist) {
		return maintenance.HandlerResult{}, artifactErr
	}
	if existing, readErr := handler.CommandStore.Receipts(command.WorkspaceID, command.JobID); readErr != nil {
		return maintenance.HandlerResult{}, readErr
	} else {
		for _, receipt := range existing {
			if receipt.OccurrenceDigest == command.OccurrenceDigest() && (receipt.State == maintenance.ReceiptSucceeded || receipt.State == maintenance.ReceiptReviewedNoChange || receipt.State == maintenance.ReceiptProposalEmitted) {
				if receipt.State == maintenance.ReceiptProposalEmitted {
					if err := proposalStore.ValidateReceipt(receipt); err != nil {
						return maintenance.HandlerResult{}, err
					}
				}
				return maintenance.HandlerResult{State: receipt.State, ProposalCount: receipt.ProposalCount, ProposalDigest: receipt.ProposalDigest, ProposalArtifactID: receipt.ProposalArtifactID, ReasonCode: receipt.ReasonCode}, nil
			}
		}
	}
	attempt, err := newAttemptID()
	if err != nil {
		return maintenance.HandlerResult{}, err
	}
	now := time.Now().UTC()
	if handler.Now != nil {
		now = handler.Now().UTC()
	}
	packet, err := handler.Build.Build(ctx, scheduler.Occurrence{JobID: command.JobID, ScheduledFor: command.ScheduledFor})
	if err != nil {
		return HandlerResultUnavailable(err)
	}
	if err := ctx.Err(); err != nil {
		return maintenance.HandlerResult{}, err
	}
	packet.Mode = DeepReview
	assessment, err := Plan(packet)
	if err != nil {
		return HandlerResultUnavailable(err)
	}
	if err := ctx.Err(); err != nil {
		return maintenance.HandlerResult{}, err
	}
	diagnostics := make([]Proposal, 0, len(assessment.Proposals))
	hasRepair := false
	for _, proposal := range assessment.Proposals {
		if isExecutableRepair(proposal) {
			hasRepair = true
		} else {
			diagnostics = append(diagnostics, proposal)
		}
	}
	if hasRepair {
		if handler.Guard == nil || handler.Invoker == nil || handler.Store.Root == "" {
			return HandlerResultUnavailable(errors.New("Darwin deep repair executor is not configured"))
		}
		repairReceipt, executeErr := Execute(ctx, packet, assessment, handler.Guard, handler.Invoker, func() time.Time { return now })
		if storeErr := handler.Store.Append(repairReceipt); storeErr != nil {
			return maintenance.HandlerResult{}, storeErr
		}
		if executeErr != nil || repairReceipt.Outcome == OutcomeFailed || repairReceipt.Outcome == OutcomeBlocked {
			if executeErr == nil {
				executeErr = errors.New("Darwin deep repair did not reach a validated outcome")
			}
			return maintenance.HandlerResult{State: maintenance.ReceiptFailed, ReasonCode: maintenance.ReasonHandlerFailure}, executeErr
		}
		if len(diagnostics) == 0 {
			base := maintenance.Receipt{SchemaVersion: maintenance.CommandSchemaVersion, AttemptID: attempt, OccurrenceDigest: command.OccurrenceDigest(), CommandID: command.CommandID, JobID: command.JobID, WorkspaceID: command.WorkspaceID, Trigger: command.Trigger, State: maintenance.ReceiptSucceeded, RecordedAt: now, Deadline: command.Deadline, ReasonCode: maintenance.ReasonCompleted}
			if err := handler.CommandStore.AppendReceipt(base); err != nil {
				return maintenance.HandlerResult{}, err
			}
			return maintenance.HandlerResult{State: maintenance.ReceiptSucceeded, ReasonCode: maintenance.ReasonCompleted}, nil
		}
		assessment.Proposals = diagnostics
	}
	state := maintenance.ReceiptProposalEmitted
	reason := maintenance.ReasonProposalEmitted
	if len(assessment.Proposals) == 0 {
		// No structural change is a successful no-change review, not an empty
		// proposal receipt that could falsely look like durable evidence.
		state = maintenance.ReceiptReviewedNoChange
		reason = maintenance.ReasonReviewedNoChange
	}
	base := maintenance.Receipt{SchemaVersion: maintenance.CommandSchemaVersion, AttemptID: attempt, OccurrenceDigest: command.OccurrenceDigest(), CommandID: command.CommandID, JobID: command.JobID, WorkspaceID: command.WorkspaceID, Trigger: command.Trigger, State: state, RecordedAt: now, Deadline: command.Deadline, ProposalCount: len(assessment.Proposals), ReasonCode: reason}
	if len(assessment.Proposals) > 0 {
		base.ProposalDigest = proposalDigest(command.OccurrenceDigest(), assessment)
		artifact := AssessmentProposalArtifact{SchemaVersion: proposalArtifactSchemaVersion, RecordType: "assessment", AgentID: AgentID, JobID: command.JobID, OccurrenceDigest: command.OccurrenceDigest(), ArtifactID: assessmentArtifactID(command.JobID, command.OccurrenceDigest()), WindowID: assessment.WindowID, ProposalDigest: base.ProposalDigest, Assessment: assessment, ScheduledFor: command.ScheduledFor.UTC(), RecordedAt: command.ScheduledFor.UTC()}
		if err := proposalStore.Append(artifact); err != nil {
			return maintenance.HandlerResult{}, err
		}
		base.ProposalArtifactID = artifact.ArtifactID
	}
	if err := handler.CommandStore.AppendReceipt(base); err != nil {
		return maintenance.HandlerResult{}, err
	}
	return maintenance.HandlerResult{State: base.State, ProposalCount: base.ProposalCount, ProposalDigest: base.ProposalDigest, ProposalArtifactID: base.ProposalArtifactID, ReasonCode: reason}, nil
}

func HandlerResultUnavailable(err error) (maintenance.HandlerResult, error) {
	if err == nil {
		err = errors.New("Darwin handler is unavailable")
	}
	return maintenance.HandlerResult{State: maintenance.ReceiptUnavailable, ReasonCode: maintenance.ReasonHandlerUnavailable}, err
}

func (handler HousekeepingHandler) ExecuteAuthorized(ctx context.Context, command maintenance.Command, grant maintenance.ExecutionGrant) (maintenance.HandlerResult, error) {
	if err := maintenance.ValidateExecutionGrant(grant, command); err != nil {
		return maintenance.HandlerResult{}, err
	}
	return handler.execute(ctx, command)
}

func (handler HousekeepingHandler) execute(ctx context.Context, command maintenance.Command) (maintenance.HandlerResult, error) {
	if command.JobID != HousekeepingJobID || command.ProposalOnly {
		return maintenance.HandlerResult{}, errors.New("Darwin housekeeping handler received an unauthorized job")
	}
	attempt, err := newAttemptID()
	if err != nil {
		return maintenance.HandlerResult{}, err
	}
	now := time.Now().UTC()
	if handler.Now != nil {
		now = handler.Now().UTC()
	}
	base := maintenance.Receipt{SchemaVersion: maintenance.CommandSchemaVersion, AttemptID: attempt, OccurrenceDigest: command.OccurrenceDigest(), CommandID: command.CommandID, JobID: command.JobID, WorkspaceID: command.WorkspaceID, Trigger: command.Trigger, State: maintenance.ReceiptFailed, RecordedAt: now, Deadline: command.Deadline, ReasonCode: maintenance.ReasonHandlerFailure}
	executor := HousekeepingExecutor{Build: handler.Build, Guard: handler.Guard, Invoker: handler.Invoker, Store: handler.Store, CommandStore: handler.CommandStore, Now: handler.Now}
	receipt, executeErr := executor.executeLeasedCommand(ctx, command, scheduler.Occurrence{JobID: command.JobID, ScheduledFor: command.ScheduledFor}, base, now)
	if executeErr != nil {
		return maintenance.HandlerResult{State: receipt.State, ReasonCode: maintenance.ReasonHandlerFailure}, executeErr
	}
	if receipt.State != maintenance.ReceiptSucceeded && receipt.State != maintenance.ReceiptReviewedNoChange {
		return maintenance.HandlerResult{State: receipt.State, ReasonCode: maintenance.ReasonHandlerFailure}, errors.New("Darwin housekeeping did not reach a successful boundary")
	}
	return maintenance.HandlerResult{State: receipt.State, ReasonCode: receipt.ReasonCode}, nil
}
