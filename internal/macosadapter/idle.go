package macosadapter

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"time"
)

const ioregPath = "/usr/sbin/ioreg"
const idleProbeTimeout = 2 * time.Second

var hidIdlePattern = regexp.MustCompile(`"HIDIdleTime"\s*=\s*([0-9]+)`)

type NativeIdleState string

const (
	NativeIdleActive    NativeIdleState = "active"
	NativeIdleConfirmed NativeIdleState = "idle"
)

// ObserveIdle reads the current user's HID idle counter from the native
// IOHIDSystem service. Any unsupported platform, malformed output or command
// failure is an error so callers can preserve the unknown/fail-closed state.
func ObserveIdle(ctx context.Context, threshold time.Duration) (NativeIdleState, time.Duration, error) {
	if runtime.GOOS != "darwin" {
		return "", 0, errors.New("native idle observation is available only on macOS")
	}
	probeCtx, cancel := context.WithTimeout(ctx, idleProbeTimeout)
	defer cancel()
	command := exec.CommandContext(probeCtx, ioregPath, "-c", "IOHIDSystem", "-d", "1")
	var output boundedBuffer
	command.Stdout, command.Stderr = &output, &output
	if err := command.Run(); err != nil {
		return "", 0, errors.New("native idle observation failed")
	}
	idleFor, err := ParseHIDIdleTime(output.String())
	if err != nil {
		return "", 0, err
	}
	state, err := ClassifyIdle(idleFor, threshold)
	return state, idleFor, err
}

// ParseHIDIdleTime accepts exactly one nanosecond counter. This keeps the
// native probe bounded and prevents ambiguous IORegistry output from becoming
// eligibility evidence.
func ParseHIDIdleTime(output string) (time.Duration, error) {
	matches := hidIdlePattern.FindAllStringSubmatch(output, 2)
	if len(matches) != 1 {
		return 0, errors.New("native idle observation did not contain one HID idle counter")
	}
	nanoseconds, err := strconv.ParseUint(matches[0][1], 10, 64)
	if err != nil || nanoseconds > uint64(^uint64(0)>>1) {
		return 0, errors.New("native idle observation contained an invalid HID idle counter")
	}
	return time.Duration(nanoseconds), nil
}

func ClassifyIdle(idleFor, threshold time.Duration) (NativeIdleState, error) {
	if idleFor < 0 || threshold <= 0 {
		return "", fmt.Errorf("positive idle duration threshold is required")
	}
	if idleFor >= threshold {
		return NativeIdleConfirmed, nil
	}
	return NativeIdleActive, nil
}
