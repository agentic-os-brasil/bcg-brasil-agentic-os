package main

import (
	"errors"
	"io"
	"path/filepath"
	"strings"
	"unicode/utf8"

	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp/packaging"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/maintenance"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

// Loading an explicit declaration composes the fixed executor; qualification and
// activation remain separate owner-reviewed maintenance transactions.
func configureWeeklyRegistration(o *product.Options, path string) error {
	if path == "" {
		seedMaintenanceExecutors(o)
		return nil
	}
	unavailable := errors.New("weekly_registration_unavailable")
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || strings.ContainsAny(path, "\x00\r\n") {
		return unavailable
	}
	f, err := scheduler.OpenRegularFile(path)
	if err != nil {
		return unavailable
	}
	defer f.Close()
	if err := scheduler.ValidateSingleLink(f); err != nil {
		return unavailable
	}
	const maximumBytes = 256 << 10
	info, err := f.Stat()
	if err != nil || info.Size() < 1 || info.Size() > maximumBytes {
		return unavailable
	}
	body, err := io.ReadAll(io.LimitReader(f, maximumBytes+1))
	if err != nil || len(body) > maximumBytes || !utf8.Valid(body) {
		return unavailable
	}
	var registration maintenance.WeeklyModelRegistration
	if err := releasecontract.DecodeStrictJSON(body, &registration); err != nil {
		return unavailable
	}
	if err := registration.Validate(); err != nil {
		return unavailable
	}
	o.WeeklyCandidate = &registration
	seedMaintenanceExecutors(o)
	return nil
}

// A cold installed wake composes fixed code only. Stored reviewed registrations,
// actual native qualification and current grants remain the sole job authority.
func seedMaintenanceExecutors(o *product.Options) {
	o.WeeklyNativeExecutor = packaging.ExecuteWeeklyNative
	o.MemoryNativeExecutor = packaging.ExecuteMemoryNative
}
