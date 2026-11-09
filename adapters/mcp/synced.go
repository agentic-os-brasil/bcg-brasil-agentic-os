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
	pinned := b.options.InstalledManifestSHA256 != "" || b.options.InstalledClientInputsSHA256 != ""
	installed := false
	if b.options.GuardProcess {
		identity, err := processidentity.Current()
		if err != nil {
			return false, err
		}
		directory := filepath.Dir(identity.Path)
		name := filepath.Base(identity.Path)
		if name == "maestro-mcp" || name == "maestro-mcp.exe" {
			if filepath.Base(directory) == "bin" {
				installed = true
				if filepath.Clean(b.options.ProductRoot) != filepath.Dir(directory) {
					return false, ErrSourceRecoveryRequired
				}
			} else if (filepath.Base(directory) == "scripts" || filepath.Base(directory) == "server") && filepath.Dir(directory) == filepath.Clean(b.options.ProductRoot) {
				// Ordinary unbound packets retain their existing MCP profile.
				// They do not become qualified installed-source launchers by
				// carrying a source-capable executable or owner selections.
				return pinned, nil
			}
		}
	}
	dataRoot := filepath.Join(b.options.UserHome, ".maestro-install")
	state, err := installtx.ReadSourceState(dataRoot, b.options.ProductRoot)
	if errors.Is(err, os.ErrNotExist) {
		return installed || pinned, nil
	}
	if err != nil {
		return false, err
	}
	// Enrollment is an owning state transition, never an option supplied by
	// the model. Once enrolled, direct callbacks and publication guards enforce
	// current source custody even when the optional carrier pins are omitted.
	// A source-capable first installation before enrollment keeps plain MCP
	// available but cannot grant a qualified source launch.
	return pinned || state.Source != nil || installed && state.SchemaVersion != 4, nil
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
