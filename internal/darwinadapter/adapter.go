// Package darwinadapter maps native Claude/Codex wake signals to the same
// platform-neutral Darwin maintenance identity. It never invokes a model or
// worker inline; callers must hand the returned command to the qualified
// maintenance executor.
package darwinadapter

import (
	"errors"
	"strings"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/darwin"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/maintenance"
)

const (
	ClaudeWakeSignal = "darwin.maintenance.wake"
	CodexWakeSignal  = "darwin_maintenance_wake"
)

type Signal struct {
	Runtime      string
	Name         string
	CommandID    string
	Trigger      maintenance.Trigger
	EventID      string
	ScheduledFor time.Time
	RequestedAt  time.Time
	Deadline     time.Time
}

func Command(signal Signal) (maintenance.Command, error) {
	if (signal.Runtime == "claude" && signal.Name != ClaudeWakeSignal) || (signal.Runtime == "codex" && signal.Name != CodexWakeSignal) || (signal.Runtime != "claude" && signal.Runtime != "codex") {
		return maintenance.Command{}, errors.New("Darwin adapter signal is not recognized")
	}
	if strings.TrimSpace(signal.CommandID) == "" || signal.RequestedAt.IsZero() || signal.ScheduledFor.IsZero() || signal.Deadline.IsZero() {
		return maintenance.Command{}, errors.New("Darwin adapter signal lacks bounded timing metadata")
	}
	command := maintenance.Command{SchemaVersion: maintenance.CommandSchemaVersion, CommandID: signal.CommandID, JobID: darwin.HousekeepingJobID, WorkspaceID: darwin.MaintenanceScope, Trigger: signal.Trigger, EventID: signal.EventID, ScheduledFor: signal.ScheduledFor.UTC(), RequestedAt: signal.RequestedAt.UTC(), Deadline: signal.Deadline.UTC()}
	if err := command.Validate(signal.RequestedAt); err != nil {
		return maintenance.Command{}, err
	}
	return command, nil
}

func PlatformSchedulerState() map[string]string {
	return map[string]string{
		"claude": "disabled_until_explicit_install_and_qualification",
		"codex":  "disabled_until_explicit_install_and_qualification",
	}
}
