//go:build !windows

package selectedhistory

import (
	"fmt"
	"golang.org/x/sys/unix"
	"os"
)

func fileIdentity(file *os.File) (physical string, snapshot string, err error) {
	var st unix.Stat_t
	if err := unix.Fstat(int(file.Fd()), &st); err != nil {
		return "", "", err
	}
	info, err := file.Stat()
	if err != nil {
		return "", "", err
	}
	return fmt.Sprintf("%d:%d", st.Dev, st.Ino), fmt.Sprintf("%d:%d:%d:%d:%d:%d", st.Dev, st.Ino, info.Size(), info.ModTime().UnixNano(), st.Ctim.Sec, st.Ctim.Nsec), nil
}
