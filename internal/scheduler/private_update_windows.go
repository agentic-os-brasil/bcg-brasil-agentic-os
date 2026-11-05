//go:build windows

package scheduler

import (
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/winprivate"
	"golang.org/x/sys/windows"
	"os"
)

func validatePrivateUpdateIdentity(file *os.File) error {
	return winprivate.Validate(windows.Handle(file.Fd()), false)
}
func (d *secureDirectory) publishReplacement(source, target string) error {
	return d.native.Rename(source, d.native, target, true)
}
func (d *secureDirectory) removePrivateReplacement(name string) error {
	return d.native.Remove(name, false)
}
