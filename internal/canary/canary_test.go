package canary

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestCanaryReportAggregatesOnlyPilotOutcomeBuckets(t *testing.T) {
	store := Store{Root: t.TempDir()}
	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	receipts := []Receipt{
		{RecordedAt: now, Event: EventFirstValue, Outcome: OutcomeSucceeded, Duration: DurationUnderTenMinutes},
		{RecordedAt: now.Add(time.Nanosecond), Event: EventGoalResume, Outcome: OutcomeSucceeded},
		{RecordedAt: now.Add(2 * time.Nanosecond), Event: EventManualIntervention, Outcome: OutcomeNeutral, Count: CountTwoToThree},
		{RecordedAt: now.Add(3 * time.Nanosecond), Event: EventCapabilityFailure, Outcome: OutcomeFailed, Capability: CapabilityLongRunningGoals},
		{RecordedAt: now.Add(4 * time.Nanosecond), Event: EventUpdate, Outcome: OutcomeFailed},
	}
	for _, receipt := range receipts {
		if err := store.Append(receipt); err != nil {
			t.Fatal(err)
		}
	}
	report, err := store.Report()
	if err != nil {
		t.Fatal(err)
	}
	if report.ReceiptCount != 5 || !hasDuration(report.FirstValue, DurationUnderTenMinutes, 1) || !hasOutcome(report.Resume, OutcomeSucceeded, 1) || !hasLifecycle(report.Lifecycle, EventUpdate, OutcomeFailed, 1) || !hasCount(report.ManualInterventions, CountTwoToThree, 1) || !hasCapability(report.CapabilityFailures, CapabilityLongRunningGoals, 1) {
		t.Fatalf("report = %#v", report)
	}
}

func TestCanaryStoreRejectsContentBearingUnknownReceiptField(t *testing.T) {
	store := Store{Root: t.TempDir()}
	if err := os.MkdirAll(filepath.Join(store.Root, "receipts"), 0o700); err != nil {
		t.Fatal(err)
	}
	secret := "client-secret-CANARY"
	if err := os.WriteFile(filepath.Join(store.Root, "receipts", "invalid.json"), []byte(`{"schema_version":1,"recorded_at":"2026-07-25T12:00:00Z","event":"first_value","outcome":"succeeded","duration":"under_ten_minutes","workspace_note":"`+secret+`"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Report(); err == nil {
		t.Fatal("content-bearing receipt was accepted")
	}
}

func TestCanaryStoreKeepsDistinctReceiptsAtTheSameTimestamp(t *testing.T) {
	store := Store{Root: t.TempDir()}
	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	for range 2 {
		if err := store.Append(Receipt{RecordedAt: now, Event: EventManualIntervention, Outcome: OutcomeNeutral, Count: CountOnce}); err != nil {
			t.Fatal(err)
		}
	}
	report, err := store.Report()
	if err != nil || !hasCount(report.ManualInterventions, CountOnce, 2) {
		t.Fatalf("report = %#v, err = %v", report, err)
	}
}

func TestCanaryStoreRejectsUnsupportedSchemaVersion(t *testing.T) {
	store := Store{Root: t.TempDir()}
	err := store.Append(Receipt{SchemaVersion: 2, RecordedAt: time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC), Event: EventFirstValue, Outcome: OutcomeSucceeded, Duration: DurationUnderFiveMinutes})
	if err == nil {
		t.Fatal("unsupported schema version was accepted")
	}
}

func TestCanaryReportAndReceiptTypesCannotLeakClientCanary(t *testing.T) {
	store := Store{Root: t.TempDir()}
	if err := store.Append(Receipt{RecordedAt: time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC), Event: EventCapabilityFailure, Outcome: OutcomeFailed, Capability: CapabilityWorkspaceAgent}); err != nil {
		t.Fatal(err)
	}
	report, err := store.Report()
	if err != nil {
		t.Fatal(err)
	}
	if encoded := report.String(); strings.Contains(encoded, "client-secret-CANARY") || strings.Contains(encoded, "workspace-path-CANARY") {
		t.Fatalf("report leaked private content: %s", encoded)
	}
}

func TestCanaryReceiptHasOnlyTheClosedMetadataSurface(t *testing.T) {
	type expectedField struct {
		name   string
		typeOf reflect.Type
	}
	expected := []expectedField{
		{name: "SchemaVersion", typeOf: reflect.TypeOf(0)},
		{name: "RecordedAt", typeOf: reflect.TypeOf(time.Time{})},
		{name: "Event", typeOf: reflect.TypeOf(Event(""))},
		{name: "Outcome", typeOf: reflect.TypeOf(Outcome(""))},
		{name: "Duration", typeOf: reflect.TypeOf(DurationBucket(""))},
		{name: "Count", typeOf: reflect.TypeOf(CountBucket(""))},
		{name: "Capability", typeOf: reflect.TypeOf(Capability(""))},
	}
	typeOfReceipt := reflect.TypeOf(Receipt{})
	if typeOfReceipt.NumField() != len(expected) {
		t.Fatalf("receipt field count = %d", typeOfReceipt.NumField())
	}
	for index, want := range expected {
		field := typeOfReceipt.Field(index)
		if field.Name != want.name || field.Type != want.typeOf {
			t.Fatalf("receipt field[%d] = %s %s, want %s %s", index, field.Name, field.Type, want.name, want.typeOf)
		}
	}
}

func TestPublishedCanarySchemaIsRecognized(t *testing.T) {
	if err := ValidateSchemaFile("../../schemas/canary-report.schema.json"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateReceiptSchemaFile("../../schemas/canary-receipt.schema.json"); err != nil {
		t.Fatal(err)
	}
}

func hasDuration(values []DurationCount, bucket DurationBucket, count int) bool {
	for _, value := range values {
		if value.Duration == bucket && value.Count == count {
			return true
		}
	}
	return false
}
func hasOutcome(values []OutcomeCount, outcome Outcome, count int) bool {
	for _, value := range values {
		if value.Outcome == outcome && value.Count == count {
			return true
		}
	}
	return false
}
func hasLifecycle(values []LifecycleCount, event Event, outcome Outcome, count int) bool {
	for _, value := range values {
		if value.Event == event && value.Outcome == outcome && value.Count == count {
			return true
		}
	}
	return false
}
func hasCount(values []CountBucketCount, bucket CountBucket, count int) bool {
	for _, value := range values {
		if value.CountBucket == bucket && value.Count == count {
			return true
		}
	}
	return false
}
func hasCapability(values []CapabilityCount, capability Capability, count int) bool {
	for _, value := range values {
		if value.Capability == capability && value.Count == count {
			return true
		}
	}
	return false
}
