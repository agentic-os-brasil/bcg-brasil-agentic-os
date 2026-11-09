//go:build windows

package releaseprovider

import (
	"errors"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"unsafe"
)

// Hold the same no-follow ancestor descriptor used by private installation
// readers, and open only this ordinary leaf without cloud recall side effects.
func openSyncedFile(path string) (*os.File, error) {
	parent, err := scheduler.OpenDirectory(filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	defer parent.Close()
	name, err := windows.NewNTUnicodeString(filepath.Base(path))
	if err != nil {
		return nil, err
	}
	attributes := windows.OBJECT_ATTRIBUTES{RootDirectory: windows.Handle(parent.Fd()), ObjectName: name, Attributes: windows.OBJ_CASE_INSENSITIVE | windows.OBJ_DONT_REPARSE}
	attributes.Length = uint32(unsafe.Sizeof(attributes))
	var status windows.IO_STATUS_BLOCK
	var allocation int64
	var handle windows.Handle
	err = windows.NtCreateFile(&handle, windows.FILE_GENERIC_READ|windows.FILE_READ_ATTRIBUTES, &attributes, &status, &allocation, 0, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, windows.FILE_OPEN, windows.FILE_NON_DIRECTORY_FILE|windows.FILE_SYNCHRONOUS_IO_NONALERT|windows.FILE_OPEN_NO_RECALL, 0, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(handle), path)
	if file == nil {
		windows.CloseHandle(handle)
		return nil, errors.New("synced input handle unavailable")
	}
	return file, nil
}

func syncedSingleLink(file *os.File, _ os.FileInfo) bool {
	var info windows.ByHandleFileInformation
	const recallOnOpen = 0x00040000
	const recallOnDataAccess = 0x00400000
	return windows.GetFileInformationByHandle(windows.Handle(file.Fd()), &info) == nil && info.NumberOfLinks == 1 && info.FileAttributes&(windows.FILE_ATTRIBUTE_REPARSE_POINT|windows.FILE_ATTRIBUTE_OFFLINE|recallOnOpen|recallOnDataAccess) == 0
}
