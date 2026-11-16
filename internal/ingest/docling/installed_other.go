//go:build !darwin && !linux && !windows

package docling

import (
	"context"
	"errors"
)

func verifyInstalled(context.Context, string, map[string]entry) error {
	return errors.New("platform_unavailable")
}
