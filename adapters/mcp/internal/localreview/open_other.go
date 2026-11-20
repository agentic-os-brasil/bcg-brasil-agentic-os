//go:build !darwin && !windows

package localreview

import (
	"context"
	"os/exec"
	"time"
)

func openBrowser(ctx context.Context, address string) error {
	bounded, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return exec.CommandContext(bounded, "/usr/bin/xdg-open", address).Run()
}
