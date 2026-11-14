package product_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
)

func TestPreflightPreservesHomeCauseWithoutExposingPrivatePaths(t *testing.T) {
	base := t.TempDir()
	err := product.Preflight(product.Options{
		ProductRoot:   filepath.Join(base, "missing-product"),
		UserHome:      filepath.Join(base, "missing-owner"),
		WorkspacePath: filepath.Join(base, "workspace"),
	})
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("preflight discarded missing-home cause: %v", err)
	}
	if err.Error() != "home_unavailable" {
		t.Fatalf("preflight exposed private cause: %q", err.Error())
	}
}
