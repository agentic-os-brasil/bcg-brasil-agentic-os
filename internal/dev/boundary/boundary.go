package boundary

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var distributionRoots = []string{"bundles", "adapters", "installers", filepath.Join("cmd", "bcgos")}

var forbiddenReferences = []string{
	"dev/harness",
	"dev/skills",
	"internal/dev",
	"docs/decisions/decision-log.md",
	"specs/005-development-harness.md",
}

var textExtensions = map[string]bool{
	".go": true, ".json": true, ".md": true, ".ps1": true, ".sh": true, ".toml": true, ".yaml": true, ".yml": true,
}

// Validate rejects explicit development-harness references in distribution surfaces.
func Validate(root string) error {
	var problems []error
	for _, relativeRoot := range distributionRoots {
		path := filepath.Join(root, relativeRoot)
		if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			problems = append(problems, fmt.Errorf("inspect %s: %w", relativeRoot, err))
			continue
		}
		err := filepath.WalkDir(path, func(filePath string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || !textExtensions[filepath.Ext(entry.Name())] {
				return nil
			}
			content, err := os.ReadFile(filePath)
			if err != nil {
				return err
			}
			text := filepath.ToSlash(string(content))
			for _, forbidden := range forbiddenReferences {
				if strings.Contains(text, forbidden) {
					return fmt.Errorf("distribution file %s references development-only path %s", filePath, forbidden)
				}
			}
			return nil
		})
		if err != nil {
			problems = append(problems, err)
		}
	}
	return errors.Join(problems...)
}
