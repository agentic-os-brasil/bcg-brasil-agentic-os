//go:build windows

package scheduler

import (
	"errors"
	"os"
)

func validatePrivateUpdateIdentity(file *os.File) error {
	return errors.New("private atomic update unavailable on this platform")
}
func (directory *secureDirectory) publishReplacement(source, target string) error {
	return errors.New("private atomic update unavailable on this platform")
}

func (directory *secureDirectory) removePrivateReplacement(name string) error {
	return errors.New("private atomic absence restore unavailable on this platform")
}
