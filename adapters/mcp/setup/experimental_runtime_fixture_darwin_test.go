//go:build maestro_experimental_installed_seed && darwin && arm64

package setup_test

import "testing"

func experimentalRuntimeFixtureHost(t *testing.T) { t.Helper() }

func experimentalRuntimeInertPack(t *testing.T, root, source, version string) string {
	t.Helper()
	return runtimeInertPack(t, root, source, version, "engineering installed-seed inert "+version)
}
