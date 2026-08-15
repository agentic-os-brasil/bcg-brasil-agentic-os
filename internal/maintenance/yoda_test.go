package maintenance

import (
	"context"
	"testing"
	"time"
)

type invalidYodaProposal func(context.Context, Command, ExecutionGrant) (HandlerResult, error)

func (proposal invalidYodaProposal) ProposeWeekly(ctx context.Context, command Command, grant ExecutionGrant) (HandlerResult, error) {
	return proposal(ctx, command, grant)
}

func TestYodaWeeklyAdapterNeverTurnsMissingProposalIntoSuccess(t *testing.T) {
	command := Command{SchemaVersion: CommandSchemaVersion, CommandID: "yoda-weekly", JobID: "yoda-self-review-weekly", WorkspaceID: "maestro-system", Trigger: TriggerWeekly, ScheduledFor: time.Now().UTC(), RequestedAt: time.Now().UTC(), Deadline: time.Now().UTC().Add(time.Minute)}
	grant, err := newExecutionGrant(command)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := (YodaWeeklyAdapter{}).ExecuteAuthorized(context.Background(), command, grant); err == nil {
		t.Fatal("missing Yoda integration was treated as success")
	}
	adapter := YodaWeeklyAdapter{Handler: invalidYodaProposal(func(context.Context, Command, ExecutionGrant) (HandlerResult, error) {
		return HandlerResult{State: ReceiptSucceeded, ReasonCode: ReasonCompleted}, nil
	})}
	if _, err := adapter.ExecuteAuthorized(context.Background(), command, grant); err == nil {
		t.Fatal("Yoda success without a proposal receipt was accepted")
	}
}

func TestYodaWeeklyAdapterRejectsInvalidGrantBeforeProposal(t *testing.T) {
	command := Command{CommandID: "yoda-weekly", JobID: YodaWeeklyJobID, WorkspaceID: "maestro-system", Trigger: TriggerWeekly, Deadline: time.Now().UTC().Add(time.Minute)}
	other := command
	other.WorkspaceID = "other-workspace"
	grant, err := newExecutionGrant(other)
	if err != nil {
		t.Fatal(err)
	}
	called := 0
	adapter := YodaWeeklyAdapter{Handler: invalidYodaProposal(func(context.Context, Command, ExecutionGrant) (HandlerResult, error) {
		called++
		return HandlerResult{State: ReceiptProposalEmitted, ProposalCount: 1, ProposalDigest: "digest", ProposalArtifactID: "artifact", ReasonCode: ReasonProposalEmitted}, nil
	})}
	if _, err := adapter.ExecuteAuthorized(context.Background(), command, nil); err == nil {
		t.Fatal("Yoda adapter accepted a missing grant")
	}
	if _, err := adapter.ExecuteAuthorized(context.Background(), command, grant); err == nil {
		t.Fatal("Yoda adapter accepted a command-mismatched grant")
	}
	if called != 0 {
		t.Fatalf("Yoda proposal handler executed before grant validation: called=%d", called)
	}
}
