package installer

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/datahome"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installtx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

func validateSeedName(path string, m releasecontract.Manifest, platform, arch string) error {
	if !m.Modern() {
		return validateBootstrapperName(path, m.Release, platform, arch)
	}
	name := "maestro-install_" + m.Delivery.InstallerVersion + "_" + platform + "_" + arch
	if platform == "windows" {
		name += ".exe"
	}
	if filepath.Base(path) != name {
		return fmt.Errorf("installer must be named %s", name)
	}
	return nil
}
func seedMatchesManifest(status seedStatus, m releasecontract.Manifest, pin string) bool {
	if !m.Modern() {
		return seedStatusMatches(status, m.Release, pin)
	}
	return status.SchemaVersion == 2 && status.Product == "maestro" && status.Component == "installer" && status.Protocol == releasecontract.InstallerProtocol && status.InstallerVersion == m.Delivery.InstallerVersion && status.AuthorityRegistrySHA256 == pin && status.BootstrapperVersion == ""
}
func seedMatchesPlan(status seedStatus, plan Plan) bool {
	if plan.Profile == "" {
		return seedStatusMatches(status, plan.Release, plan.RegistrySHA256)
	}
	return seedMatchesManifest(status, releasecontract.Manifest{SchemaVersion: 2, Delivery: releasecontract.DeliveryComponent{InstallerVersion: plan.BootstrapperVersion}}, plan.RegistrySHA256)
}
func seedPath(root, platform, profile string) string {
	if profile == "" {
		return installedBootstrapperPath(root, platform)
	}
	name := "maestro-install"
	if platform == "windows" {
		name += ".exe"
	}
	return filepath.Join(root, name)
}
func installedExecutablePath(root, platform, profile string) string {
	if profile == "" {
		return installedCLIPath(root, platform)
	}
	name := "maestro-mcp"
	if platform == "windows" {
		name += ".exe"
	}
	path := filepath.Join(root, "bin", name)
	if err := ensureNoSymlinkComponents(root, path); err != nil {
		return ""
	}
	if validateRegular(path, 1<<30) != nil {
		return ""
	}
	return path
}
func checkInstalledExecutable(ctx context.Context, run commandRunner, plan Plan, path string) error {
	if plan.Profile != "" {
		state, err := installtx.ReadStateForManagedRoot(plan.DataRoot, plan.ManagedRoot)
		if err != nil {
			return err
		}
		if plan.Server == nil || state.Server != *plan.Server || state.Profile != plan.Profile || state.Server.Version != plan.ServerVersion || state.Server.EmbeddedInputsSHA256 != plan.EmbeddedInputsSHA256 {
			return errors.New("installed modern binding differs from reviewed release")
		}
		return installtx.CheckActivatedServer(plan.ManagedRoot, state)
	}
	body, err := run(ctx, path, "version")
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(body)) != "bcgos "+plan.Release {
		return errors.New("installed CLI reported an unexpected version")
	}
	return nil
}

func validateModernCustody(o Options) error {
	if o.UserHome == "" || !filepath.IsAbs(o.UserHome) || filepath.Clean(o.UserHome) != o.UserHome || o.DataRoot != filepath.Join(o.UserHome, ".maestro-install") {
		return errors.New("modern installation state must be the reviewed private UserHome/.maestro-install")
	}
	home, err := datahome.PreviewInstallation(datahome.Options{ProductRoot: o.ManagedRoot, UserHome: o.UserHome})
	if err != nil {
		return err
	}
	if within(home.Home, o.DataRoot) || within(o.DataRoot, home.Home) {
		return errors.New("installation custody overlaps admitted home")
	}
	entries, err := os.ReadDir(o.ReleaseDir)
	if err != nil {
		return err
	}
	paths := []string{o.Bootstrapper, o.AuthorityRegistry}
	for _, entry := range entries {
		paths = append(paths, filepath.Join(o.ReleaseDir, entry.Name()))
	}
	for _, path := range paths {
		file, err := scheduler.OpenRegularFile(path)
		if err != nil {
			return err
		}
		checkErr := scheduler.ValidateSingleLink(file)
		closeErr := file.Close()
		if checkErr != nil || closeErr != nil {
			return errors.Join(checkErr, closeErr)
		}
	}
	return nil
}
