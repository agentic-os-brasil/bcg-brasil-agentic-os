//go:build !windows

package macosadapter

import (
	"errors"
	"io"
	"os"
	"syscall"
)

const maxPlistBytes = 64 * 1024

func readLaunchAgentBody(path string) ([]byte, error) {
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), path)
	if file == nil {
		_ = syscall.Close(fd)
		return nil, errors.New("LaunchAgent plist open failed")
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("LaunchAgent plist must be a regular file")
	}
	limited := io.LimitReader(file, maxPlistBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if len(body) > maxPlistBytes {
		return nil, errors.New("LaunchAgent plist exceeds bounded size")
	}
	return body, nil
}
