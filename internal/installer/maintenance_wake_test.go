package installer

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/macosadapter"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
)

func TestMaintenanceWakeIsBoundToExistingInstallationAndCarrier(t *testing.T) {
	root := t.TempDir()
	binding := MaintenanceWakeBinding{ManagedRoot: filepath.Join(root, "product"), UserHome: filepath.Join(root, "owner"), BrainHome: filepath.Join(root, "owner", "Developer", "Maestro"), Workspace: filepath.Join(root, "workspace"), WorkspaceID: strings.Repeat("a", 32), CarrierID: strings.Repeat("b", 32), AdmissionSHA256: strings.Repeat("c", 64), RegistrySHA256: strings.Repeat("d", 64), InstallerVersion: "0.1.59", SourceLocator: filepath.Join(root, "source"), Source: releasecontract.SourceTuple{SourceID: "test-source", Channel: "beta", OS: "darwin", Arch: "arm64"}}
	first, err := binding.MacSpec()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(first.Label, "com.bcg.maestro.maintenance.") || len(first.Label) != len("com.bcg.maestro.maintenance.")+64 || first.StartInterval != 900 || !first.RunAtLoad || first.Disabled {
		t.Fatalf("wake is not an exact owned discovery pulse: %+v", first)
	}
	if first.Program != filepath.Join(binding.ManagedRoot, "maestro-install") || first.Arguments[0] != "maintenance-wake" {
		t.Fatalf("wake bypasses original stable installer: %+v", first)
	}
	other := binding
	other.CarrierID = strings.Repeat("e", 32)
	second, err := other.MacSpec()
	if err != nil || first.Label == second.Label {
		t.Fatal("different carriers share an OS task")
	}
	if _, err := ReadMaintenanceWake(context.Background(), ClientOptions{}); err == nil {
		t.Fatal("unadmitted installation supplied wake authority")
	}
}

func TestMaintenanceWakeFileIdentityAndPauseDoNotTouchNeighbors(t *testing.T) {
	home := t.TempDir()
	b := MaintenanceWakeBinding{ManagedRoot: filepath.Join(home, "product"), UserHome: filepath.Join(home, "owner"), BrainHome: filepath.Join(home, "owner", "Developer", "Maestro"), Workspace: filepath.Join(home, "workspace"), WorkspaceID: strings.Repeat("a", 32), CarrierID: strings.Repeat("b", 32), AdmissionSHA256: strings.Repeat("c", 64), RegistrySHA256: strings.Repeat("d", 64), InstallerVersion: "0.1.59", SourceLocator: filepath.Join(home, "source"), Source: releasecontract.SourceTuple{SourceID: "test-source", Channel: "beta", OS: "darwin", Arch: "arm64"}}
	spec, err := b.MacSpec()
	if err != nil {
		t.Fatal(err)
	}
	neighbor := filepath.Join(home, "Library", "LaunchAgents", "unrelated.plist")
	if err := os.MkdirAll(filepath.Dir(neighbor), 0700); err != nil {
		t.Fatal(err)
	}
	marker := []byte("unrelated service remains untouched")
	if err := os.WriteFile(neighbor, marker, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := macosadapter.Install(home, spec, true); err != nil {
		t.Fatal(err)
	}
	if err := b.VerifyMacWake(home); err != nil {
		t.Fatal(err)
	}
	if _, err := macosadapter.Pause(home, spec.Label); err != nil {
		t.Fatal(err)
	}
	if err := b.VerifyMacWake(home); err == nil {
		t.Fatal("paused wake still admitted a due worker")
	}
	if err := macosadapter.Uninstall(home, spec.Label); err != nil {
		t.Fatal(err)
	}
	if err := b.VerifyMacWake(home); err == nil {
		t.Fatal("revoked wake still admitted a due worker")
	}
	after, err := os.ReadFile(neighbor)
	if err != nil || !bytes.Equal(after, marker) {
		t.Fatal("unrelated service changed", err)
	}
}

func TestMaintenanceWakeClosedStableAndChildArguments(t *testing.T) {
	root := t.TempDir()
	stable := []string{"--product-root", filepath.Join(root, "product"), "--user-home", filepath.Join(root, "owner"), "--workspace", filepath.Join(root, "workspace"), "--carrier-id", strings.Repeat("a", 32), "--binding-sha256", strings.Repeat("b", 64)}
	got, err := ParseMaintenanceWakeCommand(stable, false)
	if err != nil || got.CarrierID != strings.Repeat("a", 32) {
		t.Fatal(got, err)
	}
	child := append(append([]string{}, stable...), "--installed-manifest", strings.Repeat("c", 64), "--installed-client-inputs", strings.Repeat("d", 64), "--experimental-seed-sha256", strings.Repeat("e", 64), "--experimental-installed-root", filepath.Join(root, "product"))
	if _, err := ParseMaintenanceWakeCommand(child, true); err != nil {
		t.Fatal(err)
	}
	vectors := [][]string{stable[:len(stable)-1], append(append([]string{}, stable...), "--force", "true"), append([]string{}, stable...), append([]string{}, stable...), append([]string{}, child...)}
	vectors[2][0] = "--workspace"
	vectors[3][5] = "relative"
	vectors[4][len(child)-1] = filepath.Join(root, "foreign")
	for i, args := range vectors {
		if _, err := ParseMaintenanceWakeCommand(args, i == 4); err == nil {
			t.Fatalf("invalid selection %d accepted", i)
		}
	}
	if _, err := ParseMaintenanceWakeCommand(child, false); err == nil {
		t.Fatal("stable entry accepted child-only authority fields")
	}
	if _, err := ParseMaintenanceWakeCommand(stable, true); err == nil {
		t.Fatal("child entry accepted missing authenticated pins")
	}
}
