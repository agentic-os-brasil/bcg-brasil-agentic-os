//go:build windows

package memory

import (
	"syscall"
	"unsafe"
)

const moveFileWriteThrough = 0x8

var moveFileExW = syscall.NewLazyDLL("kernel32.dll").NewProc("MoveFileExW")

func durableRename(source, target string) error {
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
		uintptr(moveFileWriteThrough),
	)
	if result == 0 {
		return callErr
	}
	return nil
}
