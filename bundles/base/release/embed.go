package baserelease

import (
	"bytes"
	_ "embed"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseprovider"
)

//go:embed provider.json
var providerJSON []byte

func Provider() (releaseprovider.Config, error) {
	return releaseprovider.ParseConfig(bytes.NewReader(providerJSON))
}
