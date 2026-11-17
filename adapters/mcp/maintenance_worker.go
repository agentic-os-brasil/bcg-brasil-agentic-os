package product

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/macosadapter"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/maintenance"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/windowsadapter"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var errBackgroundIdleUnknown = errors.New("native background idle observation unavailable")
var errBackgroundUserActive = errors.New("native background user activity observed")

// Serve owns the worker lifetime; canonical job controls supply all authority.
func (b *Bound) Serve(ctx context.Context, transport mcp.Transport) error {
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		b.maintain(ctx)
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
	ctx, cancel := context.WithCancelCause(ctx)
	start := make(chan struct{})
	done := make(chan struct{})
	var once sync.Once
	check := func() error {
		if ctx.Err() != nil {
			return ctx.Err()
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
	service, err := b.maintenanceServiceWithEligibility(check)
	if err != nil {
		return err
	}
	_, err = service.RunDue(ctx)
	if cause := context.Cause(ctx); errors.Is(cause, errBackgroundUserActive) {
		return nil
	} else if errors.Is(cause, errBackgroundIdleUnknown) {
		return errBackgroundIdleUnknown
	}
	return err
}
