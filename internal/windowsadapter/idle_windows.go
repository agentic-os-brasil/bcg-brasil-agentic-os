//go:build windows

package windowsadapter

import (
	"context"
	"runtime"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	idleUser32             = windows.NewLazySystemDLL("user32.dll")
	idleWTS                = windows.NewLazySystemDLL("wtsapi32.dll")
	idleKernel32           = windows.NewLazySystemDLL("kernel32.dll")
	idleLastInput          = idleUser32.NewProc("GetLastInputInfo")
	idleWindowStation      = idleUser32.NewProc("GetProcessWindowStation")
	idleObjectInformation  = idleUser32.NewProc("GetUserObjectInformationW")
	idleSessionInformation = idleWTS.NewProc("WTSQuerySessionInformationW")
	idleFreeMemory         = idleWTS.NewProc("WTSFreeMemory")
	idleTickCount          = idleKernel32.NewProc("GetTickCount64")
)

const (
	idleObjectFlags    = 1
	idleVisibleStation = 1
	idleConnectState   = 8
	idleSessionActive  = 0
)

// ObserveIdle accepts only the current interactive active session's bounded counter.
func ObserveIdle(ctx context.Context) (time.Duration, error) {
	if ctx.Err() != nil {
		return 0, ctx.Err()
	}
	for _, proc := range []*windows.LazyProc{idleLastInput, idleWindowStation, idleObjectInformation, idleSessionInformation, idleFreeMemory, idleTickCount} {
		if err := proc.Find(); err != nil {
			return 0, ErrIdleUnavailable
		}
	}
	var session uint32
	if err := windows.ProcessIdToSessionId(windows.GetCurrentProcessId(), &session); err != nil || session == 0 {
		return 0, ErrIdleUnavailable
	}
	station, _, _ := idleWindowStation.Call()
	if station == 0 {
		return 0, ErrIdleUnavailable
	}
	var flags struct{ Inherit, Reserved, Flags uint32 }
	var needed uint32
	ok, _, _ := idleObjectInformation.Call(station, idleObjectFlags, uintptr(unsafe.Pointer(&flags)), unsafe.Sizeof(flags), uintptr(unsafe.Pointer(&needed)))
	runtime.KeepAlive(&flags)
	runtime.KeepAlive(&needed)
	if ok == 0 || needed > uint32(unsafe.Sizeof(flags)) || flags.Flags&idleVisibleStation == 0 {
		return 0, ErrIdleUnavailable
	}
	var state *uint32
	var size uint32
	ok, _, _ = idleSessionInformation.Call(0, uintptr(session), idleConnectState, uintptr(unsafe.Pointer(&state)), uintptr(unsafe.Pointer(&size)))
	runtime.KeepAlive(&state)
	runtime.KeepAlive(&size)
	if state != nil {
		defer idleFreeMemory.Call(uintptr(unsafe.Pointer(state)))
	}
	if ok == 0 || state == nil {
		return 0, ErrIdleUnavailable
	}
	if size != 4 || *state != idleSessionActive {
		return 0, ErrIdleUnavailable
	}
	var input struct{ Size, Tick uint32 }
	input.Size = uint32(unsafe.Sizeof(input))
	ok, _, _ = idleLastInput.Call(uintptr(unsafe.Pointer(&input)))
	runtime.KeepAlive(&input)
	if ok == 0 {
		return 0, ErrIdleUnavailable
	}
	ticks, _, _ := idleTickCount.Call()
	if ctx.Err() != nil {
		return 0, ctx.Err()
	}
	return idleDuration(input.Tick, uint64(ticks))
}
