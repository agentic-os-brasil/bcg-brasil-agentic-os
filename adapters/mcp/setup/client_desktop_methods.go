package setup

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp/packaging"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installer"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installtx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
)

func prepareCurrentDesktopMethods(ctx context.Context, o installer.ClientOptions, state installtx.State) (*installer.ClientMethodFrontend, error) {
	name := "maestro-mcp"
	if state.TargetOS == "windows" {
		name += ".exe"
	}
	server := filepath.Join(o.ManagedRoot, "bin", name)
	root := o.Request.CodeRoot + ".desktop-methods"
	archive := filepath.Join(root, "maestro-desktop-methods-"+state.Release+".zip")
	for _, command := range []string{"desktop-methods", "desktop-methods-verify"} {
		destination := archive
		if command == "desktop-methods" {
			destination = root
			if _, err := os.Lstat(root); err == nil {
				continue
			} else if !errors.Is(err, os.ErrNotExist) {
				return nil, err
			}
		}
		// Re-observe the authenticated current executable around each composition;
		// generation cannot silently use the older installer's linked method assets.
		before, err := installer.ClientFileDigest(server)
		if err != nil || before != state.Server.SHA256 {
			return nil, errors.New("current Desktop methods generator changed")
		}
		body, err := clientCommand(ctx, server, []string{command, "--destination", destination}, o.Request.Workspace)
		if err != nil {
			return nil, err
		}
		after, err := installer.ClientFileDigest(server)
		if err != nil || after != before {
			return nil, errors.New("current Desktop methods generator changed during generation")
		}
		var report packaging.DesktopFrontendReport
		if err := releasecontract.DecodeStrictJSON(body, &report); err != nil {
			return nil, err
		}
		if report.Name != "maestro-desktop-methods" || report.Version != state.Release || report.Mode != "instructions_only" || report.NativeQualification != "unavailable" || report.BootstrapProtocol != packaging.DesktopLiveMethodProtocol || report.FileCount < 1 || report.Archive != filepath.Base(archive) {
			return nil, errors.New("current Desktop methods report differs")
		}
		if command == "desktop-methods-verify" {
			digest, err := installer.ClientFileDigest(archive)
			if err != nil || digest != report.ArchiveSHA256 {
				return nil, errors.New("current Desktop methods archive changed")
			}
			return &installer.ClientMethodFrontend{Archive: archive, SHA256: digest, Version: state.Release, Files: report.FileCount, BootstrapProtocol: report.BootstrapProtocol}, nil
		}
	}
	return nil, errors.New("Desktop method verification unavailable")
}
