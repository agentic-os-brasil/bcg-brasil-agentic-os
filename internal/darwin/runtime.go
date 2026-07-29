package darwin

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

const HousekeepingJobID = "darwin-housekeeping"

// HealthPacketBuilder is the only runtime-specific input seam. Claude and
// Codex build the same closed packet; neither runtime gets a second Darwin
// implementation for housekeeping.
type HealthPacketBuilder interface {
	Build(context.Context, scheduler.Occurrence) (HealthPacket, error)
}

type HealthPacketBuilderFunc func(context.Context, scheduler.Occurrence) (HealthPacket, error)

func (function HealthPacketBuilderFunc) Build(ctx context.Context, occurrence scheduler.Occurrence) (HealthPacket, error) {
	return function(ctx, occurrence)
}

// HousekeepingExecutor is the scheduler-facing Darwin seam. Headless mode is
// explicit in the packet, but uses the same Plan, grants, invoker and receipt
// path as interactive Darwin execution.
type HousekeepingExecutor struct {
	Build   HealthPacketBuilder
	Guard   ToolGuard
	Invoker ToolInvoker
	Store   Store
	Now     func() time.Time
}

func (executor HousekeepingExecutor) Execute(ctx context.Context, occurrence scheduler.Occurrence) error {
	if occurrence.JobID != HousekeepingJobID || executor.Build == nil || executor.Guard == nil || executor.Invoker == nil {
		return errors.New("invalid Darwin housekeeping executor")
	}
	packet, err := executor.Build.Build(ctx, occurrence)
	if err != nil {
		return err
	}
	if packet.Mode != "" && packet.Mode != Interactive && packet.Mode != HeadlessHousekeeping {
		return errors.New("Darwin housekeeping packet uses an incompatible mode")
	}
	packet.Mode = HeadlessHousekeeping
	assessment, err := Plan(packet)
	if err != nil {
		return err
	}
	recordedAt := time.Now
	if executor.Now != nil {
		recordedAt = executor.Now
	}
	receipt, executeErr := Execute(ctx, packet, assessment, executor.Guard, executor.Invoker, recordedAt)
	if storeErr := executor.Store.Append(receipt); storeErr != nil {
		return storeErr
	}
	if executeErr != nil {
		return executeErr
	}
	if receipt.Outcome == OutcomeBlocked || receipt.Outcome == OutcomeFailed {
		return fmt.Errorf("Darwin housekeeping %s", receipt.Outcome)
	}
	return nil
}
