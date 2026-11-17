package maintenance

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ownerctx"
)

func weeklyRegistrationFixture() WeeklyModelRegistration {
	return WeeklyModelRegistration{SchemaVersion: 1, Descriptor: weeklyDescriptorFixture(), Policy: ownerctx.WeeklySelfOptions{OwnerID: "synthetic-owner", Facets: []string{"communication-style"}, Limits: ownerctx.DefaultWeeklySelfLimits(), WorkingLanguage: "en", TranslatorID: "identity", TranslatorVersion: "1"}}
}
func TestWeeklyRegistrationUsesTrustedReviewCASAndDoesNotQualify(t *testing.T) {
	service := productFixture(t)
	candidate := weeklyRegistrationFixture()
	service.options.WeeklyCandidate = &candidate
	op := Operation{Kind: "register", JobID: YodaSelfReviewWeeklyJobID}
	review, err := service.Review(op)
	if err != nil {
		t.Fatal(err)
	}
	if review.Scope != "owner" || review.WeeklyRegistration == nil {
		t.Fatal("weekly registration review lacks exact owner/declaration metadata")
	}
	owner := service.weeklyModelService()
	before, _ := owner.loadControl(op.JobID)
	if before.Weekly != nil {
		t.Fatal("review accepted runtime before owner confirmation")
	}
	if _, err := service.Apply(context.Background(), review, func(context.Context, ProductReview) error { return errors.New("declined") }); err == nil {
		t.Fatal("declined registration persisted")
	}
	if _, err := service.Apply(context.Background(), review, approveProduct); err != nil {
		t.Fatal(err)
	}
	control, err := owner.loadControl(op.JobID)
	if err != nil {
		t.Fatal(err)
	}
	if control.Weekly == nil || control.Weekly.Registration.Descriptor.Digest() != candidate.Descriptor.Digest() || control.State != "inactive" || control.Qualification != nil || control.OneShot != "" {
		t.Fatal("registration issued qualification or activation")
	}
	if _, err := service.Apply(context.Background(), review, approveProduct); !errors.Is(err, ErrProductConflict) {
		t.Fatal("stale registration review replay accepted", err)
	}
}

func TestWeeklyOptionalStateLeavesLegacyJSONUnchanged(t *testing.T) {
	for _, value := range []any{productControl{SchemaVersion: 1}, ProductReview{}, ProductQualification{}} {
		body, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(body), "weekly") {
			t.Fatal("absent weekly metadata changed legacy JSON")
		}
	}
	candidate := weeklyRegistrationFixture()
	candidate.Policy.SensitiveOwnerAuthorized = true
	if candidate.Validate() == nil {
		t.Fatal("caller assertion of sensitive owner authorization accepted")
	}
	candidate = weeklyRegistrationFixture()
	candidate.Policy.OccurrenceID = strings.Repeat("a", 64)
	if candidate.Validate() == nil {
		t.Fatal("registration policy selected an occurrence")
	}
}

func TestWeeklyRegistrationBoundsCompleteControlMetadata(t *testing.T) {
	r := weeklyRegistrationFixture()
	r.Environment = []string{"PATH=" + strings.Repeat("x", 32768)}
	if r.Validate() == nil {
		t.Fatal("oversized registration can make the private control unreadable")
	}
}

func TestWeeklyRegistrationRejectsSecretEnvironmentBeforeReviewOrPersistence(t *testing.T) {
	service := productFixture(t)
	candidate := weeklyRegistrationFixture()
	candidate.Environment = []string{"ANTHROPIC_API_KEY=SYNTHETIC_DO_NOT_ACCEPT"}
	service.options.WeeklyCandidate = &candidate
	before := productSnapshot(t, service.home)
	if _, err := service.Review(Operation{Kind: "register", JobID: YodaSelfReviewWeeklyJobID}); err == nil {
		t.Fatal("secret environment entered an accepted registration review")
	}
	after := productSnapshot(t, service.home)
	if len(before) != len(after) {
		t.Fatal("rejected environment changed owner state")
	}
	for path, hash := range before {
		if after[path] != hash {
			t.Fatal("rejected environment changed owner state")
		}
	}
}
