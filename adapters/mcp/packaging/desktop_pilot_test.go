package packaging

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDesktopPilotProfileRefusesOtherAuthorities(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*Options)
	}{
		{"release_label", func(o *Options) { o.Version = "0.2.0" }},
		{"stable_carrier", func(o *Options) { o.Carrier = &StableCarrier{} }},
		{"native_host", func(o *Options) { o.NativeHost = &NativeHost{} }},
		{"lifecycle", func(o *Options) { o.LifecycleContext = true }},
		{"versioned_server", func(o *Options) { o.Binding.CLIVersion = "0.2.0" }},
		{"public_registry", func(o *Options) { o.Binding.AuthorityRegistryPath = "registry.json" }},
		{"registry_identity", func(o *Options) { o.Binding.AuthorityRegistrySHA256 = strings.Repeat("a", 64) }},
		{"signed_manifest", func(o *Options) { o.Binding.InstalledManifestSHA256 = strings.Repeat("a", 64) }},
		{"signed_client", func(o *Options) { o.Binding.InstalledClientInputsSHA256 = strings.Repeat("a", 64) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := Options{DesktopPilot: true, Version: "0.0.0"}
			tc.change(&o)
			if err := prepareDesktopPilot(&o, "mcpb"); err == nil || !strings.Contains(err.Error(), "ordinary unversioned") {
				t.Fatalf("profile authority accepted: %v", err)
			}
		})
	}
	o := Options{DesktopPilot: true, Version: "0.0.0"}
	if err := prepareDesktopPilot(&o, "plugin"); err == nil {
		t.Fatal("Code identity changed by Desktop profile")
	}
	root := t.TempDir()
	o.Binding.ProductRoot = filepath.Join(root, "product")
	o.Binding.UserHome = filepath.Join(root, "owner")
	if err := os.MkdirAll(filepath.Join(o.Binding.UserHome, ".maestro-install"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := prepareDesktopPilot(&o, "mcpb"); err == nil || !strings.Contains(err.Error(), "enrolled") {
		t.Fatalf("enrolled installation accepted: %v", err)
	}
}

func TestDesktopPilotInventoryBindingIsClosed(t *testing.T) {
	root := t.TempDir()
	admitted := DesktopPilotBinding{WorkspaceID: strings.Repeat("a", 32), UserHome: filepath.Join(root, "owner"), Workspace: filepath.Join(root, "workspace")}
	if err := admitted.validate(); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*DesktopPilotBinding){
		func(p *DesktopPilotBinding) { p.WorkspaceID = "short" },
		func(p *DesktopPilotBinding) { p.WorkspaceID = strings.Repeat("A", 32) },
		func(p *DesktopPilotBinding) { p.UserHome = "relative/owner" },
		func(p *DesktopPilotBinding) { p.Workspace += "\n" },
		func(p *DesktopPilotBinding) { p.Workspace = filepath.Join(root, "other-workspace") },
	} {
		p := admitted
		change(&p)
		if err := p.validate(); err == nil {
			t.Fatal("invalid local inventory binding accepted")
		}
	}
	other := admitted
	other.WorkspaceID = strings.Repeat("a", 31) + "b"
	if admitted.Name() == other.Name() {
		t.Fatal("different workspaces share a Desktop identity")
	}
}
