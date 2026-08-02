//go:build windows

package macosadapter

import (
	"errors"
	"io"
	"os"
)

const maxPlistBytes = 64 * 1024

// Windows never activates the native LaunchAgent lifecycle. Keep filesystem
// tests target-neutral while retaining bounded and regular-file checks.
func readLaunchAgentBody(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("LaunchAgent plist must be a regular file")
	}
	body, err := io.ReadAll(io.LimitReader(file, maxPlistBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxPlistBytes {
		return nil, errors.New("LaunchAgent plist exceeds bounded size")
	}
	return body, nil
}
