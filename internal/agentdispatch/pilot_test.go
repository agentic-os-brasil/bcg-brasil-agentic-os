package agentdispatch

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/agentcatalog"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/agentorchestration"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/agentscaffold"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspaceagent"
)

func TestPilotAcceptsOnlyAuthenticatedTargetReturnForBothRuntimes(t *testing.T) {
	for _, runtimeName := range []string{"claude", "codex"} {
		t.Run(runtimeName, func(t *testing.T) {
			pilot := newTestPilot(t, runtimeName)
			now := time.Date(2026, 7, 25, 18, 0, 0, 0, time.UTC)
			pilot.now = func() time.Time { return now }
			pilot.dispatcher.now = pilot.now

			dispatch, receipt, err := pilot.Delegate(Intent{
				WorkspaceID: "alpha",
				Objective:   "Assess the approved research before the steering discussion.",
				Pointers:    []string{"bcgos://workspace/alpha/dossier/research.md"},
				Constraints: []string{"Return evidence and uncertainty."},
				TTL:         time.Hour,
			})
			if err != nil {
				t.Fatal(err)
			}
			if receipt.State != StateDelegated || dispatch.Packet.PacketID != receipt.DelegationID ||
				dispatch.Runtime != runtimeName || receipt.PacketSHA256 == "" {
				t.Fatalf("unexpected delegation: dispatch=%#v receipt=%#v", dispatch, receipt)
			}

			executor := newTestExecutor(t, runtimeName, "workspace-agent-alpha", "workspace-alpha-cap", now)
			body := ReturnBody{
				Summary:      "Two sources support the hypothesis; one market-size input is stale.",
				EvidenceRefs: []string{"bcgos://workspace/alpha/dossier/evidence/source-a.json"},
				Uncertainty:  "The source refresh date is outside the requested period.",
			}
			envelope, err := executor.SealReturn(dispatch, body)
			if err != nil {
				t.Fatal(err)
			}
			completed, err := pilot.Return(envelope, body)
			if err != nil {
				t.Fatal(err)
			}
			if completed.State != StateCompleted || completed.ResultSHA256 != envelope.ResultSHA256 ||
				completed.CompletedAt.IsZero() {
				t.Fatalf("unexpected completed receipt: %#v", completed)
			}
		})
	}
}

func TestPilotWiresMaterialMaestroOutputThroughWalterAcrossRuntimes(t *testing.T) {
	for _, runtimeName := range []string{"claude", "codex"} {
		t.Run(runtimeName, func(t *testing.T) {
			pilot := newTestPilot(t, runtimeName)
			now := time.Date(2026, 7, 29, 18, 0, 0, 0, time.UTC)
			pilot.now = func() time.Time { return now }
			pilot.dispatcher.now = pilot.now

			dispatch, sourceReceipt, err := pilot.Delegate(Intent{
				WorkspaceID: "alpha", Objective: "Prepare the bounded pilot recommendation.",
				ReviewTrigger: ReviewMaterialRecommendation,
				Pointers:      []string{"bcgos://workspace/alpha/dossier/recommendation.md"}, TTL: time.Hour,
			})
			if err != nil {
				t.Fatal(err)
			}
			producer := newTestExecutor(t, runtimeName, "workspace-agent-alpha", "workspace-alpha-cap", now)
			result := ReturnBody{
				Summary:      "The bounded pilot is supported by the reviewed evidence.",
				EvidenceRefs: []string{"bcgos://workspace/alpha/dossier/evidence.md"},
			}
			envelope, err := producer.SealReturn(dispatch, result)
			if err != nil {
				t.Fatal(err)
			}
			if pending, err := pilot.Return(envelope, result); err != nil || pending.State != StatePendingReview {
				t.Fatalf("material result bypassed pending review: %#v err=%v", pending, err)
			}

			reviewDispatch, reviewReceipt, err := pilot.RequireWalterReview(sourceReceipt.DelegationID, WalterReviewRequest{
				Trigger: ReviewMaterialRecommendation, ReviewObjective: "Pressure-test the recommendation before escalation.",
				Audience: "case sponsor", Recommendation: "Choose the bounded pilot scope.",
				DefinitionOfDone: "Sponsor can decide from the reviewed artifact.",
				ArtifactRefs:     []string{"bcgos://workspace/alpha/dossier/recommendation.md"},
				EvidenceRefs:     []string{"bcgos://workspace/alpha/dossier/evidence.md"},
				Uncertainties:    []string{"The source refresh date should be reconfirmed before publication."}, TTL: time.Hour,
			})
			if err != nil || reviewReceipt.State != StateDelegated || reviewReceipt.Review == nil ||
				reviewReceipt.Review.State != ReviewDispatched || reviewDispatch.Packet.TargetAgentID != "walter" {
				t.Fatalf("Walter wire = dispatch=%#v receipt=%#v err=%v", reviewDispatch, reviewReceipt, err)
			}
			if reviewDispatch.Packet.Review.SourcePacketID != dispatch.Packet.PacketID ||
				reviewDispatch.Packet.Review.SourceScopeID != "alpha" {
				t.Fatalf("review packet lost source binding: %#v", reviewDispatch.Packet.Review)
			}

			walter := newTestExecutor(t, runtimeName, "walter", "walter-cap", now)
			if _, err := walter.SealReturn(reviewDispatch, ReturnBody{Summary: "bypass"}); err == nil {
				t.Fatal("Walter generic return bypassed the typed verdict contract")
			}
			verdict := WalterReviewBody{
				Verdict:      WalterApproved,
				EvidenceRefs: []string{"bcgos://workspace/alpha/dossier/evidence.md"},
			}
			reviewEnvelope, err := walter.SealWalterReview(reviewDispatch, verdict)
			if err != nil {
				t.Fatal(err)
			}
			completed, err := pilot.ReturnWalterReview(reviewEnvelope, verdict)
			if err != nil || completed.State != StateCompleted || completed.Review == nil ||
				completed.Review.State != ReviewApproved || completed.Review.ObjectionCount != 0 {
				t.Fatalf("Walter verdict = %#v err=%v", completed, err)
			}
			producerReceipt, ok := pilot.Inspect(sourceReceipt.DelegationID)
			if !ok || producerReceipt.State != StateCompleted || producerReceipt.Review == nil || producerReceipt.Review.State != ReviewApproved {
				t.Fatalf("material producer was not completion-authorized by Walter: %#v", producerReceipt)
			}
			encoded, _ := json.Marshal(completed)
			for _, forbidden := range []string{"Choose the bounded pilot scope.", "source refresh date", "case sponsor"} {
				if strings.Contains(string(encoded), forbidden) {
					t.Fatalf("compact review state leaked %q: %s", forbidden, encoded)
				}
			}
		})
	}
}

func TestPilotWalterRefinementIsBoundedAndDoesNotApproveCompletion(t *testing.T) {
	pilot := newTestPilot(t, "claude")
	now := time.Date(2026, 7, 29, 18, 0, 0, 0, time.UTC)
	pilot.now = func() time.Time { return now }
	pilot.dispatcher.now = pilot.now
	dispatch, sourceReceipt, err := pilot.Delegate(Intent{WorkspaceID: "alpha", Objective: "Prepare one recommendation.", ReviewTrigger: ReviewConsequentialTradeoff, TTL: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	producer := newTestExecutor(t, "claude", "workspace-agent-alpha", "workspace-alpha-cap", now)
	result := ReturnBody{Summary: "Bounded recommendation."}
	envelope, err := producer.SealReturn(dispatch, result)
	if err != nil {
		t.Fatal(err)
	}
	if pending, err := pilot.Return(envelope, result); err != nil || pending.State != StatePendingReview {
		t.Fatalf("material result bypassed pending review: %#v err=%v", pending, err)
	}
	reviewDispatch, _, err := pilot.RequireWalterReview(sourceReceipt.DelegationID, WalterReviewRequest{
		Trigger: ReviewConsequentialTradeoff, ReviewObjective: "Pressure-test the trade-off.",
		Audience: "sponsor", Recommendation: "Choose option A.", DefinitionOfDone: "The trade-off is explicit.", TTL: time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	walter := newTestExecutor(t, "claude", "walter", "walter-cap", now)
	refinement := WalterReviewBody{Verdict: WalterRefineAndReturn, Objections: []WalterObjection{{
		Code: "missing-counterevidence", Fix: "Add the counter-evidence to the recommendation.", ExitCondition: "The evidence pointer is present and reviewed.",
	}}}
	reviewEnvelope, err := walter.SealWalterReview(reviewDispatch, refinement)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := pilot.ReturnWalterReview(reviewEnvelope, refinement)
	if err != nil || receipt.Review == nil || receipt.Review.State != ReviewRefineReturn || receipt.Review.ObjectionCount != 1 {
		t.Fatalf("refinement = %#v err=%v", receipt, err)
	}
	if receipt.Review.State == ReviewApproved {
		t.Fatal("refinement was promoted to approval")
	}
	producerReceipt, ok := pilot.Inspect(sourceReceipt.DelegationID)
	if !ok || producerReceipt.State != StatePendingReview {
		t.Fatalf("refinement incorrectly completed producer: %#v", producerReceipt)
	}
}

func TestPilotRecordsCompactWalterUnavailableState(t *testing.T) {
	pilot := newTestPilot(t, "codex")
	pilot.instances["walter"] = Instance{
		AgentID: "walter", Role: "reviewer", ScopeKind: "review", ScopeID: "review",
		ParentAgentID: "maestro", Available: false,
	}
	dispatch, sourceReceipt, err := pilot.Delegate(Intent{
		WorkspaceID: "alpha", Objective: "Prepare one material recommendation.", ReviewTrigger: ReviewExternalArtifact, TTL: time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	producer := newTestExecutor(t, "codex", "workspace-agent-alpha", "workspace-alpha-cap", time.Now())
	result := ReturnBody{Summary: "Bounded recommendation."}
	envelope, err := producer.SealReturn(dispatch, result)
	if err != nil {
		t.Fatal(err)
	}
	if pending, err := pilot.Return(envelope, result); err != nil || pending.State != StatePendingReview {
		t.Fatalf("material result bypassed pending review: %#v err=%v", pending, err)
	}
	_, receipt, err := pilot.RequireWalterReview(sourceReceipt.DelegationID, WalterReviewRequest{
		Trigger: ReviewExternalArtifact, ReviewObjective: "Check the artifact before sharing.",
		Audience: "sponsor", Recommendation: "Share the bounded artifact.",
		DefinitionOfDone: "The sponsor can inspect the artifact.", TTL: time.Hour,
	})
	if err == nil || receipt.State != StateUnavailable || receipt.FailureCode != "target_unavailable" ||
		receipt.Review == nil || receipt.Review.State != ReviewUnavailable ||
		receipt.Review.SourcePacketID != sourceReceipt.DelegationID {
		t.Fatalf("unavailable Walter state = %#v, err=%v", receipt, err)
	}
	encoded, _ := json.Marshal(receipt)
	if strings.Contains(string(encoded), "Share the bounded artifact.") {
		t.Fatalf("review prose leaked into unavailable receipt: %s", encoded)
	}
}

func TestPilotRejectsForgedReplayedCrossRuntimeAndCrossScopeReturns(t *testing.T) {
	now := time.Date(2026, 7, 25, 18, 0, 0, 0, time.UTC)
	body := ReturnBody{Summary: "Bounded finding."}

	tests := []struct {
		name   string
		mutate func(t *testing.T, dispatch Dispatch, envelope *ExecutionEnvelope)
	}{
		{
			name: "forged capability",
			mutate: func(t *testing.T, dispatch Dispatch, envelope *ExecutionEnvelope) {
				t.Helper()
				forged := newTestExecutor(t, "claude", "workspace-agent-alpha", "wrong-capability", now)
				value, err := forged.SealReturn(dispatch, body)
				if err != nil {
					t.Fatal(err)
				}
				*envelope = value
			},
		},
		{
			name: "authorized different target",
			mutate: func(t *testing.T, _ Dispatch, envelope *ExecutionEnvelope) {
				t.Helper()
				envelope.TargetAgentID = "practice-insurance"
				resignEnvelopeForTest(t, envelope, "practice-insurance-cap")
			},
		},
		{
			name: "cross runtime",
			mutate: func(t *testing.T, _ Dispatch, envelope *ExecutionEnvelope) {
				t.Helper()
				envelope.Runtime = "codex"
				resignEnvelopeForTest(t, envelope, "workspace-alpha-cap")
			},
		},
		{
			name: "cross scope",
			mutate: func(t *testing.T, _ Dispatch, envelope *ExecutionEnvelope) {
				t.Helper()
				envelope.ScopeID = "beta"
				resignEnvelopeForTest(t, envelope, "workspace-alpha-cap")
			},
		},
		{
			name: "result body changed",
			mutate: func(t *testing.T, _ Dispatch, envelope *ExecutionEnvelope) {
				t.Helper()
				envelope.ResultSHA256 = digestBody(ReturnBody{Summary: "Different result."})
				resignEnvelopeForTest(t, envelope, "workspace-alpha-cap")
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pilot := newTestPilot(t, "claude")
			pilot.now = func() time.Time { return now }
			pilot.dispatcher.now = pilot.now
			dispatch, receipt, err := pilot.Delegate(Intent{
				WorkspaceID: "alpha", Objective: "Assess one bounded source.", TTL: time.Hour,
			})
			if err != nil {
				t.Fatal(err)
			}
			executor := newTestExecutor(t, "claude", "workspace-agent-alpha", "workspace-alpha-cap", now)
			envelope, err := executor.SealReturn(dispatch, body)
			if err != nil {
				t.Fatal(err)
			}
			test.mutate(t, dispatch, &envelope)

			failed, err := pilot.Return(envelope, body)
			if err == nil || failed.State != StateFailed || failed.FailureCode != "envelope_denied" {
				t.Fatalf("forged return = %#v, %v", failed, err)
			}
			current, ok := pilot.Inspect(receipt.DelegationID)
			if !ok || current.State != StateDelegated {
				t.Fatalf("forged return changed active receipt: %#v, present=%t", current, ok)
			}
		})
	}

	t.Run("replay", func(t *testing.T) {
		pilot := newTestPilot(t, "claude")
		pilot.now = func() time.Time { return now }
		pilot.dispatcher.now = pilot.now
		dispatch, _, err := pilot.Delegate(Intent{WorkspaceID: "alpha", Objective: "Assess one bounded source.", TTL: time.Hour})
		if err != nil {
			t.Fatal(err)
		}
		executor := newTestExecutor(t, "claude", "workspace-agent-alpha", "workspace-alpha-cap", now)
		envelope, err := executor.SealReturn(dispatch, body)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pilot.Return(envelope, body); err != nil {
			t.Fatal(err)
		}
		replayed, err := pilot.Return(envelope, body)
		if err == nil || replayed.FailureCode != "envelope_replayed" {
			t.Fatalf("replayed envelope = %#v, %v", replayed, err)
		}
	})
}

func TestPilotAuthenticatesFailureEnvelopeWithoutPublishingFailureProse(t *testing.T) {
	now := time.Date(2026, 7, 25, 18, 0, 0, 0, time.UTC)
	pilot := newTestPilot(t, "claude")
	pilot.now = func() time.Time { return now }
	pilot.dispatcher.now = pilot.now
	dispatch, _, err := pilot.Delegate(Intent{WorkspaceID: "alpha", Objective: "Assess one source.", TTL: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	executor := newTestExecutor(t, "claude", "workspace-agent-alpha", "workspace-alpha-cap", now)
	body := FailureBody{Code: "runtime_disconnected", Detail: "Secret internal runtime failure detail."}
	envelope, err := executor.SealFailure(dispatch, body)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := pilot.Fail(envelope, body)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.State != StateFailed || receipt.FailureCode != body.Code ||
		strings.Contains(string(encoded), body.Detail) {
		t.Fatalf("failure receipt leaked body: %s", encoded)
	}
}

func TestPilotPublicReceiptsAreMetadataOnly(t *testing.T) {
	pilot := newTestPilot(t, "claude")
	objective := "PRIVATE OBJECTIVE BODY"
	pointer := "bcgos://workspace/alpha/dossier/private-source.md"
	constraint := "PRIVATE CONSTRAINT BODY"
	dispatch, receipt, err := pilot.Delegate(Intent{
		WorkspaceID: "alpha", Objective: objective, Pointers: []string{pointer},
		Constraints: []string{constraint}, TTL: time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{objective, pointer, constraint, dispatch.Packet.Signature, `"packet"`, `"result"`, `"failure_detail"`} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("public receipt contains %q: %s", forbidden, encoded)
		}
	}
}

func TestPilotErrandUsesClosedGrantAndDeterministicCompensation(t *testing.T) {
	for _, runtimeName := range []string{"claude", "codex"} {
		t.Run(runtimeName, func(t *testing.T) {
			pilot := newTestPilot(t, runtimeName)
			now := time.Date(2026, 7, 26, 13, 0, 0, 0, time.UTC)
			pilot.now = func() time.Time { return now }
			pilot.dispatcher.now = pilot.now
			resource := "bcgos://errand/pilot/ephemeral-notes/meeting-link.md"
			dispatch, receipt, err := pilot.DelegateErrand(ErrandIntent{
				ErrandID: "pilot", Objective: "Stage the approved meeting link.",
				Grant: ErrandGrant{Operation: ErrandCreateEphemeralNote, Resource: resource},
				TTL:   time.Hour,
			})
			if err != nil {
				t.Fatal(err)
			}
			if receipt.State != StateDelegated || dispatch.Errand == nil {
				t.Fatalf("errand delegation = %#v, %#v", dispatch, receipt)
			}
			if dispatch.Errand.Grant.Operation != ErrandCreateEphemeralNote ||
				dispatch.Errand.Compensation.Operation != ErrandDeleteEphemeralNote ||
				dispatch.Errand.Compensation.Resource != resource {
				t.Fatalf("errand contract is not deterministic: %#v", dispatch.Errand)
			}
			executor := newTestExecutor(t, runtimeName, "errand-helper", "errand-helper-cap", now)
			grantRequest, err := executor.SealErrandToolRequest(dispatch, ErrandCreateEphemeralNote, resource)
			if err != nil {
				t.Fatal(err)
			}
			forgedRequest := grantRequest
			forgedRequest.Signature = strings.Repeat("0", 64)
			if decision := pilot.GuardErrandTool(forgedRequest); decision.Allowed ||
				decision.Code != "tool_envelope_denied" {
				t.Fatalf("unauthenticated tool request accepted: %#v", decision)
			}
			otherRequest := grantRequest
			otherRequest.Resource = "bcgos://errand/pilot/ephemeral-notes/other.md"
			otherRequest.Signature, err = signErrandToolEnvelope(otherRequest, "errand-helper-cap")
			if err != nil {
				t.Fatal(err)
			}
			if decision := pilot.GuardErrandTool(otherRequest); decision.Allowed ||
				decision.Code != "resource_denied" {
				t.Fatalf("different resource accepted under static prefix: %#v", decision)
			}
			if decision := pilot.GuardErrandTool(grantRequest); !decision.Allowed {
				t.Fatalf("exact errand grant denied: %#v", decision)
			}
			earlyUndo, err := executor.SealErrandToolRequest(dispatch, ErrandDeleteEphemeralNote, resource)
			if err != nil {
				t.Fatal(err)
			}
			if decision := pilot.GuardErrandTool(earlyUndo); decision.Allowed {
				t.Fatalf("compensation before observed grant success was accepted: %#v", decision)
			}
			grantOutcome, err := executor.SealErrandToolOutcome(grantRequest, ErrandToolSucceeded)
			if err != nil {
				t.Fatal(err)
			}
			if decision := pilot.ObserveErrandTool(grantOutcome); !decision.Allowed {
				t.Fatalf("authenticated grant outcome denied: %#v", decision)
			}
			undoRequest, err := executor.SealErrandToolRequest(dispatch, ErrandDeleteEphemeralNote, resource)
			if err != nil {
				t.Fatal(err)
			}
			if decision := pilot.GuardErrandTool(undoRequest); !decision.Allowed {
				t.Fatalf("deterministic compensation denied: %#v", decision)
			}
			undoOutcome, err := executor.SealErrandToolOutcome(undoRequest, ErrandToolSucceeded)
			if err != nil {
				t.Fatal(err)
			}
			if decision := pilot.ObserveErrandTool(undoOutcome); !decision.Allowed {
				t.Fatalf("authenticated compensation outcome denied: %#v", decision)
			}
			repeatedUndo, err := executor.SealErrandToolRequest(dispatch, ErrandDeleteEphemeralNote, resource)
			if err != nil {
				t.Fatal(err)
			}
			if decision := pilot.GuardErrandTool(repeatedUndo); decision.Allowed {
				t.Fatalf("repeated compensation was accepted: %#v", decision)
			}
		})
	}
}

func TestPilotRejectsCallerInventedErrandOperationsAndResources(t *testing.T) {
	tests := []ErrandGrant{
		{Operation: ErrandOperation("update_production_config"), Resource: "bcgos://errand/pilot/ephemeral-notes/change.md"},
		{Operation: ErrandCreateEphemeralNote, Resource: "bcgos://workspace/alpha/dossier/change.md"},
		{Operation: ErrandCreateEphemeralNote, Resource: "bcgos://errand/pilot/ephemeral-notes/"},
	}
	for _, grant := range tests {
		pilot := newTestPilot(t, "claude")
		_, receipt, err := pilot.DelegateErrand(ErrandIntent{
			ErrandID: "pilot", Objective: "Perform a caller-selected mutation.", Grant: grant, TTL: time.Hour,
		})
		if err == nil || receipt.State != StateFailed || receipt.FailureCode != "intent_invalid" {
			t.Fatalf("invalid errand grant accepted: %#v, %#v, %v", grant, receipt, err)
		}
	}
}

func TestPilotFailureReceiptOmitsInvalidCallerIdentity(t *testing.T) {
	pilot := newTestPilot(t, "claude")
	invalid := strings.Repeat("client/path/", 100)
	_, receipt, err := pilot.Delegate(Intent{
		WorkspaceID: invalid,
		Objective:   "Attempt an invalid delegation.",
		TTL:         time.Minute,
	})
	if err == nil {
		t.Fatal("invalid delegation was accepted")
	}
	body, marshalErr := json.Marshal(receipt)
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	if strings.Contains(string(body), invalid) ||
		receipt.ScopeID != "" ||
		receipt.TargetAgentID != "" {
		t.Fatalf("failure receipt retained invalid caller identity: %s", body)
	}
}

func TestPilotUnknownEnvelopeDoesNotEchoUntrustedPacketID(t *testing.T) {
	pilot := newTestPilot(t, "claude")
	untrusted := strings.Repeat("private-content/", 100)
	receipt, err := pilot.Return(ExecutionEnvelope{
		SchemaVersion: 1,
		PacketID:      untrusted,
		ScopeKind:     "workspace",
		ScopeID:       "alpha",
		Outcome:       ExecutionSucceeded,
		ResultSHA256:  strings.Repeat("0", 64),
		Nonce:         strings.Repeat("a", 64),
		IssuedAt:      time.Now().UTC(),
	}, ReturnBody{Summary: "Bounded."})
	if err == nil {
		t.Fatal("unknown envelope was accepted")
	}
	body, marshalErr := json.Marshal(receipt)
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	if receipt.DelegationID != "" || strings.Contains(string(body), untrusted) {
		t.Fatalf("unknown envelope echoed untrusted packet ID: %s", body)
	}
}

func TestPilotRejectsStaleExecutionEnvelopeBeforeCompletion(t *testing.T) {
	now := time.Date(2026, 7, 25, 18, 0, 0, 0, time.UTC)
	pilot := newTestPilot(t, "claude")
	pilot.now = func() time.Time { return now }
	packetIssuedAt := now.Add(-10 * time.Minute)
	pilot.dispatcher.now = func() time.Time { return packetIssuedAt }
	dispatch, receipt, err := pilot.Delegate(Intent{
		WorkspaceID: "alpha", Objective: "Assess one source.", TTL: time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	pilot.dispatcher.now = pilot.now
	executor := newTestExecutor(t, "claude", "workspace-agent-alpha", "workspace-alpha-cap", now.Add(-maxExecutionEnvelopeAge-time.Second))
	body := ReturnBody{Summary: "Bounded result."}
	envelope, err := executor.SealReturn(dispatch, body)
	if err != nil {
		t.Fatal(err)
	}
	failed, err := pilot.Return(envelope, body)
	if err == nil || failed.FailureCode != "envelope_denied" {
		t.Fatalf("stale envelope accepted: %#v, %v", failed, err)
	}
	current, ok := pilot.Inspect(receipt.DelegationID)
	if !ok || current.State != StateDelegated {
		t.Fatalf("stale envelope closed the branch: %#v, present=%t", current, ok)
	}
}

func TestPilotRejectsMultipleErrandHelpersAndUnavailableTargetBeforeDispatch(t *testing.T) {
	dispatcher := newTestDispatcherForRuntime(t, "claude")
	if _, err := NewPilot(dispatcher, []Instance{
		{AgentID: "errand-helper", Role: "errand_helper", ScopeKind: "errand", ScopeID: "pilot", ParentAgentID: "maestro", Available: true},
		{AgentID: "errand-helper-two", Role: "errand_helper", ScopeKind: "errand", ScopeID: "pilot", ParentAgentID: "maestro", Available: true},
	}); err == nil {
		t.Fatal("multiple errand helpers were accepted")
	}

	pilot, err := NewPilot(dispatcher, []Instance{
		{AgentID: "workspace-agent-alpha", Role: "workspace_agent", ScopeKind: "workspace", ScopeID: "alpha", ParentAgentID: "maestro", Available: false},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, receipt, err := pilot.Delegate(Intent{
		WorkspaceID: "alpha", Objective: "Assess one source.", TTL: time.Hour,
	})
	if err == nil || receipt.State != StateUnavailable || receipt.FailureCode != "target_unavailable" {
		t.Fatalf("unavailable target = %#v, %v", receipt, err)
	}
	if snapshot := dispatcher.gate.Snapshot(); snapshot.BranchID != "" {
		t.Fatalf("unavailable target opened a branch: %#v", snapshot)
	}
}

func TestPilotResultStateIsExplicitlyProcessLocal(t *testing.T) {
	now := time.Date(2026, 7, 25, 18, 0, 0, 0, time.UTC)
	pilot := newTestPilot(t, "claude")
	pilot.now = func() time.Time { return now }
	pilot.dispatcher.now = pilot.now
	dispatch, _, err := pilot.Delegate(Intent{WorkspaceID: "alpha", Objective: "Assess one source.", TTL: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	executor := newTestExecutor(t, "claude", "workspace-agent-alpha", "workspace-alpha-cap", now)
	body := ReturnBody{Summary: "Bounded result."}
	envelope, err := executor.SealReturn(dispatch, body)
	if err != nil {
		t.Fatal(err)
	}

	restarted, err := NewPilot(pilot.dispatcher, testInstances())
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := restarted.Return(envelope, body)
	if err == nil || receipt.FailureCode != "delegation_unavailable_after_restart" {
		t.Fatalf("process-local state was presented as recovered: %#v, %v", receipt, err)
	}
}

func TestPilotSelectsRegisteredWorkspaceAgentInstance(t *testing.T) {
	dataRoot := t.TempDir()
	if _, err := workspaceagent.Initialize(dataRoot, "alpha"); err != nil {
		t.Fatal(err)
	}
	status, err := agentscaffold.Scaffold(dataRoot, agentscaffold.WorkspaceRequest("alpha"))
	if err != nil {
		t.Fatal(err)
	}
	instance := InstanceFromScaffold(status)
	pilot, err := NewPilot(newTestDispatcherForRuntime(t, "claude"), []Instance{instance})
	if err != nil {
		t.Fatal(err)
	}
	_, receipt, err := pilot.Delegate(Intent{
		WorkspaceID: "alpha", Objective: "Assess one bounded source.",
		Pointers: []string{"bcgos://workspace/alpha/dossier/brief.json"}, TTL: time.Hour,
	})
	if err != nil || receipt.TargetAgentID != instance.AgentID || receipt.State != StateDelegated {
		t.Fatalf("registered workspace delegation = %#v, %v", receipt, err)
	}
}

func newTestPilot(t *testing.T, runtimeName string) *Pilot {
	t.Helper()
	pilot, err := NewPilot(newTestDispatcherForRuntime(t, runtimeName), testInstances())
	if err != nil {
		t.Fatal(err)
	}
	return pilot
}

func testInstances() []Instance {
	return []Instance{
		{AgentID: "workspace-agent-alpha", Role: "workspace_agent", ScopeKind: "workspace", ScopeID: "alpha", ParentAgentID: "maestro", Available: true},
		{AgentID: "errand-helper", Role: "errand_helper", ScopeKind: "errand", ScopeID: "pilot", ParentAgentID: "maestro", Available: true},
		{AgentID: "walter", Role: "reviewer", ScopeKind: "review", ScopeID: "review", ParentAgentID: "maestro", Available: true},
	}
}

func newTestDispatcherForRuntime(t *testing.T, runtimeName string) *Dispatcher {
	t.Helper()
	catalog, err := agentcatalog.ParseFile("../../bundles/base/agents/catalog.json")
	if err != nil {
		t.Fatal(err)
	}
	store, err := agentorchestration.NewStateStore("recovery-cap")
	if err != nil {
		t.Fatal(err)
	}
	grants := []agentorchestration.Authorization{
		{AgentID: "maestro", Role: "hub", ScopeKind: "control", Capability: "maestro-cap"},
		{AgentID: "walter", Role: "reviewer", Scope: "review", ScopeKind: "review", Capability: "walter-cap"},
		{AgentID: "workspace-agent-alpha", Role: "workspace_agent", Scope: "alpha", ScopeKind: "workspace", Capability: "workspace-alpha-cap"},
		{AgentID: "errand-helper", Role: "errand_helper", Scope: "pilot", ScopeKind: "errand", Capability: "errand-helper-cap", Tools: []agentorchestration.ToolGrant{
			{Tool: errandTool, Operation: string(ErrandCreateEphemeralNote), ResourcePrefix: "bcgos://errand/pilot/"},
			{Tool: errandTool, Operation: string(ErrandDeleteEphemeralNote), ResourcePrefix: "bcgos://errand/pilot/"},
		}},
		{AgentID: "practice-insurance", Role: "practice_agent", Scope: "insurance", ScopeKind: "practice", Capability: "practice-insurance-cap"},
	}
	adapter, err := agentorchestration.NewAdapter(runtimeName, catalog, grants, store)
	if err != nil {
		t.Fatal(err)
	}
	dispatcher, err := New(adapter, "packet-signing-capability", map[string]string{
		"maestro": "maestro-cap", "walter": "walter-cap", "workspace-agent-alpha": "workspace-alpha-cap",
		"errand-helper": "errand-helper-cap", "practice-insurance": "practice-insurance-cap",
	})
	if err != nil {
		t.Fatal(err)
	}
	return dispatcher
}

func newTestExecutor(t *testing.T, runtimeName, targetID, capability string, now time.Time) *Executor {
	t.Helper()
	executor, err := NewExecutor(runtimeName, targetID, capability)
	if err != nil {
		t.Fatal(err)
	}
	executor.now = func() time.Time { return now }
	return executor
}

func resignEnvelopeForTest(t *testing.T, envelope *ExecutionEnvelope, capability string) {
	t.Helper()
	signature, err := signExecutionEnvelope(*envelope, capability)
	if err != nil {
		t.Fatal(err)
	}
	envelope.Signature = signature
}
