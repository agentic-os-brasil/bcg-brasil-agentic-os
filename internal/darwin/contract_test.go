package darwin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/agentcatalog"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/maintenance"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

func TestPacketAndPlanAreClosedAndDeterministic(t *testing.T) {
	packet := HealthPacket{
		SchemaVersion: SchemaVersion, WindowID: "window-1", Runtime: "claude", Mode: Interactive,
		Observations: []Observation{
			{Code: ObservationStateStale, Severity: SeverityMedium, Count: 4, State: "derived"},
			{Code: ObservationCapabilityUnavailable, Severity: SeverityHigh, Count: 1, State: "native"},
			{Code: ObservationOperatingFriction, Severity: SeverityLow, Count: 20},
			{Code: ObservationContractDrift, Severity: SeverityHigh, Count: 1},
		},
	}
	assessment, err := Plan(packet)
	if err != nil {
		t.Fatal(err)
	}
	if len(assessment.Proposals) != maxActions || assessment.AgentID != AgentID || assessment.Emoji != Emoji {
		t.Fatalf("assessment = %#v", assessment)
	}
	if assessment.Proposals[0].Finding != ObservationCapabilityUnavailable || assessment.Proposals[1].Finding != ObservationContractDrift {
		t.Fatalf("priority ordering = %#v", assessment.Proposals)
	}
	reordered := packet
	reordered.Observations = []Observation{packet.Observations[3], packet.Observations[0], packet.Observations[2], packet.Observations[1]}
	other, err := Plan(reordered)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(assessment, other) {
		t.Fatalf("plan changed with input order\nfirst=%#v\nsecond=%#v", assessment, other)
	}
	invalid := packet
	invalid.Observations = append([]Observation(nil), packet.Observations...)
	invalid.Observations[0].Code = packet.Observations[1].Code
	if err := invalid.Validate(); err == nil {
		t.Fatal("duplicate observation code must fail closed")
	}
	invalid = packet
	invalid.Mode = Mode("headless")
	if err := invalid.Validate(); err == nil {
		t.Fatal("unknown mode must fail closed")
	}
	invalid = packet
	invalid.Observations = append([]Observation(nil), packet.Observations...)
	invalid.Observations[0].State = "prompt body or path"
	if err := invalid.Validate(); err == nil {
		t.Fatal("free-form observation state must fail closed")
	}
}

func TestExecuteUsesSameIdentityForInteractiveAndHeadless(t *testing.T) {
	base := HealthPacket{SchemaVersion: SchemaVersion, WindowID: "window-2", Runtime: "codex", Observations: []Observation{{Code: ObservationStateStale, Severity: SeverityMedium, Count: 1}}}
	var calls []ToolCall
	guard := ToolGuardFunc(func(call ToolCall) error { calls = append(calls, call); return nil })
	invoker := InvokerFunc(func(_ context.Context, call ToolCall, artifact Artifact) (ToolResult, error) {
		if artifact.AgentID != AgentID || artifact.SchemaVersion != SchemaVersion {
			t.Fatalf("artifact identity = %#v", artifact)
		}
		if call.Resource == "" {
			t.Fatal("resource missing")
		}
		return ToolResult{Outcome: OutcomeSucceeded}, nil
	})
	for _, mode := range []Mode{Interactive, HeadlessHousekeeping} {
		packet := base
		packet.Mode = mode
		assessment, err := Plan(packet)
		if err != nil {
			t.Fatal(err)
		}
		receipt, err := Execute(context.Background(), packet, assessment, guard, invoker, func() time.Time { return time.Unix(10, 0) })
		if err != nil || receipt.Outcome != OutcomeSucceeded || receipt.AgentID != AgentID || receipt.Emoji != Emoji || receipt.Mode != mode {
			t.Fatalf("mode=%s receipt=%#v err=%v", mode, receipt, err)
		}
	}
	if len(calls) != 2 {
		t.Fatalf("calls = %#v", calls)
	}
}

func TestExecuteBlocksDeniedOrIrreversibleActionsWithoutInvoking(t *testing.T) {
	packet := HealthPacket{SchemaVersion: SchemaVersion, WindowID: "window-3", Runtime: "claude", Mode: HeadlessHousekeeping, Observations: []Observation{{Code: ObservationStateStale, Severity: SeverityHigh, Count: 1}}}
	assessment, err := Plan(packet)
	if err != nil {
		t.Fatal(err)
	}
	called := false
	receipt, err := Execute(context.Background(), packet, assessment, ToolGuardFunc(func(ToolCall) error { return errors.New("grant denied") }), InvokerFunc(func(context.Context, ToolCall, Artifact) (ToolResult, error) {
		called = true
		return ToolResult{Outcome: OutcomeSucceeded}, nil
	}), time.Now)
	if err != nil || receipt.Outcome != OutcomeBlocked || called || len(receipt.Actions) != 1 || receipt.Actions[0].Outcome != OutcomeBlocked {
		t.Fatalf("receipt=%#v err=%v called=%v", receipt, err, called)
	}

	assessment.Proposals[0].Reversible = false
	_, err = Execute(context.Background(), packet, assessment, ToolGuardFunc(func(ToolCall) error { called = true; return nil }), InvokerFunc(func(context.Context, ToolCall, Artifact) (ToolResult, error) {
		called = true
		return ToolResult{Outcome: OutcomeSucceeded}, nil
	}), time.Now)
	if err == nil || called {
		t.Fatalf("forged irreversible proposal err=%v called=%v", err, called)
	}
}

func TestExecuteRejectsForgedAssessmentIdentityAndPlan(t *testing.T) {
	packet := HealthPacket{SchemaVersion: SchemaVersion, WindowID: "window-forge", Runtime: "claude", Mode: Interactive, Observations: []Observation{{Code: ObservationStateStale, Severity: SeverityLow, Count: 1}}}
	assessment, err := Plan(packet)
	if err != nil {
		t.Fatal(err)
	}
	assessment.DisplayName = "Shadow Darwin"
	if _, err := Execute(context.Background(), packet, assessment, ToolGuardFunc(func(ToolCall) error { return nil }), InvokerFunc(func(context.Context, ToolCall, Artifact) (ToolResult, error) {
		return ToolResult{Outcome: OutcomeSucceeded}, nil
	}), time.Now); err == nil {
		t.Fatal("forged display name must fail closed")
	}
	assessment, _ = Plan(packet)
	assessment.Proposals[0].Action = ActionRecordCapabilityGap
	if _, err := Execute(context.Background(), packet, assessment, ToolGuardFunc(func(ToolCall) error { return nil }), InvokerFunc(func(context.Context, ToolCall, Artifact) (ToolResult, error) {
		return ToolResult{Outcome: OutcomeSucceeded}, nil
	}), time.Now); err == nil {
		t.Fatal("forged remediation action must fail closed")
	}
}

func TestFilesystemInvokerIsScopedAndMetadataOnly(t *testing.T) {
	root := t.TempDir()
	invoker := FilesystemInvoker{Root: root}
	artifact := Artifact{SchemaVersion: SchemaVersion, AgentID: AgentID, WindowID: "window-4", ProposalID: "proposal-1", Finding: ObservationSchedulerMissed, Action: ActionReconcileScheduler}
	result, err := invoker.Invoke(context.Background(), ToolCall{Tool: "filesystem", Operation: "write", Resource: "bcgos://health/maestro-system/derived/proposal-1.json"}, artifact)
	if err != nil || result.Outcome != OutcomeNoAction {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	body, err := os.ReadFile(filepath.Join(root, "derived", "proposal-1.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) == "" {
		t.Fatal("metadata receipt is empty")
	}
	if _, err := invoker.Invoke(context.Background(), ToolCall{Tool: "filesystem", Operation: "write", Resource: "bcgos://health/maestro-system/derived/../escape.json"}, artifact); err == nil {
		t.Fatal("traversal resource must fail closed")
	}
	if _, err := invoker.Invoke(context.Background(), ToolCall{Tool: "shell", Operation: "exec", Resource: "bcgos://health/maestro-system/derived/proposal-1.json"}, artifact); err == nil {
		t.Fatal("non-filesystem action must fail closed")
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "link")); err == nil {
		if _, err := invoker.Invoke(context.Background(), ToolCall{Tool: "filesystem", Operation: "write", Resource: "bcgos://health/maestro-system/link/escaped.json"}, artifact); err == nil {
			t.Fatal("symlink traversal must fail closed")
		}
	}
}

func TestHeadlessExecutorUsesSameDarwinContractAndLeavesFailureRecoverable(t *testing.T) {
	now := time.Now().UTC()
	store := Store{Root: t.TempDir()}
	command := commandForTest(now, HousekeepingJobID, false)
	executor := HousekeepingExecutor{
		Build: HealthPacketBuilderFunc(func(context.Context, scheduler.Occurrence) (HealthPacket, error) {
			return HealthPacket{SchemaVersion: SchemaVersion, WindowID: "window-5", Runtime: "claude", Observations: []Observation{{Code: ObservationStateStale, Severity: SeverityLow, Count: 1}}}, nil
		}),
		Guard: ToolGuardFunc(func(ToolCall) error { return errors.New("blocked by scoped grant") }),
		Invoker: InvokerFunc(func(context.Context, ToolCall, Artifact) (ToolResult, error) {
			t.Fatal("blocked housekeeping must not invoke a tool")
			return ToolResult{}, nil
		}),
		Store: store, CommandStore: maintenance.Store{Root: t.TempDir()},
		Scheduler: scheduler.Store{Root: t.TempDir()}, Authority: authorityForTest(t, command),
		Now: func() time.Time { return now },
	}
	_, err := executor.ExecuteCommand(context.Background(), command)
	if err == nil {
		t.Fatal("blocked housekeeping must remain recoverable")
	}
	receipts, err := store.Receipts()
	if err != nil || len(receipts) != 1 || receipts[0].Mode != HeadlessHousekeeping || receipts[0].AgentID != AgentID || receipts[0].Emoji != Emoji || receipts[0].Outcome != OutcomeBlocked {
		t.Fatalf("receipts=%#v err=%v", receipts, err)
	}
}

func TestHeadlessExecutorForcesHeadlessModeAndPersistsSuccess(t *testing.T) {
	now := time.Now().UTC()
	store := Store{Root: t.TempDir()}
	command := commandForTest(now, HousekeepingJobID, false)
	executor := HousekeepingExecutor{
		Build: HealthPacketBuilderFunc(func(context.Context, scheduler.Occurrence) (HealthPacket, error) {
			return HealthPacket{SchemaVersion: SchemaVersion, WindowID: "window-6", Runtime: "codex", Mode: Interactive, Observations: []Observation{{Code: ObservationStateStale, Severity: SeverityLow, Count: 1}}}, nil
		}),
		Guard: ToolGuardFunc(func(ToolCall) error { return nil }),
		Invoker: InvokerFunc(func(context.Context, ToolCall, Artifact) (ToolResult, error) {
			return ToolResult{Outcome: OutcomeSucceeded}, nil
		}),
		Store: store, CommandStore: maintenance.Store{Root: t.TempDir()},
		Scheduler: scheduler.Store{Root: t.TempDir()}, Authority: authorityForTest(t, command),
		Now: func() time.Time { return now },
	}
	if _, err := executor.ExecuteCommand(context.Background(), command); err != nil {
		t.Fatal(err)
	}
	receipts, err := store.Receipts()
	if err != nil || len(receipts) != 1 || receipts[0].Mode != HeadlessHousekeeping || receipts[0].Outcome != OutcomeSucceeded {
		t.Fatalf("receipts=%#v err=%v", receipts, err)
	}
}

func TestDarwinAuthorizationMatchesCatalogScope(t *testing.T) {
	catalog, err := agentcatalog.ParseFile("../../bundles/base/agents/catalog.json")
	if err != nil {
		t.Fatal(err)
	}
	contract, ok := catalog.ContractForRole(Role)
	if !ok || contract.ToolAccess != "scoped" || contract.MayDelegate {
		t.Fatalf("catalog contract = %#v ok=%v", contract, ok)
	}
	authorization := Authorization("darwin-cap")
	if authorization.AgentID != AgentID || authorization.Role != Role || authorization.Scope != MaintenanceScope || authorization.ScopeKind != ScopeKind || len(authorization.Tools) == 0 {
		t.Fatalf("authorization = %#v", authorization)
	}
	encoded, _ := json.Marshal(authorization)
	if string(encoded) == "" {
		t.Fatal("authorization must remain serializable for adapter evidence")
	}
}

func TestSharedDarwinHousekeepingConformanceFixtureKeepsClaudeAndCodexEquivalent(t *testing.T) {
	body, err := os.ReadFile("../../adapters/conformance/darwin-housekeeping.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		SchemaVersion int    `json:"schema_version"`
		AgentID       string `json:"agent_id"`
		DisplayName   string `json:"display_name"`
		Emoji         string `json:"emoji"`
		Scope         string `json:"scope"`
		ScopeKind     string `json:"scope_kind"`
		Modes         []struct {
			Runtime string `json:"runtime"`
			Mode    Mode   `json:"mode"`
		} `json:"modes"`
		DeniedResources []string `json:"denied_resources"`
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.SchemaVersion != SchemaVersion || fixture.AgentID != AgentID || fixture.DisplayName != DisplayName || fixture.Emoji != Emoji || fixture.Scope != MaintenanceScope || fixture.ScopeKind != ScopeKind || len(fixture.Modes) != 4 || len(fixture.DeniedResources) != 2 {
		t.Fatalf("fixture = %#v", fixture)
	}
	for _, mode := range fixture.Modes {
		packet := HealthPacket{SchemaVersion: SchemaVersion, WindowID: "fixture-" + mode.Runtime + "-" + string(mode.Mode), Runtime: mode.Runtime, Mode: mode.Mode, Observations: []Observation{{Code: ObservationStateStale, Severity: SeverityLow, Count: 1}}}
		assessment, err := Plan(packet)
		if err != nil || assessment.AgentID != AgentID || assessment.Emoji != Emoji || assessment.Mode != mode.Mode {
			t.Fatalf("mode=%#v assessment=%#v err=%v", mode, assessment, err)
		}
	}
	for _, resource := range fixture.DeniedResources {
		if validResource(resource) {
			t.Fatalf("fixture denied resource was accepted: %s", resource)
		}
	}
}
