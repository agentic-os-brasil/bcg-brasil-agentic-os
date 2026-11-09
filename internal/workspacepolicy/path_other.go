//go:build !windows

package workspacepolicy

func validatePolicyPath(string) error { return nil }
