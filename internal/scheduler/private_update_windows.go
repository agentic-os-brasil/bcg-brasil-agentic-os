//go:build windows

package scheduler

import (
	"errors"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/winprivate"
	"golang.org/x/sys/windows"
	"os"
)

func validateSingleLink(file *os.File) error {
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(windows.Handle(file.Fd()), &info); err != nil {
		return err
	}
	if info.FileAttributes&(windows.FILE_ATTRIBUTE_DIRECTORY|windows.FILE_ATTRIBUTE_REPARSE_POINT) != 0 || info.NumberOfLinks != 1 {
		return errors.New("regular file must have exactly one link and no reparse attribute")
	}
	return nil
}

func validatePrivateUpdateIdentity(file *os.File) error {
	return winprivate.Validate(windows.Handle(file.Fd()), false)
}
func (d *secureDirectory) publishReplacement(source, target string) error {
	return d.native.Rename(source, d.native, target, true)
}
func (d *secureDirectory) removePrivateReplacement(name string) error {
	return d.native.Remove(name, false)
}
