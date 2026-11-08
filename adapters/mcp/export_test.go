package product

import (
	"context"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ingest/docling"
)

// SetMaintenanceClockForTest binds one SDK fixture to a race-safe owned clock.
// It exists only in the test build; production stdio always uses system time.
func SetMaintenanceClockForTest(bound *Bound, now func() time.Time) {
	bound.maintenanceClock = now
}

// PublishContentForTest exercises the real artifact publisher with a labeled
// synthetic conversion value; it does not execute or qualify a converter.
func PublishContentForTest(bound *Bound, source string, conversion docling.Conversion) (string, error) {
	revision, err := bound.contentSourceRevision(source)
	if err != nil {
		return "", err
	}
	return bound.publishContent(context.Background(), source, conversion, revision)
}
