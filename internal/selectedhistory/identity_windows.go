//go:build windows

package selectedhistory

import (
	"fmt"
	"golang.org/x/sys/windows"
	"os"
)

func fileIdentity(file *os.File) (physical string, snapshot string, err error) {
	var st windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(windows.Handle(file.Fd()), &st); err != nil {
		return "", "", err
	}
	return fmt.Sprintf("%d:%d:%d", st.VolumeSerialNumber, st.FileIndexHigh, st.FileIndexLow), fmt.Sprintf("%d:%d:%d:%d:%d:%d:%d", st.VolumeSerialNumber, st.FileIndexHigh, st.FileIndexLow, st.FileSizeHigh, st.FileSizeLow, st.CreationTime.Nanoseconds(), st.LastWriteTime.Nanoseconds()), nil
}
