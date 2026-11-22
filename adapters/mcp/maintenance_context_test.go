package product_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
)

func TestMaintenanceCommandDeadlineCannotExtendParentOrBuiltin(t *testing.T) {
	parentDeadline := time.Now().Add(5 * time.Second)
	parent, cancelParent := context.WithDeadline(context.Background(), parentDeadline)
	defer cancelParent()
	ctx, cancel, err := product.MaintenanceCommandContext(parent, time.Now().Add(24*time.Hour).Format(time.RFC3339Nano))
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	deadline, ok := ctx.Deadline()
	if !ok || !deadline.Equal(parentDeadline) {
		t.Fatal("propagated future deadline enlarged the parent budget")
	}

	started := time.Now()
	builtin, stopBuiltin, err := product.MaintenanceCommandContext(context.Background(), "9999-01-01T00:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	defer stopBuiltin()
	deadline, ok = builtin.Deadline()
	if !ok || deadline.After(started.Add(90*time.Second+100*time.Millisecond)) || deadline.Before(started.Add(89*time.Second)) {
		t.Fatal("future input changed the original ninety-second ceiling")
	}

	earlier := time.Now().Add(time.Second)
	narrowed, stopNarrowed, err := product.MaintenanceCommandContext(parent, earlier.Format(time.RFC3339Nano))
	if err != nil {
		t.Fatal(err)
	}
	defer stopNarrowed()
	deadline, ok = narrowed.Deadline()
	if !ok || !deadline.Equal(earlier) {
		t.Fatal("child did not retain the exact earlier operation deadline")
	}
}

func TestMaintenanceCommandRejectsCanceledAndMalformedInput(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := product.MaintenanceCommandContext(parent, ""); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled parent: %v", err)
	}
	if _, _, err := product.MaintenanceCommandContext(context.Background(), "private-deadline-sentinel"); err == nil || strings.Contains(err.Error(), "private-deadline-sentinel") {
		t.Fatal("malformed deadline was accepted or echoed")
	}
}

func TestMaintenanceChildDiagnosticsStayClosed(t *testing.T) {
	cause := errors.New("private-error-sentinel /private/request-body")
	tests := []struct {
		name, input, phase, code string
		interrupted              bool
	}{
		{"terminal", "{\"schema_version\":1,\"phase\":\"verification\",\"code\":\"qualification_required\"}\n", "verification", "qualification_required", false},
		{"weekly-startup", "{\"schema_version\":1,\"phase\":\"admission\",\"code\":\"weekly_registration_unavailable\"}\n", "admission", "weekly_registration_unavailable", false},
		{"progress", "{\"schema_version\":1,\"phase\":\"execution\",\"code\":\"in_progress\"}\n", "execution", "interrupted", false},
		{"forced", "{\"schema_version\":1,\"phase\":\"verification\",\"code\":\"qualification_required\"}\n", "verification", "interrupted", true},
		{"unknown-phase", "{\"schema_version\":1,\"phase\":\"private-error-sentinel\",\"code\":\"maintenance_rejected\"}", "launch", "interrupted", false},
		{"unknown-code", "{\"schema_version\":1,\"phase\":\"execution\",\"code\":\"success\"}", "launch", "interrupted", false},
		{"unknown-field", "{\"schema_version\":1,\"phase\":\"execution\",\"code\":\"maintenance_rejected\",\"body\":\"private-error-sentinel\"}", "launch", "interrupted", false},
		{"trailing", "{\"schema_version\":1,\"phase\":\"execution\",\"code\":\"maintenance_rejected\"}{}", "launch", "interrupted", false},
		{"oversize", strings.Repeat("x", 4097), "launch", "interrupted", false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := product.MaintenanceChildFailure([]byte(test.input), cause, test.interrupted)
			if got.Diagnostic.SchemaVersion != 1 || got.Diagnostic.Phase != test.phase || got.Diagnostic.Code != test.code {
				t.Fatalf("unexpected closed diagnostic: %+v", got.Diagnostic)
			}
			if strings.Contains(got.Error(), "private-error-sentinel") || strings.Contains(got.Error(), "/private/") {
				t.Fatal("arbitrary child error escaped")
			}
		})
	}
	weekly := product.MaintenanceCommandFailure(errors.Join(product.ErrWeeklyRegistrationUnavailable, cause), "admission")
	if weekly.Diagnostic.Code != "weekly_registration_unavailable" || weekly.Diagnostic.Phase != "admission" ||
		!errors.Is(weekly, product.ErrWeeklyRegistrationUnavailable) || strings.Contains(weekly.Error(), "private-error-sentinel") {
		t.Fatal("closed weekly startup refusal was lost or exposed its cause")
	}
	untyped := product.MaintenanceCommandFailure(errors.New("weekly_registration_unavailable"), "admission")
	if untyped.Diagnostic.Code != "maintenance_rejected" {
		t.Fatal("arbitrary error text gained the weekly startup category")
	}
	forged := &product.MaintenanceCommandError{Diagnostic: product.MaintenanceCommandDiagnostic{SchemaVersion: 1, Phase: "private-error-sentinel", Code: "private-error-sentinel"}}
	if strings.Contains(product.MaintenanceCommandFailure(forged, "input").Error(), "private-error-sentinel") || strings.Contains(forged.Error(), "private-error-sentinel") {
		t.Fatal("unvalidated diagnostic was displayed")
	}
}

func TestMaintenanceInterruptedMessageDoesNotClaimUnchangedState(t *testing.T) {
	failure := product.MaintenanceChildFailure(
		[]byte("{\"schema_version\":1,\"phase\":\"execution\",\"code\":\"in_progress\"}\n"),
		context.Canceled, true,
	)
	message := failure.Error()
	if !strings.Contains(message, "completion was not confirmed") ||
		!strings.Contains(message, "inspect maintenance-status before a fresh maintenance-review") ||
		strings.Contains(message, "state was retained") {
		t.Fatalf("interruption message must preserve unknown completion: %s", message)
	}
}
