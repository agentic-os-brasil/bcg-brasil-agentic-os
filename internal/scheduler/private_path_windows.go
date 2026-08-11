//go:build windows

package scheduler

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// This seam is only used by the package's cross-platform adversarial tests.
var securePathStepHook func(string)
var secureLeafStepHook func(string)

type secureDirectory struct {
	handle windows.Handle
	path   string
}

const (
	secureWindowsDirectoryTraverse = windows.FILE_LIST_DIRECTORY | windows.FILE_READ_ATTRIBUTES | windows.FILE_TRAVERSE | windows.SYNCHRONIZE
	secureWindowsDirectoryOptions  = windows.FILE_DIRECTORY_FILE | windows.FILE_SYNCHRONOUS_IO_NONALERT
	// x/sys/windows does not expose FILE_ADD_SUBDIRECTORY on every supported
	// version, but it is a stable Windows directory access mask.
	secureWindowsDirectoryCreate = secureWindowsDirectoryTraverse | 0x0004
)

// Windows uses the native NT RootDirectory handle and OBJ_DONT_REPARSE for
// every component. This avoids the path-based Lstat/Mkdir/Chmod sequence and
// keeps the protected filesystem capability honest on reparse-point volumes.
func secureEnsurePrivatePath(path string) error {
	directory, err := openSecureDirectory(path, true)
	if err != nil {
		return err
	}
	return directory.close()
}

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
	handle, err := walkWindowsDirectory(path, create, stepHook)
	if err != nil {
		return nil, err
	}
	return &secureDirectory{handle: handle, path: path}, nil
}

func (directory *secureDirectory) close() error {
	if directory == nil || directory.handle == windows.InvalidHandle {
		return nil
	}
	err := windows.CloseHandle(directory.handle)
	directory.handle = windows.InvalidHandle
	return err
}

func (directory *secureDirectory) openFile(name string, flags int, perm os.FileMode) (*os.File, error) {
	if err := validateSecureLeaf(name); err != nil {
		return nil, err
	}
	if secureLeafStepHook != nil {
		secureLeafStepHook(filepath.Join(directory.path, name))
	}
	access := uint32(windows.FILE_GENERIC_READ | windows.FILE_READ_ATTRIBUTES)
	if flags&os.O_RDWR != 0 {
		access = windows.FILE_GENERIC_READ | windows.FILE_GENERIC_WRITE | windows.FILE_READ_ATTRIBUTES
	} else if flags&os.O_WRONLY != 0 {
		access = windows.FILE_GENERIC_WRITE | windows.FILE_READ_ATTRIBUTES
	}
	disposition := uint32(windows.FILE_OPEN)
	if flags&os.O_CREATE != 0 {
		if flags&os.O_EXCL != 0 {
			disposition = windows.FILE_CREATE
		} else if flags&os.O_TRUNC != 0 {
			disposition = windows.FILE_OVERWRITE_IF
		} else {
			disposition = windows.FILE_OPEN_IF
		}
	} else if flags&os.O_TRUNC != 0 {
		disposition = windows.FILE_OVERWRITE
	}
	handle, err := ntOpenRelative(directory.handle, name, access, disposition, windows.FILE_NON_DIRECTORY_FILE|windows.FILE_SYNCHRONOUS_IO_NONALERT)
	if err != nil {
		return nil, &os.PathError{Op: "NtCreateFile", Path: filepath.Join(directory.path, name), Err: err}
	}
	file := os.NewFile(uintptr(handle), filepath.Join(directory.path, name))
	if file == nil {
		_ = windows.CloseHandle(handle)
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
	handle, err := ntOpenRelative(directory.handle, name, windows.DELETE|windows.FILE_READ_ATTRIBUTES|windows.SYNCHRONIZE, windows.FILE_OPEN, windows.FILE_NON_DIRECTORY_FILE|windows.FILE_SYNCHRONOUS_IO_NONALERT)
	if err != nil {
		return &os.PathError{Op: "NtCreateFile", Path: filepath.Join(directory.path, name), Err: err}
	}
	defer windows.CloseHandle(handle)
	disposition := [1]byte{1}
	var iosb windows.IO_STATUS_BLOCK
	if err := windows.NtSetInformationFile(handle, &iosb, &disposition[0], uint32(len(disposition)), windows.FileDispositionInformation); err != nil {
		return err
	}
	return nil
}

func (directory *secureDirectory) readDir() ([]os.DirEntry, error) {
	file := os.NewFile(uintptr(directory.handle), directory.path)
	if file == nil {
		return nil, errors.New("scheduler secure directory handle creation failed")
	}
	directory.handle = windows.InvalidHandle
	defer file.Close()
	return file.ReadDir(-1)
}

func walkWindowsDirectory(path string, create bool, stepHook func(string)) (windows.Handle, error) {
	components, root, err := windowsPathParts(path)
	if err != nil {
		return windows.InvalidHandle, err
	}
	handle, err := ntOpenRelative(windows.InvalidHandle, `\??\`+root, secureWindowsDirectoryTraverse, windows.FILE_OPEN, secureWindowsDirectoryOptions)
	if err != nil {
		return windows.InvalidHandle, err
	}
	for index, component := range components {
		child, openErr := ntOpenRelative(handle, component, secureWindowsDirectoryTraverse, windows.FILE_OPEN, secureWindowsDirectoryOptions)
		if create && isWindowsNotExist(openErr) {
			// A relative NT name of "." is not a portable way to reopen the
			// current directory. Reopen the canonical child from the volume
			// root instead. OBJ_DONT_REPARSE remains in force, so this is still
			// a no-follow traversal and the create is anchored to the validated
			// volume path.
			childPath := `\??\` + root + strings.Join(components[:index+1], `\`)
			child, openErr = ntOpenRelative(windows.InvalidHandle, childPath, secureWindowsDirectoryCreate, windows.FILE_OPEN_IF, secureWindowsDirectoryOptions)
		}
		if openErr != nil {
			_ = windows.CloseHandle(handle)
			return windows.InvalidHandle, openErr
		}
		_ = windows.CloseHandle(handle)
		handle = child
		if stepHook != nil {
			stepHook(component)
		}
	}
	return handle, nil
}

func ntOpenRelative(root windows.Handle, name string, access, disposition, options uint32) (windows.Handle, error) {
	objectName, err := windows.NewNTUnicodeString(name)
	if err != nil {
		return windows.InvalidHandle, err
	}
	rootDirectory := root
	if root == windows.InvalidHandle {
		rootDirectory = 0
	}
	oa := &windows.OBJECT_ATTRIBUTES{ObjectName: objectName, RootDirectory: rootDirectory, Attributes: windows.OBJ_CASE_INSENSITIVE | windows.OBJ_DONT_REPARSE}
	oa.Length = uint32(unsafe.Sizeof(*oa))
	var iosb windows.IO_STATUS_BLOCK
	var allocationSize int64
	var handle windows.Handle
	err = windows.NtCreateFile(&handle, access, oa, &iosb, &allocationSize, 0, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, disposition, options, 0, 0)
	if err != nil {
		if status, ok := err.(windows.NTStatus); ok {
			err = status.Errno()
		}
		return windows.InvalidHandle, err
	}
	return handle, nil
}

func secureOpenFile(path string, flags int, perm os.FileMode) (*os.File, error) {
	parent, name, err := windowsFileParent(path)
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
	parent, name, err := windowsFileParent(path)
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
	parent, name, err := windowsFileParent(path)
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
	parent, name, err := windowsFileParent(path)
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
	parent, name, err := windowsFileParent(path)
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

func isWindowsNotExist(err error) bool {
	return errors.Is(err, os.ErrNotExist) || errors.Is(err, windows.ERROR_FILE_NOT_FOUND) || errors.Is(err, windows.ERROR_PATH_NOT_FOUND)
}

func windowsPathParts(path string) ([]string, string, error) {
	absolute, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return nil, "", err
	}
	volume := filepath.VolumeName(absolute)
	if len(volume) != 2 || volume[1] != ':' || !filepath.IsAbs(absolute) {
		return nil, "", errors.New("scheduler path must be an absolute local Windows path")
	}
	remainder := strings.TrimPrefix(absolute, volume)
	remainder = strings.TrimLeft(remainder, `\`+string(filepath.Separator))
	components := make([]string, 0)
	for _, component := range strings.FieldsFunc(remainder, func(r rune) bool { return r == '\\' || r == '/' }) {
		if component != "" && component != "." {
			components = append(components, component)
		}
	}
	return components, volume + string(filepath.Separator), nil
}

func windowsFileParent(path string) (string, string, error) {
	absolute, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", "", err
	}
	name := filepath.Base(absolute)
	if name == "." || name == ".." || name == "" || strings.ContainsRune(name, 0) {
		return "", "", errors.New("scheduler file leaf is invalid")
	}
	return filepath.Dir(absolute), name, nil
}
