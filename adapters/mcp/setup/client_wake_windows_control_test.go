package setup

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installer"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/windowsadapter"
)

func windowsWakeTransactionFixture(t *testing.T) installer.MaintenanceWakeState {
	t.Helper()
	root := t.TempDir()
	b := installer.MaintenanceWakeBinding{ManagedRoot: filepath.Join(root, "product"), UserHome: filepath.Join(root, "owner"), BrainHome: filepath.Join(root, "owner", "Developer", "Maestro"), Workspace: filepath.Join(root, "workspace"), WorkspaceID: strings.Repeat("a", 32), CarrierID: strings.Repeat("b", 32), AdmissionSHA256: strings.Repeat("c", 64), RegistrySHA256: strings.Repeat("d", 64), InstallerVersion: "0.1.59", SourceLocator: filepath.Join(root, "source"), Source: releasecontract.SourceTuple{SourceID: "test-source", Channel: "beta", OS: "windows", Arch: "amd64"}}
	if err := os.MkdirAll(filepath.Join(b.UserHome, ".maestro-install"), 0700); err != nil {
		t.Fatal(err)
	}
	digest, err := b.Digest()
	if err != nil {
		t.Fatal(err)
	}
	return installer.MaintenanceWakeState{Binding: b, BindingSHA256: digest, ClientRecordSHA: strings.Repeat("e", 64), CurrentRelease: "0.1.59", CurrentManifest: strings.Repeat("f", 64)}
}
func wakeNativeReadback(s windowsadapter.NativeWakeSpec, state string) windowsadapter.NativeWakeStatus {
	return windowsadapter.NativeWakeStatus{Name: s.Name, BindingSHA256: s.BindingSHA256, State: state, Present: state != "not_present", Enabled: state == "active"}
}
func TestWindowsWakeLateNativeAckCannotReopenPausedEpoch(t *testing.T) {
	state := windowsWakeTransactionFixture(t)
	b := state.Binding
	var taskEnabled atomic.Bool
	// The native service may complete after the caller has timed out.
	var lateAck func()
	c, _, err := mutateWindowsWakeDefinition(context.Background(), b, 0, "opt-in", func(ctx context.Context, s windowsadapter.NativeWakeSpec, a string) (windowsadapter.NativeWakeStatus, error) {
		current, e := b.ReadWindowsWakeControl()
		if e != nil || current.Enabled || current.Revision != 1 {
			t.Fatal("OS accessed before durable disabled veto", current, e)
		}
		lateAck = func() { taskEnabled.Store(true) }
		return windowsadapter.NativeWakeStatus{}, context.DeadlineExceeded
	}, func() error { return nil })
	if !errors.Is(err, context.DeadlineExceeded) || c.Enabled || c.Revision != 1 {
		t.Fatal("timed out opt-in opened veto", c, err)
	}
	beforePause, err := reviewWindowsWakeState(context.Background(), state, "opt-in")
	if err != nil {
		t.Fatal(err)
	}
	c, _, err = mutateWindowsWakeDefinition(context.Background(), b, c.Revision, "pause", func(ctx context.Context, s windowsadapter.NativeWakeSpec, a string) (windowsadapter.NativeWakeStatus, error) {
		current, e := b.ReadWindowsWakeControl()
		if e != nil || current.Enabled || current.Revision != 2 {
			t.Fatal("task-absent pause did not persist its revision before OS inspection")
		}
		return wakeNativeReadback(s, "not_present"), nil
	}, func() error { return nil })
	if err != nil || c.Enabled || c.Revision != 2 {
		t.Fatal("task-absent pause failed", c, err)
	}
	lateAck()
	if !taskEnabled.Load() {
		t.Fatal("test did not deliver late native enable")
	}
	if err := b.VerifyWindowsNativeWake(context.Background()); !errors.Is(err, installer.ErrMaintenanceWakeDisabled) {
		t.Fatal("late native enable bypassed worker/publication veto", err)
	}
	if _, err := b.WriteWindowsWakeControl(context.Background(), 1, true); !errors.Is(err, installer.ErrMaintenanceWakeControlChanged) {
		t.Fatal("old staged opener bypassed pause revision", err)
	}
	afterPause, err := reviewWindowsWakeState(context.Background(), state, "opt-in")
	if err != nil {
		t.Fatal(err)
	}
	if afterPause.ReviewDigest == beforePause.ReviewDigest {
		t.Fatal("pause with task still absent did not invalidate prior opt-in review")
	}
	calls := 0
	if _, _, err := mutateWindowsWakeDefinition(context.Background(), b, 1, "opt-in", func(context.Context, windowsadapter.NativeWakeSpec, string) (windowsadapter.NativeWakeStatus, error) {
		calls++
		return windowsadapter.NativeWakeStatus{}, nil
	}, func() error { return nil }); !errors.Is(err, installer.ErrMaintenanceWakeControlChanged) || calls != 0 {
		t.Fatal("stale review reached native effect", err, calls)
	}
}
func TestWindowsWakeSuccessfulOptInPauseRevokeAndContextFailures(t *testing.T) {
	for _, failure := range []string{"", "native", "context", "authority"} {
		t.Run(failure, func(t *testing.T) {
			state := windowsWakeTransactionFixture(t)
			b := state.Binding
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			c, _, err := mutateWindowsWakeDefinition(ctx, b, 0, "opt-in", func(ctx context.Context, s windowsadapter.NativeWakeSpec, a string) (windowsadapter.NativeWakeStatus, error) {
				if failure == "native" {
					return windowsadapter.NativeWakeStatus{}, errors.New("foreign native definition preserved")
				}
				if failure == "context" {
					cancel()
				}
				return wakeNativeReadback(s, "active"), nil
			}, func() error {
				if failure == "authority" {
					return errors.New("source changed")
				}
				return nil
			})
			if failure != "" {
				if err == nil {
					t.Fatal("failed opt-in succeeded")
				}
				current, e := b.ReadWindowsWakeControl()
				if e != nil || current.Enabled || current.Revision != 1 {
					t.Fatal("failed/timeout/context opt-in opened veto", current, e)
				}
				return
			}
			if err != nil || !c.Enabled || c.Revision != 2 {
				t.Fatal("reviewed successful opt-in unavailable", c, err)
			}
			for _, action := range []string{"pause", "revoke"} {
				c, _, err = mutateWindowsWakeDefinition(context.Background(), b, c.Revision, action, func(ctx context.Context, s windowsadapter.NativeWakeSpec, a string) (windowsadapter.NativeWakeStatus, error) {
					return wakeNativeReadback(s, "not_present"), nil
				}, func() error { return nil })
				if err != nil || c.Enabled {
					t.Fatal("normal pause/revoke reopened veto", c, err)
				}
				if _, e := b.RequireWindowsWakeControl(context.Background()); !errors.Is(e, installer.ErrMaintenanceWakeDisabled) {
					t.Fatal(e)
				}
			}
		})
	}
}
func TestWindowsWakeFreshAbsentPauseIsDurableAndReviewIsOSFree(t *testing.T) {
	state := windowsWakeTransactionFixture(t)
	initial, err := reviewWindowsWakeState(context.Background(), state, "pause")
	if err != nil || initial.Status.WindowsControl == nil || initial.Status.WindowsControl.Revision != 0 || initial.Status.WindowsNative != nil {
		t.Fatal("fresh review requires no native probe", err)
	}
	c, _, err := mutateWindowsWakeDefinition(context.Background(), state.Binding, 0, "pause", func(ctx context.Context, s windowsadapter.NativeWakeSpec, a string) (windowsadapter.NativeWakeStatus, error) {
		return wakeNativeReadback(s, "not_present"), nil
	}, func() error { return nil })
	if err != nil || c.Revision != 1 || c.Enabled {
		t.Fatal("first absent-task pause failed", c, err)
	}
	next, err := reviewWindowsWakeState(context.Background(), state, "pause")
	if err != nil || next.ReviewDigest == initial.ReviewDigest {
		t.Fatal("absent-task pause did not advance exact review")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := mutateWindowsWakeDefinition(ctx, state.Binding, 1, "opt-in", func(context.Context, windowsadapter.NativeWakeSpec, string) (windowsadapter.NativeWakeStatus, error) {
		t.Fatal("canceled transaction called OS")
		return windowsadapter.NativeWakeStatus{}, nil
	}, func() error { return nil }); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	current, err := state.Binding.ReadWindowsWakeControl()
	if err != nil || current != c {
		t.Fatal("canceled transaction changed existing control")
	}
}

func TestWindowsWakeRefusesInvalidAckWithoutOpeningControl(t *testing.T) {
	for _, kind := range []string{"wrong_identity", "not_present", "disabled_ack"} {
		t.Run(kind, func(t *testing.T) {
			state := windowsWakeTransactionFixture(t)
			c, _, err := mutateWindowsWakeDefinition(context.Background(), state.Binding, 0, "opt-in", func(ctx context.Context, s windowsadapter.NativeWakeSpec, a string) (windowsadapter.NativeWakeStatus, error) {
				out := wakeNativeReadback(s, "active")
				switch kind {
				case "wrong_identity":
					out.Name = "foreign"
				case "not_present":
					out = wakeNativeReadback(s, "not_present")
				case "disabled_ack":
					out.Enabled = false
				}
				return out, nil
			}, func() error { return nil })
			if err == nil || c.Enabled {
				t.Fatal("invalid OS acknowledgement opened control", c, err)
			}
			actual, e := state.Binding.ReadWindowsWakeControl()
			if e != nil || actual.Revision != 1 || actual.Enabled {
				t.Fatal("invalid native readback lost durable veto", actual, e)
			}
		})
	}
}
