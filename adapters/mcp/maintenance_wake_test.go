package product_test

import (
	"context"
	"errors"
	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
	"testing"
)

// No clock override, past occurrence, model or OS scheduler is used here.
func TestMaintenanceWakeNeedsLiveNativeVetoAndGrants(t *testing.T) {
	f := newOwnerFixture(t, false)
	b, err := product.Open(f.options)
	if err != nil {
		t.Fatal(err)
	}
	before := cadenceTree(t, f)
	if _, err := b.RunMaintenanceWake(context.Background(), nil); err == nil {
		t.Fatal("missing native eligibility admitted")
	}
	denied := errors.New("owned wake paused")
	if _, err := b.RunMaintenanceWake(context.Background(), func() error { return denied }); !errors.Is(err, denied) {
		t.Fatal("live native refusal lost", err)
	}
	if cadenceTree(t, f) != before {
		t.Fatal("refused wake wrote domain state")
	}
	checks := 0
	report, err := b.RunMaintenanceWake(context.Background(), func() error { checks++; return nil })
	if err != nil || report.State != "no_due_work" || len(report.Due) != 0 || len(report.Receipts) != 0 {
		t.Fatalf("wake without job grants looked like actual work: %+v %v", report, err)
	}
	if checks == 0 {
		t.Fatal("wake did not consult trusted eligibility")
	}
	if cadenceTree(t, f) != before {
		t.Fatal("no-due wake initialized authority or domain state")
	}
}
