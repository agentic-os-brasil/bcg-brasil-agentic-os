package releaseprovider

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseverify"
)

const (
	ManifestName          = releaseverify.ManifestName
	ManifestSignatureName = releaseverify.ManifestSignatureName
)

type AssetFetcher interface {
	FetchAsset(context.Context, Asset, string) error
}

func DownloadVerified(
	ctx context.Context,
	fetcher AssetFetcher,
	release Release,
	destination string,
	registry releaseverify.KeyRegistry,
) (releaseverify.VerifiedRelease, error) {
	if fetcher == nil || registry == nil {
		return releaseverify.VerifiedRelease{}, errors.New("provider fetcher and release-key registry are required")
	}
	if release.Draft {
		return releaseverify.VerifiedRelease{}, errors.New("draft provider releases cannot be installed")
	}
	if _, err := os.Stat(destination); err == nil {
		return releaseverify.VerifiedRelease{}, fmt.Errorf("verified release destination already exists: %s", destination)
	} else if !errors.Is(err, os.ErrNotExist) {
		return releaseverify.VerifiedRelease{}, err
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return releaseverify.VerifiedRelease{}, err
	}
	staging, err := os.MkdirTemp(filepath.Dir(destination), ".provider-download-")
	if err != nil {
		return releaseverify.VerifiedRelease{}, err
	}
	defer os.RemoveAll(staging)
	assets := map[string]Asset{}
	for _, asset := range release.Assets {
		if _, duplicate := assets[asset.Name]; duplicate {
			return releaseverify.VerifiedRelease{}, fmt.Errorf("provider returned duplicate asset name %s", asset.Name)
		}
		assets[asset.Name] = asset
	}
	fetch := func(name string) error {
		asset, ok := assets[name]
		if !ok {
			return fmt.Errorf("provider release is missing required asset %s", name)
		}
		return fetcher.FetchAsset(ctx, asset, filepath.Join(staging, name))
	}
	if err := fetch(ManifestName); err != nil {
		return releaseverify.VerifiedRelease{}, err
	}
	if err := fetch(ManifestSignatureName); err != nil {
		return releaseverify.VerifiedRelease{}, err
	}
	manifestBody, err := os.ReadFile(filepath.Join(staging, ManifestName))
	if err != nil {
		return releaseverify.VerifiedRelease{}, err
	}
	signature, err := os.ReadFile(filepath.Join(staging, ManifestSignatureName))
	if err != nil {
		return releaseverify.VerifiedRelease{}, err
	}
	manifest, _, err := releaseverify.VerifyManifest(manifestBody, signature, registry)
	if err != nil {
		return releaseverify.VerifiedRelease{}, err
	}
	if err := fetch(manifest.ReleaseNotes.Name); err != nil {
		return releaseverify.VerifiedRelease{}, err
	}
	for _, artifact := range manifest.Artifacts {
		if err := fetch(artifact.Name); err != nil {
			return releaseverify.VerifiedRelease{}, err
		}
		if err := fetch(artifact.SignatureRef); err != nil {
			return releaseverify.VerifiedRelease{}, err
		}
	}
	verified, err := releaseverify.VerifyDirectory(staging, registry)
	if err != nil {
		return releaseverify.VerifiedRelease{}, err
	}
	if err := os.Rename(staging, destination); err != nil {
		return releaseverify.VerifiedRelease{}, err
	}
	verified.Directory = destination
	return verified, nil
}
