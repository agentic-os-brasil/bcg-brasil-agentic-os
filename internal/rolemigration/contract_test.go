package rolemigration

import (
	"strings"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
)

func TestCrossingExpiryRequiresPinnedMigration(t *testing.T) {
	manifest := migrationManifest()
	if _, err := EnsureUpdateAllowed("0.1.9", "0.2.0", manifest); err != nil {
		t.Fatal(err)
	}
	manifest.Migrations = nil
	if _, err := EnsureUpdateAllowed("0.1.9", "0.2.0", manifest); err == nil {
		t.Fatal("expiry crossing accepted without a migration")
	}
}

func TestPostExpiryUpdatesDoNotReapplyMigration(t *testing.T) {
	manifest := migrationManifest()
	manifest.Release = "0.3.0"
	manifest.Bundle.Version = "0.3.0"
	manifest.CLI.Version = "0.3.0"
	manifest.Migrations[0].To = "0.3.0"
	binding, err := EnsureUpdateAllowed("0.2.0", "0.3.0", manifest)
	if err != nil {
		t.Fatal(err)
	}
	if binding.ID != "" {
		t.Fatal("post-expiry update reapplied the legacy-source migration")
	}
}

func TestMigrationRejectsSourceOutsideDeclaredRange(t *testing.T) {
	if _, err := EnsureUpdateAllowed("0.0.9", "0.2.0", migrationManifest()); err == nil {
		t.Fatal("migration accepted a source outside >=0.1.0 <0.2.0")
	}
}

func TestRoleBoundUpdateRejectsDowngradeAndSameRelease(t *testing.T) {
	for _, target := range []string{"0.3.0", "0.1.9"} {
		manifest := releasecontract.Manifest{Release: target}
		if _, err := EnsureUpdateAllowed("0.3.0", target, manifest); err == nil {
			t.Fatalf("update accepted non-increasing target %s", target)
		}
	}
}

func TestBindingRejectsUnpinnedIdentity(t *testing.T) {
	binding, _, err := FromManifest(migrationManifest())
	if err != nil {
		t.Fatal(err)
	}
	binding.PolicySHA256 = ""
	if err := ValidateBinding(binding); err == nil {
		t.Fatal("unpinned policy identity was accepted")
	}
}

func migrationManifest() releasecontract.Manifest {
	return releasecontract.Manifest{
		SchemaVersion: 1, Product: "maestro", Release: "0.2.0", Channel: "canary",
		CLI:       releasecontract.CLIComponent{Version: "0.2.0"},
		Bundle:    releasecontract.BundleComponent{Version: "0.2.0"},
		Artifacts: []releasecontract.Artifact{{Kind: "bundle", OS: "any", Arch: "any", SHA256: strings.Repeat("a", 64)}},
		Migrations: []releasecontract.Migration{{
			ID: MigrationID, Component: "bundle", From: SourceRange, To: AliasExpiresAfter, Required: true,
			FromRole: LegacyRole, ToRole: CanonicalRole, AliasExpiresAfter: AliasExpiresAfter,
			BundleSHA256: strings.Repeat("a", 64), CatalogSHA256: strings.Repeat("b", 64), PolicySHA256: strings.Repeat("c", 64),
		}},
	}
}
