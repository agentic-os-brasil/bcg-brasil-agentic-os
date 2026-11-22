package setup

import (
	"context"
	"errors"
	"path/filepath"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installer"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installtx"
)

func versionedClientMaterializer(o installer.ClientOptions, nativeSHA string) installer.ClientMaterializer {
	return func(ctx context.Context, state installtx.State, binding installer.ClientBinding, previous *installer.ClientRecord) (installer.ClientMaterialization, error) {
		paths, err := installer.ClientGenerationPaths(o.Request, state.Binding.ManifestSHA256)
		if err != nil {
			return installer.ClientMaterialization{}, err
		}
		generation := o
		generation.Request.VersionedSlots = false
		generation.VersionedManagement = true
		generation.Request.CodeRoot, generation.Request.DesktopPath = paths.CodeRoot, paths.DesktopPath
		// Each manifest has new slots; the existing exclusive publisher retains
		// foreign/partial slots instead of replacing an older client tree.
		material, err := currentClientMaterializer(generation, nativeSHA, previous)(ctx, state, binding, nil)
		if err != nil {
			return material, err
		}
		material.Selection = paths.Selection
		if previous != nil && previous.Materialization.Selection != nil {
			selection := previous.Materialization.Selection
			if previous.CodePreparationPending() {
				// The prepared Desktop role is not a retarget of the desired full
				// selection. Reconstruct the latter through the canonical reader.
				prior, err := installer.ClientGenerationPaths(installer.ClientRequestFromRecord(*previous), previous.Materialization.ManifestSHA256)
				if err != nil {
					return material, err
				}
				selection = prior.Selection
			}
			if *selection != *paths.Selection {
				return material, errors.New("versioned client selection changed")
			}
		}
		provider, err := ensureCommandMarketplace(o)
		if err != nil {
			return material, err
		}
		material.CodeProviderRoot = provider
		material.CodeProviderSHA256, err = installer.ClientTreeDigest(provider)
		if err != nil {
			return material, err
		}
		material.DesktopMethods, err = prepareCurrentDesktopMethods(ctx, generation, state)
		if err != nil {
			return material, err
		}
		if filepath.Dir(material.CodeRoot) != filepath.Dir(o.Request.CodeRoot) || filepath.Dir(material.DesktopPath) != filepath.Dir(o.Request.DesktopPath) {
			return material, errors.New("client generation parent changed")
		}
		return material, nil
	}
}
