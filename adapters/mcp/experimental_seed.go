package product

import (
	"errors"
	"path/filepath"
	"runtime"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installer"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installtx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/processidentity"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/updateservice"
)

func (b *Bound) validateExperimentalSeed() error {
	role := b.options.ExperimentalSeed
	if role == nil {
		if b.options.ExperimentalSeedRoot != "" {
			return errors.New("experimental_seed_unavailable")
		}
		return nil
	}
	root := b.options.ExperimentalSeedRoot
	if root == "" || !filepath.IsAbs(root) || filepath.Clean(root) != root || b.options.AuthorityRegistrySHA256 != role.Identity().RegistrySHA256 {
		return errors.New("experimental_seed_binding_unavailable")
	}
	name := "maestro-install"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	o := installer.SeededOperationOptions{ExperimentalSeed: role, Self: filepath.Join(root, name), Version: role.Identity().InstallerVersion, AuthorityRegistrySHA256: b.options.AuthorityRegistrySHA256, UserHome: b.options.UserHome, ManagedRoot: root, DataRoot: filepath.Join(b.options.UserHome, ".maestro-install")}
	if err := installer.VerifyExperimentalInstalledBinding(o); err != nil {
		return errors.New("experimental_seed_binding_unavailable")
	}
	state, err := installtx.ReadSourceState(o.DataRoot, root)
	if err != nil || state.Server.Version != b.options.CLIVersion {
		return errors.New("experimental_server_binding_unavailable")
	}
	if b.options.GuardProcess {
		actual, err := processidentity.Current()
		if err != nil || actual.SHA256 != state.Server.SHA256 {
			return errors.New("experimental_server_identity_changed")
		}
	}
	return nil
}

func (b *Bound) experimentalSourceAdmission() updateservice.SyncedAdmission {
	if b.options.ExperimentalSeed == nil {
		return nil
	}
	return b.options.ExperimentalSeed
}
