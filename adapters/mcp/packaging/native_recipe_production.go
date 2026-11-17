//go:build !maestro_experimental_installed_seed

package packaging

// Ordinary canonical native bytes and their fingerprint remain unchanged.
func canonicalNativeProfile(platform, binding string, _ *stablePacketBinding) (map[string][]byte, error) {
	return nativeProfilePayload(platform, binding)
}
