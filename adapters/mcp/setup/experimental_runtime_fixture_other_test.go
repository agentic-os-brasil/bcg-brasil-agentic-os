//go:build maestro_experimental_installed_seed && (!darwin || !arm64)

package setup_test

import "testing"

func experimentalRuntimeFixtureHost(t *testing.T) {
	t.Helper()
	t.Skip("compiled installed-seed runtime fixture is qualified only for Darwin arm64; other hosts are compile-only")
}

func experimentalRuntimeInertPack(t *testing.T, root, source, version string) string {
	t.Helper()
	t.Fatal("no inert runtime fixture qualified for this host")
	return ""
}
