package macosadapter

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

type fakeLaunchctl struct {
	calls            [][]string
	loaded, disabled bool
}

func (fake *fakeLaunchctl) Run(_ context.Context, name string, args []string) (CommandResult, error) {
	fake.calls = append(fake.calls, append([]string{name}, args...))
	if name != "launchctl" {
		return CommandResult{}, errors.New("unexpected command")
	}
	if len(args) == 0 {
		return CommandResult{}, errors.New("missing args")
	}
	switch args[0] {
	case "bootstrap":
		fake.loaded = true
	case "bootout":
		fake.loaded = false
	case "enable":
		fake.disabled = false
	case "disable":
		fake.disabled = true
	case "kickstart":
	case "print":
		if !fake.loaded {
			return CommandResult{ExitCode: 1}, errors.New("not loaded")
		}
		return CommandResult{}, nil
	case "print-disabled":
		state := "false"
		if fake.disabled {
			state = "true"
		}
		return CommandResult{Output: fmt.Sprintf("\"com.bcg.maestro.maintenance\" => %s", state)}, nil
	}
	return CommandResult{}, nil
}

func TestLaunchAgentLifecycleUsesStructuredLaunchctlAndConfirmsLoaded(t *testing.T) {
	home := t.TempDir()
	fake := &fakeLaunchctl{}
	lifecycle := Lifecycle{Runner: fake, UID: "501", CurrentHome: home, Native: true}
	spec := Spec{Label: "com.bcg.maestro.maintenance", Program: "/usr/local/bin/bcgos", Arguments: []string{"maintenance", "wake", "--trigger", "presence"}, StartInterval: 900}
	status, err := lifecycle.Install(context.Background(), home, spec, true)
	if err != nil || status.State != "active_loaded_enabled" || !status.FilePresent || !status.Loaded || !status.Enabled || !status.NativeQualified {
		t.Fatalf("install status=%#v err=%v", status, err)
	}
	if len(fake.calls) < 4 || fake.calls[0][0] != "launchctl" || fake.calls[0][1] != "bootstrap" || strings.Contains(strings.Join(fake.calls[0], " "), "sh -c") {
		t.Fatalf("launchctl calls=%#v", fake.calls)
	}
	paused, err := lifecycle.Pause(context.Background(), home, spec.Label)
	if err != nil || !paused.FilePresent || !paused.Disabled {
		t.Fatalf("pause status=%#v err=%v", paused, err)
	}
	before := len(fake.calls)
	pausedAgain, err := lifecycle.Pause(context.Background(), home, spec.Label)
	if err != nil || !pausedAgain.Disabled {
		t.Fatalf("pause idempotence status=%#v err=%v calls=%d/%d", pausedAgain, err, len(fake.calls), before)
	}
	resumed, err := lifecycle.Resume(context.Background(), home, spec.Label)
	if err != nil || resumed.Disabled || !resumed.Loaded || !resumed.Enabled {
		t.Fatalf("resume status=%#v err=%v", resumed, err)
	}
	if err := lifecycle.Uninstall(context.Background(), home, spec.Label); err != nil {
		t.Fatal(err)
	}
	removed, err := lifecycle.Status(context.Background(), home, spec.Label)
	if err != nil || removed.FilePresent || removed.State != "not_present" {
		t.Fatalf("uninstall status=%#v err=%v", removed, err)
	}
}

func TestLaunchAgentNativeInstallRollsBackWhenBootstrapFails(t *testing.T) {
	home := t.TempDir()
	lifecycle := Lifecycle{Runner: failingRunner{}, UID: "501", CurrentHome: home, Native: true}
	status, err := lifecycle.Install(context.Background(), home, Spec{Label: "com.bcg.maestro.maintenance", Program: "/usr/local/bin/bcgos", StartInterval: 900}, true)
	if err == nil || status.State != "partial_bootstrap_failed" {
		t.Fatalf("status=%#v err=%v", status, err)
	}
	fileStatus, readErr := ReadStatus(home, "com.bcg.maestro.maintenance")
	if readErr != nil || fileStatus.State != "not_installed" {
		t.Fatalf("rollback status=%#v err=%v", fileStatus, readErr)
	}
}

func TestLaunchAgentReinstallReconcilesLoadedService(t *testing.T) {
	home := t.TempDir()
	fake := &fakeLaunchctl{}
	lifecycle := Lifecycle{Runner: fake, UID: "501", CurrentHome: home, Native: true}
	spec := Spec{Label: "com.bcg.maestro.maintenance", Program: "/usr/local/bin/bcgos", StartInterval: 900}
	if _, err := lifecycle.Install(context.Background(), home, spec, true); err != nil {
		t.Fatal(err)
	}
	status, err := lifecycle.Install(context.Background(), home, spec, true)
	if err != nil || status.State != "active_loaded_enabled" {
		t.Fatalf("reinstall status=%#v err=%v", status, err)
	}
}

func TestLaunchAgentPauseReconcilesDisabledFileWithLoadedService(t *testing.T) {
	home := t.TempDir()
	fake := &fakeLaunchctl{}
	lifecycle := Lifecycle{Runner: fake, UID: "501", CurrentHome: home, Native: true}
	spec := Spec{Label: "com.bcg.maestro.maintenance", Program: "/usr/local/bin/bcgos", StartInterval: 900}
	if _, err := lifecycle.Install(context.Background(), home, spec, true); err != nil {
		t.Fatal(err)
	}
	if _, err := setDisabled(home, spec.Label, true); err != nil {
		t.Fatal(err)
	}
	status, err := lifecycle.Pause(context.Background(), home, spec.Label)
	if err != nil || !status.Disabled || status.Loaded {
		t.Fatalf("pause status=%#v err=%v", status, err)
	}
}

type failingRunner struct{}

func (failingRunner) Run(context.Context, string, []string) (CommandResult, error) {
	return CommandResult{ExitCode: 1}, errors.New("launchctl failed")
}
