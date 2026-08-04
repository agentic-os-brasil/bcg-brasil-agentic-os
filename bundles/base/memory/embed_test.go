package basememory

import "testing"

func TestRuntimeProvidesBoundedSessionContextBudgets(t *testing.T) {
	config, err := Runtime()
	if err != nil {
		t.Fatal(err)
	}
	budgets := config.ContextBudgets()
	total := 0
	for _, layer := range []string{"lifetime", "L3", "L2", "L1"} {
		if budgets[layer] < 128 || budgets[layer] > 2048 {
			t.Fatalf("%s budget=%d", layer, budgets[layer])
		}
		total += budgets[layer]
	}
	if total > 4096 {
		t.Fatalf("session context memory budget=%d", total)
	}
}
