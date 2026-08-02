package maintenance

import (
	"context"
	"testing"
	"time"
)

type invalidWalterProposal func(context.Context, Command, ExecutionGrant) (HandlerResult, error)

func (proposal invalidWalterProposal) ProposeWeekly(ctx context.Context, command Command, grant ExecutionGrant) (HandlerResult, error) {
	return proposal(ctx, command, grant)
}

func TestWalterWeeklyAdapterNeverTurnsMissingProposalIntoSuccess(t *testing.T) {
	command := Command{SchemaVersion: CommandSchemaVersion, CommandID: "walter-weekly", JobID: "walter-self-review-weekly", WorkspaceID: "maestro-system", Trigger: TriggerWeekly, ScheduledFor: time.Now().UTC(), RequestedAt: time.Now().UTC(), Deadline: time.Now().UTC().Add(time.Minute)}
	grant, err := newExecutionGrant(command)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := (WalterWeeklyAdapter{}).ExecuteAuthorized(context.Background(), command, grant); err == nil {
		t.Fatal("missing Walter integration was treated as success")
	}
	adapter := WalterWeeklyAdapter{Handler: invalidWalterProposal(func(context.Context, Command, ExecutionGrant) (HandlerResult, error) {
		return HandlerResult{State: ReceiptSucceeded, ReasonCode: ReasonCompleted}, nil
	})}
	if _, err := adapter.ExecuteAuthorized(context.Background(), command, grant); err == nil {
		t.Fatal("Walter success without a proposal receipt was accepted")
	}
}

func TestWalterWeeklyAdapterRejectsInvalidGrantBeforeProposal(t *testing.T) {
	command := Command{CommandID: "walter-weekly", JobID: WalterWeeklyJobID, WorkspaceID: "maestro-system", Trigger: TriggerWeekly, Deadline: time.Now().UTC().Add(time.Minute)}
	other := command
	other.WorkspaceID = "other-workspace"
	grant, err := newExecutionGrant(other)
	if err != nil {
		t.Fatal(err)
	}
	called := 0
	adapter := WalterWeeklyAdapter{Handler: invalidWalterProposal(func(context.Context, Command, ExecutionGrant) (HandlerResult, error) {
		called++
		return HandlerResult{State: ReceiptProposalEmitted, ProposalCount: 1, ProposalDigest: "digest", ProposalArtifactID: "artifact", ReasonCode: ReasonProposalEmitted}, nil
	})}
	if _, err := adapter.ExecuteAuthorized(context.Background(), command, nil); err == nil {
		t.Fatal("Walter adapter accepted a missing grant")
	}
	if _, err := adapter.ExecuteAuthorized(context.Background(), command, grant); err == nil {
		t.Fatal("Walter adapter accepted a command-mismatched grant")
	}
	if called != 0 {
		t.Fatalf("Walter proposal handler executed before grant validation: called=%d", called)
	}
}
