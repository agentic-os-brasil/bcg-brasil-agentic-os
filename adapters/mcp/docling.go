package product

import (
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ingest/docling"
	"path/filepath"
)

// DoclingRuntime binds the converter to this admitted owner's state. Conversion
// independently revalidates the registered workspace before consuming its source.
func (b *Bound) DoclingRuntime() *docling.Runtime {
	return docling.New(docling.Options{DataRoot: filepath.Join(b.home, "state"), AuthorityRegistryPath: b.options.AuthorityRegistryPath, AuthorityRegistrySHA256: b.options.AuthorityRegistrySHA256, CLIVersion: b.options.CLIVersion})
}
