package product

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/lifecycle"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/memory"
)

// queueLifecycleSignal preserves diagnostics even when explicit capture setup
// is absent. Sealing or queuing never admits a canonical memory capture.
func (b *Bound) queueLifecycleSignal(receipt lifecycle.Receipt) {
	if receipt.Event != lifecycle.StopFinalize {
		return
	}
	unavailable := func() {
		fmt.Fprintln(os.Stderr, "maestro: lifecycle observation retained; body-free memory signal unavailable; inspect explicit setup and capture-job status")
	}
	signal, err := lifecycle.SignalFromReceipt(b.workspaceID, receipt)
	if err != nil {
		unavailable()
		return
	}
	attestor := memory.CaptureAttestor{Root: filepath.Join(b.home, "brain", "memory")}
	signal, err = attestor.SealSignal(signal)
	if err != nil {
		unavailable()
		return
	}
	if err := b.guardPublication(func() error { return lifecycle.RecordSignalConfined(filepath.Join(b.home, "state"), signal) }); err != nil {
		unavailable()
	}
}
