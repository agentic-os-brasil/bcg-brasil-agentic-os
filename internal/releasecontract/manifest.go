// Package releasecontract defines the provider-neutral Maestro release
// manifest. Parsing validates structure and compatibility but does not establish
// signature trust; callers must verify the detached manifest signature first.
package releasecontract

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

const maximumManifestBytes = 1 << 20

var (
	exactVersionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
	identifierPattern   = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,127}$`)
	hashPattern         = regexp.MustCompile(`^[a-f0-9]{64}$`)
)

type Manifest struct {
	SchemaVersion int          `json:"schema_version"`
	Product       string       `json:"product"`
	Release       string       `json:"release"`
	Channel       string       `json:"channel"`
	Issuer        Issuer       `json:"issuer"`
	CLI           Component    `json:"cli"`
	Bundle        Component    `json:"bundle"`
	Artifacts     []Artifact   `json:"artifacts"`
	Migrations    []Migration  `json:"migrations"`
	ReleaseNotes  ReleaseNotes `json:"release_notes"`
}

type Issuer struct {
	ID    string `json:"id"`
	KeyID string `json:"key_id"`
}

type Component struct {
	Version          string `json:"version"`
	CompatibleBundle string `json:"compatible_bundle,omitempty"`
	CompatibleCLI    string `json:"compatible_cli,omitempty"`
}

type Artifact struct {
	Kind         string `json:"kind"`
	OS           string `json:"os"`
	Arch         string `json:"arch"`
	Name         string `json:"name"`
	Size         int64  `json:"size"`
	SHA256       string `json:"sha256"`
	SignatureRef string `json:"signature_ref"`
}

type Migration struct {
	ID        string `json:"id"`
	Component string `json:"component"`
	From      string `json:"from"`
	To        string `json:"to"`
	Required  bool   `json:"required"`
}

type ReleaseNotes struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
}

func Parse(reader io.Reader) (Manifest, error) {
	decoder := json.NewDecoder(io.LimitReader(reader, maximumManifestBytes+1))
	decoder.DisallowUnknownFields()
	var manifest Manifest
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, fmt.Errorf("decode release manifest: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return Manifest{}, errors.New("decode release manifest: multiple JSON values")
		}
		return Manifest{}, fmt.Errorf("decode release manifest trailing content: %w", err)
	}
	if err := manifest.Validate(); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

func ValidateSchemaFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	var schema map[string]any
	if err := decoder.Decode(&schema); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("release manifest schema contains multiple JSON values")
		}
		return err
	}
	if schema["$schema"] != "https://json-schema.org/draft/2020-12/schema" {
		return errors.New("release manifest schema must use JSON Schema draft 2020-12")
	}
	if schema["$id"] != "urn:bcg-brasil-agentic-os:schema:release-manifest:v1" {
		return errors.New("release manifest schema has an unexpected identifier")
	}
	return nil
}

func (manifest Manifest) Validate() error {
	if manifest.SchemaVersion != 1 {
		return fmt.Errorf("unsupported release manifest schema version %d", manifest.SchemaVersion)
	}
	if manifest.Product != "maestro" {
		return fmt.Errorf("release product must be maestro")
	}
	if _, err := parseVersion(manifest.Release); err != nil {
		return fmt.Errorf("invalid release version: %w", err)
	}
	if manifest.Channel != "stable" && manifest.Channel != "beta" && manifest.Channel != "canary" {
		return fmt.Errorf("unsupported release channel %q", manifest.Channel)
	}
	if !identifierPattern.MatchString(manifest.Issuer.ID) || !identifierPattern.MatchString(manifest.Issuer.KeyID) {
		return errors.New("issuer id and key_id must be bounded identifiers")
	}
	if err := validateComponents(manifest.CLI, manifest.Bundle); err != nil {
		return err
	}
	if err := validateArtifacts(manifest); err != nil {
		return err
	}
	if err := validateMigrations(manifest); err != nil {
		return err
	}
	if err := validateFileReference(manifest.ReleaseNotes.Name); err != nil {
		return fmt.Errorf("invalid release notes name: %w", err)
	}
	if !strings.Contains(manifest.ReleaseNotes.Name, manifest.Release) {
		return errors.New("release notes name must contain the release version")
	}
	if !hashPattern.MatchString(manifest.ReleaseNotes.SHA256) {
		return errors.New("release notes sha256 must be lowercase hexadecimal")
	}
	return nil
}

func validateComponents(cli, bundle Component) error {
	if _, err := parseVersion(cli.Version); err != nil {
		return fmt.Errorf("invalid CLI version: %w", err)
	}
	if _, err := parseVersion(bundle.Version); err != nil {
		return fmt.Errorf("invalid bundle version: %w", err)
	}
	if cli.CompatibleCLI != "" || bundle.CompatibleBundle != "" {
		return errors.New("component compatibility fields are assigned to the wrong component")
	}
	cliRange, err := ParseVersionRange(cli.CompatibleBundle)
	if err != nil {
		return fmt.Errorf("invalid CLI compatible_bundle range: %w", err)
	}
	bundleRange, err := ParseVersionRange(bundle.CompatibleCLI)
	if err != nil {
		return fmt.Errorf("invalid bundle compatible_cli range: %w", err)
	}
	acceptsBundle, _ := cliRange.Contains(bundle.Version)
	acceptsCLI, _ := bundleRange.Contains(cli.Version)
	if !acceptsBundle || !acceptsCLI {
		return errors.New("CLI and bundle versions are not mutually compatible")
	}
	return nil
}

func validateArtifacts(manifest Manifest) error {
	if len(manifest.Artifacts) < 2 {
		return errors.New("release requires at least one CLI artifact and one bundle artifact")
	}
	names := map[string]bool{}
	signatures := map[string]bool{}
	platforms := map[string]bool{}
	cliCount := 0
	bundleCount := 0
	for _, artifact := range manifest.Artifacts {
		if artifact.Kind != "cli" && artifact.Kind != "bundle" && artifact.Kind != "runtime_pack" {
			return fmt.Errorf("unsupported artifact kind %q", artifact.Kind)
		}
		if err := validateArtifactPlatform(artifact); err != nil {
			return err
		}
		if err := validateFileReference(artifact.Name); err != nil {
			return fmt.Errorf("invalid artifact name: %w", err)
		}
		if err := validateFileReference(artifact.SignatureRef); err != nil {
			return fmt.Errorf("invalid artifact signature_ref: %w", err)
		}
		if !strings.HasSuffix(artifact.SignatureRef, ".sig") {
			return errors.New("artifact signature_ref must name a detached .sig file")
		}
		if artifact.Size <= 0 {
			return fmt.Errorf("artifact %s size must be positive", artifact.Name)
		}
		if !hashPattern.MatchString(artifact.SHA256) {
			return fmt.Errorf("artifact %s sha256 must be lowercase hexadecimal", artifact.Name)
		}
		platform := artifact.Kind + "/" + artifact.OS + "/" + artifact.Arch
		if names[artifact.Name] || signatures[artifact.SignatureRef] || platforms[platform] {
			return fmt.Errorf("duplicate artifact identity for %s", artifact.Name)
		}
		names[artifact.Name] = true
		signatures[artifact.SignatureRef] = true
		platforms[platform] = true
		switch artifact.Kind {
		case "cli":
			cliCount++
		case "bundle":
			bundleCount++
			if !strings.Contains(artifact.Name, manifest.Bundle.Version) {
				return errors.New("bundle artifact name must contain the bundle version")
			}
		}
	}
	if cliCount == 0 || bundleCount != 1 {
		return errors.New("release requires at least one CLI artifact and exactly one bundle artifact")
	}
	return nil
}

func validateArtifactPlatform(artifact Artifact) error {
	concreteOS := artifact.OS == "windows" || artifact.OS == "darwin" || artifact.OS == "linux"
	concreteArch := artifact.Arch == "amd64" || artifact.Arch == "arm64"
	switch artifact.Kind {
	case "bundle":
		if artifact.OS != "any" || artifact.Arch != "any" {
			return errors.New("bundle artifact must use os=any and arch=any")
		}
	default:
		if !concreteOS || !concreteArch {
			return fmt.Errorf("%s artifact requires a concrete supported OS and architecture", artifact.Kind)
		}
	}
	return nil
}

func validateMigrations(manifest Manifest) error {
	if manifest.Migrations == nil {
		return errors.New("migrations must be an explicit JSON array")
	}
	seen := map[string]bool{}
	for _, migration := range manifest.Migrations {
		if !identifierPattern.MatchString(migration.ID) || seen[migration.ID] {
			return fmt.Errorf("invalid or duplicate migration id %q", migration.ID)
		}
		seen[migration.ID] = true
		var target string
		switch migration.Component {
		case "cli":
			target = manifest.CLI.Version
		case "bundle":
			target = manifest.Bundle.Version
		case "runtime_pack":
			target = migration.To
		default:
			return fmt.Errorf("unsupported migration component %q", migration.Component)
		}
		versionRange, err := ParseVersionRange(migration.From)
		if err != nil {
			return fmt.Errorf("invalid migration %s source range: %w", migration.ID, err)
		}
		if _, err := parseVersion(migration.To); err != nil || migration.To != target {
			return fmt.Errorf("migration %s target does not match component version", migration.ID)
		}
		containsTarget, _ := versionRange.Contains(migration.To)
		if containsTarget {
			return fmt.Errorf("migration %s source range contains its target", migration.ID)
		}
		if !migration.Required {
			return fmt.Errorf("migration %s must explicitly be required", migration.ID)
		}
	}
	return nil
}

func validateFileReference(value string) error {
	if value == "" || value != filepath.Base(value) || strings.ContainsAny(value, `/\`) || value == "." || value == ".." {
		return errors.New("must be a non-empty basename without traversal")
	}
	if strings.Contains(strings.ToLower(value), "latest") {
		return errors.New("mutable latest references are forbidden")
	}
	for _, character := range value {
		if character < 0x21 || character == 0x7f {
			return errors.New("control characters and whitespace are forbidden")
		}
	}
	return nil
}

type version [3]uint64

func parseVersion(value string) (version, error) {
	match := exactVersionPattern.FindStringSubmatch(value)
	if match == nil {
		return version{}, fmt.Errorf("%q must be canonical MAJOR.MINOR.PATCH", value)
	}
	var parsed version
	for index := 0; index < 3; index++ {
		number, err := strconv.ParseUint(match[index+1], 10, 64)
		if err != nil {
			return version{}, fmt.Errorf("version segment overflow: %w", err)
		}
		parsed[index] = number
	}
	return parsed, nil
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

type VersionRange struct {
	lower version
	upper version
}

func ParseVersionRange(value string) (VersionRange, error) {
	fields := strings.Fields(value)
	if len(fields) != 2 || !strings.HasPrefix(fields[0], ">=") || !strings.HasPrefix(fields[1], "<") || strings.HasPrefix(fields[1], "<=") {
		return VersionRange{}, errors.New(`range must use ">=MAJOR.MINOR.PATCH <MAJOR.MINOR.PATCH"`)
	}
	lower, err := parseVersion(strings.TrimPrefix(fields[0], ">="))
	if err != nil {
		return VersionRange{}, err
	}
	upper, err := parseVersion(strings.TrimPrefix(fields[1], "<"))
	if err != nil {
		return VersionRange{}, err
	}
	if lower.compare(upper) >= 0 {
		return VersionRange{}, errors.New("range lower bound must be less than upper bound")
	}
	return VersionRange{lower: lower, upper: upper}, nil
}

func (versionRange VersionRange) Contains(value string) (bool, error) {
	candidate, err := parseVersion(value)
	if err != nil {
		return false, err
	}
	return candidate.compare(versionRange.lower) >= 0 && candidate.compare(versionRange.upper) < 0, nil
}
