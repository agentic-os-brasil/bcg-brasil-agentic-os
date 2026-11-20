//go:build !maestro_experimental_installed_seed

package main

import (
	"context"
	"testing"
)

func TestMaintenanceWakeProductionEntryStaysClosed(t *testing.T) {
	if handled, err := experimentalMaintenanceWakeCommand(context.Background(), "maintenance-wake", "", nil, nil); handled || err != nil {
		t.Fatal("experimental wake exposed in production", handled, err)
	}
}
