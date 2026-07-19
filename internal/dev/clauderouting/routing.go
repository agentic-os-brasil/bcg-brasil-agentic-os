package clauderouting

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Manifest is the machine-readable Claude skill routing contract.
type Manifest struct {
	PrimaryRuntime string            `json:"primary_runtime"`
	CanonicalRoot  string            `json:"canonical_root"`
	ProjectionRoot string            `json:"projection_root"`
	Routes         map[string]string `json:"routes"`
	GoldenPath     []string          `json:"golden_path"`
	Fallback       string            `json:"fallback"`
}

// Load reads exactly one routing manifest and rejects unknown or trailing JSON.
func Load(root string) (Manifest, error) {
	path := filepath.Join(root, ".claude", "skill-routing.json")
	file, err := os.Open(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("open Claude skill routing manifest: %w", err)
	}
	defer file.Close()

	var manifest Manifest
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, fmt.Errorf("decode Claude skill routing manifest: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err == nil {
		return Manifest{}, fmt.Errorf("decode Claude skill routing manifest: trailing JSON value")
	} else if err != io.EOF {
		return Manifest{}, fmt.Errorf("decode Claude skill routing manifest trailing content: %w", err)
	}
	return manifest, nil
}

// HasSkill reports whether a skill is present in the declared routes.
func (manifest Manifest) HasSkill(name string) bool {
	for _, routed := range manifest.Routes {
		if routed == name {
			return true
		}
	}
	return false
}
