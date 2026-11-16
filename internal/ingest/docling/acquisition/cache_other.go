//go:build !darwin && !linux && !windows

package acquisition

import (
	"errors"
	"io"
	"os"
)

func directoryOwner(string) error  { return errors.New("acquisition_platform_unavailable") }
func mkdirPrivateNew(string) error { return errors.New("acquisition_platform_unavailable") }
func openPrivateNew(string) (*os.File, error) {
	return nil, errors.New("acquisition_platform_unavailable")
}
func holdPrivateDirectory(string) (io.Closer, error) {
	return nil, errors.New("acquisition_platform_unavailable")
}
