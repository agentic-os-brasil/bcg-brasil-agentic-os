package maintenance

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

const testOccurrenceDigest = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func testCommand(now time.Time) Command {
	return Command{
		SchemaVersion: CommandSchemaVersion,
		CommandID:     "cmd-daily-1",
		JobID:         "memory-daily",
		WorkspaceID:   "workspace-1",
		Trigger:       TriggerDaily,
		ScheduledFor:  now,
		RequestedAt:   now,
		Deadline:      now.Add(time.Minute),
	}
}

func TestReceiptStoreKeepsRetriesAsSeparateAttempts(t *testing.T) {
	now := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	store := Store{Root: t.TempDir()}
	failed := Receipt{SchemaVersion: CommandSchemaVersion, AttemptID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", OccurrenceDigest: testOccurrenceDigest, CommandID: "cmd-retry-1", JobID: "memory-daily", WorkspaceID: "workspace-1", Trigger: TriggerDaily, State: ReceiptFailed, RecordedAt: now, Deadline: now.Add(time.Minute), ReasonCode: ReasonHandlerFailure}
	succeeded := failed
	succeeded.AttemptID = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	succeeded.State = ReceiptSucceeded
	if err := store.AppendReceipt(failed); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendReceipt(succeeded); err != nil {
		t.Fatal(err)
	}
	receipts, err := store.Receipts("workspace-1", "memory-daily")
	if err != nil || len(receipts) != 2 {
		t.Fatalf("retry receipts=%#v err=%v", receipts, err)
	}
}

func TestReceiptStoreAllowsRecoveryAfterPublishedSuccess(t *testing.T) {
	now := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	store := Store{Root: t.TempDir()}
	success := Receipt{SchemaVersion: CommandSchemaVersion, AttemptID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", OccurrenceDigest: testOccurrenceDigest, CommandID: "cmd-release-1", JobID: "memory-daily", WorkspaceID: "workspace-1", Trigger: TriggerDaily, State: ReceiptSucceeded, RecordedAt: now, Deadline: now.Add(time.Minute), ReasonCode: ReasonCompleted}
	recovery := success
	recovery.AttemptID = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	recovery.State = ReceiptRecoveryRequired
	recovery.ReasonCode = ReasonRecoveryRequired
	if err := store.AppendReceipt(success); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendReceipt(recovery); err != nil {
		t.Fatalf("recovery after success was rejected: %v", err)
	}
	receipts, err := store.Receipts("workspace-1", "memory-daily")
	if err != nil || len(receipts) != 2 {
		t.Fatalf("recovery chain receipts=%#v err=%v", receipts, err)
	}
}

func TestReceiptStoreAllowsRecoveryIntentAfterPublishedSuccess(t *testing.T) {
	now := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	intent, err := NewRecoveryIntentReceipt("workspace-1", "memory-daily", TriggerDaily, now, now, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if err != nil {
		t.Fatal(err)
	}
	store := Store{Root: t.TempDir()}
	success := Receipt{SchemaVersion: CommandSchemaVersion, AttemptID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", OccurrenceDigest: intent.OccurrenceDigest, CommandID: "cmd-intent-1", JobID: "memory-daily", WorkspaceID: "workspace-1", Trigger: TriggerDaily, State: ReceiptSucceeded, RecordedAt: now, Deadline: now.Add(time.Minute), ReasonCode: ReasonCompleted}
	if err := store.AppendReceipt(success); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendReceipt(intent); err != nil {
		t.Fatalf("recovery intent after success was rejected: %v", err)
	}
	receipts, err := store.Receipts("workspace-1", "memory-daily")
	if err != nil || len(receipts) != 2 {
		t.Fatalf("recovery intent chain receipts=%#v err=%v", receipts, err)
	}
}

func TestMonthlyRecoveryReceiptsRetainProposalOnlyBinding(t *testing.T) {
	now := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	intent, err := NewRecoveryIntentReceipt("workspace-1", "darwin-structural-evolution-proposal", TriggerMonthly, now, now, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if err != nil || !intent.ProposalOnly || intent.Validate() != nil {
		t.Fatalf("monthly recovery intent=%#v err=%v validate=%v", intent, err, intent.Validate())
	}
	completed, err := NewRecoveryOutcomeReceipt(intent, now, "completed", ReasonRecoveryCompleted)
	if err != nil || !completed.ProposalOnly || completed.Validate() != nil {
		t.Fatalf("monthly recovery outcome=%#v err=%v validate=%v", completed, err, completed.Validate())
	}
}

func TestReceiptStoreRejectsRecoveryOutcomeWithoutMatchingIntent(t *testing.T) {
	now := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	intent, err := NewRecoveryIntentReceipt("workspace-1", "memory-daily", TriggerDaily, now, now, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if err != nil {
		t.Fatal(err)
	}
	outcome, err := NewRecoveryOutcomeReceipt(intent, now, "completed", ReasonRecoveryCompleted)
	if err != nil {
		t.Fatal(err)
	}
	store := Store{Root: t.TempDir()}
	if err := store.AppendReceipt(outcome); err == nil {
		t.Fatal("recovery outcome without an intent was accepted")
	}
	if err := store.AppendReceipt(intent); err != nil {
		t.Fatal(err)
	}
	forged := outcome
	forged.AttemptID = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	forged.FenceTokenDigest = digest("different-fence")
	if err := store.AppendReceipt(forged); err == nil {
		t.Fatal("recovery outcome with a forged fence binding was accepted")
	}
	forged = outcome
	forged.AttemptID = "cccccccccccccccccccccccccccccccc"
	forged.OccurrenceDigest = digest("different-occurrence")
	if err := store.AppendReceipt(forged); err == nil {
		t.Fatal("recovery outcome with a forged occurrence was accepted")
	}
	if err := store.AppendReceipt(outcome); err != nil {
		t.Fatalf("matching recovery outcome was rejected: %v", err)
	}
	if err := store.AppendReceipt(outcome); err != nil {
		t.Fatalf("matching recovery outcome was not idempotent: %v", err)
	}
}

func TestReceiptStoreRejectsEphemeralBusyResult(t *testing.T) {
	now := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	store := Store{Root: t.TempDir()}
	receipt := Receipt{SchemaVersion: CommandSchemaVersion, AttemptID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", OccurrenceDigest: testOccurrenceDigest, CommandID: "cmd-busy-1", JobID: "memory-daily", WorkspaceID: "workspace-1", Trigger: TriggerDaily, State: ReceiptBusy, RecordedAt: now, Deadline: now.Add(time.Minute), ReasonCode: ReasonLeaseBusy}
	if err := store.AppendReceipt(receipt); err == nil {
		t.Fatal("ephemeral busy result was persisted")
	}
}

func TestReceiptStoreRejectsSymlinkedWorkspaceAncestor(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink setup is host-dependent on Windows")
	}
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "workspaces")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	store := Store{Root: root}
	now := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	receipt := Receipt{SchemaVersion: CommandSchemaVersion, AttemptID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", OccurrenceDigest: testOccurrenceDigest, CommandID: "cmd-store-1", JobID: "memory-daily", WorkspaceID: "workspace-1", Trigger: TriggerDaily, State: ReceiptSucceeded, RecordedAt: now, Deadline: now.Add(time.Minute), ReasonCode: ReasonCompleted}
	if err := store.AppendReceipt(receipt); err == nil {
		t.Fatal("receipt store followed a symlinked workspace ancestor")
	}
	entries, err := os.ReadDir(outside)
	if err != nil || len(entries) != 0 {
		t.Fatalf("receipt store wrote outside its root: entries=%#v err=%v", entries, err)
	}
}

func TestCommandRequiresExplicitBoundedDeadline(t *testing.T) {
	now := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	command := testCommand(now)
	if err := command.Validate(now); err != nil {
		t.Fatal(err)
	}
	command.Deadline = now.Add(16 * time.Minute)
	if err := command.Validate(now); err == nil {
		t.Fatal("unbounded command deadline was accepted")
	}
	command = testCommand(now)
	command.Deadline = now
	if err := command.Validate(now); err == nil {
		t.Fatal("expired command deadline was accepted")
	}
}

func TestContinuousGateMapsOnlyToQualifiedEventJobs(t *testing.T) {
	catalog := Catalog{SchemaVersion: 1, CatalogState: CatalogOnly, Jobs: []Job{{
		ID: "wiki-incremental-sync", Category: "wiki", Trigger: "event", Executor: "deterministic", Scope: "workspace",
		Availability: Available, DefaultEnabled: true, Unattended: "deterministic_only",
		Writes: []string{"wiki_private"}, SuccessBoundary: "watermark committed",
	}}}
	now := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	command := testCommand(now)
	command.CommandID = "cmd-event-1"
	command.JobID = "wiki-incremental-sync"
	command.Trigger = TriggerContinuous
	command.EventID = "source-change-1"
	decision, err := Gate(catalog, command, now)
	if err != nil || decision.State != "accepted" || len(decision.PlannedJobs) != 1 {
		t.Fatalf("decision=%#v err=%v", decision, err)
	}
	catalog.Jobs[0].Availability = Unavailable
	catalog.Jobs[0].AvailabilityReason = "not installed"
	decision, err = Gate(catalog, command, now)
	if err != nil || decision.State != "unavailable" || len(decision.PlannedJobs) != 0 {
		t.Fatalf("unavailable decision=%#v err=%v", decision, err)
	}
}

func TestContinuousGateRejectsMissingEventID(t *testing.T) {
	now := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	command := testCommand(now)
	command.CommandID = "cmd-event-invalid"
	command.JobID = "wiki-incremental-sync"
	command.Trigger = TriggerContinuous
	if _, err := Gate(Catalog{}, command, now); err == nil {
		t.Fatal("continuous event without event ID was accepted")
	}
}

func TestMaintenanceReceiptRequiresEventIdentityParity(t *testing.T) {
	now := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	command := Command{SchemaVersion: CommandSchemaVersion, CommandID: "cmd-event", JobID: "wiki-incremental-sync", WorkspaceID: "workspace-1", Trigger: TriggerEvent, EventID: "source-change-1", ScheduledFor: now, RequestedAt: now, Deadline: now.Add(time.Minute)}
	if err := command.Validate(now); err != nil {
		t.Fatal(err)
	}
	if err := validatePublishedSchemaDocument(t, "maintenance-command.schema.json", command); err != nil {
		t.Fatalf("event command does not satisfy its published schema: %v", err)
	}
	receipt := Receipt{SchemaVersion: CommandSchemaVersion, AttemptID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", OccurrenceDigest: testOccurrenceDigest, CommandID: "cmd-event-receipt", JobID: "wiki-incremental-sync", WorkspaceID: "workspace-1", Trigger: TriggerEvent, State: ReceiptUnavailable, RecordedAt: now, Deadline: now.Add(time.Minute), ReasonCode: ReasonHandlerUnavailable}
	if err := receipt.Validate(); err == nil {
		t.Fatal("event receipt without event ID was accepted")
	}
	receipt.EventID = "source-change-1"
	if err := receipt.Validate(); err != nil {
		t.Fatalf("event receipt with event ID was rejected: %v", err)
	}
	if err := validatePublishedSchemaDocument(t, "maintenance-receipt.schema.json", receipt); err != nil {
		t.Fatalf("event receipt does not satisfy its published schema: %v", err)
	}
	receipt.Trigger = TriggerDaily
	if err := receipt.Validate(); err == nil {
		t.Fatal("scheduled receipt carrying event ID was accepted")
	}
}

func TestExecutionAuthorityEnforcesCatalogQualificationAndAttendance(t *testing.T) {
	now := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	command := Command{
		SchemaVersion: CommandSchemaVersion, CommandID: "cmd-monthly-authority",
		JobID: "darwin-structural-evolution-proposal", WorkspaceID: "workspace-1",
		Trigger: TriggerMonthly, ScheduledFor: now, RequestedAt: now,
		Deadline: now.Add(time.Minute), ProposalOnly: true,
	}
	occurrence := OccurrenceAuthorization{WorkspaceID: command.WorkspaceID, JobID: command.JobID, Trigger: command.Trigger, ScheduledFor: command.ScheduledFor}
	base, err := LoadFile("../../bundles/base/runtime/maintenance.json")
	if err != nil {
		t.Fatal(err)
	}
	baseAuthority, err := NewExecutionAuthority(base, []OccurrenceAuthorization{occurrence}, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := baseAuthority.Authorize(command, now); err == nil {
		t.Fatal("shipped unavailable catalog authorized Darwin execution")
	}

	qualified := qualifiedCatalogForTest(t, command.JobID)
	unattended, err := NewExecutionAuthority(qualified, []OccurrenceAuthorization{occurrence}, []string{command.JobID}, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := unattended.Authorize(command, now); err == nil {
		t.Fatal("never-unattended proposal ran without attended authority")
	}
	attended, err := NewExecutionAuthority(qualified, []OccurrenceAuthorization{occurrence}, []string{command.JobID}, true)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := attended.Authorize(command, now); err != nil || got.JobID != command.JobID || !got.ScheduledFor.Equal(command.ScheduledFor) {
		t.Fatalf("qualified authority occurrence=%#v err=%v", got, err)
	}
	qualificationDigest := ""
	for _, job := range qualified.Jobs {
		if job.ID == command.JobID {
			qualificationDigest = job.QualificationDigest
			break
		}
	}
	preauthorized, err := NewPreauthorizedLocalExecutionAuthority(qualified, []OccurrenceAuthorization{occurrence}, map[string]string{command.JobID: qualificationDigest}, []string{command.JobID})
	if err != nil {
		t.Fatal(err)
	}
	if got, err := preauthorized.Authorize(command, now); err == nil || got.JobID != "" {
		t.Fatalf("preauthorized authority bypassed never-unattended policy: occurrence=%#v err=%v", got, err)
	}
	otherWorkspace := command
	otherWorkspace.WorkspaceID = "workspace-2"
	if _, err := attended.Authorize(otherWorkspace, now); err == nil {
		t.Fatal("authority grant crossed workspace boundary")
	}
}

func qualifiedCatalogForTest(t *testing.T, jobIDs ...string) Catalog {
	t.Helper()
	catalog, err := LoadFile("../../bundles/base/runtime/maintenance.json")
	if err != nil {
		t.Fatal(err)
	}
	catalog.CatalogState = RuntimeQualified
	selected := make(map[string]bool, len(jobIDs))
	for _, jobID := range jobIDs {
		selected[jobID] = true
	}
	for index := range catalog.Jobs {
		if !selected[catalog.Jobs[index].ID] {
			continue
		}
		catalog.Jobs[index].Availability = Available
		catalog.Jobs[index].AvailabilityReason = ""
		catalog.Jobs[index].QualificationDigest = testOccurrenceDigest
	}
	if err := catalog.Validate(); err != nil {
		t.Fatal(err)
	}
	return catalog
}

func TestProposalReceiptCannotClaimApplication(t *testing.T) {
	now := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	receipt := Receipt{SchemaVersion: CommandSchemaVersion, AttemptID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", OccurrenceDigest: testOccurrenceDigest, CommandID: "cmd-monthly-1", JobID: "darwin-structural-evolution-proposal", WorkspaceID: "workspace-1", Trigger: TriggerMonthly, State: ReceiptProposalEmitted, RecordedAt: now, Deadline: now.Add(time.Minute), ProposalOnly: true, ProposalCount: 2, ProposalDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ProposalArtifactID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ReasonCode: ReasonProposalEmitted}
	if err := receipt.Validate(); err != nil {
		t.Fatal(err)
	}
	receipt.State = ReceiptSucceeded
	if err := receipt.Validate(); err == nil {
		t.Fatal("proposal-only receipt claimed applied success")
	}
}

func TestReceiptStoreKeepsOnlyTypedMetadata(t *testing.T) {
	now := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	store := Store{Root: t.TempDir()}
	receipt := Receipt{SchemaVersion: CommandSchemaVersion, AttemptID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", OccurrenceDigest: testOccurrenceDigest, CommandID: "cmd-store-1", JobID: "memory-daily", WorkspaceID: "workspace-1", Trigger: TriggerDaily, State: ReceiptSucceeded, RecordedAt: now, Deadline: now.Add(time.Minute), ReasonCode: ReasonCompleted}
	if err := store.AppendReceipt(receipt); err != nil {
		t.Fatal(err)
	}
	got, err := store.Receipts("workspace-1", "memory-daily")
	if err != nil || len(got) != 1 || got[0].CommandID != receipt.CommandID {
		t.Fatalf("receipts=%#v err=%v", got, err)
	}
	if err := store.AppendReceipt(receipt); err != nil {
		t.Fatalf("identical duplicate receipt was not idempotent: %v", err)
	}
}

func TestPublishedCommandAndReceiptSchemasAreStrictlyPresent(t *testing.T) {
	for _, name := range []string{"maintenance-command.schema.json", "maintenance-receipt.schema.json"} {
		body, err := os.ReadFile(filepath.Join("../../schemas", name))
		if err != nil {
			t.Fatal(err)
		}
		var schema map[string]any
		if err := json.Unmarshal(body, &schema); err != nil {
			t.Fatal(err)
		}
		if schema["additionalProperties"] != false || schema["$schema"] != "https://json-schema.org/draft/2020-12/schema" {
			t.Fatalf("schema %s is not strict: %#v", name, schema)
		}
	}
}

func TestPublishedSchemasAcceptWalterWeeklyProposalOnly(t *testing.T) {
	now := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	command := Command{
		SchemaVersion: CommandSchemaVersion, CommandID: "cmd-walter-weekly",
		JobID: WalterSelfReviewWeeklyJobID, WorkspaceID: "maestro-system",
		Trigger: TriggerWeekly, ScheduledFor: now, RequestedAt: now,
		Deadline: now.Add(time.Minute), ProposalOnly: true,
	}
	if err := command.Validate(now); err != nil {
		t.Fatal(err)
	}
	if err := validatePublishedSchemaDocument(t, "maintenance-command.schema.json", command); err != nil {
		t.Fatalf("Walter weekly command does not satisfy its published schema: %v", err)
	}
	receipt := Receipt{
		SchemaVersion: CommandSchemaVersion, AttemptID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		OccurrenceDigest: testOccurrenceDigest, CommandID: command.CommandID,
		JobID: command.JobID, WorkspaceID: command.WorkspaceID, Trigger: command.Trigger,
		State: ReceiptProposalEmitted, RecordedAt: now, Deadline: command.Deadline,
		ProposalOnly: true, ProposalCount: 1, ProposalDigest: testOccurrenceDigest,
		ProposalArtifactID: testOccurrenceDigest, ReasonCode: ReasonProposalEmitted,
	}
	if err := receipt.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := validatePublishedSchemaDocument(t, "maintenance-receipt.schema.json", receipt); err != nil {
		t.Fatalf("Walter weekly receipt does not satisfy its published schema: %v", err)
	}

	command.Trigger = TriggerMonthly
	if err := command.Validate(now); err == nil {
		t.Fatal("Go command validator accepted Walter on the Darwin monthly cadence")
	}
	if err := validatePublishedSchemaDocument(t, "maintenance-command.schema.json", command); err == nil {
		t.Fatal("published command schema accepted Walter on the Darwin monthly cadence")
	}
	receipt.Trigger = TriggerMonthly
	if err := receipt.Validate(); err == nil {
		t.Fatal("Go receipt validator accepted Walter on the Darwin monthly cadence")
	}
	if err := validatePublishedSchemaDocument(t, "maintenance-receipt.schema.json", receipt); err == nil {
		t.Fatal("published receipt schema accepted Walter on the Darwin monthly cadence")
	}
}

func validatePublishedSchemaDocument(t *testing.T, name string, value any) error {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("../../schemas", name))
	if err != nil {
		return err
	}
	var raw any
	if err := json.Unmarshal(body, &raw); err != nil {
		return err
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource(name, raw); err != nil {
		return err
	}
	schema, err := compiler.Compile(name)
	if err != nil {
		return err
	}
	documentBody, err := json.Marshal(value)
	if err != nil {
		return err
	}
	var document any
	if err := json.Unmarshal(documentBody, &document); err != nil {
		return err
	}
	return schema.Validate(document)
}
