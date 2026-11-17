package product

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installer"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestManagementHandoffRequiresFixedAdmittedContextAndRejectsModelPaths(t *testing.T) {
	b := localReviewFixture(t, "")
	opened := 0
	b.managementOpen = func(context.Context, string) error { opened++; return nil }
	client := localReviewSession(t, b, nil)
	call := func(fields map[string]any) {
		t.Helper()
		out, err := client.CallTool(context.Background(), &mcp.CallToolParams{Name: "maestro_apply", Arguments: fields})
		if err != nil {
			t.Fatal(err)
		}
		if !out.IsError || opened != 0 {
			t.Fatal("unqualified or model-selected handoff opened", out, opened)
		}
	}
	call(map[string]any{"operation": "installation.update_handoff"})
	for _, field := range []string{"path", "command", "source", "carrier_id", "approved", "review_digest"} {
		call(map[string]any{"operation": "installation.update_handoff", field: "model-selection"})
	}
	b.options.ManagementCarrierID = strings.Repeat("a", 32)
	b.options.ManagementInstallerVersion = "0.3.7"
	// No installer/native admission exists in this ordinary backend fixture.
	// A plausible trusted-context shape still cannot bypass the real owner.
	call(map[string]any{"operation": "installation.update_handoff"})
	b.options.ManagementCarrierID = strings.Repeat("a", 32) + "," + strings.Repeat("b", 32)
	call(map[string]any{"operation": "installation.update_handoff"})
}

func TestManagementOpenWitnessReportsOnlyRequestedHandoff(t *testing.T) {
	prepared := installer.ClientManagementHandoff{State: "handoff_prepared", Launcher: "/fixed/admitted/Update Maestro.command", Activation: "not_requested_or_approved"}
	opened := 0
	value, err := requestManagementOpen(context.Background(), prepared, func(_ context.Context, path string) error {
		opened++
		if path != prepared.Launcher {
			t.Fatal("fixed manager changed")
		}
		return nil
	})
	if err != nil || opened != 1 {
		t.Fatal(value, err)
	}
	result := value.(map[string]string)
	if result["state"] != "handoff_requested" || result["activation"] != "pending_native_installer_review" || result["client_loading"] != "pending" {
		t.Fatal("OS acknowledgement became activation", result)
	}
	if _, err := requestManagementOpen(context.Background(), prepared, func(context.Context, string) error { return errors.New("normal host opener unavailable") }); err == nil {
		t.Fatal("unavailable host opener became successful")
	}
	prepared.State = "unqualified"
	if _, err := requestManagementOpen(context.Background(), prepared, func(context.Context, string) error { t.Fatal("unqualified handoff opened"); return nil }); err == nil {
		t.Fatal("unqualified handoff accepted")
	}
}
