//go:build windows

package docling

import (
	"golang.org/x/sys/windows"
	"os"
)

func fileReadSupported() bool { return true }
func singleLink(file *os.File, _ os.FileInfo) bool {
	var info windows.ByHandleFileInformation
	return windows.GetFileInformationByHandle(windows.Handle(file.Fd()), &info) == nil && info.NumberOfLinks == 1 && info.FileAttributes&(windows.FILE_ATTRIBUTE_REPARSE_POINT|windows.FILE_ATTRIBUTE_OFFLINE|0x00040000|0x00400000) == 0
}
