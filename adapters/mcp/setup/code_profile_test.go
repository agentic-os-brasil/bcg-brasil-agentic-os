package setup

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// A bounded synthetic child only prints its environment. It is not a client witness.
func TestSelectedCodeProfileIsPerChildAndLeavesCallerUntouched(t *testing.T) {
	tool, err := exec.LookPath("printenv")
	if err != nil {
		t.Skip("printenv unavailable")
	}
	t.Setenv("CLAUDE_CONFIG_DIR", "unchanged-normal-profile")
	got, err := clientCommandWithProfile("/private/selected profile")(context.Background(), tool, []string{"CLAUDE_CONFIG_DIR"}, "")
	if err != nil || strings.TrimSpace(string(got)) != "/private/selected profile" {
		t.Fatalf("selected child profile %q %v", got, err)
	}
	if os.Getenv("CLAUDE_CONFIG_DIR") != "unchanged-normal-profile" {
		t.Fatal("calling profile changed")
	}
	got, err = clientCommand(context.Background(), tool, []string{"CLAUDE_CONFIG_DIR"}, "")
	if err != nil || strings.TrimSpace(string(got)) != "unchanged-normal-profile" {
		t.Fatalf("legacy child selection changed %q %v", got, err)
	}
}
