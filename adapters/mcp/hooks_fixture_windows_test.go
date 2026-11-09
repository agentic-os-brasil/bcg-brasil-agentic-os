//go:build windows

package product_test

import "testing"

func hookDirectoryAlias(t *testing.T, alias, target string) {
	t.Helper()
	windowsJunction(t, alias, target)
}
