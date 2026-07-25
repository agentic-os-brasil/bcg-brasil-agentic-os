// Package updateplan builds immutable, confirmation-bound Maestro updates.
package updateplan

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installtx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
)

type Plan struct {
	SchemaVersion        int    `json:"schema_version"`
	ID                   string `json:"id"`
	State                string `json:"state"`
	FromRelease          string `json:"from_release"`
	ToRelease            string `json:"to_release"`
	Channel              string `json:"channel"`
	CLIVersion           string `json:"cli_version"`
	BundleVersion        string `json:"bundle_version"`
	CLIArtifact          string `json:"cli_artifact"`
	BundleArtifact       string `json:"bundle_artifact"`
	TargetOS             string `json:"target_os"`
	TargetArch           string `json:"target_arch"`
	Provider             string `json:"provider"`
	ProviderReleaseID    int64  `json:"provider_release_id"`
	ManifestSHA256       string `json:"manifest_sha256"`
	ConfirmationRequired bool   `json:"confirmation_required"`
}

type SourceBinding struct {
	Provider          string
	ProviderReleaseID int64
	ManifestSHA256    string
}

func Build(
	current installtx.State,
	manifest releasecontract.Manifest,
	targetOS, targetArch string,
	source SourceBinding,
) (Plan, error) {
	if source.Provider != "github" ||
		source.ProviderReleaseID <= 0 ||
		len(source.ManifestSHA256) != 64 ||
		strings.Trim(source.ManifestSHA256, "0123456789abcdef") != "" {
		return Plan{}, errors.New("update source binding is invalid")
	}
	currentVersion, err := parseVersion(current.Release)
	if err != nil {
		return Plan{}, fmt.Errorf("invalid installed release: %w", err)
	}
	nextVersion, err := parseVersion(manifest.Release)
	if err != nil {
		return Plan{}, fmt.Errorf("invalid candidate release: %w", err)
	}
	if compareVersion(nextVersion, currentVersion) <= 0 {
		return Plan{}, errors.New("standard update plan must move to a newer release")
	}
	var cliName, bundleName string
	for _, artifact := range manifest.Artifacts {
		switch {
		case artifact.Kind == "cli" && artifact.OS == targetOS && artifact.Arch == targetArch:
			if cliName != "" {
				return Plan{}, errors.New("release has duplicate target CLI artifacts")
			}
			cliName = artifact.Name
		case artifact.Kind == "bundle":
			if bundleName != "" {
				return Plan{}, errors.New("release has duplicate bundle artifacts")
			}
			bundleName = artifact.Name
		}
	}
	if cliName == "" || bundleName == "" {
		return Plan{}, errors.New("release does not support the requested update target")
	}
	plan := Plan{
		SchemaVersion: 1, State: "available",
		FromRelease: current.Release, ToRelease: manifest.Release, Channel: manifest.Channel,
		CLIVersion: manifest.CLI.Version, BundleVersion: manifest.Bundle.Version,
		CLIArtifact: cliName, BundleArtifact: bundleName,
		TargetOS: targetOS, TargetArch: targetArch,
		Provider: source.Provider, ProviderReleaseID: source.ProviderReleaseID,
		ManifestSHA256: source.ManifestSHA256, ConfirmationRequired: true,
	}
	body, err := json.Marshal(plan)
	if err != nil {
		return Plan{}, err
	}
	sum := sha256.Sum256(body)
	plan.ID = hex.EncodeToString(sum[:16])
	return plan, nil
}

func CompareVersions(left, right string) (int, error) {
	leftVersion, err := parseVersion(left)
	if err != nil {
		return 0, err
	}
	rightVersion, err := parseVersion(right)
	if err != nil {
		return 0, err
	}
	return compareVersion(leftVersion, rightVersion), nil
}

type version [3]uint64

func parseVersion(value string) (version, error) {
	parts := strings.Split(value, ".")
	if len(parts) != 3 {
		return version{}, fmt.Errorf("%q is not MAJOR.MINOR.PATCH", value)
	}
	var parsed version
	for index, part := range parts {
		if part == "" || (len(part) > 1 && part[0] == '0') {
			return version{}, fmt.Errorf("%q is not canonical", value)
		}
		number, err := strconv.ParseUint(part, 10, 64)
		if err != nil {
			return version{}, fmt.Errorf("%q is not canonical: %w", value, err)
		}
		parsed[index] = number
	}
	return parsed, nil
}

func compareVersion(left, right version) int {
	for index := range left {
		if left[index] < right[index] {
			return -1
		}
		if left[index] > right[index] {
			return 1
		}
	}
	return 0
}
