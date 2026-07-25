package releasecontract

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseAcceptsCompatibleReleaseReferences(t *testing.T) {
	manifest, err := Parse(strings.NewReader(validManifestJSON()))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if manifest.Release != "0.1.0" || manifest.CLI.Version != "0.1.0" || manifest.Bundle.Version != "0.1.0" {
		t.Fatalf("unexpected versions: %#v", manifest)
	}
}

func TestParseRejectsUnknownAndTrailingJSON(t *testing.T) {
	for name, body := range map[string]string{
		"unknown":              strings.Replace(validManifestJSON(), `"product": "maestro"`, `"product": "maestro", "surprise": true`, 1),
		"trailing":             validManifestJSON() + `{}`,
		"duplicate top-level":  strings.Replace(validManifestJSON(), `"product": "maestro"`, `"product": "maestro", "product": "maestro"`, 1),
		"duplicate nested":     strings.Replace(validManifestJSON(), `"id": "maestro-release"`, `"id": "maestro-release", "id": "other"`, 1),
		"oversized whitespace": validManifestJSON() + strings.Repeat(" ", maximumManifestBytes),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse(strings.NewReader(body)); err == nil {
				t.Fatal("Parse() accepted invalid JSON")
			}
		})
	}
}

func TestParseRejectsInvalidReleaseContracts(t *testing.T) {
	tests := map[string]func(string) string{
		"wrong product": func(body string) string {
			return strings.Replace(body, `"product": "maestro"`, `"product": "other"`, 1)
		},
		"mutable release label": func(body string) string {
			return strings.Replace(body, `"release": "0.1.0"`, `"release": "latest"`, 1)
		},
		"unknown channel": func(body string) string {
			return strings.Replace(body, `"channel": "canary"`, `"channel": "nightly"`, 1)
		},
		"missing issuer key": func(body string) string {
			return strings.Replace(body, `"key_id": "maestro-pilot-2026"`, `"key_id": ""`, 1)
		},
		"cli rejects bundle": func(body string) string {
			return strings.Replace(body, `"compatible_bundle": ">=0.1.0 <0.2.0"`, `"compatible_bundle": ">=0.2.0 <0.3.0"`, 1)
		},
		"non-canonical range whitespace": func(body string) string {
			return strings.Replace(body, `">=0.1.0 <0.2.0"`, `">=0.1.0  <0.2.0"`, 1)
		},
		"versionless cli artifact": func(body string) string {
			return strings.Replace(body, `"name": "bcgos-0.1.0-windows-amd64.exe"`, `"name": "bcgos-windows-amd64.exe"`, 1)
		},
		"runtime pack unsupported in v1": func(body string) string {
			return strings.Replace(body, `"kind": "cli"`, `"kind": "runtime_pack"`, 1)
		},
		"bundle rejects cli": func(body string) string {
			return strings.Replace(body, `"compatible_cli": ">=0.1.0 <0.2.0"`, `"compatible_cli": ">=0.2.0 <0.3.0"`, 1)
		},
		"artifact contains path": func(body string) string {
			return strings.Replace(body, `"name": "bcgos-0.1.0-windows-amd64.exe"`, `"name": "workspace/bcgos.exe"`, 1)
		},
		"artifact has uppercase hash": func(body string) string {
			return strings.Replace(body, strings.Repeat("a", 64), strings.Repeat("A", 64), 1)
		},
		"artifact lacks signature": func(body string) string {
			return strings.Replace(body, `"signature_ref": "bcgos-0.1.0-windows-amd64.exe.sig"`, `"signature_ref": ""`, 1)
		},
		"bundle is platform specific": func(body string) string {
			return strings.Replace(body, `"kind": "bundle", "os": "any", "arch": "any"`, `"kind": "bundle", "os": "windows", "arch": "amd64"`, 1)
		},
		"duplicate artifact": func(body string) string {
			needle := `{"kind": "cli", "os": "windows", "arch": "amd64", "name": "bcgos-0.1.0-windows-amd64.exe", "size": 1234, "sha256": "` + strings.Repeat("a", 64) + `", "signature_ref": "bcgos-0.1.0-windows-amd64.exe.sig"}`
			return strings.Replace(body, needle, needle+","+needle, 1)
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse(strings.NewReader(mutate(validManifestJSON()))); err == nil {
				t.Fatal("Parse() accepted invalid release contract")
			}
		})
	}
}

func TestVersionRangeContainsClosedOpenBounds(t *testing.T) {
	versionRange, err := ParseVersionRange(">=0.1.0 <0.2.0")
	if err != nil {
		t.Fatalf("ParseVersionRange() error = %v", err)
	}
	for version, want := range map[string]bool{
		"0.0.9": false,
		"0.1.0": true,
		"0.1.9": true,
		"0.2.0": false,
	} {
		got, err := versionRange.Contains(version)
		if err != nil {
			t.Fatalf("Contains(%q) error = %v", version, err)
		}
		if got != want {
			t.Fatalf("Contains(%q) = %v, want %v", version, got, want)
		}
	}
}

func TestPublishedSchemaHasExpectedIdentity(t *testing.T) {
	path := filepath.Join("..", "..", "schemas", "release-manifest.schema.json")
	if err := ValidateSchemaFile(path); err != nil {
		t.Fatalf("ValidateSchemaFile() error = %v", err)
	}
}

func TestValidateSchemaFileRejectsGuttedSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "schema.json")
	body := `{"$schema":"https://json-schema.org/draft/2020-12/schema","$id":"urn:bcg-brasil-agentic-os:schema:release-manifest:v1"}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ValidateSchemaFile(path); err == nil {
		t.Fatal("ValidateSchemaFile() accepted a schema without the contract")
	}
}

func validManifestJSON() string {
	return `{
  "schema_version": 1,
  "product": "maestro",
  "release": "0.1.0",
  "channel": "canary",
  "issuer": {"id": "maestro-release", "key_id": "maestro-pilot-2026"},
  "cli": {"version": "0.1.0", "compatible_bundle": ">=0.1.0 <0.2.0"},
  "bundle": {"version": "0.1.0", "compatible_cli": ">=0.1.0 <0.2.0"},
  "artifacts": [
    {"kind": "cli", "os": "windows", "arch": "amd64", "name": "bcgos-0.1.0-windows-amd64.exe", "size": 1234, "sha256": "` + strings.Repeat("a", 64) + `", "signature_ref": "bcgos-0.1.0-windows-amd64.exe.sig"},
    {"kind": "cli", "os": "darwin", "arch": "arm64", "name": "bcgos-0.1.0-darwin-arm64", "size": 1234, "sha256": "` + strings.Repeat("b", 64) + `", "signature_ref": "bcgos-0.1.0-darwin-arm64.sig"},
    {"kind": "bundle", "os": "any", "arch": "any", "name": "maestro-base-0.1.0.tar.gz", "size": 4321, "sha256": "` + strings.Repeat("c", 64) + `", "signature_ref": "maestro-base-0.1.0.tar.gz.sig"}
  ],
  "migrations": [
    {"id": "base-0.1", "component": "bundle", "from": ">=0.0.0 <0.1.0", "to": "0.1.0", "required": true}
  ],
  "release_notes": {"name": "release-notes-0.1.0.md", "sha256": "` + strings.Repeat("d", 64) + `"}
}`
}
