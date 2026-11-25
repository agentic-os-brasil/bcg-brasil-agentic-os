package product

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installer"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/macosadapter"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/maintenance"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/windowsadapter"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var errBackgroundIdleUnknown = errors.New("native background idle observation unavailable")
var errBackgroundUserActive = errors.New("native background user activity observed")

// Serve owns the worker lifetime; canonical job controls supply all authority.
func (b *Bound) Serve(ctx context.Context, transport mcp.Transport) error {
	return b.serveMCP(ctx, transport, ordinaryServe, b.maintain)
}

// ServeInstalledCheck retains ordinary explicit MCP operation guards while
// omitting only the automatic worker for one authenticated installed child.
func (b *Bound) ServeInstalledCheck(ctx context.Context, transport mcp.Transport, checker *installer.VerifiedStableCheck) error {
	o := b.options
	binding := installer.StableCheckBinding{ProductRoot: o.ProductRoot, UserHome: o.UserHome, Workspace: o.WorkspacePath, CarrierID: o.ManagementCarrierID, InstallerVersion: o.ManagementInstallerVersion, ManifestSHA256: o.InstalledManifestSHA256, ClientInputsSHA256: o.InstalledClientInputsSHA256}
	if err := checker.ValidateContext(ctx, binding); err != nil {
		return err
	}
	if err := b.validateContext(ctx); err != nil {
		return err
	}
	return b.serveMCP(ctx, transport, installedCheckServe, b.maintain)
}

type servePurpose uint8

const (
	ordinaryServe servePurpose = iota
	installedCheckServe
)

func (b *Bound) serveMCP(ctx context.Context, transport mcp.Transport, purpose servePurpose, worker func(context.Context)) error {
	if purpose != ordinaryServe && purpose != installedCheckServe {
		return errors.New("unsupported server purpose")
	}
	if purpose == installedCheckServe {
		return b.Server().Run(ctx, transport)
	}
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		worker(ctx)
	}()
	defer func() {
		cancel()
		<-done
	}()
	return b.Server().Run(ctx, transport)
}

func (b *Bound) maintain(ctx context.Context) {
	timer := time.NewTimer(0)
	defer timer.Stop()
	lastRefusal := ""
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		if ctx.Err() != nil {
			return
		}
		service, availabilityErr := b.maintenanceService()
		if availabilityErr == nil {
			_, availabilityErr = service.RunUpdateAvailability(ctx)
		}
		err := errors.Join(availabilityErr, b.backgroundMaintenance(ctx))
		if ctx.Err() != nil {
			return
		}
		message := ""
		if err != nil {
			message = "maestro: enrolled maintenance refused; inspect maintenance.status and review the job's current qualification"
			if errors.Is(err, errBackgroundIdleUnknown) {
				message = "maestro: automatic maintenance idle observation unavailable; inspect native activity support or use an explicit foreground maintenance.run"
			}
		}
		if message != "" && message != lastRefusal {
			fmt.Fprintln(os.Stderr, message)
		}
		lastRefusal = message
		timer.Reset(30 * time.Second)
	}
}

func (b *Bound) observeMaintenanceActivity(ctx context.Context) (maintenance.IdleState, error) {
	if b.maintenanceActivity != nil {
		return b.maintenanceActivity(ctx)
	}
	switch runtime.GOOS {
	case "darwin":
		state, _, err := macosadapter.ObserveIdle(ctx, 5*time.Minute)
		return maintenance.IdleState(state), err
	case "windows":
		idleFor, err := windowsadapter.ObserveIdle(ctx)
		if err != nil {
			return maintenance.IdleUnknown, err
		}
		if idleFor >= 5*time.Minute {
			return maintenance.IdleConfirmed, nil
		}
		return maintenance.IdleActive, nil
	default:
		return maintenance.IdleUnknown, errBackgroundIdleUnknown
	}
}

func (b *Bound) backgroundMaintenance(ctx context.Context) error {
	_, err := b.runBackgroundMaintenance(ctx, nil)
	return err
}

// RunMaintenanceWake performs one original due scan; a pulse is not a commit.
// Its veto comes from trusted installed/native composition, never MCP input.
func (b *Bound) RunMaintenanceWake(ctx context.Context, wakeAllowed func() error) (maintenance.WakeReport, error) {
	if wakeAllowed == nil {
		return maintenance.WakeReport{}, errors.New("owned native wake eligibility required")
	}
	if err := wakeAllowed(); err != nil {
		return maintenance.WakeReport{}, err
	}
	// The existing read-only availability owner runs without idle, just as in
	// Serve; it still requires its own enrollment and this wake's live veto.
	service, err := b.maintenanceServiceWithGuards(wakeAllowed, func(publish func() error) error {
		return b.guardPublication(func() error {
			if err := wakeAllowed(); err != nil {
				return err
			}
			return publish()
		})
	})
	if err != nil {
		return maintenance.WakeReport{}, err
	}
	availability, err := service.RunUpdateAvailability(ctx)
	if err != nil {
		return availability, err
	}
	report, err := b.runBackgroundMaintenance(ctx, wakeAllowed)
	report.Due = append(availability.Due, report.Due...)
	report.Receipts = append(availability.Receipts, report.Receipts...)
	if report.State == "no_due_work" && availability.State != "no_due_work" {
		report.State = availability.State
	}
	return report, err
}

func (b *Bound) runBackgroundMaintenance(ctx context.Context, wakeAllowed func() error) (maintenance.WakeReport, error) {
	ctx, cancel := context.WithCancelCause(ctx)
	start := make(chan struct{})
	done := make(chan struct{})
	var once sync.Once
	check := func() error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if wakeAllowed != nil {
			if err := wakeAllowed(); err != nil {
				cancel(err)
				return err
			}
		}
		state, err := b.observeMaintenanceActivity(ctx)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil || state != maintenance.IdleConfirmed {
			cause := errBackgroundIdleUnknown
			if err == nil && state == maintenance.IdleActive {
				cause = errBackgroundUserActive
			}
			cancel(cause)
			return cause
		}
		once.Do(func() { close(start) })
		return nil
	}
	go func() {
		defer close(done)
		select {
		case <-ctx.Done():
			return
		case <-start:
		}
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if check() != nil {
					return
				}
			}
		}
	}()
	defer func() {
		cancel(nil)
		<-done
	}()
	guard := b.guardPublication
	if wakeAllowed != nil {
		guard = func(publish func() error) error {
			return b.guardPublication(func() error {
				if err := wakeAllowed(); err != nil {
					return err
				}
				return publish()
			})
		}
	}
	service, err := b.maintenanceServiceWithGuards(check, guard)
	if err != nil {
		return maintenance.WakeReport{}, err
	}
	report, err := service.RunDue(ctx)
	if cause := context.Cause(ctx); errors.Is(cause, errBackgroundUserActive) {
		return report, nil
	} else if errors.Is(cause, errBackgroundIdleUnknown) {
		return report, errBackgroundIdleUnknown
	}
	return report, err
}
