//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package scheduler

import (
	"errors"
	"io"
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
var secureLeafStepHook func(string)

type secureDirectory struct {
	fd   int
	path string
}

// secureEnsurePrivatePath walks from the filesystem root using directory
// descriptors. Every component is opened with O_NOFOLLOW and all descendants
// are created with mkdirat, so an ancestor rename cannot redirect the walk to
// a symlink between validation and creation.
func secureEnsurePrivatePath(path string) error {
	directory, err := openSecureDirectory(path, true)
	if err != nil {
		return err
	}
	return directory.close()
}

// secureLookupPrivatePath performs the same no-follow descriptor walk without
// creating or chmodding anything. Missing components remain os.ErrNotExist so
// authority preflight stays read-only.
func secureLookupPrivatePath(path string) error {
	directory, err := openSecureDirectory(path, false)
	if err != nil {
		return err
	}
	return directory.close()
}

func openSecureDirectory(path string, create bool) (*secureDirectory, error) {
	stepHook := securePathStepHook
	if !create {
		stepHook = nil
	}
	fd, err := walkUnixDirectory(path, create, stepHook)
	if err != nil {
		return nil, err
	}
	return &secureDirectory{fd: fd, path: path}, nil
}

func (directory *secureDirectory) close() error {
	if directory == nil || directory.fd < 0 {
		return nil
	}
	err := unix.Close(directory.fd)
	directory.fd = -1
	return err
}

func (directory *secureDirectory) openFile(name string, flags int, perm os.FileMode) (*os.File, error) {
	if err := validateSecureLeaf(name); err != nil {
		return nil, err
	}
	if secureLeafStepHook != nil {
		secureLeafStepHook(filepath.Join(directory.path, name))
	}
	fd, err := unix.Openat(directory.fd, name, flags|unix.O_CLOEXEC|unix.O_NOFOLLOW, uint32(perm.Perm()))
	if err != nil {
		return nil, &os.PathError{Op: "openat", Path: filepath.Join(directory.path, name), Err: err}
	}
	file := os.NewFile(uintptr(fd), filepath.Join(directory.path, name))
	if file == nil {
		_ = unix.Close(fd)
		return nil, errors.New("scheduler secure file handle creation failed")
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		_ = file.Close()
		if err != nil {
			return nil, err
		}
		return nil, errors.New("scheduler secure file must be regular")
	}
	return file, nil
}

func (directory *secureDirectory) readFile(name string) ([]byte, error) {
	file, err := directory.openFile(name, os.O_RDONLY, 0)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	body, err := io.ReadAll(io.LimitReader(file, 1<<20))
	if err != nil {
		return nil, err
	}
	if len(body) >= 1<<20 {
		return nil, errors.New("scheduler secure file exceeds bounded size")
	}
	return body, nil
}

func (directory *secureDirectory) writeNewFile(name string, body []byte) error {
	file, err := directory.openFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = file.Close()
			_ = directory.removeFile(name)
		}
	}()
	if _, err := file.Write(body); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	committed = true
	return nil
}

func (directory *secureDirectory) writeFile(name string, body []byte) error {
	file, err := directory.openFile(name, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	if _, err := file.Write(body); err != nil {
		return err
	}
	return file.Sync()
}

func (directory *secureDirectory) removeFile(name string) error {
	if err := validateSecureLeaf(name); err != nil {
		return err
	}
	if secureLeafStepHook != nil {
		secureLeafStepHook(filepath.Join(directory.path, name))
	}
	if err := unix.Unlinkat(directory.fd, name, 0); err != nil {
		return &os.PathError{Op: "unlinkat", Path: filepath.Join(directory.path, name), Err: err}
	}
	return nil
}

func (directory *secureDirectory) readDir() ([]os.DirEntry, error) {
	file := os.NewFile(uintptr(directory.fd), directory.path)
	if file == nil {
		return nil, errors.New("scheduler secure directory handle creation failed")
	}
	// Transfer the descriptor to the os.File; callers close the directory after
	// ReadDir, so prevent close from being attempted twice.
	directory.fd = -1
	defer file.Close()
	return file.ReadDir(-1)
}

func walkUnixDirectory(path string, create bool, stepHook func(string)) (int, error) {
	root, components, err := unixPathParts(path)
	if err != nil {
		return -1, err
	}
	fd, err := unix.Open(root, secureDirectoryFlags, 0)
	if err != nil {
		return -1, err
	}
	for index, component := range components {
		child, openErr := unix.Openat(fd, component, secureDirectoryFlags, 0)
		created := false
		if create && errors.Is(openErr, unix.ENOENT) {
			if mkdirErr := unix.Mkdirat(fd, component, 0o700); mkdirErr != nil && !errors.Is(mkdirErr, unix.EEXIST) {
				_ = unix.Close(fd)
				return -1, mkdirErr
			}
			child, openErr = unix.Openat(fd, component, secureDirectoryFlags, 0)
			created = openErr == nil
		}
		if openErr != nil {
			_ = unix.Close(fd)
			return -1, &os.PathError{Op: "openat", Path: filepath.Join(path, component), Err: openErr}
		}
		if create && (created || index == len(components)-1) {
			if chmodErr := unix.Fchmod(child, 0o700); chmodErr != nil {
				_ = unix.Close(child)
				_ = unix.Close(fd)
				return -1, chmodErr
			}
		}
		_ = unix.Close(fd)
		fd = child
		if stepHook != nil {
			stepHook(component)
		}
	}
	return fd, nil
}

func secureOpenFile(path string, flags int, perm os.FileMode) (*os.File, error) {
	parent, name, err := unixFileParent(path)
	if err != nil {
		return nil, err
	}
	directory, err := openSecureDirectory(parent, false)
	if err != nil {
		return nil, err
	}
	defer directory.close()
	return directory.openFile(name, flags, perm)
}

func secureOpenLock(path string) (*os.File, error) {
	return secureOpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
}

func secureReadFile(path string) ([]byte, error) {
	parent, name, err := unixFileParent(path)
	if err != nil {
		return nil, err
	}
	directory, err := openSecureDirectory(parent, false)
	if err != nil {
		return nil, err
	}
	defer directory.close()
	return directory.readFile(name)
}

func secureWriteNewFile(path string, body []byte) error {
	parent, name, err := unixFileParent(path)
	if err != nil {
		return err
	}
	directory, err := openSecureDirectory(parent, false)
	if err != nil {
		return err
	}
	defer directory.close()
	return directory.writeNewFile(name, body)
}

func secureWriteFile(path string, body []byte) error {
	parent, name, err := unixFileParent(path)
	if err != nil {
		return err
	}
	directory, err := openSecureDirectory(parent, false)
	if err != nil {
		return err
	}
	defer directory.close()
	return directory.writeFile(name, body)
}

func secureRemoveFile(path string) error {
	parent, name, err := unixFileParent(path)
	if err != nil {
		return err
	}
	directory, err := openSecureDirectory(parent, false)
	if err != nil {
		return err
	}
	defer directory.close()
	return directory.removeFile(name)
}

func secureReadDir(path string) ([]os.DirEntry, error) {
	directory, err := openSecureDirectory(path, false)
	if err != nil {
		return nil, err
	}
	return directory.readDir()
}

func unixFileParent(path string) (string, string, error) {
	absolute, err := filepath.Abs(filepath.Clean(path))
	if err != nil || !filepath.IsAbs(absolute) || filepath.VolumeName(absolute) != "" {
		return "", "", errors.New("scheduler file path must be an absolute POSIX path")
	}
	name := filepath.Base(absolute)
	if name == "." || name == ".." || name == "" || strings.ContainsRune(name, 0) {
		return "", "", errors.New("scheduler file leaf is invalid")
	}
	return filepath.Dir(absolute), name, nil
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
