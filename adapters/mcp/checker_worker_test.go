package product

import (
	"context"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installer"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestCheckerTransportOmitsOnlyOwnedAutomaticWorker(t *testing.T) {
	for _, automatic := range []bool{true, false} {
		t.Run(map[bool]string{true: "ordinary", false: "checker"}[automatic], func(t *testing.T) {
			f := newProductSeedFixture(t)
			b, err := Open(f.options)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			observed := make(chan struct{}, 1)
			worker := func(ctx context.Context) { observed <- struct{}{}; <-ctx.Done() }
			a, z := mcp.NewInMemoryTransports()
			done := make(chan error, 1)
			purpose := ordinaryServe
			if !automatic {
				purpose = installedCheckServe
			}
			go func() { done <- b.serveMCP(ctx, a, purpose, worker) }()
			client, err := mcp.NewClient(&mcp.Implementation{Name: "checker-worker-contract", Version: "1"}, nil).Connect(ctx, z, nil)
			if err != nil {
				t.Fatal(err)
			}
			tools, err := client.ListTools(ctx, nil)
			if err != nil || len(tools.Tools) != 3 {
				t.Fatalf("same MCP tools unavailable: %v %v", tools, err)
			}
			select {
			case <-observed:
				if !automatic {
					t.Fatal("checker transport started automatic maintenance")
				}
			case <-time.After(50 * time.Millisecond):
				if automatic {
					t.Fatal("ordinary transport omitted its automatic worker")
				}
			}
			_ = client.Close()
			cancel()
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("owned server/worker did not stop")
			}
		})
	}
}

func TestCheckerTransportRejectsMissingOrUnverifiedCapability(t *testing.T) {
	b := &Bound{}
	for _, capability := range []*installer.VerifiedStableCheck{nil, {}} {
		if err := b.ServeInstalledCheck(context.Background(), nil, capability); err == nil {
			t.Fatal("unadmitted checker capability served")
		}
	}
	if err := b.serveMCP(context.Background(), nil, servePurpose(255), nil); err == nil {
		t.Fatal("unknown child purpose served")
	}
}
