package darwinadapter

import (
	"testing"
	"time"
)

func TestClaudeCodexWakeSignalsHaveSemanticParity(t *testing.T) {
	now := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	for _, test := range []struct{ runtime, name string }{{"claude", ClaudeWakeSignal}, {"codex", CodexWakeSignal}} {
		command, err := Command(Signal{Runtime: test.runtime, Name: test.name, CommandID: "wake-1", Trigger: "presence", ScheduledFor: now, RequestedAt: now, Deadline: now.Add(time.Minute)})
		if err != nil || command.JobID != "darwin-housekeeping" || command.WorkspaceID != "maestro-system" {
			t.Fatalf("%s command = %#v %v", test.runtime, command, err)
		}
	}
	if _, err := Command(Signal{Runtime: "claude", Name: CodexWakeSignal, CommandID: "wake-2", Trigger: "presence", ScheduledFor: now, RequestedAt: now, Deadline: now.Add(time.Minute)}); err == nil {
		t.Fatal("cross-runtime wake signal accepted")
	}
}

func TestNativeSchedulersRemainDisabled(t *testing.T) {
	state := PlatformSchedulerState()
	if state["claude"] == "" || state["codex"] == "" || state["claude"] != state["codex"] {
		t.Fatalf("scheduler parity = %#v", state)
	}
}
