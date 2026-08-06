//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package workspaceimport

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
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
	rootFD, err := openSecureDirectoryPath(root, false)
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

func openSecureDirectoryPath(path string, create bool) (int, error) {
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
		if create && errors.Is(openErr, unix.ENOENT) {
			if mkdirErr := unix.Mkdirat(fd, component, 0o700); mkdirErr != nil && !errors.Is(mkdirErr, unix.EEXIST) {
				_ = unix.Close(fd)
				return -1, mkdirErr
			}
			child, openErr = unix.Openat(fd, component, secureSourceDirectoryFlags, 0)
		}
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
	parentFD, err := openSecureDirectoryPath(filepath.Dir(path), false)
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

func digestFileSecure(root, relative string, maxBytes int64) (string, bool, error) {
	file, err := openSourceFileSecure(root, relative)
	if errors.Is(err, unix.ENOENT) || errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	defer file.Close()
	hasher := sha256.New()
	written, err := io.Copy(hasher, io.LimitReader(file, maxBytes+1))
	if err != nil {
		return "", false, err
	}
	if written > maxBytes {
		return "", false, errors.New("file exceeds import limit")
	}
	return hex.EncodeToString(hasher.Sum(nil)), true, nil
}

// secureCommitFile installs a staged regular file through directory handles.
// Linkat is used instead of rename so an existing destination is never
// replaced, and both parent directories remain anchored despite concurrent
// path swaps. The stage file is removed only after the destination link exists.
func secureCommitFile(stageRoot, stageRelative, destinationRoot, destinationRelative string) (bool, error) {
	stagePath, err := pathSafe(stageRoot, stageRelative)
	if err != nil {
		return false, err
	}
	destinationPath, err := pathSafe(destinationRoot, destinationRelative)
	if err != nil {
		return false, err
	}
	stageParent, err := openSecureDirectoryPath(filepath.Dir(stagePath), false)
	if err != nil {
		return false, err
	}
	defer unix.Close(stageParent)
	destinationParent, err := openSecureDirectoryPath(filepath.Dir(destinationPath), true)
	if err != nil {
		return false, err
	}
	defer unix.Close(destinationParent)
	if secureCommitParentHook != nil {
		secureCommitParentHook(filepath.Dir(destinationPath))
	}
	if err := unix.Linkat(stageParent, filepath.Base(stagePath), destinationParent, filepath.Base(destinationPath), 0); err != nil {
		return false, &os.PathError{Op: "linkat", Path: destinationPath, Err: err}
	}
	if err := unix.Unlinkat(stageParent, filepath.Base(stagePath), 0); err != nil {
		return true, &os.PathError{Op: "unlinkat", Path: stagePath, Err: err}
	}
	return true, nil
}

// removeFileIfDigestSecure hashes and unlinks through the same opened parent
// directory. This prevents rollback from following a swapped parent between
// verification and deletion.
func removeFileIfDigestSecure(root, relative, expectedDigest string, maxBytes int64) error {
	path, err := pathSafe(root, relative)
	if err != nil {
		return err
	}
	parentFD, err := openSecureDirectoryPath(filepath.Dir(path), false)
	if err != nil {
		if errors.Is(err, unix.ENOENT) {
			return nil
		}
		return err
	}
	defer unix.Close(parentFD)
	leaf, err := unix.Openat(parentFD, filepath.Base(path), unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if errors.Is(err, unix.ENOENT) {
		return nil
	}
	if err != nil {
		return &os.PathError{Op: "openat", Path: path, Err: err}
	}
	file := os.NewFile(uintptr(leaf), path)
	if file == nil {
		_ = unix.Close(leaf)
		return errors.New("rollback file handle creation failed")
	}
	hasher := sha256.New()
	written, hashErr := io.Copy(hasher, io.LimitReader(file, maxBytes+1))
	closeErr := file.Close()
	if hashErr != nil {
		return hashErr
	}
	if closeErr != nil {
		return closeErr
	}
	if written > maxBytes || hex.EncodeToString(hasher.Sum(nil)) != expectedDigest {
		return errors.New("refusing rollback: destination changed after execution")
	}
	if err := unix.Unlinkat(parentFD, filepath.Base(path), 0); err != nil {
		return &os.PathError{Op: "unlinkat", Path: path, Err: err}
	}
	return nil
}

type planLock struct {
	file *os.File
}

func (lock *planLock) release() error {
	if lock == nil || lock.file == nil {
		return nil
	}
	err := unix.Flock(int(lock.file.Fd()), unix.LOCK_UN)
	closeErr := lock.file.Close()
	if err != nil {
		return err
	}
	return closeErr
}

func acquirePlanLock(root, planDigest string) (*planLock, error) {
	if !isSHA256(planDigest) {
		return nil, errors.New("invalid workspace import plan digest for lock")
	}
	directory, err := openSecureDirectoryPath(filepath.Join(root, "workspace-import", "locks"), true)
	if err != nil {
		return nil, err
	}
	defer unix.Close(directory)
	fd, err := unix.Openat(directory, planDigest+".lock", os.O_RDWR|os.O_CREATE|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0o600)
	if err != nil {
		return nil, &os.PathError{Op: "openat", Path: planDigest + ".lock", Err: err}
	}
	file := os.NewFile(uintptr(fd), filepath.Join(root, "workspace-import", "locks", planDigest+".lock"))
	if file == nil {
		_ = unix.Close(fd)
		return nil, errors.New("workspace import lock handle creation failed")
	}
	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		_ = file.Close()
		if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
			return nil, errors.New("workspace import is already executing for this plan")
		}
		return nil, err
	}
	return &planLock{file: file}, nil
}

func syncDirectoryPath(path string) error {
	fd, err := openSecureDirectoryPath(path, false)
	if err != nil {
		return err
	}
	defer unix.Close(fd)
	return unix.Fsync(fd)
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
