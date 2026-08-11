//go:build !windows

package userlevel

func ensurePlatformUserLevelForPlatform() error {
	return nil
}
