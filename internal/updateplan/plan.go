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
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/rolemigration"
)

type Plan struct {
	SchemaVersion        int    `json:"schema_version"`
	ID                   string `json:"id"`
	State                string `json:"state"`
	FromRelease          string `json:"from_release"`
	FromChannel          string `json:"from_channel"`
	FromCLIVersion       string `json:"from_cli_version"`
	FromBundleVersion    string `json:"from_bundle_version"`
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
	RoleMigrationID      string `json:"role_migration_id,omitempty"`
	CatalogSHA256        string `json:"catalog_sha256,omitempty"`
	PolicySHA256         string `json:"policy_sha256,omitempty"`
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
	if _, err := parseVersion(current.CLIVersion); err != nil {
		return Plan{}, fmt.Errorf("invalid installed CLI version: %w", err)
	}
	if _, err := parseVersion(current.BundleVersion); err != nil {
		return Plan{}, fmt.Errorf("invalid installed bundle version: %w", err)
	}
	if (current.Channel != "canary" && current.Channel != "beta" && current.Channel != "stable") ||
		current.TargetOS != targetOS ||
		current.TargetArch != targetArch {
		return Plan{}, errors.New("installed state does not match the requested update target")
	}
	nextVersion, err := parseVersion(manifest.Release)
	if err != nil {
		return Plan{}, fmt.Errorf("invalid candidate release: %w", err)
	}
	if compareVersion(nextVersion, currentVersion) <= 0 {
		return Plan{}, errors.New("standard update plan must move to a newer release")
	}
	roleBinding, err := rolemigration.EnsureUpdateAllowed(current.Release, manifest.Release, manifest)
	if err != nil {
		return Plan{}, err
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
		SchemaVersion: 2, State: "available",
		FromRelease: current.Release, FromChannel: current.Channel,
		FromCLIVersion: current.CLIVersion, FromBundleVersion: current.BundleVersion,
		ToRelease: manifest.Release, Channel: manifest.Channel,
		CLIVersion: manifest.CLI.Version, BundleVersion: manifest.Bundle.Version,
		CLIArtifact: cliName, BundleArtifact: bundleName,
		TargetOS: targetOS, TargetArch: targetArch,
		Provider: source.Provider, ProviderReleaseID: source.ProviderReleaseID,
		ManifestSHA256: source.ManifestSHA256, ConfirmationRequired: true,
		RoleMigrationID: roleBinding.ID, CatalogSHA256: roleBinding.CatalogSHA256,
		PolicySHA256: roleBinding.PolicySHA256,
	}
	identifier, err := computePlanID(plan)
	if err != nil {
		return Plan{}, err
	}
	plan.ID = identifier
	return plan, nil
}

func Validate(plan Plan) error {
	if plan.SchemaVersion != 2 ||
		plan.State != "available" ||
		!plan.ConfirmationRequired ||
		plan.Provider != "github" ||
		plan.ProviderReleaseID <= 0 ||
		len(plan.ManifestSHA256) != 64 ||
		strings.Trim(plan.ManifestSHA256, "0123456789abcdef") != "" ||
		(plan.Channel != "canary" && plan.Channel != "beta" && plan.Channel != "stable") ||
		(plan.FromChannel != "canary" && plan.FromChannel != "beta" && plan.FromChannel != "stable") ||
		(plan.TargetOS != "darwin" && plan.TargetOS != "windows") ||
		(plan.TargetArch != "amd64" && plan.TargetArch != "arm64") ||
		plan.CLIArtifact == "" ||
		plan.BundleArtifact == "" {
		return errors.New("update plan contract is invalid")
	}
	from, err := parseVersion(plan.FromRelease)
	if err != nil {
		return fmt.Errorf("invalid update-plan source release: %w", err)
	}
	to, err := parseVersion(plan.ToRelease)
	if err != nil {
		return fmt.Errorf("invalid update-plan target release: %w", err)
	}
	if compareVersion(to, from) <= 0 {
		return errors.New("update plan must move to a newer release")
	}
	if _, err := parseVersion(plan.FromCLIVersion); err != nil {
		return fmt.Errorf("invalid update-plan source CLI: %w", err)
	}
	if _, err := parseVersion(plan.FromBundleVersion); err != nil {
		return fmt.Errorf("invalid update-plan source bundle: %w", err)
	}
	if _, err := parseVersion(plan.CLIVersion); err != nil {
		return fmt.Errorf("invalid update-plan target CLI: %w", err)
	}
	if _, err := parseVersion(plan.BundleVersion); err != nil {
		return fmt.Errorf("invalid update-plan target bundle: %w", err)
	}
	if plan.RoleMigrationID != "" {
		if plan.RoleMigrationID != rolemigration.MigrationID ||
			len(plan.CatalogSHA256) != 64 || strings.Trim(plan.CatalogSHA256, "0123456789abcdef") != "" ||
			len(plan.PolicySHA256) != 64 || strings.Trim(plan.PolicySHA256, "0123456789abcdef") != "" {
			return errors.New("update plan role migration binding is invalid")
		}
	}
	required, err := rolemigration.RequiresMigration(plan.FromRelease, plan.ToRelease)
	if err != nil {
		return err
	}
	if required && plan.RoleMigrationID != rolemigration.MigrationID {
		return errors.New("update plan crosses the practice-agent alias expiry without a migration binding")
	}
	expected, err := computePlanID(plan)
	if err != nil {
		return err
	}
	if plan.ID != expected {
		return errors.New("update plan ID does not match its canonical fields")
	}
	return nil
}

func computePlanID(plan Plan) (string, error) {
	plan.ID = ""
	body, err := json.Marshal(plan)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:16]), nil
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
