package baseprofile

import (
	"bytes"
	_ "embed"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/profile"
)

//go:embed policy.json
var policyJSON []byte

// Policy returns the versioned managed profile contract consumed by all product
// skills and runtime adapters.
func Policy() (profile.Policy, error) {
	return profile.Load(bytes.NewReader(policyJSON))
}
