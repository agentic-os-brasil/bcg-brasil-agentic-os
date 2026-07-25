package federation

import (
	"context"
	"errors"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

const WeeklyFederationJobID = "federation-weekly"

// LocalDarwinPacketBuilder is the runtime adapter boundary. Claude and Codex
// can construct equivalent closed packets, while this executor owns the same
// queue-and-delivery operation for either runtime.
type LocalDarwinPacketBuilder interface {
	Build(context.Context, scheduler.Occurrence) (LocalDarwinPacket, error)
}

type LocalDarwinPacketBuilderFunc func(context.Context, scheduler.Occurrence) (LocalDarwinPacket, error)

func (function LocalDarwinPacketBuilderFunc) Build(ctx context.Context, occurrence scheduler.Occurrence) (LocalDarwinPacket, error) {
	return function(ctx, occurrence)
}

// WeeklyFederationExecutor is invoked by a native scheduler, lifecycle
// presence recovery or a managed runtime adapter. It does not ask the pilot
// user for a per-send approval and does not permit the Darwin builder to send.
type WeeklyFederationExecutor struct {
	Store  ExportStore
	Bridge Bridge
	Build  LocalDarwinPacketBuilder
	Now    func() time.Time
}

func (executor WeeklyFederationExecutor) Execute(ctx context.Context, occurrence scheduler.Occurrence) error {
	if occurrence.JobID != WeeklyFederationJobID || executor.Bridge == nil || executor.Build == nil {
		return errors.New("invalid weekly federation executor")
	}
	now := time.Now().UTC()
	if executor.Now != nil {
		now = executor.Now().UTC()
	}
	// Recover any due structural batch before recording the new weekly packet.
	if _, err := executor.Store.Flush(ctx, executor.Bridge, now); err != nil && !errors.Is(err, ErrNotEnrolled) && !errors.Is(err, ErrExportRevoked) {
		return err
	}
	packet, err := executor.Build.Build(ctx, occurrence)
	if err != nil {
		return err
	}
	batch, err := FederateLocal(packet)
	if err != nil {
		return err
	}
	if err := executor.Store.Enqueue(batch, now); err != nil {
		return err
	}
	_, err = executor.Store.Flush(ctx, executor.Bridge, now)
	return err
}
