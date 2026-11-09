//go:build !windows

package atlasops

import (
	"os"
	"path/filepath"
	"testing"
)

func atlasTestDataRoot(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "local", "BCGOS")
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	return root
}
