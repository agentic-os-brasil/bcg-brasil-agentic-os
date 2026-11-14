//go:build !windows

package main

import "path/filepath"

func managedPacketParent(path string) (string, error) {
	return filepath.EvalSymlinks(path)
}
