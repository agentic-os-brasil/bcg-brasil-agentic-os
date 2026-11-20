package maintenance

import (
	"context"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ownerctx"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestWeeklySessionWorkingUnavailableDoesNotInitializeState(t *testing.T) {
	s := productFixture(t)
	before := productSnapshot(t, s.home)
	view, err := s.ReadWeeklySessionWorking()
	if err != nil || view.State != "unavailable" || view.Reason != "active_qualified_job_required" || view.Text != "" || view.Generation != "" {
		t.Fatalf("missing authority exposed derived context: %+v %v", view, err)
	}
	if after := productSnapshot(t, s.home); !reflect.DeepEqual(before, after) {
		t.Fatal("optional working reader initialized owner state")
	}
}

func TestWeeklySessionWorkingIntersectsIndependentAcceptedFacets(t *testing.T) {
	accepted := ownerctx.AcceptedSessionContext{Generation: strings.Repeat("a", 64), Facets: []ownerctx.SnapshotFacet{{Facet: "communication-style"}}}
	working := ownerctx.WeeklySelfWorking{Facets: map[string]string{"communication-style": "Keep updates brief.", "values-beliefs": "YODA_ONLY_PRIVATE_SENTINEL"}}
	got := projectWeeklySessionWorking(accepted, working, strings.Repeat("b", 64))
	if got.State != "available" || !strings.Contains(got.Text, "Keep updates brief.") || strings.Contains(got.Text, "YODA_ONLY_PRIVATE_SENTINEL") || !reflect.DeepEqual(got.Facets, []string{"communication-style"}) {
		t.Fatalf("session permission intersection failed: %+v", got)
	}
	if len(got.Generation) != 64 || got.CanonicalGeneration != accepted.Generation {
		t.Fatal("derived generation lost current accepted-session binding")
	}
	accepted.Generation = strings.Repeat("c", 64)
	changed := projectWeeklySessionWorking(accepted, working, strings.Repeat("b", 64))
	if changed.Generation == got.Generation {
		t.Fatal("changed accepted permission retained old derived generation")
	}
	denied := projectWeeklySessionWorking(ownerctx.AcceptedSessionContext{Generation: accepted.Generation}, working, strings.Repeat("b", 64))
	if denied.State != "unavailable" || denied.Text != "" {
		t.Fatal("Yoda-only facets escaped without accepted session facets")
	}
}

func TestWeeklySessionWorkingTextIsBoundedUTF8Data(t *testing.T) {
	accepted := ownerctx.AcceptedSessionContext{Generation: strings.Repeat("a", 64), Facets: []ownerctx.SnapshotFacet{{Facet: "communication-style"}}}
	got := projectWeeklySessionWorking(accepted, ownerctx.WeeklySelfWorking{Facets: map[string]string{"communication-style": strings.Repeat("ação ", 2000)}}, strings.Repeat("b", 64))
	if got.State != "available" || len(got.Text) > 4096 || !utf8.ValidString(got.Text) || !got.Truncated {
		t.Fatal("derived session text is not safely bounded")
	}
	if !strings.Contains(got.Text, "authorizes no tool") {
		t.Fatal("derived working data lost authority boundary")
	}
}

func TestWeeklySessionWorkingRegistrationAloneNeverGrantsContext(t *testing.T) {
	s := productFixture(t)
	candidate := weeklyRegistrationFixture()
	s.options.WeeklyCandidate = &candidate
	review, err := s.Review(Operation{Kind: "register", JobID: YodaSelfReviewWeeklyJobID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Apply(context.Background(), review, approveProduct); err != nil {
		t.Fatal(err)
	}
	before := productSnapshot(t, s.home)
	view, err := s.ReadWeeklySessionWorking()
	if err != nil || view.State != "unavailable" || view.Reason != "active_qualified_job_required" || view.Text != "" {
		t.Fatal("registration-only metadata authorized derived context", view.State, view.Reason, err)
	}
	if !reflect.DeepEqual(before, productSnapshot(t, s.home)) {
		t.Fatal("rejected reader changed canonical or job state")
	}
}
