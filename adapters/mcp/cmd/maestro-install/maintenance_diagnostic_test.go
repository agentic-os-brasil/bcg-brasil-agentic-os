package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installer"
)

func TestMaintenanceRevalidationKeepsClosedFailureBeforePending(t *testing.T) {
	for _, test := range []struct {
		name, code string
		canceled   bool
	}{
		{"canceled", "canceled", true},
		{"unavailable", "maintenance_rejected", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if test.canceled {
				cancel()
			}
			options := installer.ClientOptions{SeededOperationOptions: installer.SeededOperationOptions{
				ManagedRoot: "/synthetic/private-manager-path-sentinel/product",
				UserHome:    "/synthetic/private-manager-path-sentinel/owner",
				DataRoot:    "/synthetic/private-manager-path-sentinel/state",
			}}
			var output bytes.Buffer
			asked := false
			err := revalidateClientMaintenance(ctx, options, func(any) error { asked = true; return nil }, &output)
			if err != nil {
				t.Fatalf("pending inspection should remain pending: %v", err)
			}
			text := output.String()
			if !strings.Contains(text, "\"code\":\""+test.code+"\"") || !strings.Contains(text, "\"phase\":\"admission\"") {
				t.Fatalf("manager discarded closed diagnostic: %s", text)
			}
			if !strings.Contains(text, "\"maintenance_qualification\":\"waiting_input\"") || !strings.Contains(text, "\"functional_acceptance\":\"pending\"") {
				t.Fatalf("manager changed pending state: %s", text)
			}
			if asked || strings.Contains(text, "private-manager-path-sentinel") {
				t.Fatal("failed inspection asked confirmation or displayed a private path")
			}
		})
	}
}
