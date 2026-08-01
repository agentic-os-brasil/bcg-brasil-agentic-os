package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/walterselfreview"
)

// runWalterSelfReview is intentionally a typed, unavailable-by-default seam.
// Scheduler integration belongs to the separate orchestration change; this
// command only proves that Maestro can submit a bounded request and receive a
// metadata-safe fail-closed receipt.
func runWalterSelfReview(args []string, in io.Reader, out, errOut io.Writer) int {
	if len(args) != 2 || args[0] != "self-review" || args[1] != "--stdin" {
		fmt.Fprintln(errOut, "usage: bcgos agent walter self-review --stdin")
		return ExitUsage
	}
	var request walterselfreview.Request
	if err := decodeActivationJSON(in, &request); err != nil {
		return reportError(errOut, err)
	}
	_, receipt, err := walterselfreview.Review(context.Background(), request, nil, nil, nil, time.Now().UTC())
	if writeErr := writeJSON(out, receipt, errOut); writeErr != ExitOK {
		return writeErr
	}
	if errors.Is(err, walterselfreview.ErrUnavailable) {
		return ExitUnavailable
	}
	if err != nil {
		return ExitFailure
	}
	return ExitOK
}
