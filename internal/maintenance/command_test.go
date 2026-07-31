package maintenance

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
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
	failed := Receipt{SchemaVersion: CommandSchemaVersion, AttemptID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", OccurrenceDigest: testOccurrenceDigest, CommandID: "cmd-retry-1", JobID: "memory-daily", WorkspaceID: "workspace-1", Trigger: TriggerDaily, State: ReceiptFailed, RecordedAt: now, Deadline: now.Add(time.Minute)}
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

func TestReceiptStoreRejectsEphemeralBusyResult(t *testing.T) {
	now := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	store := Store{Root: t.TempDir()}
	receipt := Receipt{SchemaVersion: CommandSchemaVersion, AttemptID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", OccurrenceDigest: testOccurrenceDigest, CommandID: "cmd-busy-1", JobID: "memory-daily", WorkspaceID: "workspace-1", Trigger: TriggerDaily, State: ReceiptBusy, RecordedAt: now, Deadline: now.Add(time.Minute)}
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
	receipt := Receipt{SchemaVersion: CommandSchemaVersion, AttemptID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", OccurrenceDigest: testOccurrenceDigest, CommandID: "cmd-store-1", JobID: "memory-daily", WorkspaceID: "workspace-1", Trigger: TriggerDaily, State: ReceiptSucceeded, RecordedAt: now, Deadline: now.Add(time.Minute)}
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
	receipt := Receipt{SchemaVersion: CommandSchemaVersion, AttemptID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", OccurrenceDigest: testOccurrenceDigest, CommandID: "cmd-monthly-1", JobID: "darwin-structural-evolution-proposal", WorkspaceID: "workspace-1", Trigger: TriggerMonthly, State: ReceiptProposalEmitted, RecordedAt: now, Deadline: now.Add(time.Minute), ProposalOnly: true, ProposalCount: 2, ProposalDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}
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
	receipt := Receipt{SchemaVersion: CommandSchemaVersion, AttemptID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", OccurrenceDigest: testOccurrenceDigest, CommandID: "cmd-store-1", JobID: "memory-daily", WorkspaceID: "workspace-1", Trigger: TriggerDaily, State: ReceiptSucceeded, RecordedAt: now, Deadline: now.Add(time.Minute)}
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
