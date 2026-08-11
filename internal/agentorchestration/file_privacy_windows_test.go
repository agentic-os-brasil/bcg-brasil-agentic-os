//go:build windows

package agentorchestration

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateDurableStateFilePrivacyAcceptsCurrentUserState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "maestro-orchestration-state.json")
	if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := validateDurableStateFilePrivacy(path); err != nil {
		t.Fatalf("validateDurableStateFilePrivacy() error = %v", err)
	}
}
