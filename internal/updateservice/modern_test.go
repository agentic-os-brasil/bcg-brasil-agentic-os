package updateservice

import (
	"context"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installtx"
	"strings"
	"testing"
)

func TestCheckModernStateRefusesBeforeProviderAccess(t *testing.T) {
	_, err := Check(context.Background(), CheckOptions{Current: installtx.State{SchemaVersion: 3, Profile: "mcp-local-v1"}})
	if err == nil || !strings.Contains(err.Error(), "modern standard update unavailable") {
		t.Fatalf("modern state entered legacy provider path: %v", err)
	}
}
