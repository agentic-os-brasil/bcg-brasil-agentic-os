package packaging_test

import (
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp/packaging"
	"testing"
)

func TestPackageRequiresAdmittedBinding(t *testing.T) {
	if packaging.BuildPlugin(packaging.Options{}) == nil {
		t.Fatal("unbound package admitted")
	}
}
