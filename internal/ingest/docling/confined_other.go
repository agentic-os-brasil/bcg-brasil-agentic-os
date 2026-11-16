//go:build !darwin && !windows

package docling

import (
	"context"
	"errors"
)

func runConfinedBridge(context.Context, string, string, []byte, *boundedOutput, *boundedOutput) error {
	return errors.New("platform_unavailable")
}
