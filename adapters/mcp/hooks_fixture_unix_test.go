//go:build !windows

package product_test

import (
	"os"
	"testing"
)

func hookDirectoryAlias(t *testing.T, alias, target string) {
	t.Helper()
	if err := os.Symlink(target, alias); err != nil {
		t.Fatal(err)
	}
}
