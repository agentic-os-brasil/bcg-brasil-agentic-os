// Package rolemigration defines the release-boundary contract for retiring
// practice_agent in favor of the single pa_expert authority.
package rolemigration

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
)

const (
	MigrationID       = "practice-agent-to-pa-expert"
	LegacyRole        = "practice_agent"
	CanonicalRole     = "pa_expert"
	AliasExpiresAfter = "0.2.0"
	SourceRange       = ">=0.1.0 <0.2.0"
)

var digestPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

// Binding is the immutable identity carried by a release migration and by
// the installed state after the migration has been applied.
type Binding struct {
	ID                string
	FromRole          string
	ToRole            string
	AliasExpiresAfter string
	BundleSHA256      string
	CatalogSHA256     string
	PolicySHA256      string
}

// FromManifest returns the role migration binding, if this release carries
// one. A release may omit it when it is already entirely post-migration.
func FromManifest(manifest releasecontract.Manifest) (Binding, bool, error) {
	var found *releasecontract.Migration
	for index := range manifest.Migrations {
		migration := &manifest.Migrations[index]
		if migration.ID != MigrationID {
			continue
		}
		if found != nil {
			return Binding{}, false, errors.New("release contains duplicate practice-agent role migrations")
		}
		found = migration
	}
	if found == nil {
		return Binding{}, false, nil
	}
	binding := Binding{
		ID:                found.ID,
		FromRole:          found.FromRole,
		ToRole:            found.ToRole,
		AliasExpiresAfter: found.AliasExpiresAfter,
		BundleSHA256:      found.BundleSHA256,
		CatalogSHA256:     found.CatalogSHA256,
		PolicySHA256:      found.PolicySHA256,
	}
	if err := ValidateBinding(binding); err != nil {
		return Binding{}, false, err
	}
	expired, err := IsExpired(manifest.Release)
	if err != nil || !expired || found.To != manifest.Bundle.Version || found.From != SourceRange || found.Component != "bundle" {
		return Binding{}, false, errors.New("practice-agent role migration must be carried by post-expiry releases from the bounded 0.1.x range")
	}
	var bundleDigest string
	for _, artifact := range manifest.Artifacts {
		if artifact.Kind == "bundle" {
			bundleDigest = artifact.SHA256
			break
		}
	}
	if bundleDigest == "" || bundleDigest != binding.BundleSHA256 {
		return Binding{}, false, errors.New("practice-agent role migration bundle digest does not match the release artifact")
	}
	return binding, true, nil
}

func ValidateBinding(binding Binding) error {
	if binding.ID != MigrationID || binding.FromRole != LegacyRole || binding.ToRole != CanonicalRole ||
		binding.AliasExpiresAfter != AliasExpiresAfter ||
		!digestPattern.MatchString(binding.BundleSHA256) ||
		!digestPattern.MatchString(binding.CatalogSHA256) ||
		!digestPattern.MatchString(binding.PolicySHA256) {
		return errors.New("practice-agent role migration binding is incomplete or unpinned")
	}
	return nil
}

// RequiresMigration is true only for the crossing that can otherwise carry a
// legacy authority past the bounded compatibility window.
func RequiresMigration(fromRelease, toRelease string) (bool, error) {
	from, err := parseVersion(fromRelease)
	if err != nil {
		return false, fmt.Errorf("invalid migration source release: %w", err)
	}
	to, err := parseVersion(toRelease)
	if err != nil {
		return false, fmt.Errorf("invalid migration target release: %w", err)
	}
	expires, _ := parseVersion(AliasExpiresAfter)
	return from.compare(expires) < 0 && to.compare(expires) >= 0, nil
}

func EnsureUpdateAllowed(fromRelease, toRelease string, manifest releasecontract.Manifest) (Binding, error) {
	from, err := parseVersion(fromRelease)
	if err != nil {
		return Binding{}, fmt.Errorf("invalid migration source release: %w", err)
	}
	to, err := parseVersion(toRelease)
	if err != nil {
		return Binding{}, fmt.Errorf("invalid migration target release: %w", err)
	}
	if to.compare(from) <= 0 {
		return Binding{}, errors.New("role-bound update must move to a newer release")
	}
	binding, present, err := FromManifest(manifest)
	if err != nil {
		return Binding{}, err
	}
	required, err := RequiresMigration(fromRelease, toRelease)
	if err != nil {
		return Binding{}, err
	}
	eligible, err := sourceInMigrationRange(fromRelease)
	if err != nil {
		return Binding{}, err
	}
	if required && !eligible {
		return Binding{}, errors.New("update source is outside the bounded practice-agent migration range")
	}
	if required && !present {
		return Binding{}, errors.New("update crosses the practice-agent alias expiry without a signed role migration")
	}
	// A post-expiry installation may consume a release that still advertises
	// compatibility for legacy 0.1.x clients, but must not reapply or persist
	// that migration binding.
	if !eligible {
		return Binding{}, nil
	}
	return binding, nil
}

func sourceInMigrationRange(release string) (bool, error) {
	value, err := parseVersion(release)
	if err != nil {
		return false, fmt.Errorf("invalid migration source release: %w", err)
	}
	minimum, _ := parseVersion("0.1.0")
	expires, _ := parseVersion(AliasExpiresAfter)
	return value.compare(minimum) >= 0 && value.compare(expires) < 0, nil
}

func IsExpired(release string) (bool, error) {
	value, err := parseVersion(release)
	if err != nil {
		return false, err
	}
	expires, _ := parseVersion(AliasExpiresAfter)
	return value.compare(expires) >= 0, nil
}

type version [3]uint64

func parseVersion(value string) (version, error) {
	parts := strings.Split(value, ".")
	if len(parts) != 3 {
		return version{}, fmt.Errorf("%q is not MAJOR.MINOR.PATCH", value)
	}
	var result version
	for index, part := range parts {
		if part == "" || (len(part) > 1 && part[0] == '0') {
			return version{}, fmt.Errorf("%q is not canonical", value)
		}
		number, err := strconv.ParseUint(part, 10, 64)
		if err != nil {
			return version{}, fmt.Errorf("%q is not canonical: %w", value, err)
		}
		result[index] = number
	}
	return result, nil
}

func (left version) compare(right version) int {
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
