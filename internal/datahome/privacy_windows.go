//go:build windows

package datahome

// Windows discovery remains read-only until its owner/DACL checks are qualified.
func validateOwnership(string) error { return nil }

func ownershipVerified() bool { return false }
