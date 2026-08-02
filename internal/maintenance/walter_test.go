package maintenance

import (
	"context"
	"testing"
	"time"
)

type invalidWalterProposal func(context.Context, Command) (HandlerResult, error)

func (proposal invalidWalterProposal) ProposeWeekly(ctx context.Context, command Command) (HandlerResult, error) {
	return proposal(ctx, command)
}

func TestWalterWeeklyAdapterNeverTurnsMissingProposalIntoSuccess(t *testing.T) {
	command := Command{SchemaVersion: CommandSchemaVersion, CommandID: "walter-weekly", JobID: "walter-self-review-weekly", WorkspaceID: "maestro-system", Trigger: TriggerWeekly, ScheduledFor: time.Now().UTC(), RequestedAt: time.Now().UTC(), Deadline: time.Now().UTC().Add(time.Minute)}
	if _, err := (WalterWeeklyAdapter{}).Execute(context.Background(), command); err == nil {
		t.Fatal("missing Walter integration was treated as success")
	}
	adapter := WalterWeeklyAdapter{Handler: invalidWalterProposal(func(context.Context, Command) (HandlerResult, error) {
		return HandlerResult{State: ReceiptSucceeded, ReasonCode: ReasonCompleted}, nil
	})}
	if _, err := adapter.Execute(context.Background(), command); err == nil {
		t.Fatal("Walter success without a proposal receipt was accepted")
	}
}
