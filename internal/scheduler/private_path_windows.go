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

const secureWindowsDirectoryAccess = windows.FILE_GENERIC_READ | windows.FILE_GENERIC_WRITE | windows.DELETE

// Windows uses the native NT RootDirectory handle and OBJ_DONT_REPARSE for
// every component. This avoids the path-based Lstat/Mkdir/Chmod sequence and
// keeps the protected filesystem capability honest on reparse-point volumes.
func secureEnsurePrivatePath(path string) error {
	handle, err := walkWindowsDirectory(path, true, securePathStepHook)
	if err != nil {
		return err
	}
	return windows.CloseHandle(handle)
}

func secureLookupPrivatePath(path string) error {
	handle, err := walkWindowsDirectory(path, false, nil)
	if err != nil {
		return err
	}
	return windows.CloseHandle(handle)
}

func walkWindowsDirectory(path string, create bool, stepHook func(string)) (windows.Handle, error) {
	components, root, err := windowsPathParts(path)
	if err != nil {
		return windows.InvalidHandle, err
	}
	handle, err := ntOpenRelative(windows.InvalidHandle, `\??\`+root, secureWindowsDirectoryAccess, windows.FILE_OPEN, windows.FILE_DIRECTORY_FILE)
	if err != nil {
		return windows.InvalidHandle, err
	}
	for _, component := range components {
		disposition := uint32(windows.FILE_OPEN)
		if create {
			disposition = windows.FILE_OPEN_IF
		}
		child, openErr := ntOpenRelative(handle, component, secureWindowsDirectoryAccess, disposition, windows.FILE_DIRECTORY_FILE)
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
	oa := &windows.OBJECT_ATTRIBUTES{ObjectName: objectName, RootDirectory: root, Attributes: windows.OBJ_CASE_INSENSITIVE | windows.OBJ_DONT_REPARSE}
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
	parentHandle, err := walkWindowsDirectory(parent, false, nil)
	if err != nil {
		return nil, err
	}
	if secureLeafStepHook != nil {
		secureLeafStepHook(path)
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
	handle, err := ntOpenRelative(parentHandle, name, access, disposition, windows.FILE_NON_DIRECTORY_FILE|windows.FILE_SYNCHRONOUS_IO_NONALERT)
	_ = windows.CloseHandle(parentHandle)
	if err != nil {
		return nil, &os.PathError{Op: "NtCreateFile", Path: path, Err: err}
	}
	file := os.NewFile(uintptr(handle), path)
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

func secureOpenLock(path string) (*os.File, error) {
	return secureOpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
}

func secureReadFile(path string) ([]byte, error) {
	file, err := secureOpenFile(path, os.O_RDONLY, 0)
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

func secureWriteNewFile(path string, body []byte) error {
	file, err := secureOpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = file.Close()
			_ = secureRemoveFile(path)
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

func secureWriteFile(path string, body []byte) error {
	file, err := secureOpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	if _, err := file.Write(body); err != nil {
		return err
	}
	return file.Sync()
}

func secureRemoveFile(path string) error {
	parent, name, err := windowsFileParent(path)
	if err != nil {
		return err
	}
	parentHandle, err := walkWindowsDirectory(parent, false, nil)
	if err != nil {
		return err
	}
	if secureLeafStepHook != nil {
		secureLeafStepHook(path)
	}
	handle, err := ntOpenRelative(parentHandle, name, windows.DELETE|windows.FILE_GENERIC_READ, windows.FILE_OPEN, windows.FILE_NON_DIRECTORY_FILE|windows.FILE_SYNCHRONOUS_IO_NONALERT)
	_ = windows.CloseHandle(parentHandle)
	if err != nil {
		return &os.PathError{Op: "NtCreateFile", Path: path, Err: err}
	}
	defer windows.CloseHandle(handle)
	disposition := [1]byte{1}
	var iosb windows.IO_STATUS_BLOCK
	if err := windows.NtSetInformationFile(handle, &iosb, &disposition[0], uint32(len(disposition)), windows.FileDispositionInformation); err != nil {
		return err
	}
	return nil
}

func secureReadDir(path string) ([]os.DirEntry, error) {
	handle, err := walkWindowsDirectory(path, false, nil)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(handle), path)
	if file == nil {
		_ = windows.CloseHandle(handle)
		return nil, errors.New("scheduler secure directory handle creation failed")
	}
	defer file.Close()
	return file.ReadDir(-1)
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
