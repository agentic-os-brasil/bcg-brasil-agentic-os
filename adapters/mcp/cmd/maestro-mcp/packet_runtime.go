package main

import (
	"errors"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp/packaging"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/processidentity"
)

// Desktop can install below an existing application-support ancestor alias.
// The fallback belongs only to this executing local MCPB, never private state.
func runtimePacketRoot(root string, pin processidentity.Identity) (string, packaging.PacketStatus, error) {
	status, strictErr := packaging.ValidatePacket(root)
	if strictErr == nil {
		return root, status, nil
	}
	clean, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return "", status, strictErr
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(clean))
	if err != nil {
		return "", status, strictErr
	}
	physical := filepath.Join(parent, filepath.Base(clean))
	samePath := func(left, right string) bool {
		if runtime.GOOS == "windows" {
			return strings.EqualFold(left, right)
		}
		return left == right
	}
	if samePath(physical, clean) {
		return "", status, strictErr
	}
	status, err = packaging.ValidatePacket(physical)
	if err != nil {
		return "", status, err
	}
	if status.Format != "mcpb" || status.Version != "0.0.0" || status.DesktopPilot == nil || status.Stable != nil || status.RegistrySHA256 != "" || status.NativeMode != "mcp_only" {
		return "", status, errors.New("local Desktop packet alias unavailable")
	}
	server := filepath.Join(physical, "server", "maestro-mcp")
	if status.Target.OS == "windows" {
		server += ".exe"
	}
	if !samePath(server, pin.Path) || status.ServerSHA256 != pin.SHA256 {
		return "", status, errors.New("local Desktop packet executing image mismatch")
	}
	// Current rechecks the existing startup file pin, not kernel image attestation.
	current, err := processidentity.Current()
	if err != nil {
		return "", status, err
	}
	if current != pin {
		return "", status, processidentity.ErrChanged
	}
	return physical, status, nil
}
