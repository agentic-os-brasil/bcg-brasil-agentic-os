// Package updateservice binds provider discovery, signed release verification,
// and confirmation-bound update planning without activating an update.
package updateservice

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installtx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseprovider"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseverify"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/updateplan"
)

var ErrCurrent = errors.New("installed Maestro release is current")

type Provider interface {
	releaseprovider.AssetFetcher
	ListReleases(context.Context) ([]releaseprovider.Release, error)
}

type DownloadFunc func(
	context.Context,
	releaseprovider.AssetFetcher,
	releaseprovider.Release,
	string,
	releaseverify.KeyRegistry,
) (releaseverify.VerifiedRelease, error)

type CheckOptions struct {
	Current     installtx.State
	TargetOS    string
	TargetArch  string
	StagingRoot string
	Provider    Provider
	Registry    releaseverify.KeyRegistry
	Download    DownloadFunc
}

type CheckResult struct {
	Plan     updateplan.Plan
	Verified releaseverify.VerifiedRelease
}

func Check(ctx context.Context, options CheckOptions) (CheckResult, error) {
	if options.Provider == nil || options.Registry == nil {
		return CheckResult{}, errors.New("update provider and release-key registry are required")
	}
	if options.StagingRoot == "" {
		return CheckResult{}, errors.New("update staging root is required")
	}
	releases, err := options.Provider.ListReleases(ctx)
	if err != nil {
		return CheckResult{}, err
	}
	selected, err := selectLatestRelease(releases, options.Current.Release)
	if err != nil {
		return CheckResult{}, err
	}
	if options.Download == nil {
		options.Download = releaseprovider.DownloadVerified
	}
	destination := filepath.Join(options.StagingRoot, "github-release-"+strconv.FormatInt(selected.ID, 10))
	verified, err := options.Download(ctx, options.Provider, selected, destination, options.Registry)
	if err != nil {
		return CheckResult{}, err
	}
	failAfterDownload := func(cause error) (CheckResult, error) {
		if cleanupErr := os.RemoveAll(destination); cleanupErr != nil {
			return CheckResult{}, fmt.Errorf("%w; remove provisional download: %v", cause, cleanupErr)
		}
		return CheckResult{}, cause
	}
	if filepath.Clean(verified.Directory) != filepath.Clean(destination) {
		return failAfterDownload(errors.New("verified release directory does not match the provisional destination"))
	}
	if selected.TagName != "maestro-v"+verified.Manifest.Release {
		return failAfterDownload(errors.New("provider release tag does not match the authenticated manifest version"))
	}
	if len(verified.ManifestSHA256) != 64 ||
		strings.Trim(verified.ManifestSHA256, "0123456789abcdef") != "" {
		return failAfterDownload(errors.New("verified release did not retain an authenticated manifest digest"))
	}
	plan, err := updateplan.Build(
		options.Current,
		verified.Manifest,
		options.TargetOS,
		options.TargetArch,
		updateplan.SourceBinding{
			Provider:          "github",
			ProviderReleaseID: selected.ID,
			ManifestSHA256:    verified.ManifestSHA256,
		},
	)
	if err != nil {
		return failAfterDownload(err)
	}
	return CheckResult{Plan: plan, Verified: verified}, nil
}

func selectLatestRelease(releases []releaseprovider.Release, current string) (releaseprovider.Release, error) {
	if _, err := updateplan.CompareVersions(current, current); err != nil {
		return releaseprovider.Release{}, fmt.Errorf("invalid installed release: %w", err)
	}
	var selected releaseprovider.Release
	selectedVersion := ""
	seen := map[string]bool{}
	for _, release := range releases {
		if release.Draft || !strings.HasPrefix(release.TagName, "maestro-v") {
			continue
		}
		version := strings.TrimPrefix(release.TagName, "maestro-v")
		comparison, err := updateplan.CompareVersions(version, current)
		if err != nil {
			return releaseprovider.Release{}, fmt.Errorf("provider returned malformed Maestro release tag %q", release.TagName)
		}
		if seen[version] {
			return releaseprovider.Release{}, fmt.Errorf("provider returned duplicate Maestro release version %s", version)
		}
		seen[version] = true
		if release.ID <= 0 {
			return releaseprovider.Release{}, errors.New("provider returned an invalid release identity")
		}
		if comparison <= 0 {
			continue
		}
		if selectedVersion == "" {
			selected, selectedVersion = release, version
			continue
		}
		newer, err := updateplan.CompareVersions(version, selectedVersion)
		if err != nil {
			return releaseprovider.Release{}, err
		}
		if newer > 0 {
			selected, selectedVersion = release, version
		}
	}
	if selectedVersion == "" {
		return releaseprovider.Release{}, ErrCurrent
	}
	return selected, nil
}
