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

func digestFileSecure(root, relative string, maxBytes int64) (string, bool, error) {
	return "", false, errors.New("secure workspace import file access is unavailable on this platform")
}

func secureCommitFile(stageRoot, stageRelative, destinationRoot, destinationRelative string) (bool, error) {
	return false, errors.New("secure workspace import destination commit is unavailable on this platform")
}

func removeFileIfDigestSecure(root, relative, expectedDigest string, maxBytes int64) error {
	return errors.New("secure workspace import rollback is unavailable on this platform")
}

type planLock struct{}

func (lock *planLock) release() error { return nil }

func acquirePlanLock(root, planDigest string) (*planLock, error) {
	return nil, errors.New("secure workspace import locking is unavailable on this platform")
}

func syncDirectoryPath(path string) error {
	return errors.New("durable workspace import journal sync is unavailable on this platform")
}
