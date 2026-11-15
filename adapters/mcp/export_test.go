package product

import (
	"context"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/maintenance"
)

// SetMaintenanceClockForTest binds one SDK fixture to a race-safe owned clock.
// It exists only in the test build; production stdio always uses system time.
func SetMaintenanceClockForTest(bound *Bound, now func() time.Time) {
	bound.maintenanceClock = now
}

// SetMaintenanceActivityForTest supplies a controlled native-constructor observation.
// It exists only in test builds and never qualifies actual native activity.
func SetMaintenanceActivityForTest(bound *Bound, observe func(context.Context) (maintenance.IdleState, error)) {
	bound.maintenanceActivity = observe
}
