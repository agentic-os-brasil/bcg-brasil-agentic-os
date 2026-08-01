// Package topologygate rejects reintroduction of retired product roles from
// canonical source, schemas, fixtures and documentation. The forbidden
// markers intentionally live only in this development gate.
package topologygate

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

var forbiddenMarkers = []string{"capability_specialist", "capability specialist"}

func Validate(root string) error {
	paths := []string{"README.md", "ROADMAP.md", "specs", "schemas", "bundles", "internal", "adapters", "docs"}
	for _, relative := range paths {
		path := filepath.Join(root, relative)
		if _, err := os.Stat(path); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		if err := filepath.WalkDir(path, func(current string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				if entry.Name() == ".git" || entry.Name() == "topologygate" {
					return filepath.SkipDir
				}
				return nil
			}
			if !isTextFile(current) {
				return nil
			}
			body, err := os.ReadFile(current)
			if err != nil {
				return err
			}
			lower := strings.ToLower(string(body))
			for _, marker := range forbiddenMarkers {
				if strings.Contains(lower, marker) {
					return fmt.Errorf("retired agent role marker %q found in canonical product surface %s", marker, filepath.ToSlash(current))
				}
			}
			return nil
		}); err != nil {
			return err
		}
	}
	return nil
}

func isTextFile(path string) bool {
	ext := filepath.Ext(path)
	switch ext {
	case ".go", ".json", ".md", ".yaml", ".yml", ".tmpl", ".txt", ".toml":
		return true
	default:
		return false
	}
}
