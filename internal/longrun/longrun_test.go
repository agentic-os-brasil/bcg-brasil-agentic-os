package longrun

import (
	"strings"
	"testing"
)

func TestGoalRequiresCurrentWalterApprovalAndCompletionAudit(t *testing.T) {
	goal := readyForWalter(t)
	if err := goal.applyWalterReview(approvedReview(goal)); err != nil {
		t.Fatal(err)
	}
	if err := goal.complete(); err == nil {
		t.Fatal("goal completed without Maestro completion audit")
	}
	if err := goal.recordCompletionAudit(completionAudit(goal)); err != nil {
		t.Fatal(err)
	}
	if err := goal.complete(); err != nil {
		t.Fatal(err)
	}
	if goal.Status() != Completed {
		t.Fatalf("status = %q", goal.Status())
	}
}

func TestEvidenceAfterApprovalInvalidatesWalterApproval(t *testing.T) {
	goal := readyForWalter(t)
	if err := goal.applyWalterReview(approvedReview(goal)); err != nil {
		t.Fatal(err)
	}
	if err := goal.recordEvidence(Evidence{ID: "review-suite", Class: EvidenceReview, Reference: "review://completion", Verified: true}); err != nil {
		t.Fatal(err)
	}
	if !goal.NeedsFreshWalterReview() || goal.LedgerRevision() == 0 {
		t.Fatalf("approval was not invalidated: %#v", goal)
	}
	if err := goal.recordCompletionAudit(completionAudit(goal)); err != nil {
		t.Fatal(err)
	}
	if err := goal.complete(); err == nil {
		t.Fatal("completion accepted stale Walter approval")
	}
}

func TestWalterRefineReturnsToWorkspaceLoopWithBreadcrumb(t *testing.T) {
	goal := readyForWalter(t)
	review := WalterReview{GoalID: goal.ID(), ContractRevision: goal.Contract().Revision, LedgerRevision: goal.LedgerRevision(), Verdict: WalterRefine, Reason: ReviewEvidenceGap}
	if err := goal.applyWalterReview(review); err != nil {
		t.Fatal(err)
	}
	breadcrumbs := goal.Breadcrumbs()
	if goal.Status() != Active || len(breadcrumbs) == 0 || breadcrumbs[len(breadcrumbs)-1].Action != ActionReturnToWorkspace {
		t.Fatalf("goal after refine = %#v", goal)
	}
}

func TestWalterNeedsHumanDecisionPreservesResumableEvidenceLedger(t *testing.T) {
	goal := readyForWalter(t)
	review := WalterReview{GoalID: goal.ID(), ContractRevision: goal.Contract().Revision, LedgerRevision: goal.LedgerRevision(), Verdict: WalterNeedsHumanDecision, Reason: ReviewAuthorityBoundary}
	if err := goal.applyWalterReview(review); err != nil {
		t.Fatal(err)
	}
	if goal.Status() != AwaitingHuman || len(goal.Evidence()) != 1 {
		t.Fatalf("goal after decision = %#v", goal)
	}
	if err := goal.resumeAfterHumanDecision(); err != nil {
		t.Fatal(err)
	}
	if goal.Status() != Active || !goal.NeedsFreshWalterReview() {
		t.Fatalf("goal after resume = %#v", goal)
	}
}

func TestWalterRecordCannotContainWorkspacePrivateCanary(t *testing.T) {
	goal := readyForWalter(t)
	record, err := goal.walterRecord()
	if err != nil {
		t.Fatal(err)
	}
	if !record.CompletionReady {
		t.Fatal("Walter did not receive the completion-readiness digest")
	}
	encoded := record.String()
	if strings.Contains(encoded, "client-secret-CANARY") || strings.Contains(encoded, "workspace-path-CANARY") || strings.Contains(encoded, "test://federation") {
		t.Fatalf("Walter record leaked workspace context: %s", encoded)
	}
}

func TestGoalRejectsFileReferencesAndRawSpecialistBypass(t *testing.T) {
	if _, err := NewGoal("maestro-pilot", DoneContract{Revision: 1, ObjectiveRef: "file:///client-secret-CANARY", RequiredEvidence: []EvidenceClass{EvidenceTest}, Deliverables: []Deliverable{{ID: "runtime", Kind: DeliverableCapability}}, NonGoalRefs: []string{"decision://no-release"}, Authority: AuthorityHumanForExternalAction}); err == nil {
		t.Fatal("file URI was accepted in Done Contract")
	}
	goal, err := NewGoal("maestro-pilot", validDoneContract())
	if err != nil {
		t.Fatal(err)
	}
	if err := goal.activate("pilot-runtime"); err != nil {
		t.Fatal(err)
	}
	if err := goal.recordEvidence(Evidence{ID: "bad-ref", Class: EvidenceTest, Reference: "file:///client-secret-CANARY", Verified: true}); err == nil {
		t.Fatal("file URI was accepted in evidence")
	}
	// There is no Goal.RecordSpecialistResult API: only a workspace-minimized
	// WorkspaceResult can enter Maestro state after an authorized delegation.
	if err := goal.recordWorkspaceResult(WorkspaceResult{GoalID: goal.ID(), DelegationID: "maestro-pilot--runtime-safety--r2", FindingRefs: []string{"finding://runtime"}}); err == nil {
		t.Fatal("workspace result bypassed checkpoint and delegation")
	}
}

func TestCompletionAuditRejectsBlockersOrMissingDeliverable(t *testing.T) {
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
	if err := goal.recordWorkspaceCheckpoint(WorkspaceCheckpoint{GoalID: goal.ID(), Phase: goal.Phase(), State: WorkspaceReady, EvidenceRefs: []string{"test://federation"}}); err != nil {
		t.Fatal(err)
	}
	packet, err := goal.delegate(SpecialistQuestion{ID: "runtime-safety", Capability: "runtime-safety", Purpose: "verify-delivery"})
	if err != nil {
		t.Fatal(err)
	}
	if err := goal.recordWorkspaceResult(WorkspaceResult{GoalID: goal.ID(), DelegationID: packet.DelegationID, FindingRefs: []string{"finding://runtime"}, EvidenceRefs: []string{"test://federation"}, BlockerRefs: []string{"blocker://approval"}}); err != nil {
		t.Fatal(err)
	}
	if err := goal.requestWalterReview(); err != nil {
		t.Fatal(err)
	}
	if err := goal.applyWalterReview(approvedReview(goal)); err != nil {
		t.Fatal(err)
	}
	if err := goal.recordCompletionAudit(completionAudit(goal)); err == nil {
		t.Fatal("completion audit accepted blocker and missing deliverable")
	}
}

func readyForWalter(t *testing.T) *Goal {
	t.Helper()
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
	if err := goal.recordWorkspaceCheckpoint(WorkspaceCheckpoint{GoalID: goal.ID(), Phase: goal.Phase(), State: WorkspaceReady, EvidenceRefs: []string{"test://federation"}}); err != nil {
		t.Fatal(err)
	}
	packet, err := goal.delegate(SpecialistQuestion{ID: "runtime-safety", Capability: "runtime-safety", Purpose: "verify-delivery"})
	if err != nil {
		t.Fatal(err)
	}
	if err := goal.recordWorkspaceResult(WorkspaceResult{GoalID: goal.ID(), DelegationID: packet.DelegationID, FindingRefs: []string{"finding://runtime-safety"}, EvidenceRefs: []string{"test://federation"}, CompletedDeliverables: []string{"runtime"}}); err != nil {
		t.Fatal(err)
	}
	if err := goal.requestWalterReview(); err != nil {
		t.Fatal(err)
	}
	return goal
}

func approvedReview(goal *Goal) WalterReview {
	return WalterReview{GoalID: goal.ID(), ContractRevision: goal.Contract().Revision, LedgerRevision: goal.LedgerRevision(), Verdict: WalterApproved}
}
func completionAudit(goal *Goal) CompletionAudit {
	return CompletionAudit{GoalID: goal.ID(), LedgerRevision: goal.LedgerRevision(), Phase: goal.Phase(), PhaseComplete: true, CompletedDeliverables: []string{"runtime"}, NoBlockers: true}
}

func validDoneContract() DoneContract {
	return DoneContract{Revision: 1, ObjectiveRef: "goal://maestro-pilot", RequiredEvidence: []EvidenceClass{EvidenceTest}, Deliverables: []Deliverable{{ID: "runtime", Kind: DeliverableCapability}}, NonGoalRefs: []string{"decision://no-autonomous-release"}, Authority: AuthorityHumanForExternalAction}
}
