//go:build !aix && !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris

package workspaceimport

import (
	"errors"
	"os"
)

// Platforms without descriptor-relative no-follow primitives fail closed
// instead of silently reverting to path-based source reads.
func openSourceFileSecure(root, relative string) (*os.File, error) {
	return nil, errors.New("secure workspace import source access is unavailable on this platform")
}

func removeSourceFileSecure(root, relative string) error {
	return errors.New("secure workspace import source access is unavailable on this platform")
}
