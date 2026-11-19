//go:build darwin

package setup

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installer"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/macosadapter"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
)

type macWakeRunner struct {
	spec                      macosadapter.Spec
	home                      string
	loaded                    bool
	failFinalPrint            bool
	barrierOpenedDuringNative bool
	failDisabledReadback      bool
	foreignArguments          bool
	unknownBeforeMutation     bool
	cancelAfterReadback       context.CancelFunc
	calls                     []string
}

func (r *macWakeRunner) Run(ctx context.Context, _ string, args []string) (macosadapter.CommandResult, error) {
	r.calls = append(r.calls, args[0])
	if r.loaded {
		file, err := macosadapter.ReadStatus(r.home, r.spec.Label)
		if err == nil && !file.Disabled {
			r.barrierOpenedDuringNative = true
		}
	}
	switch args[0] {
	case "bootstrap":
		r.loaded = true
	case "print":
		if r.unknownBeforeMutation {
			return macosadapter.CommandResult{}, errors.New("native state unavailable")
		}
		if !r.loaded {
			return macosadapter.CommandResult{ExitCode: 113}, errors.New("not loaded")
		}
		if r.failFinalPrint {
			return macosadapter.CommandResult{}, context.DeadlineExceeded
		}
		path := filepath.Join(r.home, "Library", "LaunchAgents", r.spec.Label+".plist")
		argv := append([]string{r.spec.Program}, r.spec.Arguments...)
		if r.foreignArguments {
			argv = append(argv, "--foreign")
		}
		return macosadapter.CommandResult{Output: fmt.Sprintf("path = %s\nprogram = %s\nlabel = %s\narguments = {\n%s\n}\n", path, r.spec.Program, r.spec.Label, strings.Join(argv, "\n"))}, nil
	case "print-disabled":
		if r.failDisabledReadback {
			return macosadapter.CommandResult{}, errors.New("enabled state unavailable")
		}
		if r.cancelAfterReadback != nil {
			r.cancelAfterReadback()
		}
		return macosadapter.CommandResult{Output: fmt.Sprintf("\"%s\" => false", r.spec.Label)}, nil
	case "bootout":
		r.loaded = false
	}
	return macosadapter.CommandResult{}, nil
}

func macWakeFixture(t *testing.T) (string, installer.MaintenanceWakeBinding, *macWakeRunner, macosadapter.Lifecycle) {
	t.Helper()
	home := t.TempDir()
	binding := installer.MaintenanceWakeBinding{ManagedRoot: filepath.Join(home, "product"), UserHome: filepath.Join(home, "owner"), BrainHome: filepath.Join(home, "owner", "Developer", "Maestro"), Workspace: filepath.Join(home, "workspace"), WorkspaceID: strings.Repeat("a", 32), CarrierID: strings.Repeat("b", 32), AdmissionSHA256: strings.Repeat("c", 64), RegistrySHA256: strings.Repeat("d", 64), InstallerVersion: "0.1.59", SourceLocator: filepath.Join(home, "source"), Source: releasecontract.SourceTuple{SourceID: "test-source", Channel: "beta", OS: "darwin", Arch: "arm64"}}
	spec, err := binding.MacSpec()
	if err != nil {
		t.Fatal(err)
	}
	runner := &macWakeRunner{spec: spec, home: home}
	return home, binding, runner, macosadapter.Lifecycle{Runner: runner, UID: "501", CurrentHome: home, Native: true, Timeout: time.Second}
}

func TestMacWakeFinalReadbackFailureKeepsEligibilityClosed(t *testing.T) {
	home, binding, runner, lifecycle := macWakeFixture(t)
	runner.failFinalPrint = true
	_, err := mutateMacWakeDefinition(context.Background(), lifecycle, home, binding, "opt-in", func() error { return nil })
	file, readErr := macosadapter.ReadStatus(home, runner.spec.Label)
	if err == nil || readErr != nil || !file.Disabled {
		t.Fatalf("unknown final readback must fail closed: operation=%v file=%+v read=%v", err, file, readErr)
	}
}

func TestMacWakeAcknowledgementAndCurrentAuthorityBeforeOpening(t *testing.T) {
	for _, scenario := range []string{"enabled_readback_unknown", "foreign_arguments", "cancelled_readback", "current_authority_changed", "cancelled_current_recheck"} {
		t.Run(scenario, func(t *testing.T) {
			home, binding, runner, lifecycle := macWakeFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			recheck := func() error { return nil }
			switch scenario {
			case "enabled_readback_unknown":
				runner.failDisabledReadback = true
			case "foreign_arguments":
				runner.foreignArguments = true
			case "cancelled_readback":
				runner.cancelAfterReadback = cancel
			case "current_authority_changed":
				recheck = func() error { return errors.New("current authority changed") }
			case "cancelled_current_recheck":
				recheck = func() error { cancel(); return nil }
			}
			_, err := mutateMacWakeDefinition(ctx, lifecycle, home, binding, "opt-in", recheck)
			file, readErr := macosadapter.ReadStatus(home, runner.spec.Label)
			if err == nil || readErr != nil || !file.Disabled || runner.barrierOpenedDuringNative {
				t.Fatalf("unverified open: err=%v file=%+v read=%v openDuringNative=%t", err, file, readErr, runner.barrierOpenedDuringNative)
			}
		})
	}
}

func TestMacWakeStagesClosedAndOpensOnlyAfterVerifiedAcknowledgement(t *testing.T) {
	home, binding, runner, lifecycle := macWakeFixture(t)
	checked := false
	status, err := mutateMacWakeDefinition(context.Background(), lifecycle, home, binding, "opt-in", func() error {
		file, err := macosadapter.ReadStatus(home, runner.spec.Label)
		if err != nil || !file.Disabled {
			t.Fatalf("recheck was not behind closed barrier: %+v %v", file, err)
		}
		checked = true
		return nil
	})
	if err != nil || !checked || status.Disabled || !status.Loaded || !status.Enabled || !status.NativeQualified || status.State != "active_loaded_enabled" || runner.barrierOpenedDuringNative {
		t.Fatalf("opt-in status=%+v err=%v checked=%t openDuringNative=%t", status, err, checked, runner.barrierOpenedDuringNative)
	}
	if err := binding.VerifyMacWake(home); err != nil {
		t.Fatal("verified wake remains closed", err)
	}
	for _, action := range []string{"pause", "opt-in", "revoke"} {
		status, err = mutateMacWakeDefinition(context.Background(), lifecycle, home, binding, action, func() error { return nil })
		if err != nil {
			t.Fatalf("%s: %+v %v", action, status, err)
		}
		if action == "opt-in" {
			continue
		}
		if status.Loaded || status.Enabled || binding.VerifyMacWake(home) == nil {
			t.Fatalf("%s did not close: %+v", action, status)
		}
	}
	if runner.barrierOpenedDuringNative {
		t.Fatal("native operation ran with an open file barrier")
	}
}

func TestMacWakePauseAndRevokeCloseBeforeUnreachableNativeState(t *testing.T) {
	for _, action := range []string{"pause", "revoke"} {
		t.Run(action, func(t *testing.T) {
			home, binding, runner, lifecycle := macWakeFixture(t)
			if _, err := macosadapter.Install(home, runner.spec, true); err != nil {
				t.Fatal(err)
			}
			runner.loaded, runner.unknownBeforeMutation = true, true
			_, err := mutateMacWakeDefinition(context.Background(), lifecycle, home, binding, action, func() error { return nil })
			file, readErr := macosadapter.ReadStatus(home, runner.spec.Label)
			if err == nil || readErr != nil || !file.Disabled || runner.barrierOpenedDuringNative || binding.VerifyMacWake(home) == nil {
				t.Fatalf("%s did not retain closed veto: err=%v file=%+v read=%v openDuringNative=%t", action, err, file, readErr, runner.barrierOpenedDuringNative)
			}
		})
	}
}

func TestMacWakeRefusesForeignFileWithoutNativeMutation(t *testing.T) {
	home, binding, runner, lifecycle := macWakeFixture(t)
	foreign := runner.spec
	foreign.Arguments = []string{"other-workspace"}
	file, err := macosadapter.Install(home, foreign, true)
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(file.Path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mutateMacWakeDefinition(context.Background(), lifecycle, home, binding, "pause", func() error { return nil }); err == nil {
		t.Fatal("foreign definition adopted")
	}
	after, err := os.ReadFile(file.Path)
	if err != nil || string(before) != string(after) || len(runner.calls) != 0 {
		t.Fatal("foreign definition changed or native action attempted", err, runner.calls)
	}
}
