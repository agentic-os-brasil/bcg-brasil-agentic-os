//go:build windows

package datahome

import (
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/winprivate"
	"golang.org/x/sys/windows"
)

func validateOwnership(path string) error {
	d, e := winprivate.Inspect(path)
	if e == nil {
		defer d.Close()
		return winprivate.Validate(d.Handle(), true)
	}
	f, fileErr := winprivate.OpenRegular(path)
	if fileErr != nil {
		return e
	}
	defer f.Close()
	return winprivate.Validate(windows.Handle(f.Fd()), false)
}
func ownershipVerified() bool {
	sid, e := winprivate.CurrentUser()
	return e == nil && sid != nil && sid.IsValid()
}
