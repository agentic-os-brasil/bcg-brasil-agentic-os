//go:build windows

package workspaceagent

import (
	"syscall"
	"unsafe"
)

const moveFileReplaceExistingWriteThrough = 0x1 | 0x8

var moveFileExW = syscall.NewLazyDLL("kernel32.dll").NewProc("MoveFileExW")

func durableReplace(source, target string) error {
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
		uintptr(moveFileReplaceExistingWriteThrough),
	)
	if result == 0 {
		return callErr
	}
	return nil
}
