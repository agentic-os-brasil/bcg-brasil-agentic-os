//go:build windows

package main

import (
	"errors"
	"os"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/winprivate"
)

// Resolve only this managed parent; private paths still use strict no-follow.
func managedPacketParent(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.IsDir() {
		return "", errors.New("managed packet parent unavailable")
	}
	return winprivate.FinalLocalPath(file)
}
