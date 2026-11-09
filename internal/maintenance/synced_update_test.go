package maintenance

import (
	"reflect"
	"strings"
	"testing"
)

func TestUpdateCheckDeclaresAttendedSourceCustodyAndPlanEffects(t *testing.T) {
	catalog, err := LoadFile("../../bundles/base/runtime/maintenance.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, job := range catalog.Jobs {
		if job.ID != "update-check" {
			continue
		}
		if job.Unattended != "never" || !reflect.DeepEqual(job.Writes, []string{"source_custody", "candidate_plan", "local_diagnostics"}) {
			t.Fatalf("update-check does not disclose its actual attended effects: unattended=%q writes=%v", job.Unattended, job.Writes)
		}
		return
	}
	t.Fatal("update-check missing")
}

func TestUpdateCheckCannotEnrollAsRecurringWork(t *testing.T) {
	s := productFixture(t)
	before := productSnapshot(t, s.home)
	for _, kind := range []string{"enroll", "resume", "pause", "revoke"} {
		_, err := s.Review(Operation{Kind: kind, JobID: "update-check", Timezone: "UTC"})
		if err == nil || !strings.Contains(err.Error(), "attended-only") {
			t.Fatalf("%s did not reach the attended-only fence: %v", kind, err)
		}
	}
	if !reflect.DeepEqual(before, productSnapshot(t, s.home)) {
		t.Fatal("refused recurring update-check changed owner bytes")
	}
}
