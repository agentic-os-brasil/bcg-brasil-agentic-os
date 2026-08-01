package darwin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
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
	Scheduler scheduler.Store
	Workspace string
	Runtime   string
	Now       func() time.Time
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
	request := HealthRequest{SchemaVersion: SchemaVersion, WindowID: "wake-" + hex.EncodeToString(windowDigest[:])[:16], Runtime: runtimeName, Mode: HeadlessHousekeeping, Surfaces: HealthSurfaces{
		Doctor: ProductSurface{State: "healthy"}, Capability: ProductSurface{State: "healthy"}, Validation: ProductSurface{State: "healthy"},
		Scheduler: ProductSurface{State: map[bool]string{true: "warning", false: "healthy"}[missed > 0], Count: missed}, ManagedState: ProductSurface{State: "healthy"},
	}}
	return BuildHealthPacket(request)
}

type DeepReviewHandler struct {
	Build         HealthPacketBuilder
	CommandStore  maintenance.Store
	ProposalStore ProposalStore
	Now           func() time.Time
}

func (handler DeepReviewHandler) Execute(ctx context.Context, command maintenance.Command) (maintenance.HandlerResult, error) {
	if command.JobID != "darwin-deep-weekly" || handler.Build == nil || handler.CommandStore.Root == "" {
		return maintenance.HandlerResult{}, errors.New("Darwin deep review handler is not configured")
	}
	if existing, readErr := handler.CommandStore.Receipts(command.WorkspaceID, command.JobID); readErr != nil {
		return maintenance.HandlerResult{}, readErr
	} else {
		for _, receipt := range existing {
			if receipt.OccurrenceDigest == command.OccurrenceDigest() && (receipt.State == maintenance.ReceiptSucceeded || receipt.State == maintenance.ReceiptProposalEmitted) {
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
	state := maintenance.ReceiptProposalEmitted
	reason := maintenance.ReasonProposalEmitted
	if len(assessment.Proposals) == 0 {
		// No structural change is a successful no-change review, not an empty
		// proposal receipt that could falsely look like durable evidence.
		state = maintenance.ReceiptSucceeded
		reason = maintenance.ReasonCompleted
	}
	base := maintenance.Receipt{SchemaVersion: maintenance.CommandSchemaVersion, AttemptID: attempt, OccurrenceDigest: command.OccurrenceDigest(), CommandID: command.CommandID, JobID: command.JobID, WorkspaceID: command.WorkspaceID, Trigger: command.Trigger, State: state, RecordedAt: now, Deadline: command.Deadline, ProposalCount: len(assessment.Proposals), ReasonCode: reason}
	if len(assessment.Proposals) > 0 {
		base.ProposalDigest = proposalDigest(command.OccurrenceDigest(), assessment)
		proposalStore := handler.ProposalStore
		if proposalStore.Root == "" {
			proposalStore = ProposalStore{Root: handler.CommandStore.Root}
		}
		artifact := AssessmentProposalArtifact{SchemaVersion: proposalArtifactSchemaVersion, RecordType: "assessment", AgentID: AgentID, JobID: command.JobID, OccurrenceDigest: command.OccurrenceDigest(), WindowID: assessment.WindowID, ProposalDigest: base.ProposalDigest, Assessment: assessment, ScheduledFor: command.ScheduledFor.UTC(), RecordedAt: command.ScheduledFor.UTC()}
		if err := proposalStore.Append(artifact); err != nil {
			return maintenance.HandlerResult{}, err
		}
		base.ProposalArtifactID = base.ProposalDigest
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

func (handler HousekeepingHandler) Execute(ctx context.Context, command maintenance.Command) (maintenance.HandlerResult, error) {
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
	if receipt.State != maintenance.ReceiptSucceeded {
		return maintenance.HandlerResult{State: receipt.State, ReasonCode: maintenance.ReasonHandlerFailure}, errors.New("Darwin housekeeping did not reach a successful boundary")
	}
	return maintenance.HandlerResult{State: maintenance.ReceiptSucceeded, ReasonCode: maintenance.ReasonCompleted}, nil
}
