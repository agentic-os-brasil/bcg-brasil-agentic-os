package setup

import (
	"context"
	"strings"
	"testing"

	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installtx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseverify"
)

func TestInitialRuntimeOlderUnboundConsumerRemainsExplicitlyPending(t *testing.T) {
	calls := 0
	config := Config{Run: func(context.Context, string, ...string) ([]byte, error) { calls++; return nil, nil }}
	err := placeInitialRuntime(context.Background(), product.Options{}, config, installtx.State{SchemaVersion: 3}, releaseverify.VerifiedRelease{})
	if err == nil || !strings.Contains(err.Error(), "consumer has no authenticated runtime-update binding") || !strings.Contains(err.Error(), "converter setup pending") || calls != 0 {
		t.Fatalf("older unbound consumer was invoked or falsely admitted: calls=%d err=%v", calls, err)
	}
}
