//go:build windows

package execution

import (
	"syscall"
	"unsafe"
)

const (
	moveFileReplaceExisting = 0x1
	moveFileWriteThrough    = 0x8
)

var moveFileExW = syscall.NewLazyDLL("kernel32.dll").NewProc("MoveFileExW")

func durableRename(source, target string) error {
	return moveFile(source, target, moveFileWriteThrough)
}

func durableReplace(source, target string) error {
	return moveFile(source, target, moveFileReplaceExisting|moveFileWriteThrough)
}

func durablePublishNoClobber(source, target string) error {
	return moveFile(source, target, moveFileWriteThrough)
}

func moveFile(source, target string, flags uintptr) error {
	sourcePointer, err := syscall.UTF16PtrFromString(source)
	if err != nil {
		return err
	}
	targetPointer, err := syscall.UTF16PtrFromString(target)
	if err != nil {
		return err
	}
	result, _, callErr := moveFileExW.Call(
		uintptr(unsafe.Pointer(sourcePointer)),
		uintptr(unsafe.Pointer(targetPointer)),
		flags,
	)
	if result == 0 {
		return callErr
	}
	return nil
}
