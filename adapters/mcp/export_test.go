package product

import "time"

// SetMaintenanceClockForTest binds one SDK fixture to a race-safe owned clock.
// It exists only in the test build; production stdio always uses system time.
func SetMaintenanceClockForTest(bound *Bound, now func() time.Time) {
	bound.maintenanceClock = now
}
