package product

import (
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installtx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/processidentity"
)

var ErrSourceRecoveryRequired = errors.New("signed source unavailable; use the independent maestro-install source-review/source-check recovery commands")

func (b *Bound) installedExpectation() installtx.ReleaseBinding {
	return installtx.ReleaseBinding{ManifestSHA256: b.options.InstalledManifestSHA256, ClientInputsSHA256: b.options.InstalledClientInputsSHA256}
}

func (b *Bound) sourceRequired() (bool, error) {
	dataRoot := filepath.Join(b.options.UserHome, ".maestro-install")
	state, err := installtx.ReadSourceState(dataRoot, b.options.ProductRoot)
	if errors.Is(err, os.ErrNotExist) {
		return b.options.GuardProcess || b.options.InstalledManifestSHA256 != "" || b.options.InstalledClientInputsSHA256 != "", nil
	}
	if err != nil {
		return false, err
	}
	return b.options.GuardProcess || state.Source != nil || b.options.InstalledManifestSHA256 != "" || b.options.InstalledClientInputsSHA256 != "", nil
}

func (b *Bound) validateInstalledSource() error {
	required, err := b.sourceRequired()
	if err != nil {
		return ErrSourceRecoveryRequired
	}
	if !required {
		return nil
	}
	state, err := installtx.ValidateQualifiedLaunch(b.options.ProductRoot, filepath.Join(b.options.UserHome, ".maestro-install"), b.installedExpectation(), time.Now())
	if err != nil {
		return ErrSourceRecoveryRequired
	}
	if b.options.GuardProcess {
		identity, err := processidentity.Current()
		if err != nil {
			return err
		}
		if identity.SHA256 != state.Server.SHA256 || b.options.CLIVersion != state.Server.Version {
			return processidentity.ErrChanged
		}
	}
	return nil
}

func (b *Bound) guardPublication(publish func() error) error {
	if err := b.validate(); err != nil {
		return err
	}
	required, err := b.sourceRequired()
	if err != nil {
		return err
	}
	if !required {
		return publish()
	}
	return installtx.GuardQualifiedCurrent(b.options.ProductRoot, filepath.Join(b.options.UserHome, ".maestro-install"), b.installedExpectation(), time.Now(), func() error {
		// The installation can change after outer request validation. Repeat
		// the actual running SHA/version comparison under this same lock,
		// including direct serve processes without optional carrier pins.
		if err := b.validateInstalledSource(); err != nil {
			return err
		}
		return publish()
	})
}
