//go:build maestro_experimental_installed_seed

package main

import (
	"context"
	"testing"
)

func TestMaintenanceWakeEntryRequiresOriginalRole(t *testing.T) {
	if handled, err := experimentalMaintenanceWakeCommand(context.Background(), "maintenance-wake", "", nil, nil); !handled || err == nil {
		t.Fatal("unadmitted stable entry passed", handled, err)
	}
	if handled, err := experimentalMaintenanceWakeCommand(context.Background(), "status", "", nil, nil); handled || err != nil {
		t.Fatal("unrelated original command intercepted", handled, err)
	}
}
