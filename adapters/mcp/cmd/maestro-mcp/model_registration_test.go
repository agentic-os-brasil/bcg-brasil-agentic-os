package main

import (
	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
	"testing"
)

func TestColdMaintenanceCompositionDoesNotRegisterAJob(t *testing.T) {
	var options product.Options
	if err := configureWeeklyRegistration(&options, ""); err != nil {
		t.Fatal(err)
	}
	if options.WeeklyNativeExecutor == nil || options.MemoryNativeExecutor == nil || options.WeeklyCandidate != nil {
		t.Fatal("cold startup lost fixed executors or invented a registration")
	}
}
