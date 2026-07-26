package longrun

import (
	"context"
	"reflect"
	"testing"
)

func TestLoopEngineChainsWorkspaceSpecialistWalterInOrder(t *testing.T) {
	goal, err := NewGoal("maestro-pilot", validDoneContract())
	if err != nil {
		t.Fatal(err)
	}
	if err := goal.activate("pilot-runtime"); err != nil {
		t.Fatal(err)
	}
	if err := goal.recordEvidence(Evidence{ID: "test-suite", Class: EvidenceTest, Reference: "test://federation", Verified: true}); err != nil {
		t.Fatal(err)
	}
	store := testStore(t)
	if err := store.Create(goal); err != nil {
		t.Fatal(err)
	}
	trace := []string{}
	engine := LoopEngine{
		Workspace: workspaceLoopFunc{checkpoint: func(context.Context, GoalView) (WorkspaceCheckpoint, error) {
			trace = append(trace, "workspace:checkpoint")
			return WorkspaceCheckpoint{GoalID: goal.ID(), Phase: goal.Phase(), State: WorkspaceReady, EvidenceRefs: []string{"test://federation"}}, nil
		}, receive: func(_ context.Context, result SpecialistResult) (WorkspaceResult, error) {
			trace = append(trace, "workspace:receive")
			return WorkspaceResult{GoalID: result.GoalID, DelegationID: result.DelegationID, FindingRefs: []string{"finding://runtime"}, EvidenceRefs: []string{"test://federation"}, CompletedDeliverables: []string{"runtime"}}, nil
		}},
		Specialist: specialistLoopFunc(func(_ context.Context, packet SpecialistWorkPacket) (SpecialistResult, error) {
			trace = append(trace, "specialist:run")
			return SpecialistResult{GoalID: packet.GoalID, DelegationID: packet.DelegationID, FindingRefs: []string{"finding://untrusted"}, EvidenceRefs: []string{"test://federation"}}, nil
		}),
		Walter: walterLoopFunc(func(_ context.Context, record WalterRecord) (WalterReview, error) {
			trace = append(trace, "walter:review")
			return WalterReview{GoalID: record.GoalID, ContractRevision: record.ContractRevision, LedgerRevision: record.LedgerRevision, Verdict: WalterApproved}, nil
		}),
	}
	if err := engine.RunCycle(context.Background(), goal, SpecialistQuestion{ID: "runtime-safety", Capability: "runtime-safety", Purpose: "verify-delivery"}); err != nil {
		t.Fatal(err)
	}
	want := []string{"workspace:checkpoint", "specialist:run", "workspace:receive", "walter:review"}
	if !reflect.DeepEqual(trace, want) || goal.NeedsFreshWalterReview() || goal.Status() != Active || !hasAction(goal.Breadcrumbs(), ActionComposeAdvancement) {
		t.Fatalf("trace = %#v, goal = %#v", trace, goal)
	}
	if err := store.Save(goal); err != nil {
		t.Fatal(err)
	}
	restored, err := store.Load(goal.ID())
	if err != nil {
		t.Fatal(err)
	}
	if err := restored.recordCompletionAudit(completionAudit(restored)); err != nil {
		t.Fatal(err)
	}
	if err := restored.complete(); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(restored); err != nil {
		t.Fatal(err)
	}
	completed, err := store.Load(goal.ID())
	if err != nil || completed.Status() != Completed {
		t.Fatalf("recovered completion = %#v, err = %v", completed, err)
	}
}

func TestLoopEngineRejectsWorkspaceResultForDifferentDelegation(t *testing.T) {
	goal, err := NewGoal("maestro-pilot", validDoneContract())
	if err != nil {
		t.Fatal(err)
	}
	if err := goal.activate("pilot-runtime"); err != nil {
		t.Fatal(err)
	}
	if err := goal.recordEvidence(Evidence{ID: "test-suite", Class: EvidenceTest, Reference: "test://federation", Verified: true}); err != nil {
		t.Fatal(err)
	}
	engine := LoopEngine{Workspace: workspaceLoopFunc{checkpoint: func(context.Context, GoalView) (WorkspaceCheckpoint, error) {
		return WorkspaceCheckpoint{GoalID: goal.ID(), Phase: goal.Phase(), State: WorkspaceReady, EvidenceRefs: []string{"test://federation"}}, nil
	}, receive: func(context.Context, SpecialistResult) (WorkspaceResult, error) {
		return WorkspaceResult{GoalID: goal.ID(), DelegationID: "maestro-pilot--other-question--r2", FindingRefs: []string{"finding://runtime"}, EvidenceRefs: []string{"test://federation"}}, nil
	}}, Specialist: specialistLoopFunc(func(_ context.Context, packet SpecialistWorkPacket) (SpecialistResult, error) {
		return SpecialistResult{GoalID: packet.GoalID, DelegationID: packet.DelegationID}, nil
	}), Walter: walterLoopFunc(func(context.Context, WalterRecord) (WalterReview, error) {
		t.Fatal("Walter must not run")
		return WalterReview{}, nil
	})}
	if err := engine.RunCycle(context.Background(), goal, SpecialistQuestion{ID: "runtime-safety", Capability: "runtime-safety", Purpose: "verify-delivery"}); err == nil {
		t.Fatal("mismatched workspace result was accepted")
	}
}

type workspaceLoopFunc struct {
	checkpoint func(context.Context, GoalView) (WorkspaceCheckpoint, error)
	receive    func(context.Context, SpecialistResult) (WorkspaceResult, error)
}

func (loop workspaceLoopFunc) Checkpoint(ctx context.Context, view GoalView) (WorkspaceCheckpoint, error) {
	return loop.checkpoint(ctx, view)
}
func (loop workspaceLoopFunc) ReceiveSpecialistResult(ctx context.Context, result SpecialistResult) (WorkspaceResult, error) {
	return loop.receive(ctx, result)
}

type specialistLoopFunc func(context.Context, SpecialistWorkPacket) (SpecialistResult, error)

func (loop specialistLoopFunc) Run(ctx context.Context, packet SpecialistWorkPacket) (SpecialistResult, error) {
	return loop(ctx, packet)
}

type walterLoopFunc func(context.Context, WalterRecord) (WalterReview, error)

func (loop walterLoopFunc) Review(ctx context.Context, record WalterRecord) (WalterReview, error) {
	return loop(ctx, record)
}
func hasAction(breadcrumbs []Breadcrumb, action Action) bool {
	for _, breadcrumb := range breadcrumbs {
		if breadcrumb.Action == action {
			return true
		}
	}
	return false
}
