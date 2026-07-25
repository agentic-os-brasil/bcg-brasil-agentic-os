package federation

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

type recordingBridge struct {
	batches []Batch
	err     error
}

func (bridge *recordingBridge) Submit(_ context.Context, batch Batch) error {
	bridge.batches = append(bridge.batches, batch)
	return bridge.err
}

func TestAutomaticExporterDeliversApprovedQueuedBatch(t *testing.T) {
	now := time.Date(2026, 7, 25, 15, 0, 0, 0, time.UTC)
	store := ExportStore{Root: t.TempDir()}
	enrollment := Enrollment{
		InstallationID:  "0123456789abcdef",
		BridgeEndpoint:  "https://bridge.maestro.example/federation/v1/batches",
		ContractVersion: PilotContractVersion,
		AcceptedAt:      now.Add(-time.Hour),
		AutomaticExport: true,
	}
	if err := store.Enroll(enrollment); err != nil {
		t.Fatal(err)
	}
	batch := validBatch()
	if err := store.Enqueue(batch, now); err != nil {
		t.Fatal(err)
	}
	bridge := &recordingBridge{}
	report, err := store.Flush(context.Background(), bridge, now)
	if err != nil {
		t.Fatal(err)
	}
	if report.Delivered != 1 || report.Retained != 0 {
		t.Fatalf("report = %#v", report)
	}
	if !reflect.DeepEqual(bridge.batches, []Batch{batch}) {
		t.Fatalf("submitted batches = %#v", bridge.batches)
	}
	pending, err := store.Pending()
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Fatalf("pending = %#v", pending)
	}
}

func TestAutomaticExporterRetainsOfflineBatchWithBoundedRetries(t *testing.T) {
	now := time.Date(2026, 7, 25, 15, 0, 0, 0, time.UTC)
	store := enrolledStore(t, now)
	if err := store.Enqueue(validBatch(), now); err != nil {
		t.Fatal(err)
	}
	bridge := &recordingBridge{err: errors.New("network unavailable")}

	for attempt := 1; attempt <= MaximumDeliveryAttempts; attempt++ {
		report, err := store.Flush(context.Background(), bridge, now.Add(time.Duration(attempt)*24*time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		if report.Retained != 1 {
			t.Fatalf("attempt %d report = %#v", attempt, report)
		}
	}
	report, err := store.Flush(context.Background(), bridge, now.Add(48*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(bridge.batches) != MaximumDeliveryAttempts || report.Exhausted != 1 {
		t.Fatalf("submissions = %d, report = %#v", len(bridge.batches), report)
	}
	pending, err := store.Pending()
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].Attempts != MaximumDeliveryAttempts || !pending[0].Exhausted {
		t.Fatalf("pending = %#v", pending)
	}
}

func TestExporterCannotSendBeforeContractualEnrollmentOrAfterRevocation(t *testing.T) {
	now := time.Date(2026, 7, 25, 15, 0, 0, 0, time.UTC)
	store := ExportStore{Root: t.TempDir()}
	if err := store.Enqueue(validBatch(), now); !errors.Is(err, ErrNotEnrolled) {
		t.Fatalf("enqueue error = %v, want ErrNotEnrolled", err)
	}
	store = enrolledStore(t, now)
	if err := store.Revoke(now); err != nil {
		t.Fatal(err)
	}
	if err := store.Enqueue(validBatch(), now); !errors.Is(err, ErrExportRevoked) {
		t.Fatalf("enqueue error = %v, want ErrExportRevoked", err)
	}
}

func TestEnrollmentNeverPersistsWorkspaceIdentityOrGitHubCredentials(t *testing.T) {
	now := time.Date(2026, 7, 25, 15, 0, 0, 0, time.UTC)
	store := enrolledStore(t, now)
	contents, err := store.LocalState()
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"workspace", "github", "token", "secret", "authorization"} {
		if strings.Contains(strings.ToLower(contents), forbidden) {
			t.Fatalf("local enrollment state contains forbidden term %q: %s", forbidden, contents)
		}
	}
}

func TestAutomaticConfiguredFlushUsesOnlyTheEnrolledHTTPSBridge(t *testing.T) {
	now := time.Date(2026, 7, 25, 15, 0, 0, 0, time.UTC)
	requests := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests++
		writer.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()
	store := ExportStore{Root: t.TempDir()}
	if err := store.Enroll(Enrollment{
		InstallationID:  "0123456789abcdef",
		BridgeEndpoint:  server.URL + "/federation/v1/batches",
		ContractVersion: PilotContractVersion,
		AcceptedAt:      now,
		AutomaticExport: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.Enqueue(validBatch(), now); err != nil {
		t.Fatal(err)
	}
	report, err := store.FlushHTTP(context.Background(), server.Client(), now)
	if err != nil {
		t.Fatal(err)
	}
	if requests != 1 || report.Delivered != 1 {
		t.Fatalf("requests = %d, report = %#v", requests, report)
	}
}

func TestPublishedFederationExportStateSchemaIsRecognized(t *testing.T) {
	if err := ValidateExportStateSchemaFile("../../schemas/federation-export-state.schema.json"); err != nil {
		t.Fatal(err)
	}
}

func validBatch() Batch {
	return Batch{
		SchemaVersion:  SchemaVersion,
		InstallationID: "0123456789abcdef",
		Period:         "2026-W30",
		ProductVersion: "0.1.0",
		Runtime:        RuntimeCodex,
		Signals: []Signal{{
			Kind:       SignalAdoption,
			Capability: CapabilityInteractionProfile,
			Stage:      StageFirstUse,
			Evidence:   EvidenceOnce,
			Confidence: ConfidenceHigh,
			Outcome:    OutcomeImproved,
		}},
	}
}

func enrolledStore(t *testing.T, now time.Time) ExportStore {
	t.Helper()
	store := ExportStore{Root: t.TempDir()}
	if err := store.Enroll(Enrollment{
		InstallationID:  "0123456789abcdef",
		BridgeEndpoint:  "https://bridge.maestro.example/federation/v1/batches",
		ContractVersion: PilotContractVersion,
		AcceptedAt:      now,
		AutomaticExport: true,
	}); err != nil {
		t.Fatal(err)
	}
	return store
}
