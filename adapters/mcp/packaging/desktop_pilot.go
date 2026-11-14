package packaging

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/datahome"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
)

// DesktopPilotBinding identifies one local engineering rehearsal, never a
// release authority. Its paths make this MCPB unsuitable for redistribution.
type DesktopPilotBinding struct {
	WorkspaceID string `json:"workspace_id"`
	UserHome    string `json:"user_home"`
	Workspace   string `json:"workspace"`
}

func (p DesktopPilotBinding) Name() string { return "maestro-pilot-" + p.WorkspaceID }
func (p DesktopPilotBinding) DisplayName() string {
	return "Maestro MCP — Ensaio " + p.WorkspaceID[:8]
}

func (p DesktopPilotBinding) validate() error {
	if !regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(p.WorkspaceID) {
		return errors.New("Desktop pilot workspace identity unavailable")
	}
	for _, path := range []string{p.UserHome, p.Workspace} {
		if !filepath.IsAbs(path) || filepath.Clean(path) != path || strings.ContainsAny(path, "\x00\r\n") {
			return errors.New("Desktop pilot requires exact absolute local selections")
		}
	}
	if filepath.Base(p.UserHome) != "owner" || p.Workspace != filepath.Join(filepath.Dir(p.UserHome), "workspace") {
		return errors.New("Desktop pilot requires prepared rehearsal layout")
	}
	return nil
}

func prepareDesktopPilot(o *Options, format string) error {
	if !o.DesktopPilot {
		return nil
	}
	if format != "mcpb" || o.Version != "0.0.0" || o.Carrier != nil || o.LifecycleContext || o.NativeHost != nil || o.native != nil || o.hookOverride != nil || o.Binding.CLIVersion != "" || o.Binding.AuthorityRegistryPath != "" || o.Binding.AuthorityRegistrySHA256 != "" || o.Binding.InstalledManifestSHA256 != "" || o.Binding.InstalledClientInputsSHA256 != "" {
		return errors.New("Desktop pilot requires ordinary unversioned engineering MCPB inputs")
	}
	if o.Binding.ProductRoot != filepath.Join(filepath.Dir(o.Binding.UserHome), "product") {
		return errors.New("Desktop pilot requires prepared rehearsal product")
	}
	if _, err := os.Lstat(filepath.Join(o.Binding.UserHome, ".maestro-install")); !errors.Is(err, os.ErrNotExist) {
		return errors.New("Desktop pilot cannot use an enrolled installation")
	}
	home, err := datahome.Resolve(datahome.Options{ProductRoot: o.Binding.ProductRoot, UserHome: o.Binding.UserHome, LookupEnv: o.Binding.LookupEnv})
	if err != nil || !home.Writable {
		return errors.New("Desktop pilot binding unavailable")
	}
	inspection, err := workspace.Inspect(o.Binding.WorkspacePath, filepath.Join(home.Home, "state"))
	if err != nil || inspection.State != "ready" {
		return errors.New("Desktop pilot workspace unavailable")
	}
	o.desktopPilot = &DesktopPilotBinding{WorkspaceID: inspection.WorkspaceID, UserHome: o.Binding.UserHome, Workspace: o.Binding.WorkspacePath}
	return o.desktopPilot.validate()
}

// ValidateSelection seals editable Desktop fields to this local packet's
// preparation; directory defaults alone cannot enforce a runtime binding.
func (p DesktopPilotBinding) ValidateSelection(o product.Options) error {
	if err := p.validate(); err != nil {
		return err
	}
	if p.UserHome != o.UserHome || p.Workspace != o.WorkspacePath {
		return errors.New("Desktop pilot selection mismatch")
	}
	home, err := datahome.Resolve(datahome.Options{ProductRoot: o.ProductRoot, UserHome: o.UserHome, LookupEnv: o.LookupEnv})
	if err != nil || !home.Writable {
		return errors.New("Desktop pilot binding unavailable")
	}
	state := filepath.Join(home.Home, "state")
	inspection, err := workspace.Inspect(o.WorkspacePath, state)
	if err != nil || inspection.State != "ready" || inspection.WorkspaceID != p.WorkspaceID {
		return errors.New("Desktop pilot workspace identity mismatch")
	}
	reference, err := workspace.ResolveReference(p.WorkspaceID, state)
	if err != nil || reference != o.WorkspacePath {
		return errors.New("Desktop pilot workspace reference mismatch")
	}
	return nil
}
