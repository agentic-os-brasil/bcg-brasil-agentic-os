package maintenance

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ownerctx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

func TestWeeklyProductRoutesOwnerAndRefusesUnregisteredProbe(t *testing.T) {
	service := productFixture(t)
	owner := service.weeklyModelService()
	if !owner.weeklyScoped || owner.workspaceID == service.workspaceID || owner.binding == service.binding {
		t.Fatal("weekly control not isolated at owner scope")
	}
	if scheduleFor(YodaSelfReviewWeeklyJobID).Cadence != scheduler.Weekly {
		t.Fatal("weekly job has nonweekly schedule")
	}
	for _, kind := range []string{"qualify", "enroll"} {
		op := Operation{Kind: kind, JobID: YodaSelfReviewWeeklyJobID}
		if kind == "enroll" {
			op.Timezone = "UTC"
		}
		if _, err := service.Review(op); err == nil {
			t.Fatalf("unregistered %s accepted", kind)
		}
	}
}
func TestWeeklyProductFactoryCannotFabricateNativeCompletion(t *testing.T) {
	service := productFixture(t).weeklyModelService()
	registration := weeklyRegistrationFixture()
	calls := 0
	service.options.WeeklyNativeExecutor = func(context.Context, WeeklyNativeAttemptRequest, *WeeklyAttemptSession) (ownerctx.WeeklySelfCandidate, error) {
		calls++
		return ownerctx.WeeklySelfCandidate{}, nil
	}
	handler := productWeeklyModelHandler{service: service, plan: &weeklyExecutionPlan{mode: WeeklyNativeProbe, registration: registration}}
	if _, err := handler.ExecuteAuthorized(context.Background(), Command{}, nil); err == nil || calls != 0 {
		t.Fatal("unbound factory invoked")
	}
	if _, _, err := service.weeklyDomainEvidence(); err == nil {
		t.Fatal("missing evidence succeeded")
	}
}
func TestWeeklyStoredEvidenceCannotQualifyWithoutAcceptedRegistration(t *testing.T) {
	s := productFixture(t).weeklyModelService()
	now := time.Now().UTC()
	e := WeeklyNativeEvidence{SchemaVersion: 1, AttemptID: strings.Repeat("a", 32), CommandID: "synthetic-weekly", OwnerScope: s.workspaceID, CompletedAt: now, OccurrenceSHA256: strings.Repeat("b", 64), ImplementationSHA256: s.build, RegistrationSHA256: strings.Repeat("c", 64), InputSHA256: strings.Repeat("d", 64), PolicySHA256: strings.Repeat("e", 64), ChildIdentitySHA256: strings.Repeat("f", 64), ResponseSHA256: strings.Repeat("a", 64)}
	if err := s.writeWeeklyEvidence(e); err != nil {
		t.Fatal(err)
	}
	_, artifact, err := s.weeklyDomainEvidence()
	if err != nil {
		t.Fatal(err)
	}
	q := &ProductQualification{Artifact: artifact, JobID: YodaSelfReviewWeeklyJobID, RequestedAt: now, VerifiedAt: now.Add(time.Second), Receipt: Receipt{AttemptID: e.AttemptID, CommandID: e.CommandID, OccurrenceDigest: e.OccurrenceSHA256, WeeklyMode: WeeklyNativeProbe}}
	if _, err = s.verifyWeeklyQualification(q); err == nil {
		t.Fatal("imported metadata granted qualification")
	}
	if filepath.IsAbs(artifact) {
		t.Fatal("artifact escaped metadata namespace")
	}
}
