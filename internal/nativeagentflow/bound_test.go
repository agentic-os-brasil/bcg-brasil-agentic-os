package nativeagentflow_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/nativeagentflow"
)

func boundStore(t *testing.T) nativeagentflow.Store {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	store, err := nativeagentflow.NewBound(root, "workspace-a")
	if err != nil {
		t.Fatal(err)
	}
	return store
}
func TestBoundReservationCorrelationAndObservation(t *testing.T) {
	s := boundStore(t)
	if err := s.BeginOccurrence("session", "p1"); err != nil {
		t.Fatal(err)
	}
	if s.Correlate("session", "p1", "forged", "case-agent") == nil {
		t.Fatal("unreserved agent admitted")
	}
	if err := s.Admit("session", "p1", "tool", "case-agent"); err != nil {
		t.Fatal(err)
	}
	if s.CheckMain("session", "p1") == nil {
		t.Fatal("pending reservation admitted unidentified main tools")
	}
	if err := s.Admit("session", "p1", "tool", "case-agent"); err != nil {
		t.Fatalf("exact pending dispatch retry refused: %v", err)
	}
	if s.Correlate("session", "p1", "agent", "pa-expert") == nil {
		t.Fatal("wrong target admitted")
	}
	if err := s.Correlate("session", "p1", "agent", "case-agent"); err != nil {
		t.Fatal(err)
	}
	if s.Authorize("session", "p1", "other", "case-agent") == nil {
		t.Fatal("other native ID admitted")
	}
	if s.ObserveStop("session", "p1", "other", "case-agent") == nil {
		t.Fatal("other native ID stopped route")
	}
	if s.BeginOccurrence("session", "p2") == nil {
		t.Fatal("new occurrence erased active child")
	}
	if err := s.ObserveStop("session", "p1", "agent", "case-agent"); err != nil {
		t.Fatal(err)
	}
	if s.Authorize("session", "p1", "agent", "case-agent") == nil {
		t.Fatal("stopped agent retained authority")
	}
	if err := s.ObserveFinalize("session", "p1"); err != nil {
		t.Fatal(err)
	}
	if err := s.BeginOccurrence("session", "p2"); err != nil {
		t.Fatal(err)
	}
	if s.Admit("session", "p2", "tool", "case-agent") == nil {
		t.Fatal("old dispatch replay regained admission")
	}
	if s.BeginOccurrence("session", "p1") == nil {
		t.Fatal("old prompt replay reset state")
	}
}
func TestBoundOptionalOccurrenceAndReplayOverflowKeepFences(t *testing.T) {
	s := boundStore(t)
	if err := s.BeginOccurrence("session", ""); err != nil {
		t.Fatal(err)
	}
	if err := s.ObserveFinalize("session", ""); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(s.BeginOccurrence("session", ""), nativeagentflow.ErrTurnResetUnavailable) {
		t.Fatal("missing occurrence reset terminal state")
	}
	// Exhaust through public metadata transitions; a cap must refuse, not prune.
	refused := false
	for i := 0; i < 150; i++ {
		if err := s.BeginOccurrence("session", fmt.Sprintf("p%d", i)); err != nil {
			refused = true
			break
		}
	}
	if !refused {
		t.Fatal("unbounded replay identities")
	}
	if s.BeginOccurrence("session", "p0") == nil {
		t.Fatal("old occurrence fence lost after overflow")
	}
}
func TestBoundStrategicGraphCountsStopsNotVerdicts(t *testing.T) {
	s := boundStore(t)
	if err := s.BeginOccurrence("session", "p"); err != nil {
		t.Fatal(err)
	}
	step := func(id, kind string) {
		t.Helper()
		if err := s.Admit("session", "p", "tool-"+id, kind); err != nil {
			t.Fatal(err)
		}
		if err := s.Correlate("session", "p", id, kind); err != nil {
			t.Fatal(err)
		}
		if err := s.ObserveStop("session", "p", id, kind); err != nil {
			t.Fatal(err)
		}
	}
	step("a", "client-account-agent")
	if s.ObserveFinalize("session", "p") == nil {
		t.Fatal("unresolved strategic route finalized")
	}
	step("b", "case-agent")
	if s.Admit("session", "p", "early-gamma", "gamma-guardian") == nil {
		t.Fatal("required account return bypassed")
	}
	step("c", "client-account-agent")
	step("d", "gamma-guardian")
	if err := s.ObserveFinalize("session", "p"); err != nil {
		t.Fatal(err)
	}
}

func TestRepositoryPairFailureCorrelationAndResume(t *testing.T) {
	s := boundStore(t)
	const spec, standards = "repository-spec-reviewer", "repository-standards-reviewer"
	if err := s.BeginOccurrence("session", "pair"); err != nil {
		t.Fatal(err)
	}
	if err := s.Admit("session", "pair", "spec-call", spec); err != nil {
		t.Fatal(err)
	}
	if err := s.Correlate("session", "pair", "spec-worker", spec); err != nil {
		t.Fatal(err)
	}
	if s.ObserveStop("session", "pair", "spec-worker", spec) == nil {
		t.Fatal("sequential first return admitted")
	}
	if s.BeginOccurrence("session", "next") == nil || s.CheckMain("session", "pair") == nil || s.ObserveFinalize("session", "pair") == nil {
		t.Fatal("incomplete pair lost its fence")
	}
	if s.Correlate("session", "pair", "spoofed", standards) == nil {
		t.Fatal("unreserved complementary worker admitted")
	}
	if err := s.Admit("session", "pair", "standards-call", standards); err != nil {
		t.Fatal(err)
	}
	if err := s.Admit("session", "pair", "standards-call", standards); err != nil {
		t.Fatal("pending exact retry lost")
	}
	if s.Admit("session", "pair", "different-call", standards) == nil {
		t.Fatal("different pending identity admitted")
	}
	if err := s.Correlate("session", "pair", "standards-worker", standards); err != nil {
		t.Fatal(err)
	}
	if s.Authorize("session", "pair", "spec-worker", standards) == nil {
		t.Fatal("axis spoof admitted")
	}
	if s.ObserveStop("session", "old", "spec-worker", spec) == nil {
		t.Fatal("foreign occurrence stop admitted")
	}
	if err := s.ObserveStop("session", "pair", "spec-worker", spec); err != nil {
		t.Fatal(err)
	}
	if err := s.ObserveStop("session", "pair", "spec-worker", spec); err != nil {
		t.Fatal("exact stop retry lost")
	}
	if s.Authorize("session", "pair", "spec-worker", spec) == nil {
		t.Fatal("stopped reviewer kept tools")
	}
	if s.ObserveReviewReturn("session", "pair", "spoofed", spec) == nil || s.ObserveReviewReturn("session", "pair", "standards-call", standards) == nil {
		t.Fatal("unmatched or unfinished return admitted")
	}
	if err := s.ObserveReviewReturn("session", "pair", "spec-call", spec); err != nil {
		t.Fatal(err)
	}
	if err := s.ObserveStop("session", "pair", "standards-worker", standards); err != nil {
		t.Fatal(err)
	}
	// The pair does not manufacture Case completion or widen the ordinary route.
	if s.Admit("session", "pair", "early-refiner", "gamma-guardian") == nil {
		t.Fatal("pair impersonated Case completion")
	}
	if err := s.Admit("session", "pair", "case-call", "case-agent"); err != nil {
		t.Fatal(err)
	}
	if err := s.Correlate("session", "pair", "case-worker", "case-agent"); err != nil {
		t.Fatal(err)
	}
	if err := s.ObserveStop("session", "pair", "case-worker", "case-agent"); err != nil {
		t.Fatal(err)
	}
	if err := s.ObserveFinalize("session", "pair"); err != nil {
		t.Fatal(err)
	}
	if err := s.BeginOccurrence("session", "next"); err != nil {
		t.Fatal(err)
	}
	if s.Correlate("session", "next", "spec-worker", spec) == nil || s.ObserveStop("session", "pair", "standards-worker", standards) == nil {
		t.Fatal("late prior-pair event revived authority")
	}
}

func TestRepositoryPairReservesBudgetOnlyWhenOpened(t *testing.T) {
	const spec, standards = "repository-spec-reviewer", "repository-standards-reviewer"
	prefix := func(t *testing.T, count int) nativeagentflow.Store {
		t.Helper()
		s := boundStore(t)
		if err := s.BeginOccurrence("session", "budget"); err != nil {
			t.Fatal(err)
		}
		for i := 0; i < count; i++ {
			id := fmt.Sprintf("prefix-%d", i)
			if err := s.Admit("session", "budget", id, "pa-expert"); err != nil {
				t.Fatal(err)
			}
			if err := s.Correlate("session", "budget", id, "pa-expert"); err != nil {
				t.Fatal(err)
			}
			if err := s.ObserveStop("session", "budget", id, "pa-expert"); err != nil {
				t.Fatal(err)
			}
		}
		return s
	}
	for _, reserveBothFirst := range []bool{false, true} {
		t.Run(fmt.Sprintf("reserve_both_first_%t", reserveBothFirst), func(t *testing.T) {
			s := prefix(t, 14) // Fourteen real completed ordinary calls consume 28 transitions.
			if err := s.Admit("session", "budget", "spec-call", spec); err != nil {
				t.Fatal(err)
			}
			if reserveBothFirst {
				if err := s.Admit("session", "budget", "standards-call", standards); err != nil {
					t.Fatal(err)
				}
			}
			if err := s.Correlate("session", "budget", "spec-worker", spec); err != nil {
				t.Fatal(err)
			}
			if err := s.Admit("session", "budget", "standards-call", standards); err != nil {
				t.Fatalf("complement/retry lost its reserved budget after first start: %v", err)
			}
			if err := s.Admit("session", "budget", "standards-call", standards); err != nil {
				t.Fatalf("exact pending retry lost: %v", err)
			}
			if err := s.Correlate("session", "budget", "standards-worker", standards); err != nil {
				t.Fatal(err)
			}
			if err := s.ObserveStop("session", "budget", "spec-worker", spec); err != nil {
				t.Fatal(err)
			}
			if err := s.ObserveStop("session", "budget", "standards-worker", standards); err != nil {
				t.Fatal(err)
			}
			if err := s.ObserveReviewReturn("session", "budget", "standards-call", standards); err != nil {
				t.Fatal(err)
			}
			if s.Admit("session", "budget", "beyond-ceiling", "pa-expert") == nil {
				t.Fatal("completed pair exceeded the global 32-transition ceiling")
			}
			if err := s.ObserveFinalize("session", "budget"); err != nil {
				t.Fatal(err)
			}
		})
	}
	t.Run("insufficient_budget_does_not_fence", func(t *testing.T) {
		s := prefix(t, 15) // Thirty transitions cannot reserve four more.
		if s.Admit("session", "budget", "spec-call", spec) == nil {
			t.Fatal("pair opened without four transitions available")
		}
		if err := s.CheckMain("session", "budget"); err != nil {
			t.Fatalf("refused pair left a fence: %v", err)
		}
		if err := s.Admit("session", "budget", "last-ordinary", "pa-expert"); err != nil {
			t.Fatal(err)
		}
		if err := s.Correlate("session", "budget", "last-ordinary", "pa-expert"); err != nil {
			t.Fatal(err)
		}
		if err := s.ObserveStop("session", "budget", "last-ordinary", "pa-expert"); err != nil {
			t.Fatal(err)
		}
		if err := s.ObserveFinalize("session", "budget"); err != nil {
			t.Fatal(err)
		}
	})
}
