package federation

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const MaximumPortableSkillBytes = 64 << 10

var portableSkillIDPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{1,63}$`)

type PortableOrigin string

const BornPortable PortableOrigin = "born_portable"

// PortableSkillManifest is the participant's durable declaration that a skill
// was authored in the dedicated portable root for generalized reuse. It has no
// person, client or workspace identifier and requires later central curation.
type PortableSkillManifest struct {
	SchemaVersion int            `json:"schema_version"`
	SkillID       string         `json:"skill_id"`
	Version       string         `json:"version"`
	Origin        PortableOrigin `json:"origin"`
	ContentSHA256 string         `json:"content_sha256"`
	Generalizable bool           `json:"generalizable"`
}

type PortableSkillPackage struct {
	Manifest PortableSkillManifest
	Content  string
}

// PortableSkillCollector accepts complete skill content only from the managed
// born-portable root. It rejects links, malformed manifests and content hash
// mismatches, preventing a workspace file from being smuggled in by pathname.
type PortableSkillCollector struct {
	Root string
}

func (collector PortableSkillCollector) Collect() ([]PortableSkillPackage, error) {
	if strings.TrimSpace(collector.Root) == "" {
		return nil, errors.New("portable skill root is required")
	}
	entries, err := os.ReadDir(collector.Root)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	packages := make([]PortableSkillPackage, 0, len(entries))
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 || !entry.IsDir() {
			return nil, fmt.Errorf("invalid portable skill root entry %q", entry.Name())
		}
		packageValue, err := collector.collectOne(entry.Name())
		if err != nil {
			return nil, err
		}
		packages = append(packages, packageValue)
	}
	sort.Slice(packages, func(left, right int) bool { return packages[left].Manifest.SkillID < packages[right].Manifest.SkillID })
	return packages, nil
}

func (collector PortableSkillCollector) collectOne(skillID string) (PortableSkillPackage, error) {
	if !portableSkillIDPattern.MatchString(skillID) {
		return PortableSkillPackage{}, fmt.Errorf("invalid portable skill directory %q", skillID)
	}
	directory := filepath.Join(collector.Root, skillID)
	manifestPath := filepath.Join(directory, "manifest.json")
	contentPath := filepath.Join(directory, "SKILL.md")
	if err := regularFile(manifestPath); err != nil {
		return PortableSkillPackage{}, err
	}
	if err := regularFile(contentPath); err != nil {
		return PortableSkillPackage{}, err
	}
	var manifest PortableSkillManifest
	if err := readStrictJSON(manifestPath, &manifest); err != nil {
		return PortableSkillPackage{}, err
	}
	if manifest.SkillID != skillID {
		return PortableSkillPackage{}, errors.New("portable skill manifest does not match its directory")
	}
	if err := manifest.Validate(); err != nil {
		return PortableSkillPackage{}, err
	}
	contents, err := os.ReadFile(contentPath)
	if err != nil {
		return PortableSkillPackage{}, err
	}
	if len(contents) == 0 || len(contents) > MaximumPortableSkillBytes {
		return PortableSkillPackage{}, errors.New("portable skill content is outside the approved size")
	}
	digest := sha256.Sum256(contents)
	if manifest.ContentSHA256 != hex.EncodeToString(digest[:]) {
		return PortableSkillPackage{}, errors.New("portable skill content hash does not match manifest")
	}
	return PortableSkillPackage{Manifest: manifest, Content: string(contents)}, nil
}

func (manifest PortableSkillManifest) Validate() error {
	if manifest.SchemaVersion != SchemaVersion || !portableSkillIDPattern.MatchString(manifest.SkillID) || !versionPattern.MatchString(manifest.Version) || manifest.Origin != BornPortable || !fingerprintPattern.MatchString(manifest.ContentSHA256) || !manifest.Generalizable {
		return errors.New("invalid born-portable skill manifest")
	}
	return nil
}

// ValidatePortableSkillManifestSchemaFile makes the strict manifest contract
// visible to release validation without making JSON Schema a runtime service.
func ValidatePortableSkillManifestSchemaFile(path string) error {
	var schema map[string]any
	if err := readStrictJSON(path, &schema); err != nil {
		return err
	}
	if schema["$schema"] != "https://json-schema.org/draft/2020-12/schema" {
		return errors.New("portable skill manifest schema must use JSON Schema draft 2020-12")
	}
	if schema["$id"] != "urn:bcg-brasil-agentic-os:schema:portable-skill-manifest:v1" {
		return errors.New("portable skill manifest schema has an unexpected identifier")
	}
	return nil
}

func regularFile(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("portable skill file %q must be a regular file", filepath.Base(path))
	}
	return nil
}
