// Package userlevel contains the small process-identity boundary shared by
// user-space installation and workspace initialization.
package userlevel

// ensurePlatformUserLevel is implemented per operating system. Tests may
// replace it within this package to exercise the public contract without
// requiring an elevated test runner.
var ensurePlatformUserLevel = ensurePlatformUserLevelForPlatform

// EnsureNotElevated rejects a process that would create user-owned Maestro
// state under an administrator or system token. It is intentionally a
// preflight: callers must invoke it before their first filesystem mutation.
func EnsureNotElevated() error {
	return ensurePlatformUserLevel()
}
