//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package workspaceimport

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

const secureSourceDirectoryFlags = unix.O_RDONLY | unix.O_DIRECTORY | unix.O_CLOEXEC | unix.O_NOFOLLOW

// openSourceFileSecure resolves each source component relative to an already
// opened directory descriptor. This prevents a concurrent parent-directory
// rename from redirecting the copy through a symlink.
func openSourceFileSecure(root, relative string) (*os.File, error) {
	path, err := pathSafe(root, relative)
	if err != nil {
		return nil, err
	}
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(relative)))
	components := splitSecurePath(clean)
	if len(components) == 0 {
		return nil, errors.New("source file path is empty")
	}
	rootFD, err := openSecureDirectoryPath(root)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: root, Err: err}
	}
	fd := rootFD
	defer func() { _ = unix.Close(fd) }()
	for _, component := range components[:len(components)-1] {
		child, openErr := unix.Openat(fd, component, secureSourceDirectoryFlags, 0)
		if openErr != nil {
			return nil, &os.PathError{Op: "openat", Path: path, Err: openErr}
		}
		_ = unix.Close(fd)
		fd = child
	}
	leaf, err := unix.Openat(fd, components[len(components)-1], unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, &os.PathError{Op: "openat", Path: path, Err: err}
	}
	file := os.NewFile(uintptr(leaf), path)
	if file == nil {
		_ = unix.Close(leaf)
		return nil, errors.New("source file handle creation failed")
	}
	info, statErr := file.Stat()
	if statErr != nil || !info.Mode().IsRegular() {
		_ = file.Close()
		if statErr != nil {
			return nil, statErr
		}
		return nil, errors.New("source file must be regular")
	}
	return file, nil
}

func openSecureDirectoryPath(path string) (int, error) {
	canonical, err := canonicalSecureRoot(path)
	if err != nil {
		return -1, err
	}
	clean := filepath.Clean(canonical)
	if !filepath.IsAbs(clean) {
		return -1, errors.New("secure directory path must be absolute")
	}
	fd, err := unix.Open(string(filepath.Separator), secureSourceDirectoryFlags, 0)
	if err != nil {
		return -1, err
	}
	for _, component := range splitSecurePath(clean) {
		child, openErr := unix.Openat(fd, component, secureSourceDirectoryFlags, 0)
		if openErr != nil {
			_ = unix.Close(fd)
			return -1, openErr
		}
		_ = unix.Close(fd)
		fd = child
	}
	return fd, nil
}

func canonicalSecureRoot(path string) (string, error) {
	absolute, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", err
	}
	temporary, err := filepath.Abs(filepath.Clean(os.TempDir()))
	if err != nil {
		return "", err
	}
	physicalTemporary, err := filepath.EvalSymlinks(temporary)
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(temporary, absolute)
	if err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return filepath.Join(physicalTemporary, relative), nil
	}
	return absolute, nil
}

func removeSourceFileSecure(root, relative string) error {
	path, err := pathSafe(root, relative)
	if err != nil {
		return err
	}
	parentFD, err := openSecureDirectoryPath(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer unix.Close(parentFD)
	if err := unix.Unlinkat(parentFD, filepath.Base(path), 0); err != nil {
		if errors.Is(err, unix.ENOENT) {
			return nil
		}
		return &os.PathError{Op: "unlinkat", Path: path, Err: err}
	}
	return nil
}

func splitSecurePath(path string) []string {
	parts := strings.Split(filepath.ToSlash(path), "/")
	result := make([]string, 0, len(parts))
	for _, component := range parts {
		if component != "" && component != "." {
			result = append(result, component)
		}
	}
	return result
}
