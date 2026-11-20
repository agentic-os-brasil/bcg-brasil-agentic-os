package setup

import (
	"errors"
	"io"
	"os"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installer"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

// Keep the exact admitted external-launcher archive in the new private release
// slot. The old path and bytes remain untouched; compatible reconnects therefore
// preserve the installation confirmation instead of requesting reinstallation.
func retainCurrentDesktopCarrier(source, destination, expected string) error {
	if actual, err := installer.ClientArchiveDigest(destination); err == nil {
		if actual == expected {
			return nil
		}
		return errors.New("new Desktop carrier slot occupied; preserved")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	before, err := installer.ClientArchiveDigest(source)
	if err != nil || before != expected {
		return errors.New("previous Desktop carrier changed")
	}
	in, err := scheduler.OpenRegularFile(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	count, copyErr := io.Copy(out, io.LimitReader(in, (128<<20)+1))
	syncErr := out.Sync()
	closeErr := out.Close()
	if err := errors.Join(copyErr, syncErr, closeErr); err != nil {
		return err
	}
	if count <= 0 || count > 128<<20 {
		return errors.New("Desktop carrier copy bound")
	}
	after, err := installer.ClientArchiveDigest(source)
	if err != nil || after != expected {
		return errors.New("previous Desktop carrier changed during copy")
	}
	actual, err := installer.ClientArchiveDigest(destination)
	if err != nil || actual != expected {
		return errors.New("retained Desktop carrier differs")
	}
	return nil
}
