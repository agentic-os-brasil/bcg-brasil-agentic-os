//go:build !windows

package agentorchestration

func validateDurableStateFilePrivacy(path string) error {
	return nil
}
