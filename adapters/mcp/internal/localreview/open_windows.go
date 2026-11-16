package localreview

import (
	"context"
	"golang.org/x/sys/windows"
	"os/exec"
	"path/filepath"
	"time"
)

func openBrowser(ctx context.Context, address string) error {
	directory, err := windows.GetSystemDirectory()
	if err != nil {
		return errUnavailable
	}
	bounded, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return exec.CommandContext(bounded, filepath.Join(directory, "rundll32.exe"), "url.dll,FileProtocolHandler", address).Run()
}
