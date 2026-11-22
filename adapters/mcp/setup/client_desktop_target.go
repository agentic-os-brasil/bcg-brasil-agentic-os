package setup

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp/packaging"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installer"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installtx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

// The same current-client owner publishes Desktop without creating a Code slot,
// provider, profile or qualification. Current source and carrier custody remain.
func desktopClientMaterializer(o installer.ClientOptions) installer.ClientMaterializer {
	return func(ctx context.Context, s installtx.State, b installer.ClientBinding, previous *installer.ClientRecord) (installer.ClientMaterialization, error) {
		material, err := installer.ClientGenerationPaths(o.Request, s.Binding.ManifestSHA256)
		if err != nil {
			return material, err
		}
		generation := o
		generation.VersionedManagement = true
		generation.Request.VersionedSlots = false
		generation.Request.DesktopPath = material.DesktopPath
		server := filepath.Join(o.ManagedRoot, "bin", targetServerName(s.TargetOS))
		identity, err := releasecontract.ReadServerIdentity(server)
		if err != nil {
			return material, err
		}
		if identity.Version != s.Server.Version || identity.EmbeddedInputsSHA256 != s.Server.EmbeddedInputsSHA256 {
			return material, errors.New("actual current Desktop generator identity changed")
		}
		stage := material.DesktopPath + ".candidate-" + o.Request.CarrierID + "-" + s.Binding.ManifestSHA256 + ".mcpb"
		if _, e := os.Lstat(stage); errors.Is(e, os.ErrNotExist) {
			if _, err = clientCommand(ctx, server, currentGeneratorArgs("mcpb", stage, server, generation, s), b.Workspace); err != nil {
				return material, err
			}
		} else if e != nil {
			return material, e
		}
		expected, err := installer.ClientArchiveDigest(stage)
		if err != nil {
			return material, err
		}
		status, err := inspectCurrentPacket(ctx, stage, "mcpb", generation, identity, s.Server.SHA256)
		if err != nil {
			return material, err
		}
		contract := status.NativeContractSHA256
		if err = inspectDesktopCandidateBinding(stage, generation, s.TargetOS, contract); err != nil {
			return material, err
		}
		if err = requireInspectedClientArchive(stage, expected); err != nil {
			return material, err
		}
		material.Release = s.Release
		material.ManifestSHA256 = s.Binding.ManifestSHA256
		material.ClientInputsSHA256 = s.Binding.ClientInputsSHA256
		material.ServerSHA256 = s.Server.SHA256
		material.EmbeddedInputsSHA256 = s.Server.EmbeddedInputsSHA256
		material.BundleSHA256 = s.Binding.BundleSHA256
		material.DesktopNativeContractSHA256 = contract
		material.BootstrapServerSHA256 = s.Server.SHA256
		material.BootstrapEmbeddedInputsSHA256 = s.Server.EmbeddedInputsSHA256
		material.OwnerReviewChannel = o.Request.OwnerReviewChannel
		// Retain the exact authenticated compatible bootstrap across source updates.
		reused := previous != nil && !o.Request.ManualDesktopUpdate && previous.Materialization.Target == "desktop" && previous.Materialization.DesktopNativeContractSHA256 == contract
		if reused {
			old := previous.Materialization
			if err = packaging.InspectReusedStableExtension(old.DesktopPath, stableCarrier(o.Request.CarrierID), old.DesktopSHA256, old.BootstrapServerSHA256, old.BootstrapEmbeddedInputsSHA256, contract); err != nil {
				return material, err
			}
			if err = inspectDesktopCandidateBinding(old.DesktopPath, generation, s.TargetOS, contract); err != nil {
				return material, err
			}
			expected = old.DesktopSHA256
			material.BootstrapServerSHA256 = old.BootstrapServerSHA256
			material.BootstrapEmbeddedInputsSHA256 = old.BootstrapEmbeddedInputsSHA256
			if old.DesktopPath != material.DesktopPath {
				if err = retainCurrentDesktopCarrier(old.DesktopPath, material.DesktopPath, expected); err != nil {
					return material, err
				}
			}
		}
		if _, e := os.Lstat(material.DesktopPath); e == nil {
			if err = inspectFirstDesktopContract(ctx, generation, s, b, previous, identity, contract); err != nil {
				return material, err
			}
			if err = requireInspectedClientArchive(material.DesktopPath, expected); err != nil {
				return material, err
			}
		} else if !errors.Is(e, os.ErrNotExist) {
			return material, e
		} else {
			if reused {
				return material, errors.New("retained Desktop carrier unavailable")
			}
			if err = swapOwnedDesktop(ctx, stage, generation, nil, identity, contract); err != nil {
				return material, err
			}
		}
		if err = requireInspectedClientArchive(material.DesktopPath, expected); err != nil {
			return material, err
		}
		material.DesktopSHA256 = expected
		material.DesktopMethods, err = prepareCurrentDesktopMethods(ctx, generation, s)
		if err != nil {
			return material, err
		}
		if err = writeDesktopManagementLaunchers(generation, material); err != nil {
			return material, err
		}
		return material, nil
	}
}
func targetServerName(goos string) string {
	if goos == "windows" {
		return "maestro-mcp.exe"
	}
	return "maestro-mcp"
}
func writeDesktopManagementLaunchers(o installer.ClientOptions, m installer.ClientMaterialization) error {
	root := installer.ClientManagementRoot(m)
	for _, update := range []bool{false, true} {
		name, body := installer.ClientManagementLauncher(o, update)
		path := filepath.Join(root, name)
		if err := writeOrVerifyOwnedLauncher(path, body, filepath.Ext(name) == ".command"); err != nil {
			return err
		}
	}
	// The normal Windows management entry is a small fixed native launcher.
	if runtime.GOOS == "windows" {
		return writeOrVerifyOwnedLauncher(filepath.Join(root, "Update Maestro.cmd"), installer.ClientWindowsManagementEntry(), false)
	}
	return nil
}
func writeOrVerifyOwnedLauncher(path string, body []byte, executable bool) error {
	mode := os.FileMode(0600)
	if executable {
		mode = 0700
	}
	if file, err := scheduler.OpenRegularFile(path); err == nil {
		defer file.Close()
		if err = scheduler.ValidateSingleLink(file); err != nil {
			return err
		}
		if err = scheduler.ValidatePrivateFile(file, mode); err != nil {
			return err
		}
		existing, err := io.ReadAll(io.LimitReader(file, 64<<10+1))
		if err != nil {
			return err
		}
		if string(existing) != string(body) {
			return errors.New("Desktop management recipe changed; existing bytes preserved")
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := scheduler.WriteNewPrivateFile(path, body); err != nil {
		return err
	}
	if executable {
		file, err := scheduler.OpenRegularFile(path)
		if err != nil {
			return err
		}
		defer file.Close()
		if err = scheduler.ValidateSingleLink(file); err != nil {
			return err
		}
		return file.Chmod(mode)
	}
	return nil
}
