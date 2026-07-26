package longrun

import (
	"context"
	"errors"
)

// GoalView is the workspace-agent input exposed by the core. It contains the
// current operational contract but no workspace source, path or transcript.
type GoalView struct {
	GoalID         string
	Revision       int
	LedgerRevision int
	Phase          string
}

// WorkspaceLoop is implemented by the workspace-agent adapter. The adapter
// resolves raw context under its existing scope before emitting only a
// checkpoint or accepting a bounded specialist result.
type WorkspaceLoop interface {
	Checkpoint(context.Context, GoalView) (WorkspaceCheckpoint, error)
	ReceiveSpecialistResult(context.Context, SpecialistResult) (WorkspaceResult, error)
}

// SpecialistLoop is a capability adapter. It receives the minimum work packet
// prepared by the workspace loop, not broad workspace access.
type SpecialistLoop interface {
	Run(context.Context, SpecialistWorkPacket) (SpecialistResult, error)
}

// WalterLoop is intentionally input-only. Walter reviews the sanitized record
// and cannot mutate a goal, source workspace or runtime.
type WalterLoop interface {
	Review(context.Context, WalterRecord) (WalterReview, error)
}

// LoopEngine enforces the canonical chained loop: Maestro sends work down to
// the workspace agent and specialist, receives it back through the workspace
// agent, composes the advancement, and only then calls Walter. Walter returns
// only to Maestro; a refine decision sends the next cycle back down the chain.
type LoopEngine struct {
	Workspace  WorkspaceLoop
	Specialist SpecialistLoop
	Walter     WalterLoop
}

func (engine LoopEngine) RunCycle(ctx context.Context, goal *Goal, question SpecialistQuestion) error {
	if goal == nil || goal.Status() != Active || engine.Workspace == nil || engine.Specialist == nil || engine.Walter == nil {
		return errors.New("long-running loop is not ready")
	}
	checkpoint, err := engine.Workspace.Checkpoint(ctx, GoalView{GoalID: goal.ID(), Revision: goal.Contract().Revision, LedgerRevision: goal.LedgerRevision(), Phase: goal.Phase()})
	if err != nil {
		return err
	}
	if err := goal.recordWorkspaceCheckpoint(checkpoint); err != nil {
		return err
	}
	packet, err := goal.delegate(question)
	if err != nil {
		return err
	}
	result, err := engine.Specialist.Run(ctx, packet)
	if err != nil {
		return err
	}
	workspaceResult, err := engine.Workspace.ReceiveSpecialistResult(ctx, result)
	if err != nil {
		return err
	}
	if err := goal.recordWorkspaceResult(workspaceResult); err != nil {
		return err
	}
	if err := goal.requestWalterReview(); err != nil {
		return err
	}
	record, err := goal.walterRecord()
	if err != nil {
		return err
	}
	review, err := engine.Walter.Review(ctx, record)
	if err != nil {
		return err
	}
	return goal.applyWalterReview(review)
}
