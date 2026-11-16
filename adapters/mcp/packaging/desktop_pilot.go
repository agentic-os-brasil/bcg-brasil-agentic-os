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

// ErrDesktopPilotInterpolation permits a finite path-selection diagnostic.
var ErrDesktopPilotInterpolation = errors.New("Desktop pilot path contains MCPB interpolation; select a local rehearsal path without ${")

func packetModes(platform string, pilot *DesktopPilotBinding) (os.FileMode, os.FileMode) {
	if platform == "darwin" && pilot != nil {
		return 0600, 0700
	}
	return 0644, 0755
}

// DesktopPilotBinding identifies one local engineering rehearsal, never a
// release authority. Its paths make this MCPB unsuitable for redistribution.
type DesktopPilotBinding struct {
	WorkspaceID        string                     `json:"workspace_id"`
	UserHome           string                     `json:"user_home"`
	Workspace          string                     `json:"workspace"`
	OwnerReviewChannel string                     `json:"owner_review_channel,omitempty"`
	Runtime            *EngineeringRuntimeBinding `json:"runtime,omitempty"`
}

func (p DesktopPilotBinding) Name() string { return "maestro-pilot-" + p.WorkspaceID }
func (p DesktopPilotBinding) DisplayName() string {
	return "Maestro MCP — Ensaio " + p.WorkspaceID[:8]
}

func (p DesktopPilotBinding) validate() error {
	if p.Runtime != nil {
		if err := p.Runtime.validate(); err != nil {
			return err
		}
	}
	if p.OwnerReviewChannel != "" && p.OwnerReviewChannel != "local-browser" {
		return errors.New("Desktop pilot owner review channel unavailable")
	}
	if !regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(p.WorkspaceID) {
		return errors.New("Desktop pilot workspace identity unavailable")
	}
	for _, path := range []string{p.UserHome, p.Workspace} {
		if strings.Contains(path, "${") {
			return ErrDesktopPilotInterpolation
		}
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
		if o.EngineeringRuntimePilot {
			return errors.New("engineering runtime selection requires the explicit Desktop pilot")
		}
		if o.Binding.OwnerReviewChannel != "" {
			return errors.New("packaged local-browser owner review requires explicit Desktop pilot")
		}
		return nil
	}
	if format != "mcpb" || o.Carrier != nil || o.LifecycleContext || o.NativeHost != nil || o.native != nil || o.hookOverride != nil || o.Binding.InstalledManifestSHA256 != "" || o.Binding.InstalledClientInputsSHA256 != "" {
		return errors.New("Desktop pilot requires ordinary unversioned engineering MCPB inputs")
	}
	if !o.EngineeringRuntimePilot && (o.Version != "0.0.0" || o.Binding.CLIVersion != "" || o.Binding.AuthorityRegistryPath != "" || o.Binding.AuthorityRegistrySHA256 != "") {
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
	o.desktopPilot = &DesktopPilotBinding{WorkspaceID: inspection.WorkspaceID, UserHome: o.Binding.UserHome, Workspace: o.Binding.WorkspacePath, OwnerReviewChannel: o.Binding.OwnerReviewChannel}
	if o.EngineeringRuntimePilot {
		if o.Version != o.Binding.CLIVersion {
			return errors.New("engineering Desktop version differs from its constructor")
		}
		binding, err := preparedEngineeringRuntime(o.Binding, o.Server)
		if err != nil {
			return err
		}
		o.desktopPilot.Runtime = &binding
	}
	return o.desktopPilot.validate()
}

// ValidateSelection checks this packet's prepared paths and current workspace
// identity independently of the manifest's launch arguments.
func (p DesktopPilotBinding) ValidateSelection(o product.Options) error {
	if err := p.validate(); err != nil {
		return err
	}
	if p.UserHome != o.UserHome || p.Workspace != o.WorkspacePath || (o.OwnerReviewChannel != "" && p.OwnerReviewChannel != o.OwnerReviewChannel) {
		return errors.New("Desktop pilot selection mismatch")
	}
	if _, err := os.Lstat(filepath.Join(p.UserHome, ".maestro-install")); !errors.Is(err, os.ErrNotExist) {
		return errors.New("Desktop pilot cannot use an enrolled installation")
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
	if p.Runtime != nil {
		return p.Runtime.ValidateSelection(o)
	}
	return nil
}
