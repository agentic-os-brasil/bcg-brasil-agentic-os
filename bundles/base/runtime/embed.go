package baseruntime

import (
	"bytes"
	_ "embed"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/runtimecap"
)

//go:embed capabilities.json
var capabilitiesJSON []byte

func Manifest() (runtimecap.Manifest, error) {
	return runtimecap.Parse(bytes.NewReader(capabilitiesJSON))
}
