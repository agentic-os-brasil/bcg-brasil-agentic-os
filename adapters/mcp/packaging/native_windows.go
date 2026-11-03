//go:build windows

package packaging

import (
	"errors"
	"os/exec"
)

func validateNativeScratchIdentity(string) error {
	return errors.New("native private scratch admission unavailable on Windows")
}
func configureNativeProcess(*exec.Cmd) {}
