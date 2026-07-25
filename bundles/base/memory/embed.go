package basememory

import (
	"bytes"
	_ "embed"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/memory"
)

//go:embed policy.json
var policyJSON []byte

func Policy() (memory.Policy, error) {
	return memory.Load(bytes.NewReader(policyJSON))
}
