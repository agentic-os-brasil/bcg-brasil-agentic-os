package bundlecatalog

import (
	"bytes"
	_ "embed"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/capabilitybundle"
)

//go:embed catalog.json
var catalogJSON []byte

func Catalog() (capabilitybundle.Catalog, error) {
	return capabilitybundle.Parse(bytes.NewReader(catalogJSON))
}
