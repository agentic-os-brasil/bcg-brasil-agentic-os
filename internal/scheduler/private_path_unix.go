//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package scheduler

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

const secureDirectoryFlags = unix.O_RDONLY | unix.O_DIRECTORY | unix.O_CLOEXEC | unix.O_NOFOLLOW

// securePathStepHook is a package-local adversarial-test seam. It is nil in
// production and lets tests replace a path ancestor after its descriptor has
// been opened, proving subsequent components stay relative to that descriptor.
var securePathStepHook func(string)

// secureEnsurePrivatePath walks from the filesystem root using directory
// descriptors. Every component is opened with O_NOFOLLOW and all descendants
// are created with mkdirat, so an ancestor rename cannot redirect the walk to
// a symlink between validation and creation.
func secureEnsurePrivatePath(path string) error {
	root, components, err := unixPathParts(path)
	if err != nil {
		return err
	}
	fd, err := unix.Open(root, secureDirectoryFlags, 0)
	if err != nil {
		return err
	}
	defer func() { _ = unix.Close(fd) }()
	for index, component := range components {
		child, openErr := unix.Openat(fd, component, secureDirectoryFlags, 0)
		created := false
		if errors.Is(openErr, unix.ENOENT) {
			if mkdirErr := unix.Mkdirat(fd, component, 0o700); mkdirErr != nil && !errors.Is(mkdirErr, unix.EEXIST) {
				return mkdirErr
			}
			child, openErr = unix.Openat(fd, component, secureDirectoryFlags, 0)
			created = openErr == nil
		}
		if openErr != nil {
			return openErr
		}
		if created || index == len(components)-1 {
			if chmodErr := unix.Fchmod(child, 0o700); chmodErr != nil {
				_ = unix.Close(child)
				return chmodErr
			}
		}
		_ = unix.Close(fd)
		fd = child
		if securePathStepHook != nil {
			securePathStepHook(component)
		}
	}
	return nil
}

// secureLookupPrivatePath performs the same no-follow descriptor walk without
// creating or chmodding anything. Missing components remain os.ErrNotExist so
// authority preflight stays read-only.
func secureLookupPrivatePath(path string) error {
	root, components, err := unixPathParts(path)
	if err != nil {
		return err
	}
	fd, err := unix.Open(root, secureDirectoryFlags, 0)
	if err != nil {
		return err
	}
	defer func() { _ = unix.Close(fd) }()
	for _, component := range components {
		child, err := unix.Openat(fd, component, secureDirectoryFlags, 0)
		if err != nil {
			return &os.PathError{Op: "openat", Path: filepath.Join(path, component), Err: err}
		}
		_ = unix.Close(fd)
		fd = child
	}
	return nil
}

func unixPathParts(path string) (string, []string, error) {
	absolute, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", nil, err
	}
	if !filepath.IsAbs(absolute) || filepath.VolumeName(absolute) != "" {
		return "", nil, errors.New("scheduler path must be an absolute POSIX path")
	}
	remainder := strings.TrimPrefix(absolute, string(filepath.Separator))
	components := make([]string, 0)
	for _, component := range strings.Split(remainder, string(filepath.Separator)) {
		if component != "" && component != "." {
			components = append(components, component)
		}
	}
	return string(filepath.Separator), components, nil
}
