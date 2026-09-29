package basememory

import "testing"

func TestRuntimeProvidesBoundedSessionContextBudgets(t *testing.T) {
	config, err := Runtime()
	if err != nil {
		t.Fatal(err)
	}
	budgets := config.ContextBudgets()
	if len(budgets) == 0 {
		t.Fatal("no context budgets declared")
	}
	total := 0
	for layer, budget := range budgets {
		if budget < 128 || budget > 16384 {
			t.Fatalf("%s budget=%d outside bounds", layer, budget)
		}
		total += budget
	}
	// The block caps are maxima, not reservations: they are allowed to sum past
	// the total, because the hook cuts each block at its own cap and then holds
	// the envelope to the total separately. What must hold is that the total can
	// accommodate the largest single block — otherwise that cap is unreachable.
	largest := 0
	for _, budget := range budgets {
		if budget > largest {
			largest = budget
		}
	}
	if config.SessionContextTotalMaxBytes < largest {
		t.Fatalf("total %d cannot hold the largest block cap %d",
			config.SessionContextTotalMaxBytes, largest)
	}
	if total < config.SessionContextTotalMaxBytes {
		t.Logf("note: block caps sum to %d, under the total of %d — the total is not the binding constraint",
			total, config.SessionContextTotalMaxBytes)
	}
}

// The v1 caps were a declaration nothing enforced, and their values prove it:
// the lifetime cap was 1024 against a real measured 3197. They are kept in the
// file deliberately, as a record of the original intent, and this asserts they
// stay parked there rather than drifting back into the live set.
func TestSupersededRuneBudgetsStayOutOfTheLiveSet(t *testing.T) {
	config, err := Runtime()
	if err != nil {
		t.Fatal(err)
	}
	if config.SchemaVersion != 2 {
		t.Fatalf("schema_version=%d, expected 2", config.SchemaVersion)
	}
	if len(config.SupersededRunes) == 0 {
		t.Fatal("the superseded rune budgets were dropped; they record why the caps changed")
	}
	for layer := range config.ContextBudgets() {
		if _, live := config.SupersededRunes[layer]; live {
			t.Fatalf("%s appears in both the live budgets and the superseded set", layer)
		}
	}
}
