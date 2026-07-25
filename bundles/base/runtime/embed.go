package baseruntime

import (
	"bytes"
	_ "embed"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/hookpolicy"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/runtimecap"
)

//go:embed capabilities.json
var capabilitiesJSON []byte

//go:embed hook-policy.json
var hookPolicyJSON []byte

func Manifest() (runtimecap.Manifest, error) {
	return runtimecap.Parse(bytes.NewReader(capabilitiesJSON))
}

func HookPolicy() (hookpolicy.Policy, error) {
	return hookpolicy.Parse(bytes.NewReader(hookPolicyJSON))
}
