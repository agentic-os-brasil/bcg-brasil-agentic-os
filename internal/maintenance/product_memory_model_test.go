package maintenance

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ownerctx"
)

func memoryModelRegistrationFixture() WeeklyModelRegistration {
	r := weeklyRegistrationFixture()
	r.Descriptor.JobID = MemoryModelJobID
	r.Descriptor.ValidatorVersion = "weekly-memory-v1"
	r.Policy = ownerctx.WeeklySelfOptions{}
	policy := DefaultMemoryModelPolicy()
	r.Memory = &policy
	return r
}

func TestWorkspaceModelRegistrationCannotBorrowOwnerAuthority(t *testing.T) {
	borrowed := memoryModelRegistrationFixture()
	borrowed.Policy = weeklyRegistrationFixture().Policy
	if borrowed.Validate() == nil {
		t.Fatal("workspace model borrowed owner SELF policy")
	}
	ownerBorrow := weeklyRegistrationFixture()
	policy := DefaultMemoryModelPolicy()
	ownerBorrow.Memory = &policy
	if ownerBorrow.Validate() == nil {
		t.Fatal("owner model borrowed workspace memory policy")
	}
	s := productFixture(t)
	r := memoryModelRegistrationFixture()
	s.options.WeeklyCandidate = &r
	review, err := s.Review(Operation{Kind: "register", JobID: MemoryModelJobID})
	if err != nil {
		t.Fatal("supported workspace model registration is unavailable:", err)
	}
	if review.Scope != "workspace" || review.WorkspaceID != s.workspaceID {
		t.Fatal("memory registration borrowed owner scope")
	}
	if _, err := s.Apply(context.Background(), review, func(context.Context, ProductReview) error { return errors.New("declined") }); err == nil {
		t.Fatal("declined registration applied")
	}
	if _, err := s.Apply(context.Background(), review, approveProduct); err != nil {
		t.Fatal(err)
	}
	c, err := s.loadControl(MemoryModelJobID)
	if err != nil || c.Weekly == nil || c.State != "inactive" || c.Qualification != nil {
		t.Fatal("registration granted model execution", err)
	}
	owner, err := s.weeklyModelService().loadControl(YodaSelfReviewWeeklyJobID)
	if err != nil || owner.Weekly != nil {
		t.Fatal("workspace registration activated owner job", err)
	}
}

func TestWeeklyMemoryEngineSwitchNeedsExplicitPauseAndQualification(t *testing.T) {
	for _, other := range []string{MemoryDeepDreamJobID, MemoryModelJobID} {
		t.Run(other, func(t *testing.T) {
			s := productFixture(t)
			job := MemoryModelJobID
			if other == MemoryModelJobID {
				job = MemoryDeepDreamJobID
			}
			control, err := s.loadControl(other)
			if err != nil {
				t.Fatal(err)
			}
			// Synthetic control metadata tests denial only. No native qualification
			// or successful occurrence is manufactured by this fixture.
			control.Revision = 1
			control.State = "active"
			control.Schedule = scheduleFor(other)
			control.Schedule.Timezone = "UTC"
			control.Boundary = s.now().UTC()
			control.Writes = []string{"memory_rollups"}
			if err = s.withControl(other, func() error { return s.saveControl(control) }); err != nil {
				t.Fatal(err)
			}
			op := Operation{Kind: "enroll", JobID: job, Timezone: "UTC"}
			if _, err = s.Review(op); err == nil || !strings.Contains(err.Error(), "pause the existing") {
				t.Fatal("weekly engine was implicitly switched", err)
			}
			unchanged, err := s.loadControl(other)
			if err != nil || unchanged.State != "active" {
				t.Fatal("review paused another engine", err)
			}
			control.Revision++
			control.State = "paused"
			if err = s.withControl(other, func() error { return s.saveControl(control) }); err != nil {
				t.Fatal(err)
			}
			if _, err = s.Review(op); !errors.Is(err, ErrQualificationRequired) {
				t.Fatal("pausing another engine fabricated qualification", err)
			}
		})
	}
}
